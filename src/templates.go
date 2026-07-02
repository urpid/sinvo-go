package sinvo

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var builtinTemplates = []Template{
	{ID: "professional-modern", Name: "Professional Modern", IsDefault: false, TemplateType: "builtin"},
	{ID: "minimalist-clean", Name: "Minimalist Clean", IsDefault: true, TemplateType: "builtin"},
	{ID: "nova", Name: "Nova", IsDefault: false, TemplateType: "builtin"},
	{ID: "slate", Name: "Slate", IsDefault: false, TemplateType: "builtin"},
}

type templateManifest struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version"`
	Invio   string `json:"invio"`
	HTML    struct {
		Path   string `json:"path"`
		URL    string `json:"url"`
		SHA256 string `json:"sha256"`
	} `json:"html"`
}

func (a *App) handleTemplates(w http.ResponseWriter, r *http.Request, parts []string) {
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			items, err := a.listTemplates()
			writeResult(w, items, err)
		case http.MethodPost:
			var input Template
			if err := readJSON(r, &input); err != nil {
				writeError(w, err, http.StatusBadRequest)
				return
			}
			item, err := a.saveTemplate(input)
			writeResult(w, item, err)
		default:
			writeError(w, errors.New("method not allowed"), http.StatusMethodNotAllowed)
		}
		return
	}

	if len(parts) == 2 && parts[1] == "upload" {
		if r.Method != http.MethodPost {
			writeError(w, errors.New("method not allowed"), http.StatusMethodNotAllowed)
			return
		}
		data, err := readTemplateUpload(w, r)
		if err != nil {
			writeError(w, err, http.StatusBadRequest)
			return
		}
		item, err := a.installTemplateFromZip(data)
		writeResult(w, item, err)
		return
	}

	if len(parts) == 2 && (parts[1] == "install-from-manifest" || parts[1] == "install") {
		if r.Method != http.MethodPost {
			writeError(w, errors.New("method not allowed"), http.StatusMethodNotAllowed)
			return
		}
		var data struct {
			URL string `json:"url"`
		}
		if err := readJSON(r, &data); err != nil {
			writeError(w, err, http.StatusBadRequest)
			return
		}
		item, err := a.installTemplateFromManifest(data.URL)
		writeResult(w, item, err)
		return
	}

	if len(parts) == 2 && parts[1] == "load-from-file" {
		if r.Method != http.MethodPost {
			writeError(w, errors.New("method not allowed"), http.StatusMethodNotAllowed)
			return
		}
		var data struct {
			FilePath  string `json:"filePath"`
			Name      string `json:"name"`
			IsDefault bool   `json:"isDefault"`
		}
		if err := readJSON(r, &data); err != nil {
			writeError(w, err, http.StatusBadRequest)
			return
		}
		item, err := a.loadTemplateFromFile(data.FilePath, data.Name, data.IsDefault)
		writeResult(w, item, err)
		return
	}

	id := parts[1]
	if len(parts) == 2 {
		switch r.Method {
		case http.MethodGet:
			item, err := a.getTemplate(id)
			writeResult(w, item, err)
		case http.MethodDelete:
			writeResult(w, map[string]bool{"success": true}, a.deleteTemplate(id))
		default:
			writeError(w, errors.New("method not allowed"), http.StatusMethodNotAllowed)
		}
		return
	}

	if len(parts) == 3 && parts[2] == "update" && r.Method == http.MethodPost {
		item, err := a.updateTemplateFromSource(id)
		writeResult(w, map[string]any{"ok": err == nil, "template": item}, err)
		return
	}

	if len(parts) == 3 && parts[2] == "preview" && r.Method == http.MethodPost {
		var data map[string]any
		_ = json.NewDecoder(r.Body).Decode(&data)
		html, err := a.previewTemplate(id, data)
		if err != nil {
			writeError(w, err, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(html))
		return
	}

	writeError(w, errors.New("not found"), http.StatusNotFound)
}

func (a *App) seedTemplates() error {
	now := nowText()
	for _, item := range builtinTemplates {
		body, err := staticFiles.ReadFile("frontend/templates/" + item.ID + ".html")
		if err != nil {
			return err
		}
		_, err = a.db.Exec(`INSERT INTO templates
			(id, name, html, is_default, template_type, created_at)
			VALUES (?, ?, ?, ?, 'builtin', ?)
			ON CONFLICT(id) DO UPDATE SET
				name = excluded.name,
				html = excluded.html,
				template_type = 'builtin'`,
			item.ID, item.Name, string(body), boolInt(item.IsDefault), now)
		if err != nil {
			return err
		}
	}
	var count int
	if err := a.db.QueryRow("SELECT COUNT(*) FROM templates WHERE is_default = 1").Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		if _, err := a.db.Exec("UPDATE templates SET is_default = 1 WHERE id = ?", "minimalist-clean"); err != nil {
			return err
		}
	}
	_, err := a.db.Exec("UPDATE settings SET value = 'minimalist-clean' WHERE key = 'templateId' AND lower(trim(value)) = 'simple'")
	return err
}

