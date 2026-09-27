// Package web embeds the built Vue frontend located in web/dist.
//
// The dist directory is a build artifact produced by `npm run build` (see
// the Makefile "web" target). It is committed so that pure Go checkouts
// build a fully functional binary without a Node toolchain.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

// Dist returns the embedded frontend files rooted at dist/.
func Dist() fs.FS {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		// Impossible: the dist tree is embedded at compile time.
		panic("web: embedded dist missing: " + err.Error())
	}
	return sub
}
