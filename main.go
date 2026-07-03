package main

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	sinvo "sinvo-go/src"

	_ "modernc.org/sqlite"
)

const instanceAddr = "0.0.0.0:8123"

var devMode = "true"

type authConfig struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func main() {
	paths, err := resolvePaths()
	if err != nil {
		log.Fatal(err)
	}
	if err := ensureAppDirs(paths); err != nil {
		log.Fatal(err)
	}
	auth, err := loadAuthConfig(paths.BaseDir)
	if err != nil {
		log.Fatal(err)
	}

	ln, err := net.Listen("tcp", instanceAddr)
	if err != nil {
		url, probeErr := runningInstanceURL(instanceAddr, auth)
		if probeErr == nil {
			log.Printf("Sinvo Go läuft bereits: %s", url)
			openBrowser(url)
			return
		}
		log.Fatalf("Port %s ist belegt, aber keine laufende Sinvo-Go-Instanz antwortet: %v", instanceAddr, probeErr)
	}

	db, err := sql.Open("sqlite", paths.DBPath)
	if err != nil {
		_ = ln.Close()
		log.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	defer db.Close()

	server := &http.Server{}
	application := sinvo.NewApp(db, paths, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	})
	if err := application.InitDatabase(); err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	application.Routes(mux)
	server.Handler = withBasicAuth(mux, auth)

	url := "http://" + ln.Addr().String()
	log.Printf("Sinvo Go startet: %s", url)
	log.Printf("Datenbank: %s", paths.DBPath)
	if auth.enabled() {
		log.Printf("HTTP Basic Auth ist aktiv")
	} else {
		log.Printf("HTTP Basic Auth ist deaktiviert")
	}
	go openBrowser(url)

	if err := server.Serve(ln); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func resolvePaths() (sinvo.Paths, error) {
	var base string
	if devMode == "true" {
		wd, err := os.Getwd()
		if err != nil {
			return sinvo.Paths{}, err
		}
		base = wd
	} else {
		exe, err := os.Executable()
		if err != nil {
			return sinvo.Paths{}, err
		}
		base = filepath.Dir(exe)
	}
	return sinvo.Paths{
		BaseDir:    base,
		DataDir:    filepath.Join(base, "data"),
		DBPath:     filepath.Join(base, "data", "sinvo-go.sqlite"),
		BackupsDir: filepath.Join(base, "backups"),
		ExportsDir: filepath.Join(base, "exports"),
	}, nil
}

func loadAuthConfig(baseDir string) (authConfig, error) {
	path := filepath.Join(baseDir, "config.json")
	body, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return authConfig{}, nil
	}
	if err != nil {
		return authConfig{}, err
	}
	var config authConfig
	if err := json.Unmarshal(body, &config); err != nil {
		return authConfig{}, fmt.Errorf("config.json konnte nicht gelesen werden: %w", err)
	}
	return config, nil
}

func (config authConfig) enabled() bool {
	return config.Username != "" && config.Password != ""
}

func withBasicAuth(next http.Handler, config authConfig) http.Handler {
	if !config.enabled() {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok || !sameCredential(username, config.Username) || !sameCredential(password, config.Password) {
			w.Header().Set("WWW-Authenticate", `Basic realm="Sinvo Go"`)
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func sameCredential(left, right string) bool {
	return subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}

func ensureAppDirs(paths sinvo.Paths) error {
	for _, dir := range []string{paths.DataDir, paths.BackupsDir, paths.ExportsDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return nil
}

func runningInstanceURL(addr string, auth authConfig) (string, error) {
	endpoint := "http://" + addr + "/api/instance"
	client := http.Client{Timeout: 300 * time.Millisecond}
	var lastErr error
	deadline := time.Now().Add(2 * time.Second)
	for {
		req, err := http.NewRequest(http.MethodGet, endpoint, nil)
		if err != nil {
			return "", err
		}
		if auth.enabled() {
			req.SetBasicAuth(auth.Username, auth.Password)
		}
		resp, err := client.Do(req)
		if err == nil {
			var data struct {
				App string `json:"app"`
				URL string `json:"url"`
			}
			decodeErr := json.NewDecoder(resp.Body).Decode(&data)
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK && decodeErr == nil && data.App == sinvo.AppID && data.URL != "" {
				return data.URL, nil
			}
			if decodeErr != nil {
				lastErr = decodeErr
			} else {
				lastErr = fmt.Errorf("unexpected response from %s", endpoint)
			}
		} else {
			lastErr = err
		}
		if time.Now().After(deadline) {
			return "", lastErr
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func openBrowser(url string) {
	time.Sleep(300 * time.Millisecond)
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		log.Printf("Browser konnte nicht automatisch geöffnet werden: %v", err)
	}
}
