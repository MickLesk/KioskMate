// Command minify writes minified copies of the Admin UI to
// internal/admin/webmin/web for release builds with -tags minified.
//
// JavaScript modules are minified one by one (no bundling): import paths and
// exported names stay as they are, so the files load exactly like the
// sources. Development builds keep embedding the readable sources.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/evanw/esbuild/pkg/api"
)

func main() {
	src := flag.String("src", "internal/admin/web", "Admin UI source directory")
	out := flag.String("out", "internal/admin/webmin/web", "output directory")
	flag.Parse()
	before, after, err := run(*src, *out)
	if err != nil {
		fmt.Fprintln(os.Stderr, "minify:", err)
		os.Exit(1)
	}
	fmt.Printf("minify: %d -> %d bytes (%s)\n", before, after, *out)
}

func run(src, out string) (int, int, error) {
	if err := os.RemoveAll(out); err != nil {
		return 0, 0, err
	}
	if err := os.MkdirAll(filepath.Join(out, "assets"), 0o755); err != nil {
		return 0, 0, err
	}
	index, err := os.ReadFile(filepath.Join(src, "index.html"))
	if err != nil {
		return 0, 0, err
	}
	if err := os.WriteFile(filepath.Join(out, "index.html"), index, 0o644); err != nil {
		return 0, 0, err
	}
	entries, err := os.ReadDir(filepath.Join(src, "assets"))
	if err != nil {
		return 0, 0, err
	}
	before, after := len(index), len(index)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		data, err := os.ReadFile(filepath.Join(src, "assets", name))
		if err != nil {
			return 0, 0, err
		}
		minified, err := minify(name, data)
		if err != nil {
			return 0, 0, fmt.Errorf("%s: %w", name, err)
		}
		before += len(data)
		after += len(minified)
		if err := os.WriteFile(filepath.Join(out, "assets", name), minified, 0o644); err != nil {
			return 0, 0, err
		}
	}
	return before, after, nil
}

func minify(name string, data []byte) ([]byte, error) {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".js":
		return transform(data, api.LoaderJS)
	case ".css":
		return transform(data, api.LoaderCSS)
	case ".json":
		var compact bytes.Buffer
		if err := json.Compact(&compact, data); err != nil {
			return nil, err
		}
		return compact.Bytes(), nil
	default:
		return data, nil
	}
}

func transform(data []byte, loader api.Loader) ([]byte, error) {
	result := api.Transform(string(data), api.TransformOptions{
		Loader:            loader,
		Format:            formatFor(loader),
		Target:            api.ES2022,
		MinifyWhitespace:  true,
		MinifyIdentifiers: true,
		MinifySyntax:      true,
		LegalComments:     api.LegalCommentsNone,
	})
	if len(result.Errors) > 0 {
		return nil, fmt.Errorf("%s (line %d)", result.Errors[0].Text, lineOf(result.Errors[0]))
	}
	return result.Code, nil
}

func formatFor(loader api.Loader) api.Format {
	if loader == api.LoaderJS {
		return api.FormatESModule
	}
	return api.FormatDefault
}

func lineOf(message api.Message) int {
	if message.Location == nil {
		return 0
	}
	return message.Location.Line
}
