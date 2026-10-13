//go:build !minified

package admin

import (
	"embed"
	"io/fs"
)

//go:embed web/index.html web/assets/*
var sourceAssets embed.FS

// content serves the Admin UI sources as written. Release packages build
// with -tags minified instead (see embed_minified.go).
var content fs.ReadFileFS = sourceAssets
