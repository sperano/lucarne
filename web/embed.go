// Package web embeds the built frontend. Until `npm run build` has run in
// this directory, dist/ holds only its placeholder and the server answers
// UI requests with 503.
package web

import (
	"embed"
	"io/fs"
)

// all: keeps the placeholder dotfile, so the pattern matches before a build.
//
//go:embed all:dist
var dist embed.FS

// Dist returns the built frontend, rooted at dist/.
func Dist() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err) // "dist" is a valid path, so Sub cannot fail.
	}
	return sub
}