func (a *App) listTemplates() ([]Template, error) {
	settings, _ := a.settingsMap()
	current := normalizeTemplateID(settings["templateId"])
	rows, err := a.db.Query(`SELECT id, name, html, is_default, template_type, created_at
		FROM templates ORDER BY created_at DESC, name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []Template{}
	for rows.Next() {
		item, err := scanTemplate(rows)
		if err != nil {
			return nil, err
		}
		if current != "" {
			item.IsDefault = item.ID == current
		}
		item.Updatable = settings["templateSource:"+item.ID] != ""
		items = append(items, item)
	}
	return items, rows.Err()
}

func (a *App) getTemplate(id string) (Template, error) {
	row := a.db.QueryRow(`SELECT id, name, html, is_default, template_type, created_at
		FROM templates WHERE id = ?`, normalizeTemplateID(id))
	return scanTemplate(row)
}

func (a *App) defaultTemplate() (Template, error) {
	row := a.db.QueryRow(`SELECT id, name, html, is_default, template_type, created_at
		FROM templates WHERE is_default = 1 ORDER BY created_at DESC LIMIT 1`)
	item, err := scanTemplate(row)
	if err == nil {
		return item, nil
	}
	row = a.db.QueryRow(`SELECT id, name, html, is_default, template_type, created_at
		FROM templates ORDER BY created_at DESC LIMIT 1`)
	return scanTemplate(row)
}

func scanTemplate(scanner interface{ Scan(dest ...any) error }) (Template, error) {
	var item Template
	var templateType sql.NullString
	var isDefault int
	err := scanner.Scan(&item.ID, &item.Name, &item.HTML, &isDefault, &templateType, &item.CreatedAt)
	if err != nil {
		return Template{}, err
	}
	item.IsDefault = isDefault == 1
	item.TemplateType = templateType.String
	if item.TemplateType == "" {
		item.TemplateType = "local"
	}
	return item, nil
}

func (a *App) saveTemplate(input Template) (Template, error) {
	input.ID = normalizeTemplateID(input.ID)
	input.Name = trim(input.Name)
	if input.ID == "" {
		input.ID = newID()
	}
	if input.Name == "" {
		return Template{}, errors.New("template name is required")
	}
	if strings.TrimSpace(input.HTML) == "" {
		return Template{}, errors.New("template html is required")
	}
	if err := validateTemplateID(input.ID); err != nil {
		return Template{}, err
	}
	if err := basicTemplateHTMLSanity(input.HTML); err != nil {
		return Template{}, err
	}
	if input.TemplateType == "" || input.TemplateType == "builtin" {
		input.TemplateType = "local"
	}

	tx, err := a.db.Begin()
	if err != nil {
		return Template{}, err
	}
	defer tx.Rollback()

	var exists int
	_ = tx.QueryRow("SELECT COUNT(*) FROM templates WHERE id = ?", input.ID).Scan(&exists)
	if input.IsDefault {
		if _, err := tx.Exec("UPDATE templates SET is_default = 0"); err != nil {
			return Template{}, err
		}
	}
	if exists == 0 {
		_, err = tx.Exec(`INSERT INTO templates
			(id, name, html, is_default, template_type, created_at)
			VALUES (?, ?, ?, ?, ?, ?)`,
			input.ID, input.Name, input.HTML, boolInt(input.IsDefault), input.TemplateType, nowText())
	} else {
		_, err = tx.Exec(`UPDATE templates SET name = ?, html = ?, is_default = ?, template_type = ? WHERE id = ?`,
			input.Name, input.HTML, boolInt(input.IsDefault), input.TemplateType, input.ID)
	}
	if err != nil {
		return Template{}, err
	}
	if err := tx.Commit(); err != nil {
		return Template{}, err
	}
	if input.IsDefault {
		_, _ = a.updateSettings(map[string]string{"templateId": input.ID})
	}
	return a.getTemplate(input.ID)
}

func (a *App) upsertTemplateWithID(id string, input Template) (Template, error) {
	input.ID = normalizeTemplateID(id)
	if err := validateTemplateID(input.ID); err != nil {
		return Template{}, err
	}
	if input.Name == "" {
		input.Name = input.ID
	}
	if input.TemplateType == "" {
		input.TemplateType = "local"
	}
	_, err := a.db.Exec(`INSERT INTO templates
		(id, name, html, is_default, template_type, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			html = excluded.html,
			is_default = excluded.is_default,
			template_type = excluded.template_type`,
		input.ID, input.Name, input.HTML, boolInt(input.IsDefault), input.TemplateType, nowText())
	if err != nil {
		return Template{}, err
	}
	return a.getTemplate(input.ID)
}

func (a *App) deleteTemplate(id string) error {
	id = normalizeTemplateID(id)
	if id == "professional-modern" || id == "minimalist-clean" {
		return errors.New("cannot delete built-in templates")
	}
	if _, err := a.getTemplate(id); err != nil {
		return err
	}
	settings, _ := a.settingsMap()
	if normalizeTemplateID(settings["templateId"]) == id {
		_, _ = a.updateSettings(map[string]string{"templateId": "minimalist-clean"})
	}
	res, err := a.db.Exec("DELETE FROM templates WHERE id = ?", id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errors.New("template not found")
	}
	_ = os.RemoveAll(filepath.Join(a.paths.DataDir, "templates", id))
	return nil
}

func (a *App) setTemplateDefault(id string) error {
	id = normalizeTemplateID(id)
	tx, err := a.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("UPDATE templates SET is_default = 0"); err != nil {
		return err
	}
	if _, err := tx.Exec("UPDATE templates SET is_default = 1 WHERE id = ?", id); err != nil {
		return err
	}
	return tx.Commit()
}

func (a *App) installTemplateFromManifest(rawURL string) (Template, error) {
	manifestURL, err := safeTemplateURL(rawURL)
	if err != nil {
		return Template{}, err
	}
	body, err := fetchLimitedURL(manifestURL, 64*1024)
	if err != nil {
		return Template{}, err
	}
	manifest, err := parseTemplateManifest(body, true)
	if err != nil {
		return Template{}, err
	}
	if err := validateTemplateID(manifest.ID); err != nil {
		return Template{}, err
	}
	version := manifest.Version
	if version == "" {
		version = "1.0.0"
	}
	if err := validateTemplateID(version); err != nil {
		return Template{}, err
	}
	htmlURL, err := safeTemplateURL(manifest.HTML.URL)
	if err != nil {
		return Template{}, err
	}
	htmlBytes, err := fetchLimitedURL(htmlURL, 128*1024)
	if err != nil {
		return Template{}, err
	}
	if manifest.HTML.SHA256 != "" {
		sum := sha256.Sum256(htmlBytes)
		if !strings.EqualFold(hex.EncodeToString(sum[:]), manifest.HTML.SHA256) {
			return Template{}, errors.New("template html sha256 mismatch")
		}
	}
	html := string(htmlBytes)
	if err := basicTemplateHTMLSanity(html); err != nil {
		return Template{}, err
	}
	name := strings.TrimSpace(manifest.Name + " v" + version)
	item, err := a.upsertTemplateWithID(manifest.ID, Template{
		Name:         name,
		HTML:         html,
		IsDefault:    false,
		TemplateType: "remote",
	})
	if err != nil {
		return Template{}, err
	}
	_, _ = a.updateSettings(map[string]string{"templateSource:" + item.ID: rawURL})
	return item, nil
}

func (a *App) updateTemplateFromSource(id string) (Template, error) {
	id = normalizeTemplateID(id)
	settings, _ := a.settingsMap()
	source := settings["templateSource:"+id]
	if source == "" {
		return Template{}, errors.New("no stored manifest url for this template")
	}
	item, err := a.installTemplateFromManifest(source)
	if err != nil {
		return Template{}, err
	}
	if item.ID != id {
		return Template{}, errors.New("manifest id does not match template id")
	}
	return item, nil
}

func (a *App) installTemplateFromZip(data []byte) (Template, error) {
	if len(data) > 5*1024*1024 {
		return Template{}, errors.New("file too large (max 5MB)")
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return Template{}, err
	}

	manifestFile, rootPrefix := findManifestFile(reader.File)
	if manifestFile == nil {
		return Template{}, errors.New("no manifest.yaml or manifest.json found in zip root or single subfolder")
	}
	manifestBytes, err := readZipFile(manifestFile, 64*1024)
	if err != nil {
		return Template{}, err
	}
	manifest, err := parseTemplateManifest(manifestBytes, false)
	if err != nil {
		return Template{}, err
	}
	if err := validateTemplateID(manifest.ID); err != nil {
		return Template{}, err
	}
	version := manifest.Version
	if version == "" {
		version = "1.0.0"
	}
	if err := validateTemplateID(version); err != nil {
		return Template{}, err
	}
	htmlPath, err := sanitizeManifestPath(manifest.HTML.Path)
	if err != nil {
		return Template{}, err
	}
	htmlFile := findZipFile(reader.File, []string{
		rootPrefix + htmlPath,
		rootPrefix + "./" + htmlPath,
		htmlPath,
		"./" + htmlPath,
	})
	if htmlFile == nil {
		return Template{}, fmt.Errorf("html file not found in zip: %s", htmlPath)
	}
	htmlBytes, err := readZipFile(htmlFile, 128*1024)
	if err != nil {
		return Template{}, err
	}
	html := string(htmlBytes)
	if err := basicTemplateHTMLSanity(html); err != nil {
		return Template{}, err
	}

	target := filepath.Join(a.paths.DataDir, "templates", manifest.ID, version, filepath.FromSlash(htmlPath))
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return Template{}, err
	}
	if err := os.WriteFile(target, htmlBytes, 0644); err != nil {
		return Template{}, err
	}

	name := strings.TrimSpace(manifest.Name + " v" + version)
	return a.upsertTemplateWithID(manifest.ID, Template{
		Name:         name,
		HTML:         html,
		IsDefault:    false,
		TemplateType: "local",
	})
}

func (a *App) loadTemplateFromFile(filePath, name string, isDefault bool) (Template, error) {
	if strings.TrimSpace(filePath) == "" {
		return Template{}, errors.New("filePath is required")
	}
	body, err := os.ReadFile(filePath)
	if err != nil {
		return Template{}, err
	}
	if len(body) > 128*1024 {
		return Template{}, errors.New("html too large (>128KB)")
	}
	html := string(body)
	if err := basicTemplateHTMLSanity(html); err != nil {
		return Template{}, err
	}
	if strings.TrimSpace(name) == "" {
		name = strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
	}
	return a.saveTemplate(Template{Name: name, HTML: html, IsDefault: isDefault, TemplateType: "local"})
}

func (a *App) previewTemplate(id string, data map[string]any) (string, error) {
	tmpl, err := a.getTemplate(id)
	if err != nil {
		return "", err
	}
	sample := sampleInvoiceForTemplate()
	ctx := buildInvoiceTemplateContext(sample)
	for key, value := range data {
		ctx[key] = value
	}
	return renderTemplateHTML(tmpl.HTML, ctx), nil
}

func (a *App) renderInvoiceHTML(inv InvoiceWithDetails) (string, error) {
	withXML := func(html string) string {
		if setting(inv.Settings, "embedXmlInHtml", "false") != "true" {
			return html
		}
		xml, profile := renderInvoiceXML(inv)
		return embedInvoiceXMLInHTML(html, xml, profile)
	}
	selected := normalizeTemplateID(setting(inv.Settings, "templateId", ""))
	if selected == "simple" {
		return withXML(renderInvoiceSimpleHTML(inv)), nil
	}
	if selected != "" {
		if tmpl, err := a.getTemplate(selected); err == nil {
			return withXML(renderTemplateHTML(tmpl.HTML, buildInvoiceTemplateContext(inv))), nil
		}
	}
	tmpl, err := a.defaultTemplate()
	if err != nil {
		return withXML(renderInvoiceSimpleHTML(inv)), nil
	}
	return withXML(renderTemplateHTML(tmpl.HTML, buildInvoiceTemplateContext(inv))), nil
}

func readTemplateUpload(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	contentType := r.Header.Get("Content-Type")
	r.Body = http.MaxBytesReader(w, r.Body, 5*1024*1024+1024)
	if strings.Contains(contentType, "multipart/form-data") {
		file, header, err := r.FormFile("file")
		if err != nil {
			return nil, errors.New("no file uploaded")
		}
		defer file.Close()
		if header != nil && !strings.HasSuffix(strings.ToLower(header.Filename), ".zip") {
			return nil, errors.New("file must be a .zip archive")
		}
		data, err := io.ReadAll(io.LimitReader(file, 5*1024*1024+1))
		if err != nil {
			return nil, err
		}
		if len(data) > 5*1024*1024 {
			return nil, errors.New("file too large (max 5MB)")
		}
		return data, nil
	}
	if strings.Contains(contentType, "application/zip") || strings.Contains(contentType, "application/octet-stream") {
		data, err := io.ReadAll(io.LimitReader(r.Body, 5*1024*1024+1))
		if err != nil {
			return nil, err
		}
		if len(data) > 5*1024*1024 {
			return nil, errors.New("file too large (max 5MB)")
		}
		return data, nil
	}
	return nil, errors.New("invalid content type. expected multipart/form-data or application/zip")
}

func findManifestFile(files []*zip.File) (*zip.File, string) {
	names := []string{"manifest.yaml", "manifest.yml", "manifest.json"}
	if file := findZipFile(files, names); file != nil {
		return file, ""
	}
	folders := map[string]bool{}
	for _, file := range files {
		parts := strings.Split(file.Name, "/")
		if len(parts) > 1 && parts[0] != "" {
			folders[parts[0]] = true
		}
	}
	if len(folders) != 1 {
		return nil, ""
	}
	for folder := range folders {
		prefix := folder + "/"
		prefixed := []string{prefix + "manifest.yaml", prefix + "manifest.yml", prefix + "manifest.json"}
		return findZipFile(files, prefixed), prefix
	}
	return nil, ""
}

func findZipFile(files []*zip.File, names []string) *zip.File {
	lookup := map[string]bool{}
	for _, name := range names {
		lookup[strings.TrimPrefix(name, "./")] = true
	}
	for _, file := range files {
		name := strings.TrimPrefix(file.Name, "./")
		if lookup[name] {
			return file
		}
	}
	return nil
}

func readZipFile(file *zip.File, maxBytes int64) ([]byte, error) {
	reader, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	body, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maxBytes {
		return nil, errors.New("file too large")
	}
	return body, nil
}

func parseTemplateManifest(body []byte, remote bool) (templateManifest, error) {
	var manifest templateManifest
	if err := json.Unmarshal(body, &manifest); err == nil && manifest.ID != "" {
		return manifest, validateManifestShape(manifest, remote)
	}
	manifest = parseSimpleManifestYAML(string(body))
	return manifest, validateManifestShape(manifest, remote)
}

func validateManifestShape(manifest templateManifest, remote bool) error {
	if manifest.ID == "" || manifest.Name == "" {
		return errors.New("manifest missing id or name")
	}
	if manifest.HTML.Path == "" {
		return errors.New("manifest missing html.path")
	}
	if remote && manifest.HTML.URL == "" {
		return errors.New("manifest missing html.url")
	}
	return nil
}

func parseSimpleManifestYAML(text string) templateManifest {
	var manifest templateManifest
	section := ""
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasSuffix(line, ":") && !strings.Contains(strings.TrimSuffix(line, ":"), " ") {
			section = strings.TrimSuffix(line, ":")
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if section == "html" {
			switch key {
			case "path":
				manifest.HTML.Path = value
			case "url":
				manifest.HTML.URL = value
			case "sha256":
				manifest.HTML.SHA256 = value
			}
			continue
		}
		switch key {
		case "id":
			manifest.ID = value
		case "name":
			manifest.Name = value
		case "version":
			manifest.Version = value
		case "invio":
			manifest.Invio = value
		}
	}
	return manifest
}

func safeTemplateURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return nil, errors.New("url must be a valid https url")
	}
	return parsed, nil
}

func fetchLimitedURL(parsed *url.URL, maxBytes int64) ([]byte, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(parsed.String())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("fetch failed %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maxBytes {
		return nil, errors.New("download too large")
	}
	return body, nil
}

func sanitizeManifestPath(pathValue string) (string, error) {
	value := strings.TrimSpace(strings.ReplaceAll(pathValue, "\\", "/"))
	if value == "" || strings.HasPrefix(value, "/") {
		return "", errors.New("html.path must be relative")
	}
	clean := filepath.ToSlash(filepath.Clean(value))
	if clean == "." || strings.HasPrefix(clean, "../") || clean == ".." || strings.Contains(clean, "/../") {
		return "", errors.New("html.path must stay within the template directory")
	}
	return clean, nil
}

func validateTemplateID(id string) error {
	ok, _ := regexp.MatchString(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`, id)
	if !ok {
		return errors.New("template id must be 1-64 letters, numbers, dashes, underscores or dots")
	}
	return nil
}

func normalizeTemplateID(id string) string {
	value := strings.ToLower(strings.TrimSpace(id))
	switch value {
	case "professional":
		return "professional-modern"
	case "minimalist":
		return "minimalist-clean"
	default:
		return value
	}
}

func basicTemplateHTMLSanity(html string) error {
	lower := strings.ToLower(html)
	for _, tag := range []string{"<iframe", "<object", "<embed", "<script"} {
		if strings.Contains(lower, tag) {
			return fmt.Errorf("html contains disallowed tag: %s", tag)
		}
	}
	if regexp.MustCompile(`(?i)(\s|<)on[a-z]+\s*=`).MatchString(html) {
		return errors.New("inline event handlers not allowed")
	}
	return nil
}

func renderTemplateHTML(tpl string, ctx map[string]any) string {
	for {
		start := regexp.MustCompile(`\{\{#\s*([^}]+?)\s*\}\}`).FindStringSubmatchIndex(tpl)
		if start == nil {
			break
		}
		key := strings.TrimSpace(tpl[start[2]:start[3]])
		closeTag := "{{/" + key + "}}"
		closeAt := strings.Index(tpl[start[1]:], closeTag)
		if closeAt < 0 {
			break
		}
		innerStart := start[1]
		innerEnd := start[1] + closeAt
		inner := tpl[innerStart:innerEnd]
		replacement := renderTemplateBlock(inner, lookupTemplateValue(ctx, key), ctx)
		tpl = tpl[:start[0]] + replacement + tpl[innerEnd+len(closeTag):]
	}
	tpl = regexp.MustCompile(`\{\{\{\s*([^}]+?)\s*\}\}\}`).ReplaceAllStringFunc(tpl, func(match string) string {
		key := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(match, "{{{"), "}}}"))
		value := lookupTemplateValue(ctx, key)
		if value == nil {
			return ""
		}
		return fmt.Sprint(value)
	})
	return regexp.MustCompile(`\{\{\s*([^}]+?)\s*\}\}`).ReplaceAllStringFunc(tpl, func(match string) string {
		key := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(match, "{{"), "}}"))
		if strings.HasPrefix(key, "#") || strings.HasPrefix(key, "/") {
			return match
		}
		if strings.Contains(key, "||") {
			left, right, _ := strings.Cut(key, "||")
			value := lookupTemplateValue(ctx, strings.TrimSpace(left))
			if isEmptyTemplateValue(value) {
				return htmlEscape(strings.Trim(strings.TrimSpace(right), `"'`))
			}
			return htmlEscape(fmt.Sprint(value))
		}
		value := lookupTemplateValue(ctx, key)
		if value == nil {
			return ""
		}
		return htmlEscape(fmt.Sprint(value))
	})
}

