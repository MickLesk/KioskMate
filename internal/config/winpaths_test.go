package config

import (
	"path/filepath"
	"testing"
)

func envMap(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestWindowsConfigPathUsesAppData(t *testing.T) {
	got := windowsConfigPath(envMap(map[string]string{"APPDATA": `C:\Users\kiosk\AppData\Roaming`}), `C:\Users\kiosk`)
	want := filepath.Join(`C:\Users\kiosk\AppData\Roaming`, "KioskMate", "config.json")
	if got != want {
		t.Fatalf("config path = %q, want %q", got, want)
	}
}

func TestWindowsConfigPathFallsBackToHome(t *testing.T) {
	got := windowsConfigPath(envMap(nil), "home")
	if want := filepath.Join("home", "AppData", "Roaming", "KioskMate", "config.json"); got != want {
		t.Fatalf("config path = %q, want %q", got, want)
	}
	if got := windowsConfigPath(envMap(nil), ""); got != "kioskmate.json" {
		t.Fatalf("config path without home = %q", got)
	}
}

func TestWindowsBrowserDataDirUsesLocalAppData(t *testing.T) {
	env := envMap(map[string]string{"APPDATA": "roaming", "LOCALAPPDATA": "local"})
	if got, want := windowsBrowserDataDir(env, "home"), filepath.Join("local", "KioskMate", "Browser"); got != want {
		t.Fatalf("browser dir = %q, want %q", got, want)
	}
	if got, want := windowsBrowserDataDir(envMap(nil), "home"), filepath.Join("home", "AppData", "Local", "KioskMate", "Browser"); got != want {
		t.Fatalf("browser dir fallback = %q, want %q", got, want)
	}
}
