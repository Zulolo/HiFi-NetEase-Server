// Package web serves the embedded mobile PWA (ADR-0003).
package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed static
var files embed.FS

// Handler serves the PWA at the server root.
func Handler() http.Handler {
	sub, err := fs.Sub(files, "static")
	if err != nil {
		panic(err)
	}
	return http.FileServer(http.FS(sub))
}
