package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/MickLesk/KioskMate/internal/admin"
	"github.com/MickLesk/KioskMate/internal/config"
)

func loadTestConfig(t *testing.T) (*config.Config, string) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	path := filepath.Join(t.TempDir(), "config.json")
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return cfg, path
}

func TestSetAdminPasswordFromStdin(t *testing.T) {
	cfg, path := loadTestConfig(t)
	if err := handleSetup(cfg, true, strings.NewReader("correct horse\r\n"), ""); err != nil {
		t.Fatalf("set password: %v", err)
	}
	reloaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !admin.VerifyPassword("correct horse", reloaded.Admin.PasswordHash) {
		t.Fatalf("stored hash does not verify the password read from stdin")
	}
}

func TestSetAdminPasswordRejectsInvalidInput(t *testing.T) {
	for name, input := range map[string]string{
		"short":     "short\n",
		"empty":     "",
		"multiline": "first line\nsecond line\n",
		"too long":  strings.Repeat("x", 5000),
	} {
		t.Run(name, func(t *testing.T) {
			cfg, _ := loadTestConfig(t)
			if err := setAdminPassword(cfg, strings.NewReader(input)); err == nil {
				t.Fatalf("expected an error")
			}
			if cfg.Snapshot().Admin.PasswordHash != "" {
				t.Fatalf("password hash changed on invalid input")
			}
		})
	}
}

func TestSetStartURLReplacesFirstPage(t *testing.T) {
	cfg, path := loadTestConfig(t)
	if err := cfg.Mutate(func(next *config.Config) error {
		next.Kiosk.Pages = append(next.Kiosk.Pages, config.KioskPage{Name: "Second", URL: "https://second.example"})
		return nil
	}); err != nil {
		t.Fatalf("add page: %v", err)
	}
	if err := handleSetup(cfg, false, strings.NewReader(""), " http://homeassistant.lan:8123/lovelace/0 "); err != nil {
		t.Fatalf("set start url: %v", err)
	}
	reloaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	pages := reloaded.Kiosk.Pages
	if len(pages) != 2 {
		t.Fatalf("pages = %d, want 2", len(pages))
	}
	if pages[0].URL != "http://homeassistant.lan:8123/lovelace/0" || pages[0].Name != "Kiosk" || pages[0].SourceType != "home_assistant" {
		t.Fatalf("first page = %+v", pages[0])
	}
	if pages[1].URL != "https://second.example" {
		t.Fatalf("second page changed: %+v", pages[1])
	}
	if reloaded.Kiosk.URLs[0] != pages[0].URL {
		t.Fatalf("urls not derived from pages: %v", reloaded.Kiosk.URLs)
	}
}

func TestSetStartURLCreatesPage(t *testing.T) {
	cfg, _ := loadTestConfig(t)
	cfg.Kiosk.Pages = nil
	if err := setStartURL(cfg, "https://dashboard.example/"); err != nil {
		t.Fatalf("set start url: %v", err)
	}
	pages := cfg.Snapshot().Kiosk.Pages
	if len(pages) != 1 || pages[0].URL != "https://dashboard.example/" || pages[0].Name != "Kiosk" {
		t.Fatalf("pages = %+v", pages)
	}
}

func TestSetStartURLRejectsInvalidURL(t *testing.T) {
	cfg, _ := loadTestConfig(t)
	before := cfg.Snapshot().Kiosk.Pages[0].URL
	if err := setStartURL(cfg, "file:///etc/passwd"); err == nil {
		t.Fatalf("expected validation error")
	}
	if got := cfg.Snapshot().Kiosk.Pages[0].URL; got != before {
		t.Fatalf("url changed to %q after rejected update", got)
	}
}
