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

const fallbackIndexHTML = `<!doctype html>
<html lang="zh-Hant">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>企業活動票務系統</title>
  </head>
  <body>
    <div id="root"></div>
  </body>
</html>
`

func handleIndex(w http.ResponseWriter, r *http.Request) {
	requestPath := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if requestPath == "." {
		requestPath = ""
	}

	if requestPath == "" {
		serveIndex(w, r)
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

	serveIndex(w, r)
}

func serveIndex(w http.ResponseWriter, r *http.Request) {
	if _, err := fs.Stat(staticRoot, "index.html"); err == nil {
		http.ServeFileFS(w, r, staticRoot, "index.html")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(fallbackIndexHTML))
}

func mustSubFS(source fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(source, dir)
	if err != nil {
		panic(err)
	}
	return sub
}
