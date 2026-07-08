package config

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type Config struct {
	Path        string         `json:"-"`
	Version     int            `json:"version"`
	Admin       AdminConfig    `json:"admin"`
	Kiosk       KioskConfig    `json:"kiosk"`
	Performance PerfConfig     `json:"performance"`
	Watchdog    WatchdogConfig `json:"watchdog"`
	MQTT        MQTTConfig     `json:"mqtt"`
	Update      UpdateConfig   `json:"update"`
}

type AdminConfig struct {
	Bind         string `json:"bind"`
	Port         int    `json:"port"`
	Token        string `json:"token"`
	PasswordHash string `json:"password_hash,omitempty"`
}

func (c AdminConfig) Addr() string {
	return fmt.Sprintf("%s:%d", c.Bind, c.Port)
}

type KioskConfig struct {
	URLs           []string       `json:"urls"`
	Pages          []KioskPage    `json:"pages"`
	BrowserCommand string         `json:"browser_command"`
	ExtraArgs      []string       `json:"extra_args"`
	UserDataDir    string         `json:"user_data_dir"`
	Theme          string         `json:"theme"`
	ZoomPercent    int            `json:"zoom_percent"`
	Widget         bool           `json:"widget"`
	Scheduler      KioskScheduler `json:"scheduler"`
	Rotation       []RotationItem `json:"rotation"`
	TimeRules      []TimeRule     `json:"time_rules"`
}

type KioskPage struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	Disabled bool   `json:"disabled"`
}

type KioskScheduler struct {
	Enabled      bool          `json:"enabled"`
	Mode         string        `json:"mode"`
	TickInterval time.Duration `json:"tick_interval"`
}

type RotationItem struct {
	Page            int `json:"page"`
	DurationSeconds int `json:"duration_seconds"`
}

type TimeRule struct {
	Name     string   `json:"name"`
	Page     int      `json:"page"`
	Start    string   `json:"start"`
	End      string   `json:"end"`
	Days     []string `json:"days"`
	Disabled bool     `json:"disabled"`
}

type PerfConfig struct {
	Profile      string `json:"profile"`
	GPUMode      string `json:"gpu_mode"`
	ReduceMotion bool   `json:"reduce_motion"`
}

type WatchdogConfig struct {
	Enabled       bool          `json:"enabled"`
	CheckInterval time.Duration `json:"check_interval"`
	MaxRSSMB      uint64        `json:"max_rss_mb"`
	MaxCPUPercent float64       `json:"max_cpu_percent"`
	CPUGrace      time.Duration `json:"cpu_grace"`
}

type MQTTConfig struct {
	Enabled   bool          `json:"enabled"`
	URL       string        `json:"url"`
	Version   string        `json:"version"`
	Username  string        `json:"username"`
	Password  string        `json:"password"`
	Discovery string        `json:"discovery"`
	Node      string        `json:"node"`
	Interval  time.Duration `json:"interval"`
}

type UpdateConfig struct {
	Repository string `json:"repository"`
	Prerelease bool   `json:"prerelease"`
	Service    string `json:"service"`
}

func (k KioskConfig) PageURLs() []string {
	var urls []string
	for _, page := range k.Pages {
		url := strings.TrimSpace(page.URL)
		if page.Disabled || url == "" {
			continue
		}
		urls = append(urls, url)
	}
	if len(urls) > 0 {
		return urls
	}
	for _, url := range k.URLs {
		if strings.TrimSpace(url) != "" {
			urls = append(urls, strings.TrimSpace(url))
		}
	}
	return urls
}

func (k KioskConfig) PageName(index int) string {
	enabled := k.enabledPages()
	if index >= 0 && index < len(enabled) {
		if enabled[index].Name != "" {
			return enabled[index].Name
		}
		return fmt.Sprintf("Kiosk %d", index+1)
	}
	return ""
}

func (k KioskConfig) PageCount() int {
	return len(k.PageURLs())
}

func (k KioskConfig) enabledPages() []KioskPage {
	var pages []KioskPage
	for _, page := range k.Pages {
		if !page.Disabled && strings.TrimSpace(page.URL) != "" {
			pages = append(pages, page)
		}
	}
	return pages
}

