// Package web serves the embedded mobile PWA (ADR-0003).
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed static
var files embed.FS

// Handler serves the PWA at the server root. Embedded files carry no
// modification time, so without help browsers cache them heuristically and
// keep an old page after an upgrade. Every response therefore asks the browser
// to revalidate, and an ETag tied to the build version turns that revalidation
// into a cheap 304 until the binary actually changes.
func Handler(version string) http.Handler {
	sub, err := fs.Sub(files, "static")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(sub))
	etag := `"` + strings.ReplaceAll(version, `"`, "") + `"`
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("ETag", etag)
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}
