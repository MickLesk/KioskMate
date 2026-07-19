package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNormalizeV3EnablesSchedulerForTimeRules(t *testing.T) {
	cfg := &Config{
		Version: 2,
		Kiosk: KioskConfig{
			Scheduler: KioskScheduler{Enabled: false, Mode: "rotation"},
			TimeRules: []TimeRule{{Name: "Day", Page: 0, Start: "07:00", End: "23:00"}},
			Pages:     []KioskPage{{Name: "Main", URL: "http://ha.local", DisplayMode: "schedule"}},
		},
	}
	normalize(cfg)
	if cfg.Version != 3 {
		t.Fatalf("version = %d, want 3", cfg.Version)
	}
	if !cfg.Kiosk.Scheduler.Enabled {
		t.Fatal("scheduler should be enabled for existing time rules")
	}
	if cfg.Kiosk.Scheduler.Mode != "time" {
		t.Fatalf("mode = %q, want time", cfg.Kiosk.Scheduler.Mode)
	}
	if !cfg.Kiosk.Pages[0].DisplayOptions.PowerOffAfter {
		t.Fatal("schedule page should default power_off_after")
	}
}

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

func TestLoadMigratesPagesToWorkflowMetadata(t *testing.T) {
	home := testHome(t)
	path := filepath.Join(home, ".config", "kioskmate", "config.json")
	writeFile(t, path, `{
  "version": 2,
  "admin": {"bind": "0.0.0.0", "port": 33333},
  "kiosk": {
    "pages": [
      {"name": "Dashboard", "url": "http://homeassistant.local:8123"},
      {"name": "Weather", "url": "https://example.test/weather"}
    ],
    "rotation": [{"page": 0, "duration_seconds": 45}],
    "time_rules": [{"name": "Weather", "page": 1, "start": "13:00", "end": "14:00", "days": ["mon"]}]
  },
  "update": {"repository": "MickLesk/KioskMate", "service": "kioskmate.service"}
}`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	first, second := cfg.Kiosk.Pages[0], cfg.Kiosk.Pages[1]
	if first.PageID == "" || second.PageID == "" || first.PageID == second.PageID {
		t.Fatalf("page ids were not generated uniquely: %q %q", first.PageID, second.PageID)
	}
	if first.SourceType != "home_assistant" || first.DisplayMode != "duration" || first.DurationSeconds != 45 {
		t.Fatalf("first page metadata = %#v", first)
	}
	if second.DisplayMode != "schedule" || second.Schedule.Start != "13:00" || len(second.Schedule.Days) != 1 {
		t.Fatalf("second page schedule = %#v", second)
	}
	if first.DisplayOptions.Brightness == nil || *first.DisplayOptions.Brightness != 100 {
		t.Fatalf("default brightness = %#v", first.DisplayOptions.Brightness)
	}
}

func TestLoadMigratesCustomRotationIntoPageSequence(t *testing.T) {
	home := testHome(t)
	path := filepath.Join(home, ".config", "kioskmate", "config.json")
	writeFile(t, path, `{
  "version": 2,
  "admin": {"bind": "0.0.0.0", "port": 33333},
  "kiosk": {
    "pages": [
      {"name": "Home", "url": "https://example.test/home"},
      {"name": "Weather", "url": "https://example.test/weather"}
    ],
    "rotation": [
      {"page": 1, "duration_seconds": 15},
      {"page": 0, "duration_seconds": 60},
      {"page": 1, "duration_seconds": 10}
    ]
  },
  "update": {"repository": "MickLesk/KioskMate", "service": "kioskmate.service"}
}`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Kiosk.Pages) != 3 || cfg.Kiosk.Pages[0].Name != "Weather" || cfg.Kiosk.Pages[1].Name != "Home" || cfg.Kiosk.Pages[2].Name != "Weather" {
		t.Fatalf("migrated page sequence = %#v", cfg.Kiosk.Pages)
	}
	for index, want := range []int{15, 60, 10} {
		if cfg.Kiosk.Pages[index].DurationSeconds != want || cfg.Kiosk.Rotation[index].Page != index {
			t.Fatalf("step %d = page %#v rotation %#v", index, cfg.Kiosk.Pages[index], cfg.Kiosk.Rotation[index])
		}
	}
	if cfg.Kiosk.Pages[0].PageID == cfg.Kiosk.Pages[2].PageID {
		t.Fatalf("duplicate sequence entries share page id %q", cfg.Kiosk.Pages[0].PageID)
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
	cfg.Kiosk.UserDataDir = filepath.Join(home, ".config", "touch"+"kio-v2", "Browser")

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
