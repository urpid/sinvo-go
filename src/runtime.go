package sinvo

import (
	"crypto/rand"
	"database/sql"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
	"time"
)

const AppID = "sinvo-go"

//go:embed frontend/*
var staticFiles embed.FS

func NewApp(db *sql.DB, paths Paths, shutdown func()) *App {
	return &App{db: db, paths: paths, shutdown: shutdown}
}

func (a *App) InitDatabase() error {
	return a.initDatabase()
}

func (a *App) Routes(mux *http.ServeMux) {
	staticRoot, err := fs.Sub(staticFiles, "frontend")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(staticRoot))

	mux.Handle("/static/", http.StripPrefix("/static/", fileServer))
	mux.HandleFunc("/api/", a.handleAPI)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		http.ServeFileFS(w, r, staticRoot, "index.html")
	})
}

func newID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%s-%s-%s-%s-%s",
		hex.EncodeToString(b[0:4]),
		hex.EncodeToString(b[4:6]),
		hex.EncodeToString(b[6:8]),
		hex.EncodeToString(b[8:10]),
		hex.EncodeToString(b[10:16]),
	)
}

func nowText() string {
	return time.Now().Format(time.RFC3339)
}

func todayText() string {
	return time.Now().Format("2006-01-02")
}

func trim(s string) string {
	return strings.TrimSpace(s)
}

func nullable(s string) any {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return s
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
