package logutil

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

func Rotate(path string, maxBytes int64, keep int) error {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) || (err == nil && info.Size() <= maxBytes) {
		return nil
	}
	if err != nil {
		return err
	}
	for index := keep - 1; index >= 1; index-- {
		from := fmt.Sprintf("%s.%d", path, index)
		to := fmt.Sprintf("%s.%d", path, index+1)
		_ = os.Remove(to)
		_ = os.Rename(from, to)
	}
	first := path + ".1"
	_ = os.Remove(first)
	return os.Rename(path, first)
}

func PruneFiles(dir string, keep int) error {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var files []os.DirEntry
	for _, entry := range entries {
		if !entry.IsDir() {
			files = append(files, entry)
		}
	}
	sort.Slice(files, func(i, j int) bool {
		left, _ := files[i].Info()
		right, _ := files[j].Info()
		return left.ModTime().After(right.ModTime())
	})
	if len(files) <= keep {
		return nil
	}
	for _, entry := range files[keep:] {
		_ = os.Remove(filepath.Join(dir, entry.Name()))
	}
	return nil
}
