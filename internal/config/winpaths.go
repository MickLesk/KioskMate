package config

import (
	"path/filepath"
	"strings"
)

// windowsConfigPath and windowsBrowserDataDir hold the Windows path rules
// without a build tag so they are unit-tested on every platform.
func windowsConfigPath(getenv func(string) string, home string) string {
	base := windowsProfileDir(getenv("APPDATA"), home, "Roaming")
	if base == "" {
		return "kioskmate.json"
	}
	return filepath.Join(base, "KioskMate", "config.json")
}

func windowsBrowserDataDir(getenv func(string) string, home string) string {
	base := windowsProfileDir(getenv("LOCALAPPDATA"), home, "Local")
	if base == "" {
		return "kioskmate-browser"
	}
	return filepath.Join(base, "KioskMate", "Browser")
}

func windowsProfileDir(value string, home string, fallback string) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	if strings.TrimSpace(home) == "" {
		return ""
	}
	return filepath.Join(home, "AppData", fallback)
}
