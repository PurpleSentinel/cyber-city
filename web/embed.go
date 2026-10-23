// SPDX-License-Identifier: MIT

// Package web embeds the built frontend so the binary serves the page and the
// WebSocket from one origin (docs/DECISIONS.md D3).
package web

import (
	"embed"
	"io/fs"
)

// dist holds the output of `vite build`. dist/.gitkeep is committed so this
// compiles before the frontend has been built.
//
//go:embed all:dist
var dist embed.FS

// Dist returns the built frontend rooted at dist/.
func Dist() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err) // unreachable: "dist" is a valid literal path
	}
	return sub
}
