//go:build !linux

package main

import (
	"io"
	"os"
	"path/filepath"
)

func acquireInstanceLock(path string) (io.Closer, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	return os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
}
