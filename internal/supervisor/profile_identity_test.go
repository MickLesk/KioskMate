package supervisor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/MickLesk/KioskMate/internal/config"
)

func TestProfileIdentitySurvivesRenameAndReorder(t *testing.T) {
	root := t.TempDir()
	cfg := &config.Config{Kiosk: config.KioskConfig{
		UserDataDir: root, IsolateSessions: true,
		Pages: []config.KioskPage{
			{PageID: "weather", Name: "Weather", URL: "https://example.test/weather"},
			{PageID: "home", Name: "Home", URL: "https://example.test/home"},
		},
	}}
	weather := browserUserDataDir(cfg, 0)
	cfg.Kiosk.Pages[0].Name = "Forecast"
	cfg.Kiosk.Pages[0], cfg.Kiosk.Pages[1] = cfg.Kiosk.Pages[1], cfg.Kiosk.Pages[0]
	if got := browserUserDataDir(cfg, 1); got != weather {
		t.Fatalf("profile changed after rename/reorder: %q != %q", got, weather)
	}
}

func TestMigratePageProfilesMovesLegacyDirectory(t *testing.T) {
	root := t.TempDir()
	cfg := &config.Config{Kiosk: config.KioskConfig{
		UserDataDir: root, IsolateSessions: true,
		Pages: []config.KioskPage{{PageID: "home", Name: "Home", URL: "https://example.test"}},
	}}
	legacy := filepath.Join(root, "pages", pathSegment("Home", 0))
	if err := os.MkdirAll(legacy, 0o700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(legacy, "Cookies")
	if err := os.WriteFile(marker, []byte("session"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := migratePageProfiles(cfg); err != nil {
		t.Fatal(err)
	}
	target := browserUserDataDir(cfg, 0)
	if data, err := os.ReadFile(filepath.Join(target, "Cookies")); err != nil || string(data) != "session" {
		t.Fatalf("legacy profile was not preserved: %q, %v", data, err)
	}
}
