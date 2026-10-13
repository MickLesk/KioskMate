//go:build windows

package config

import "os"

// defaultPath keeps the configuration in the roaming profile
// (%APPDATA%\KioskMate\config.json), next to logs and backups.
func defaultPath() string {
	return windowsConfigPath(os.Getenv, userHome())
}

// defaultBrowserDataDir keeps browser profiles in the local profile
// (%LOCALAPPDATA%\KioskMate\Browser) because caches must not roam.
func defaultBrowserDataDir() string {
	return windowsBrowserDataDir(os.Getenv, userHome())
}

func userHome() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}
