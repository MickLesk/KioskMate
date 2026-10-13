//go:build minified

package admin

import (
	"embed"
	"io/fs"
)

// Run `go run ./tools/minify` before building with -tags minified; it writes
// minified copies of web/ into webmin/web/.
//
//go:embed webmin/web
var minifiedAssets embed.FS

var content = mustSub(minifiedAssets, "webmin")

func mustSub(fsys embed.FS, dir string) fs.ReadFileFS {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		panic(err)
	}
	return sub.(fs.ReadFileFS)
}
