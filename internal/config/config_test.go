package config

import (
	"os"
	"path/filepath"
	"testing"
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
