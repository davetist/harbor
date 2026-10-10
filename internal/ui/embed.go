package ui

import (
	"embed"
	"io/fs"
)

//go:embed dist
var files embed.FS

func Files() fs.FS {
	f, err := fs.Sub(files, "dist")
	if err != nil {
		panic(err)
	}
	return f
}
