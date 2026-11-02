// Package web holds the GUI's page: plain HTML, CSS and JavaScript with no
// framework and no build step, embedded in the binary. The page only lays
// out views and forms, builds command lines, and renders what commands
// print; it decides nothing itself (docs/gui.md).
package web

import (
	"embed"
	"io/fs"
)

//go:embed static
var files embed.FS

// FS is the page's files, rooted at index.html.
func FS() fs.FS {
	sub, err := fs.Sub(files, "static")
	if err != nil {
		panic(err)
	}
	return sub
}
