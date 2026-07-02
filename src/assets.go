package sinvo

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const maxLogoBytes = 5 * 1024 * 1024

func (a *App) uploadLogo(w http.ResponseWriter, r *http.Request) (string, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxLogoBytes+1024)
	file, header, err := r.FormFile("file")
	if err != nil {
		return "", errors.New("no logo file uploaded")
	}
	defer file.Close()

	ext := logoExtension(header.Filename, header.Header.Get("Content-Type"))
	if ext == "" {
		return "", errors.New("logo must be png, jpg, gif, webp or svg")
	}
	body, err := io.ReadAll(io.LimitReader(file, maxLogoBytes+1))
	if err != nil {
		return "", err
	}
	if len(body) > maxLogoBytes {
		return "", errors.New("logo is too large")
	}

	dir := filepath.Join(a.paths.DataDir, "logos")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	stamp := time.Now().Format("20060102-150405")
	base := safeFileName(strings.TrimSuffix(header.Filename, filepath.Ext(header.Filename)))
	if base == "" {
		base = newID()
	}
	name := stamp + "-" + base + ext
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, body, 0644); err != nil {
		return "", err
	}

	logo := "/api/logos/" + name
	if _, err := a.updateSettings(map[string]string{"logo": logo}); err != nil {
		return "", err
	}
	return logo, nil
}

func (a *App) serveLogo(w http.ResponseWriter, r *http.Request, parts []string) {
	if r.Method != http.MethodGet {
		writeError(w, errors.New("method not allowed"), http.StatusMethodNotAllowed)
		return
	}
	if len(parts) != 2 || parts[1] == "" || safeFileName(parts[1]) != parts[1] {
		writeError(w, errors.New("not found"), http.StatusNotFound)
		return
	}
	w.Header().Set("Cache-Control", "private, max-age=31536000")
	http.ServeFile(w, r, filepath.Join(a.paths.DataDir, "logos", parts[1]))
}

func logoExtension(filename, contentType string) string {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".png":
		return ".png"
	case ".jpg", ".jpeg":
		return ".jpg"
	case ".gif":
		return ".gif"
	case ".webp":
		return ".webp"
	case ".svg":
		return ".svg"
	}
	switch strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0])) {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "image/svg+xml":
		return ".svg"
	default:
		return ""
	}
}
