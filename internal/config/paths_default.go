//go:build !windows

package config

import (
	"os"
	"path/filepath"
)

func defaultPath() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "kioskmate", "config.json")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "kioskmate.json"
	}
	return filepath.Join(home, ".config", "kioskmate", "config.json")
}

func defaultBrowserDataDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "kioskmate", "Browser")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "kioskmate-browser"
	}
	return filepath.Join(home, ".config", "kioskmate", "Browser")
}