func renderTemplateBlock(inner string, value any, ctx map[string]any) string {
	if isEmptyTemplateValue(value) {
		return ""
	}
	switch items := value.(type) {
	case []map[string]any:
		var out strings.Builder
		for _, item := range items {
			child := map[string]any{}
			for key, value := range ctx {
				child[key] = value
			}
			for key, value := range item {
				child[key] = value
			}
			out.WriteString(renderTemplateHTML(inner, child))
		}
		return out.String()
	default:
		return renderTemplateHTML(inner, ctx)
	}
}

func lookupTemplateValue(ctx map[string]any, path string) any {
	path = strings.Trim(strings.TrimSpace(path), `"'`)
	var current any = ctx
	for _, part := range strings.Split(path, ".") {
		part = strings.TrimSpace(part)
		switch typed := current.(type) {
		case map[string]any:
			current = typed[part]
		case map[string]string:
			current = typed[part]
		default:
			return nil
		}
	}
	return current
}

func isEmptyTemplateValue(value any) bool {
	switch v := value.(type) {
	case nil:
		return true
	case bool:
		return !v
	case string:
		return v == ""
	case int:
		return v == 0
	case float64:
		return v == 0
	case []map[string]any:
		return len(v) == 0
	default:
		return false
	}
}

func buildInvoiceTemplateContext(inv InvoiceWithDetails) map[string]any {
	settings := inv.Settings
	currency := inv.Currency
	if currency == "" {
		currency = setting(settings, "currency", "EUR")
	}
	numberFormat := setting(settings, "numberFormat", "comma")
	locale := normalizeLocale(setting(settings, "locale", "de"))
	labels := invoiceTemplateLabels(locale)

	items := []map[string]any{}
	hasItemUnits := false
	for _, item := range inv.Items {
		unit := strings.TrimSpace(item.Unit)
		if unit != "" {
			hasItemUnits = true
		}
		items = append(items, map[string]any{
			"description": item.Description,
			"quantity":    formatPlainNumber(item.Quantity, numberFormat),
			"unit":        unit,
			"unitPrice":   formatTemplateMoney(item.UnitPrice, currency, numberFormat),
			"lineTotal":   formatTemplateMoney(item.LineTotal, currency, numberFormat),
			"notes":       item.Notes,
		})
	}

	taxLabel := labels["taxLabel"]
	taxSummary := []map[string]any{}
	if inv.TaxAmount > 0 {
		taxable := inv.Subtotal - inv.DiscountAmount
		if taxable < 0 {
			taxable = 0
		}
		taxSummary = append(taxSummary, map[string]any{
			"label":   fmt.Sprintf("%s %s%%", taxLabel, formatPlainNumber(inv.TaxRate, numberFormat)),
			"percent": formatPlainNumber(inv.TaxRate, numberFormat),
			"taxable": formatTemplateMoney(taxable, currency, numberFormat),
			"amount":  formatTemplateMoney(inv.TaxAmount, currency, numberFormat),
		})
	}

	highlight := normalizeHex(setting(settings, "highlight", "#2563eb"))
	if highlight == "" {
		highlight = "#2563eb"
	}
	paymentTerms := inv.PaymentTerms
	if paymentTerms == "" {
		paymentTerms = settings["paymentTerms"]
	}
	notes := inv.Notes
	if notes == "" {
		notes = settings["defaultNotes"]
	}

	return map[string]any{
		"companyName":         setting(settings, "companyName", "Meine Firma"),
		"companyAddress":      htmlEscapeWithBreaks(settings["companyAddress"]),
		"companyCity":         strings.TrimSpace(settings["companyCity"]),
		"companyPostalCode":   strings.TrimSpace(settings["companyPostalCode"]),
		"companyCountryCode":  strings.TrimSpace(settings["companyCountryCode"]),
		"companyPostalCity":   formatPostalCityLine(settings["companyPostalCode"], settings["companyCity"], settings["companyCountryCode"], settings["postalCityFormat"]),
		"companyEmail":        settings["companyEmail"],
		"companyPhone":        settings["companyPhone"],
		"companyTaxId":        settings["companyTaxId"],
		"invoiceNumber":       inv.InvoiceNumber,
		"issueDate":           formatTemplateDate(inv.IssueDate, setting(settings, "dateFormat", "DD.MM.YYYY")),
		"dueDate":             formatTemplateDate(inv.DueDate, setting(settings, "dateFormat", "DD.MM.YYYY")),
		"currency":            currency,
		"status":              inv.Status,
		"customerName":        inv.Customer.Name,
		"customerContactName": inv.Customer.ContactName,
		"customerEmail":       inv.Customer.Email,
		"customerPhone":       inv.Customer.Phone,
		"customerAddress":     htmlEscapeWithBreaks(inv.Customer.Address),
		"customerCity":        inv.Customer.City,
		"customerPostalCode":  inv.Customer.PostalCode,
		"customerPostalCity":  formatPostalCityLine(inv.Customer.PostalCode, inv.Customer.City, inv.Customer.CountryCode, settings["postalCityFormat"]),
		"customerCountryCode": inv.Customer.CountryCode,
		"customerTaxId":       inv.Customer.TaxID,
		"items":               items,
		"hasItemUnits":        hasItemUnits,
		"subtotal":            formatTemplateMoney(inv.Subtotal, currency, numberFormat),
		"discountAmount":      formatTemplateMoney(inv.DiscountAmount, currency, numberFormat),
		"discountPercentage":  formatPlainNumber(inv.DiscountPercentage, numberFormat),
		"taxRate":             formatPlainNumber(inv.TaxRate, numberFormat),
		"taxAmount":           formatTemplateMoney(inv.TaxAmount, currency, numberFormat),
		"total":               formatTemplateMoney(inv.Total, currency, numberFormat),
		"taxSummary":          taxSummary,
		"hasTaxSummary":       len(taxSummary) > 0,
		"netSubtotal":         formatTemplateMoney(inv.Subtotal-inv.DiscountAmount, currency, numberFormat),
		"hasDiscount":         inv.DiscountAmount > 0,
		"hasTax":              inv.TaxAmount > 0,
		"paymentTerms":        paymentTerms,
		"paymentMethods":      settings["paymentMethods"],
		"bankAccount":         settings["bankAccount"],
		"notes":               notes,
		"locale":              locale,
		"labels":              labels,
		"logoUrl":             settings["logo"],
		"brandLogoLeft":       true,
		"highlightColor":      highlight,
		"highlightColorLight": lightenHex(highlight, 0.86),
	}
}

