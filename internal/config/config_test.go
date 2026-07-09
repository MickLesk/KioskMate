package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadCreatesKioskMateConfig(t *testing.T) {
	home := testHome(t)
	path := filepath.Join(home, ".config", "kioskmate", "config.json")

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Path != path {
		t.Fatalf("path = %s, want %s", cfg.Path, path)
	}
	if cfg.MQTT.Node != "kioskmate" {
		t.Fatalf("mqtt node = %s, want kioskmate", cfg.MQTT.Node)
	}
	if cfg.Update.Repository != "MickLesk/KioskMate" || cfg.Update.Service != "kioskmate.service" {
		t.Fatalf("update config = %#v", cfg.Update)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected config to be saved: %v", err)
	}
}

func TestLoadPreservesExistingKioskMateConfig(t *testing.T) {
	home := testHome(t)
	path := filepath.Join(home, ".config", "kioskmate", "config.json")
	writeFile(t, path, `{
  "version": 2,
  "admin": {"bind": "0.0.0.0", "port": 33333, "token": "keep-token"},
  "kiosk": {"pages": [{"name": "Main", "url": "http://homeassistant.local:8123"}]},
  "mqtt": {"enabled": true, "url": "mqtt://ha.local:1883", "node": "panel"},
  "update": {"repository": "MickLesk/KioskMate", "service": "kioskmate.service"}
}`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Admin.Token != "keep-token" {
		t.Fatalf("admin token = %s", cfg.Admin.Token)
	}
	if got := cfg.Kiosk.PageURLs(); len(got) != 1 || got[0] != "http://homeassistant.local:8123" {
		t.Fatalf("kiosk pages = %#v", got)
	}
	if cfg.MQTT.Node != "panel" || cfg.MQTT.URL != "mqtt://ha.local:1883" {
		t.Fatalf("mqtt config = %#v", cfg.MQTT)
	}
}

func TestLoadNormalizesStaleProjectIdentity(t *testing.T) {
	home := testHome(t)
	path := filepath.Join(home, ".config", "kioskmate", "config.json")
	writeFile(t, path, `{
  "version": 2,
  "admin": {"bind": "0.0.0.0", "port": 33333, "token": "keep-token"},
  "kiosk": {"pages": [{"name": "Main", "url": "http://homeassistant.local:8123"}]},
  "mqtt": {"node": "kioskmate"},
  "update": {"repository": "MickLesk/`+"touch"+`kio", "service": "`+"touch"+`kio-v2.service"}
}`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Update.Repository != "MickLesk/KioskMate" {
		t.Fatalf("repository = %s", cfg.Update.Repository)
	}
	if cfg.Update.Service != "kioskmate.service" {
		t.Fatalf("service = %s", cfg.Update.Service)
	}
}

func TestLoadPreservesMQTT5(t *testing.T) {
	home := testHome(t)
	path := filepath.Join(home, ".config", "kioskmate", "config.json")
	writeFile(t, path, `{
  "version": 2,
  "admin": {"bind": "0.0.0.0", "port": 33333},
  "kiosk": {"pages": [{"name": "Main", "url": "http://homeassistant.local:8123"}]},
  "mqtt": {"enabled": true, "url": "mqtt://ha.local:1883", "node": "panel", "version": "5.0"},
  "update": {"repository": "MickLesk/KioskMate", "service": "kioskmate.service"}
}`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MQTT.Version != "5.0" {
		t.Fatalf("mqtt version = %s, want 5.0", cfg.MQTT.Version)
	}
	report := Repair(cfg)
	if cfg.MQTT.Version != "5.0" {
		t.Fatalf("repair reset mqtt version to %s", cfg.MQTT.Version)
	}
	for _, issue := range report.Issues {
		if issue.ID == "mqtt_version" && issue.Fixed {
			t.Fatalf("repair unexpectedly fixed mqtt_version: %#v", report)
		}
	}
}

func TestLoadMigratesAggressiveWatchdogDefaults(t *testing.T) {
	home := testHome(t)
	path := filepath.Join(home, ".config", "kioskmate", "config.json")
	writeFile(t, path, `{
  "version": 2,
  "admin": {"bind": "0.0.0.0", "port": 33333},
  "kiosk": {"pages": [{"name": "Main", "url": "http://homeassistant.local:8123"}]},
  "watchdog": {"enabled": true, "check_interval": 10000000000, "max_rss_mb": 900, "max_cpu_percent": 180, "cpu_grace": 45000000000},
  "update": {"repository": "MickLesk/KioskMate", "service": "kioskmate.service"}
}`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Watchdog.CPUGrace != 10*time.Minute {
		t.Fatalf("cpu grace = %s, want 10m", cfg.Watchdog.CPUGrace)
	}
	if cfg.Watchdog.MaxCPUPercent != 300 {
		t.Fatalf("max cpu = %.1f, want 300", cfg.Watchdog.MaxCPUPercent)
	}
}

func TestLoadNormalizesKioskTheme(t *testing.T) {
	home := testHome(t)
	path := filepath.Join(home, ".config", "kioskmate", "config.json")
	writeFile(t, path, `{
  "version": 2,
  "admin": {"bind": "0.0.0.0", "port": 33333},
  "kiosk": {"theme": "force-dark", "pages": [{"name": "Main", "url": "http://homeassistant.local:8123"}]},
  "update": {"repository": "MickLesk/KioskMate", "service": "kioskmate.service"}
}`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Kiosk.Theme != "force-dark" {
		t.Fatalf("theme = %s, want force-dark", cfg.Kiosk.Theme)
	}

	cfg.Kiosk.Theme = "invalid"
	normalize(cfg)
	if cfg.Kiosk.Theme != "dark" {
		t.Fatalf("invalid theme normalized to %s, want dark", cfg.Kiosk.Theme)
	}
}

func TestRaspberrySafeModeUsesLowPowerBrowser(t *testing.T) {
	cfg := defaults("")
	ApplyRaspberrySafeMode(&cfg)

	if cfg.Kiosk.BrowserPreset != "chromium-lite" {
		t.Fatalf("browser preset = %s, want chromium-lite", cfg.Kiosk.BrowserPreset)
	}
	if cfg.Performance.Profile != "low-power" {
		t.Fatalf("profile = %s, want low-power", cfg.Performance.Profile)
	}
	if cfg.Performance.GPUMode != "auto" {
		t.Fatalf("gpu mode = %s, want auto", cfg.Performance.GPUMode)
	}
}

func TestRepairMigratesStaleBrowserProfilePath(t *testing.T) {
	home := testHome(t)
	cfg := defaults(filepath.Join(home, ".config", "kioskmate", "config.json"))
	cfg.Kiosk.UserDataDir = filepath.Join(home, ".config", "touchkio-v2", "Browser")

	report := Repair(&cfg)

	if cfg.Kiosk.UserDataDir != filepath.Join(home, ".config", "kioskmate", "Browser") {
		t.Fatalf("user data dir = %s", cfg.Kiosk.UserDataDir)
	}
	if !report.Changed {
		t.Fatal("expected repair report to mark config as changed")
	}
}

func TestSaveBacksUpChangedConfig(t *testing.T) {
	home := testHome(t)
	path := filepath.Join(home, ".config", "kioskmate", "config.json")
	writeFile(t, path, `{"version":2,"admin":{"bind":"0.0.0.0","port":33333,"token":"old"},"kiosk":{"urls":["http://old.local"]}}`)

	cfg := defaults(path)
	cfg.Admin.Token = "new"
	if err := Save(&cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".bak"); err != nil {
		t.Fatalf("expected backup before rewrite: %v", err)
	}
}

func testHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	return home
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