func Load(path string) (*Config, error) {
	if path == "" {
		path = defaultPath()
	}
	cfg := defaults(path)
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	cfg.Path = path
	normalize(&cfg)
	if err := Save(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func Save(cfg *Config) error {
	if cfg.Path == "" {
		cfg.Path = defaultPath()
	}
	normalize(cfg)
	if err := os.MkdirAll(filepath.Dir(cfg.Path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := backupIfChanged(cfg.Path, data); err != nil {
		return err
	}
	return os.WriteFile(cfg.Path, append(data, '\n'), 0o600)
}

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

func defaults(path string) Config {
	return Config{
		Path:    path,
		Version: 2,
		Admin: AdminConfig{
			Bind:  "0.0.0.0",
			Port:  33333,
			Token: randomToken(),
		},
		Kiosk: KioskConfig{
			URLs:        []string{"https://demo.home-assistant.io"},
			Pages:       []KioskPage{{Name: "Home Assistant Demo", URL: "https://demo.home-assistant.io"}},
			Theme:       "dark",
			ZoomPercent: 125,
			Widget:      true,
			Scheduler: KioskScheduler{
				Enabled:      false,
				Mode:         "rotation",
				TickInterval: 15 * time.Second,
			},
			Rotation: []RotationItem{{Page: 0, DurationSeconds: 3600}},
		},
		Performance: PerfConfig{
			Profile:      defaultProfile(),
			GPUMode:      "auto",
			ReduceMotion: true,
		},
		Watchdog: WatchdogConfig{
			Enabled:       true,
			CheckInterval: 10 * time.Second,
			MaxRSSMB:      900,
			MaxCPUPercent: 180,
			CPUGrace:      45 * time.Second,
		},
		MQTT: MQTTConfig{
			Enabled:   false,
			Discovery: "homeassistant",
			Node:      "kioskmate",
			Interval:  30 * time.Second,
		},
		Update: UpdateConfig{
			Repository: "MickLesk/KioskMate",
			Service:    "kioskmate.service",
		},
	}
}

func normalize(cfg *Config) {
	if cfg.Version == 0 {
		cfg.Version = 2
	}
	if cfg.Admin.Bind == "127.0.0.1" || cfg.Admin.Bind == "localhost" {
		cfg.Admin.Bind = "0.0.0.0"
	}
	if cfg.Admin.Bind == "" {
		cfg.Admin.Bind = "0.0.0.0"
	}
	if cfg.Admin.Port == 0 {
		cfg.Admin.Port = 33333
	}
	if cfg.Admin.Token == "" {
		cfg.Admin.Token = randomToken()
	}
	if len(cfg.Kiosk.URLs) == 0 {
		cfg.Kiosk.URLs = []string{"https://demo.home-assistant.io"}
	}
	if len(cfg.Kiosk.Pages) == 0 {
		for i, url := range cfg.Kiosk.URLs {
			cfg.Kiosk.Pages = append(cfg.Kiosk.Pages, KioskPage{Name: fmt.Sprintf("Kiosk %d", i+1), URL: url})
		}
	}
	cfg.Kiosk.URLs = cfg.Kiosk.PageURLs()
	if cfg.Kiosk.UserDataDir == "" {
		cfg.Kiosk.UserDataDir = defaultBrowserDataDir()
	}
	if cfg.Kiosk.Theme == "" {
		cfg.Kiosk.Theme = "dark"
	}
	if cfg.Kiosk.ZoomPercent == 0 {
		cfg.Kiosk.ZoomPercent = 125
	}
	if cfg.Kiosk.Scheduler.Mode == "" {
		cfg.Kiosk.Scheduler.Mode = "rotation"
	}
	if cfg.Kiosk.Scheduler.TickInterval == 0 {
		cfg.Kiosk.Scheduler.TickInterval = 15 * time.Second
	}
	if len(cfg.Kiosk.Rotation) == 0 && len(cfg.Kiosk.PageURLs()) > 0 {
		cfg.Kiosk.Rotation = []RotationItem{{Page: 0, DurationSeconds: 3600}}
	}
	if cfg.Performance.Profile == "" {
		cfg.Performance.Profile = defaultProfile()
	}
	if cfg.Performance.GPUMode == "" {
		cfg.Performance.GPUMode = "auto"
	}
	if cfg.Watchdog.CheckInterval == 0 {
		cfg.Watchdog.CheckInterval = 10 * time.Second
	}
	if cfg.Watchdog.CPUGrace == 0 {
		cfg.Watchdog.CPUGrace = 45 * time.Second
	}
	if cfg.Watchdog.MaxRSSMB == 0 {
		cfg.Watchdog.MaxRSSMB = 900
	}
	if cfg.Watchdog.MaxCPUPercent == 0 {
		cfg.Watchdog.MaxCPUPercent = 180
	}
	if cfg.MQTT.Discovery == "" {
		cfg.MQTT.Discovery = "homeassistant"
	}
	if cfg.MQTT.Version == "" || cfg.MQTT.Version != "3.1.1" {
		cfg.MQTT.Version = "3.1.1"
	}
	if cfg.MQTT.Node == "" {
		cfg.MQTT.Node = "kioskmate"
	}
	if cfg.MQTT.Interval == 0 {
		cfg.MQTT.Interval = 30 * time.Second
	}
	if cfg.Update.Repository == "" {
		cfg.Update.Repository = "MickLesk/KioskMate"
	}
	if cfg.Update.Service == "" {
		cfg.Update.Service = "kioskmate.service"
	}
}

func isDemoURL(url string) bool {
	normalized := strings.TrimRight(strings.ToLower(strings.TrimSpace(url)), "/")
	return normalized == "https://demo.home-assistant.io" || normalized == "http://homeassistant.local:8123"
}

func backupIfChanged(path string, next []byte) error {
	current, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(current)) == strings.TrimSpace(string(next)) {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	return os.WriteFile(path+".bak", current, info.Mode().Perm())
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

func saveIfMissing(path string, cfg *Config) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

func defaultProfile() string {
	if runtime.GOARCH == "arm64" {
		return "raspberry"
	}
	return "balanced"
}

func randomToken() string {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		return strings.ReplaceAll(time.Now().UTC().Format(time.RFC3339Nano), ":", "")
	}
	return base64.RawURLEncoding.EncodeToString(b[:])
}