func sampleInvoiceForTemplate() InvoiceWithDetails {
	settings := map[string]string{
		"companyName":        "Sample Company",
		"companyAddress":     "Musterstrasse 1",
		"companyPostalCode":  "10115",
		"companyCity":        "Berlin",
		"companyCountryCode": "DE",
		"companyEmail":       "kontakt@example.com",
		"companyPhone":       "+49 30 123456",
		"companyTaxId":       "DE123456789",
		"currency":           "EUR",
		"locale":             "de",
		"dateFormat":         "DD.MM.YYYY",
		"numberFormat":       "comma",
		"highlight":          "#2563eb",
		"paymentTerms":       "Zahlbar innerhalb von 14 Tagen",
		"paymentMethods":     "Ueberweisung",
		"bankAccount":        "IBAN DE00 0000 0000 0000 0000 00",
	}
	return InvoiceWithDetails{
		Invoice: Invoice{
			ID:                 "sample",
			InvoiceNumber:      "RE-2026-0001",
			IssueDate:          "2026-07-01",
			DueDate:            "2026-07-15",
			Currency:           "EUR",
			Status:             "draft",
			Subtotal:           2500,
			DiscountAmount:     250,
			DiscountPercentage: 10,
			TaxRate:            19,
			TaxAmount:          427.5,
			Total:              2677.5,
			PaymentTerms:       "Zahlbar innerhalb von 14 Tagen",
			Notes:              "Vielen Dank fuer den Auftrag.",
		},
		Customer: Customer{
			Name:        "Max Mustermann",
			Email:       "max@example.com",
			Address:     "Kundenweg 5",
			PostalCode:  "20095",
			City:        "Hamburg",
			CountryCode: "DE",
		},
		Items: []InvoiceItem{
			{Description: "Website Development", Quantity: 1, Unit: "Stk", UnitPrice: 2500, LineTotal: 2500},
		},
		Settings: settings,
	}
}

