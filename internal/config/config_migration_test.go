package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMergesLegacyIntoDefaultV2Config(t *testing.T) {
	home := testHome(t)
	v2Path := filepath.Join(home, ".config", "kioskmate", "config.json")
	writeFile(t, v2Path, `{
  "version": 2,
  "admin": {"bind": "0.0.0.0", "port": 33333, "token": "keep-token"},
  "kiosk": {"urls": ["https://demo.home-assistant.io"], "pages": [{"name": "Home Assistant Demo", "url": "https://demo.home-assistant.io"}]},
  "mqtt": {}
}`)
	writeFile(t, filepath.Join(home, ".config", LegacyAppLower(), "Arguments.json"), `{
  "web_url": "http://homeassistant.local:8123/lovelace/main,http://homeassistant.local:8123/lovelace/calendar",
  "mqtt_url": "mqtt://homeassistant.local:1883",
  "mqtt_user": "kiosk",
  "mqtt_password": "secret",
  "web_theme": "light",
  "web_zoom": "1.5",
  "admin_password_hash": "legacy-hash"
}`)

	cfg, err := Load(v2Path)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Kiosk.PageURLs(); len(got) != 2 || got[0] != "http://homeassistant.local:8123/lovelace/main" {
		t.Fatalf("legacy kiosk pages were not imported: %#v", got)
	}
	if cfg.MQTT.URL != "mqtt://homeassistant.local:1883" || cfg.MQTT.Username != "kiosk" || cfg.MQTT.Password != "secret" {
		t.Fatalf("legacy mqtt was not imported: %#v", cfg.MQTT)
	}
	if cfg.Admin.Token != "keep-token" || cfg.Admin.PasswordHash != "legacy-hash" {
		t.Fatalf("admin fields not preserved/imported: %#v", cfg.Admin)
	}
	if _, err := os.Stat(v2Path + ".bak"); err != nil {
		t.Fatalf("expected backup before rewrite: %v", err)
	}
}

func TestLoadDoesNotOverwriteCustomV2KioskWithLegacy(t *testing.T) {
	home := testHome(t)
	v2Path := filepath.Join(home, ".config", "kioskmate", "config.json")
	writeFile(t, v2Path, `{
  "version": 2,
  "admin": {"bind": "0.0.0.0", "port": 33333, "token": "keep-token"},
  "kiosk": {"pages": [{"name": "Custom", "url": "http://custom.local"}]},
  "mqtt": {}
}`)
	writeFile(t, filepath.Join(home, ".config", LegacyAppLower(), "Arguments.json"), `{
  "web_url": "http://legacy.local",
  "mqtt_url": "mqtt://legacy.local:1883"
}`)

	cfg, err := Load(v2Path)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Kiosk.PageURLs(); len(got) != 1 || got[0] != "http://custom.local" {
		t.Fatalf("custom v2 kiosk was overwritten: %#v", got)
	}
	if cfg.MQTT.URL != "mqtt://legacy.local:1883" {
		t.Fatalf("empty v2 mqtt should still import legacy mqtt: %#v", cfg.MQTT)
	}
}

func TestLoadMigratesPreviousV2ConfigPath(t *testing.T) {
	home := testHome(t)
	newPath := filepath.Join(home, ".config", "kioskmate", "config.json")
	oldPath := filepath.Join(home, ".config", LegacyAppLower()+"-v2", "config.json")
	writeFile(t, oldPath, `{
  "version": 2,
  "admin": {"bind": "0.0.0.0", "port": 33333, "token": "keep-token"},
  "kiosk": {"pages": [{"name": "Kitchen", "url": "http://ha.local/kitchen"}]},
  "mqtt": {"enabled": true, "url": "mqtt://ha.local:1883", "node": "old_node"}
}`)

	cfg, err := Load(newPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Path != newPath {
		t.Fatalf("config path = %s, want %s", cfg.Path, newPath)
	}
	if got := cfg.Kiosk.PageURLs(); len(got) != 1 || got[0] != "http://ha.local/kitchen" {
		t.Fatalf("previous v2 kiosk was not imported: %#v", got)
	}
	if cfg.MQTT.URL != "mqtt://ha.local:1883" || cfg.MQTT.Node != "old_node" {
		t.Fatalf("previous v2 mqtt was not imported: %#v", cfg.MQTT)
	}
	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("expected migrated config to be saved: %v", err)
	}
}

func TestLoadMigratesPreviousBrandConfigPath(t *testing.T) {
	home := testHome(t)
	newPath := filepath.Join(home, ".config", "kioskmate", "config.json")
	oldPath := filepath.Join(home, ".config", PreviousBrandLower(), "config.json")
	writeFile(t, oldPath, `{
  "version": 2,
  "admin": {"bind": "0.0.0.0", "port": 33333, "token": "keep-token"},
  "kiosk": {"pages": [{"name": "Hallway", "url": "http://ha.local/hallway"}]},
  "mqtt": {"enabled": true, "url": "mqtt://ha.local:1883", "node": "old_node"}
}`)

	cfg, err := Load(newPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Kiosk.PageURLs(); len(got) != 1 || got[0] != "http://ha.local/hallway" {
		t.Fatalf("previous brand kiosk was not imported: %#v", got)
	}
	if cfg.MQTT.URL != "mqtt://ha.local:1883" || cfg.MQTT.Node != "old_node" {
		t.Fatalf("previous brand mqtt was not imported: %#v", cfg.MQTT)
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
