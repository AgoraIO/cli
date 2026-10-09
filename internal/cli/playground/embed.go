package playground

import (
	"embed"
	"io/fs"
)

//go:embed webassets
var assets embed.FS

// Assets returns the built frontend rooted at the webassets directory.
func Assets() (fs.FS, error) {
	return fs.Sub(assets, "webassets")
}