func invoiceTemplateLabels(locale string) map[string]any {
	labels := map[string]string{
		"invoiceTitle":             "Rechnung",
		"invoiceNumberShortLabel":  "Nr.",
		"invoiceDateLabel":         "Datum",
		"dueDateLabel":             "Faellig",
		"statusLabel":              "Status",
		"billToHeading":            "Rechnung an",
		"itemsHeading":             "Positionen",
		"itemHeaderDescription":    "Beschreibung",
		"itemHeaderQuantityShort":  "Menge",
		"itemHeaderUnit":           "Einheit",
		"itemHeaderUnitPrice":      "Einzelpreis",
		"itemHeaderUnitPriceShort": "Preis",
		"itemHeaderAmount":         "Summe",
		"summaryHeading":           "Zusammenfassung",
		"subtotalLabel":            "Zwischensumme",
		"discountLabel":            "Rabatt",
		"taxLabel":                 "Steuer",
		"taxSummaryHeading":        "Steueruebersicht",
		"taxableLabel":             "Netto",
		"taxAmountLabel":           "Steuer",
		"totalLabel":               "Gesamt",
		"notesHeading":             "Notizen",
		"paymentTermsLabel":        "Zahlungsziel",
		"paymentMethodsLabel":      "Zahlungsart",
		"bankAccountLabel":         "Bankdaten",
		"taxIdLabel":               "Steuernummer",
	}
	if strings.HasPrefix(strings.ToLower(locale), "en") {
		labels["invoiceTitle"] = "Invoice"
		labels["invoiceDateLabel"] = "Invoice date"
		labels["dueDateLabel"] = "Due date"
		labels["billToHeading"] = "Bill to"
		labels["itemsHeading"] = "Items"
		labels["itemHeaderDescription"] = "Description"
		labels["itemHeaderQuantityShort"] = "Qty"
		labels["itemHeaderUnit"] = "Unit"
		labels["itemHeaderUnitPrice"] = "Unit price"
		labels["itemHeaderUnitPriceShort"] = "Price"
		labels["itemHeaderAmount"] = "Amount"
		labels["summaryHeading"] = "Summary"
		labels["subtotalLabel"] = "Subtotal"
		labels["discountLabel"] = "Discount"
		labels["taxLabel"] = "Tax"
		labels["taxSummaryHeading"] = "Tax summary"
		labels["taxableLabel"] = "Taxable"
		labels["totalLabel"] = "Total"
		labels["notesHeading"] = "Notes"
		labels["paymentTermsLabel"] = "Payment terms"
		labels["paymentMethodsLabel"] = "Payment methods"
		labels["bankAccountLabel"] = "Bank account"
		labels["taxIdLabel"] = "Tax ID"
	}
	result := map[string]any{}
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		result[key] = labels[key]
	}
	return result
}

