package web

import (
	"embed"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
)

//go:embed index.html assets/*
var content embed.FS

// Handler returns a static file handler for the embedded frontend.
func Handler() http.Handler {
	sub, err := fs.Sub(content, ".")
	if err != nil {
		return http.NotFoundHandler()
	}
	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/", "/index.html":
			serveIndex(w, r, sub)
			return
		}

		if strings.HasPrefix(r.URL.Path, "/assets/") {
			fileServer.ServeHTTP(w, r)
			return
		}

		http.NotFound(w, r)
	})
}

func serveIndex(w http.ResponseWriter, r *http.Request, filesystem fs.FS) {
	data, err := fs.ReadFile(filesystem, "index.html")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", mime.TypeByExtension(path.Ext("index.html")))
	_, _ = w.Write(data)
}
