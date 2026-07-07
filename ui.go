package main

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed web
var webFiles embed.FS

func registerUI(mux *http.ServeMux, api *API) {
	staticFS, err := fs.Sub(webFiles, "web/static")
	if err == nil {
		mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))
	}
	mux.HandleFunc("GET /", api.index)
}

func (a *API) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "/ui" && !strings.HasPrefix(r.URL.Path, "/ui/") {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	http.ServeFileFS(w, r, webFiles, "web/index.html")
}
