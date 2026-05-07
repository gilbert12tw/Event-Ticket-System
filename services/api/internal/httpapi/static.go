package httpapi

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed static/*
var staticFiles embed.FS

var staticRoot = mustSubFS(staticFiles, "static")

func handleIndex(w http.ResponseWriter, r *http.Request) {
	requestPath := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if requestPath == "." {
		requestPath = ""
	}

	if requestPath == "" {
		http.ServeFileFS(w, r, staticRoot, "index.html")
		return
	}
	if strings.HasPrefix(requestPath, "api/") {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if info, err := fs.Stat(staticRoot, requestPath); err == nil && !info.IsDir() {
		http.ServeFileFS(w, r, staticRoot, requestPath)
		return
	}
	if path.Ext(requestPath) != "" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	http.ServeFileFS(w, r, staticRoot, "index.html")
}

func mustSubFS(source fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(source, dir)
	if err != nil {
		panic(err)
	}
	return sub
}