func formatTemplateDate(value, format string) string {
	if value == "" {
		return ""
	}
	t, err := time.Parse("2006-01-02", value)
	if err != nil {
		return value
	}
	if format == "YYYY-MM-DD" {
		return t.Format("2006-01-02")
	}
	return t.Format("02.01.2006")
}

func formatTemplateMoney(value float64, currency, numberFormat string) string {
	return formatPlainNumber(value, numberFormat) + " " + currency
}

func formatPlainNumber(value float64, numberFormat string) string {
	out := strconv.FormatFloat(value, 'f', 2, 64)
	if strings.HasSuffix(out, ".00") {
		out = strings.TrimSuffix(out, ".00")
	}
	if numberFormat == "comma" {
		out = strings.ReplaceAll(out, ".", ",")
	}
	return out
}

func formatPostalCityLine(postalCode, city, countryCode, format string) string {
	postal := strings.TrimSpace(postalCode)
	place := strings.TrimSpace(city)
	if postal == "" {
		return place
	}
	if place == "" {
		return postal
	}
	if format == "city-postal" {
		return place + ", " + postal
	}
	return postal + " " + place
}

func htmlEscapeWithBreaks(value string) string {
	return strings.ReplaceAll(htmlEscape(value), "\n", "<br />")
}

func normalizeHex(value string) string {
	value = strings.TrimSpace(value)
	if matched, _ := regexp.MatchString(`^#?[0-9a-fA-F]{6}$`, value); !matched {
		return ""
	}
	if strings.HasPrefix(value, "#") {
		return value
	}
	return "#" + value
}

func lightenHex(hexValue string, amount float64) string {
	hexValue = strings.TrimPrefix(normalizeHex(hexValue), "#")
	if len(hexValue) != 6 {
		return "#dbeafe"
	}
	r, _ := strconv.ParseInt(hexValue[0:2], 16, 64)
	g, _ := strconv.ParseInt(hexValue[2:4], 16, 64)
	b, _ := strconv.ParseInt(hexValue[4:6], 16, 64)
	mix := func(v int64) int64 {
		return int64(float64(v) + (255-float64(v))*amount)
	}
	return fmt.Sprintf("#%02x%02x%02x", mix(r), mix(g), mix(b))
}
