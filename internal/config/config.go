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
	"sync"
	"time"
)

type Config struct {
	mu          *sync.RWMutex  `json:"-"`
	changeCh    chan struct{}  `json:"-"`
	Path        string         `json:"-"`
	Version     int            `json:"version"`
	Admin       AdminConfig    `json:"admin"`
	Kiosk       KioskConfig    `json:"kiosk"`
	Performance PerfConfig     `json:"performance"`
	Watchdog    WatchdogConfig `json:"watchdog"`
	MQTT        MQTTConfig     `json:"mqtt"`
	Time        TimeConfig     `json:"time"`
	Update      UpdateConfig   `json:"update"`
}

type AdminConfig struct {
	Bind         string `json:"bind"`
	Port         int    `json:"port"`
	Token        string `json:"token"`
	PasswordHash string `json:"password_hash,omitempty"`
	TLSCert      string `json:"tls_cert,omitempty"`
	TLSKey       string `json:"tls_key,omitempty"`
}

func (c AdminConfig) Addr() string {
	return fmt.Sprintf("%s:%d", c.Bind, c.Port)
}

type KioskConfig struct {
	URLs            []string       `json:"urls"`
	Pages           []KioskPage    `json:"pages"`
	BrowserPreset   string         `json:"browser_preset"`
	BrowserCommand  string         `json:"browser_command"`
	ExtraArgs       []string       `json:"extra_args"`
	UserDataDir     string         `json:"user_data_dir"`
	IsolateSessions bool           `json:"isolate_page_sessions"`
	Theme           string         `json:"theme"`
	ZoomPercent     int            `json:"zoom_percent"`
	Widget          bool           `json:"widget"`
	Scheduler       KioskScheduler `json:"scheduler"`
	Rotation        []RotationItem `json:"rotation"`
	TimeRules       []TimeRule     `json:"time_rules"`
}

type KioskPage struct {
	PageID          string              `json:"page_id"`
	Name            string              `json:"name"`
	URL             string              `json:"url"`
	SourceType      string              `json:"source_type,omitempty"`
	DisplayMode     string              `json:"display_mode,omitempty"`
	DurationSeconds int                 `json:"duration_seconds,omitempty"`
	Schedule        KioskPageSchedule   `json:"schedule,omitempty"`
	Trigger         KioskPageTrigger    `json:"trigger,omitempty"`
	DisplayOptions  KioskDisplayOptions `json:"display_options,omitempty"`
	Disabled        bool                `json:"disabled"`
}

type KioskPageSchedule struct {
	Start string   `json:"start,omitempty"`
	End   string   `json:"end,omitempty"`
	Days  []string `json:"days,omitempty"`
}

type KioskPageTrigger struct {
	Topic   string `json:"topic,omitempty"`
	Payload string `json:"payload,omitempty"`
}

type KioskDisplayOptions struct {
	PowerOffAfter bool `json:"power_off_after,omitempty"`
	Screensaver   bool `json:"screensaver,omitempty"`
	Brightness    *int `json:"brightness,omitempty"`
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
	Enabled            bool          `json:"enabled"`
	URL                string        `json:"url"`
	Version            string        `json:"version"`
	Username           string        `json:"username"`
	Password           string        `json:"password"`
	PasswordConfigured bool          `json:"password_configured,omitempty"`
	Discovery          string        `json:"discovery"`
	BaseTopic          string        `json:"base_topic"`
	Node               string        `json:"node"`
	ClientID           string        `json:"client_id"`
	KeepAlive          time.Duration `json:"keepalive"`
	ForceDisableRetain bool          `json:"force_disable_retain"`
	Interval           time.Duration `json:"interval"`
}

type TimeConfig struct {
	NTPServer string `json:"ntp_server"`
	Timezone  string `json:"timezone"`
}

type UpdateConfig struct {
	Repository string `json:"repository"`
	Prerelease bool   `json:"prerelease"`
	Service    string `json:"service"`
}

type RepairReport struct {
	Changed bool          `json:"changed"`
	Issues  []RepairIssue `json:"issues"`
}

type RepairIssue struct {
	ID      string `json:"id"`
	Message string `json:"message"`
	Fixed   bool   `json:"fixed"`
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
	cfg.mu = &sync.RWMutex{}
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
	mu := cfg.mutex()
	mu.Lock()
	defer mu.Unlock()
	return saveLocked(cfg)
}

func saveLocked(cfg *Config) error {
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
	return atomicWriteFile(cfg.Path, append(data, '\n'), 0o600)
}

func (cfg *Config) mutex() *sync.RWMutex {
	if cfg.mu == nil {
		// Configs constructed by tests or embedders are initialized lazily before
		// they are shared with worker goroutines.
		cfg.mu = &sync.RWMutex{}
	}
	return cfg.mu
}

func (cfg *Config) Snapshot() *Config {
	mu := cfg.mutex()
	mu.RLock()
	defer mu.RUnlock()
	clone := *cfg
	clone.mu = mu
	clone.changeCh = nil
	return &clone
}

func (cfg *Config) Changes() <-chan struct{} {
	mu := cfg.mutex()
	mu.Lock()
	defer mu.Unlock()
	if cfg.changeCh == nil {
		cfg.changeCh = make(chan struct{}, 1)
	}
	return cfg.changeCh
}

func (cfg *Config) notifyChangedLocked() {
	if cfg.changeCh == nil {
		cfg.changeCh = make(chan struct{}, 1)
	}
	select {
	case cfg.changeCh <- struct{}{}:
	default:
	}
}

func (cfg *Config) ReadLock() {
	cfg.mutex().RLock()
}

func (cfg *Config) ReadUnlock() {
	cfg.mutex().RUnlock()
}

func (cfg *Config) Mutate(change func(*Config) error) error {
	mu := cfg.mutex()
	mu.Lock()
	defer mu.Unlock()
	data, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	var next Config
	if err := json.Unmarshal(data, &next); err != nil {
		return err
	}
	next.Path = cfg.Path
	next.mu = mu
	next.MQTT.PasswordConfigured = false
	if err := change(&next); err != nil {
		return err
	}
	next.MQTT.PasswordConfigured = false
	if err := saveLocked(&next); err != nil {
		return err
	}
	changeCh := cfg.changeCh
	*cfg = next
	cfg.mu = mu
	cfg.changeCh = changeCh
	cfg.notifyChangedLocked()
	return nil
}

func (cfg *Config) Replace(next *Config) error {
	if next == nil {
		return errors.New("replacement config is nil")
	}
	mu := cfg.mutex()
	mu.Lock()
	defer mu.Unlock()
	next.Path = cfg.Path
	next.mu = mu
	next.MQTT.PasswordConfigured = false
	if err := saveLocked(next); err != nil {
		return err
	}
	changeCh := cfg.changeCh
	*cfg = *next
	cfg.mu = mu
	cfg.changeCh = changeCh
	cfg.notifyChangedLocked()
	return nil
}

func atomicWriteFile(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".kioskmate-config-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
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

func ConfigDir(path string) string {
	if path == "" {
		path = defaultPath()
	}
	return filepath.Dir(path)
}

func LogFilePath(path string) string {
	return filepath.Join(ConfigDir(path), "logs", "kioskmate.log")
}

func BrowserLogFilePath(path string) string {
	return filepath.Join(ConfigDir(path), "logs", "browser.log")
}

func defaults(path string) Config {
	return Config{
		mu:      &sync.RWMutex{},
		Path:    path,
		Version: 2,
		Admin: AdminConfig{
			Bind:  "0.0.0.0",
			Port:  33333,
			Token: randomToken(),
		},
		Kiosk: KioskConfig{
			URLs:          []string{"https://demo.home-assistant.io"},
			Pages:         []KioskPage{{Name: "Home Assistant Demo", URL: "https://demo.home-assistant.io"}},
			BrowserPreset: "chromium",
			Theme:         "dark",
			ZoomPercent:   125,
			Widget:        true,
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
			MaxCPUPercent: 300,
			CPUGrace:      10 * time.Minute,
		},
		MQTT: MQTTConfig{
			Enabled:   false,
			Discovery: "homeassistant",
			BaseTopic: "kioskmate",
			Node:      "kioskmate",
			KeepAlive: 60 * time.Second,
			Interval:  30 * time.Second,
		},
		Time: TimeConfig{
			NTPServer: "pool.ntp.org",
		},
		Update: UpdateConfig{
			Repository: "MickLesk/KioskMate",
			Service:    "kioskmate.service",
		},
	}
}

func normalize(cfg *Config) {
	cfg.MQTT.PasswordConfigured = false
	if cfg.Version == 0 {
		cfg.Version = 2
	}
	// v3: schedule workflows historically saved time_rules with scheduler.enabled=false,
	// which made Zeitplan/display power a no-op until the user found "Run automatically".
	if cfg.Version < 3 {
		if len(cfg.Kiosk.TimeRules) > 0 {
			cfg.Kiosk.Scheduler.Enabled = true
			if cfg.Kiosk.Scheduler.Mode == "" || cfg.Kiosk.Scheduler.Mode == "rotation" {
				if len(cfg.Kiosk.Rotation) == 0 {
					cfg.Kiosk.Scheduler.Mode = "time"
				} else {
					cfg.Kiosk.Scheduler.Mode = "hybrid"
				}
			}
		}
		for i := range cfg.Kiosk.Pages {
			if cfg.Kiosk.Pages[i].DisplayMode == "schedule" {
				cfg.Kiosk.Pages[i].DisplayOptions.PowerOffAfter = true
			}
		}
		cfg.Version = 3
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
	migrateLegacySequence(&cfg.Kiosk)
	normalizeKioskPages(&cfg.Kiosk)
	cfg.Kiosk.URLs = cfg.Kiosk.PageURLs()
	if cfg.Kiosk.BrowserPreset == "" {
		cfg.Kiosk.BrowserPreset = "chromium"
	}
	if cfg.Kiosk.UserDataDir == "" || staleProjectValue(cfg.Kiosk.UserDataDir) {
		cfg.Kiosk.UserDataDir = defaultBrowserDataDir()
	}
	if cfg.Kiosk.Theme == "" {
		cfg.Kiosk.Theme = "dark"
	}
	cfg.Kiosk.Theme = NormalizeKioskTheme(cfg.Kiosk.Theme)
	if cfg.Kiosk.ZoomPercent == 0 {
		cfg.Kiosk.ZoomPercent = 125
	}
	if cfg.Kiosk.Scheduler.Mode == "" {
		cfg.Kiosk.Scheduler.Mode = "rotation"
	}
	if cfg.Kiosk.Scheduler.TickInterval == 0 {
		cfg.Kiosk.Scheduler.TickInterval = 15 * time.Second
	}
	if len(cfg.Kiosk.Rotation) == 0 && len(cfg.Kiosk.PageURLs()) > 0 && cfg.Kiosk.Scheduler.Mode != "time" {
		cfg.Kiosk.Rotation = []RotationItem{{Page: 0, DurationSeconds: 3600}}
	}
	if cfg.Performance.Profile == "" {
		cfg.Performance.Profile = defaultProfile()
	}
	if !supportedPerformanceProfile(cfg.Performance.Profile) {
		cfg.Performance.Profile = defaultProfile()
	}
	if cfg.Performance.GPUMode == "" {
		cfg.Performance.GPUMode = "auto"
	}
	if cfg.Watchdog.CheckInterval == 0 {
		cfg.Watchdog.CheckInterval = 10 * time.Second
	}
	if cfg.Watchdog.CPUGrace == 0 {
		cfg.Watchdog.CPUGrace = 10 * time.Minute
	}
	if cfg.Watchdog.CPUGrace == 45*time.Second || cfg.Watchdog.CPUGrace == 120*time.Second {
		cfg.Watchdog.CPUGrace = 10 * time.Minute
	}
	if cfg.Watchdog.MaxRSSMB == 0 {
		cfg.Watchdog.MaxRSSMB = 900
	}
	if cfg.Performance.Profile == "raspberry" && cfg.Watchdog.MaxRSSMB == 700 {
		cfg.Watchdog.MaxRSSMB = 1200
	}
	if cfg.Performance.Profile == "raspberry" && cfg.Watchdog.MaxCPUPercent == 160 {
		cfg.Watchdog.MaxCPUPercent = 220
	}
	if cfg.Watchdog.MaxCPUPercent == 0 || cfg.Watchdog.MaxCPUPercent == 180 || cfg.Watchdog.MaxCPUPercent == 220 {
		cfg.Watchdog.MaxCPUPercent = 300
	}
	if cfg.MQTT.Discovery == "" {
		cfg.MQTT.Discovery = "homeassistant"
	}
	if cfg.MQTT.BaseTopic == "" {
		cfg.MQTT.BaseTopic = "kioskmate"
	}
	if cfg.MQTT.Version == "" || !supportedMQTTVersion(cfg.MQTT.Version) {
		cfg.MQTT.Version = "3.1.1"
	}
	if cfg.MQTT.Node == "" {
		cfg.MQTT.Node = "kioskmate"
	}
	if cfg.MQTT.Interval == 0 {
		cfg.MQTT.Interval = 30 * time.Second
	}
	if cfg.MQTT.KeepAlive == 0 {
		cfg.MQTT.KeepAlive = 60 * time.Second
	}
	if strings.TrimSpace(cfg.Time.NTPServer) == "" {
		cfg.Time.NTPServer = "pool.ntp.org"
	}
	cfg.Time.NTPServer = strings.TrimSpace(cfg.Time.NTPServer)
	cfg.Time.Timezone = strings.TrimSpace(cfg.Time.Timezone)
	if cfg.Update.Repository != "MickLesk/KioskMate" {
		cfg.Update.Repository = "MickLesk/KioskMate"
	}
	if cfg.Update.Service == "" || staleProjectValue(cfg.Update.Service) {
		cfg.Update.Service = "kioskmate.service"
	}
}

func migrateLegacySequence(kiosk *KioskConfig) {
	if len(kiosk.Rotation) == 0 || len(kiosk.Pages) == 0 {
		return
	}
	for _, page := range kiosk.Pages {
		if page.PageID != "" {
			return
		}
	}
	var enabled []KioskPage
	for _, page := range kiosk.Pages {
		if !page.Disabled && strings.TrimSpace(page.URL) != "" {
			enabled = append(enabled, page)
		}
	}
	if len(enabled) == 0 {
		return
	}
	canonical := len(kiosk.Rotation) == len(enabled)
	for index, item := range kiosk.Rotation {
		if item.Page < 0 || item.Page >= len(enabled) {
			return
		}
		if item.Page != index {
			canonical = false
		}
	}
	if canonical {
		return
	}

	rotationCount := len(kiosk.Rotation)
	sequence := make([]KioskPage, 0, rotationCount+len(kiosk.Pages))
	firstIndex := make(map[int]int, len(enabled))
	referenced := make(map[int]bool, len(enabled))
	for _, item := range kiosk.Rotation {
		page := enabled[item.Page]
		page.DurationSeconds = item.DurationSeconds
		if _, exists := firstIndex[item.Page]; !exists {
			firstIndex[item.Page] = len(sequence)
		}
		referenced[item.Page] = true
		sequence = append(sequence, page)
	}
	for oldIndex, page := range enabled {
		if referenced[oldIndex] {
			continue
		}
		firstIndex[oldIndex] = len(sequence)
		sequence = append(sequence, page)
	}
	for _, page := range kiosk.Pages {
		if page.Disabled || strings.TrimSpace(page.URL) == "" {
			sequence = append(sequence, page)
		}
	}
	for index := range kiosk.TimeRules {
		if mapped, ok := firstIndex[kiosk.TimeRules[index].Page]; ok {
			kiosk.TimeRules[index].Page = mapped
		}
	}
	kiosk.Pages = sequence
	kiosk.Rotation = make([]RotationItem, 0, rotationCount)
	for index, page := range sequence[:rotationCount] {
		duration := page.DurationSeconds
		if duration <= 0 {
			duration = 3600
		}
		kiosk.Rotation = append(kiosk.Rotation, RotationItem{Page: index, DurationSeconds: duration})
	}
}

func normalizeKioskPages(kiosk *KioskConfig) {
	used := make(map[string]bool, len(kiosk.Pages))
	for index := range kiosk.Pages {
		page := &kiosk.Pages[index]
		if page.PageID == "" {
			page.PageID = legacyPageID(page.Name, index, used)
		} else if used[page.PageID] {
			page.PageID = uniquePageID(page.PageID, used)
		}
		used[page.PageID] = true
		if page.SourceType == "" {
			if strings.Contains(strings.ToLower(page.URL), "home-assistant") || strings.Contains(strings.ToLower(page.URL), ":8123") {
				page.SourceType = "home_assistant"
			} else {
				page.SourceType = "url"
			}
		}
		if page.DisplayMode == "" {
			page.DisplayMode = "duration"
			for _, rule := range kiosk.TimeRules {
				if rule.Page == index && !rule.Disabled {
					page.DisplayMode = "schedule"
					page.Schedule = KioskPageSchedule{Start: rule.Start, End: rule.End, Days: append([]string(nil), rule.Days...)}
					break
				}
			}
		}
		if page.DurationSeconds <= 0 {
			page.DurationSeconds = 3600
			for _, item := range kiosk.Rotation {
				if item.Page == index && item.DurationSeconds > 0 {
					page.DurationSeconds = item.DurationSeconds
					break
				}
			}
		}
		if page.DisplayOptions.Brightness == nil {
			brightness := 100
			page.DisplayOptions.Brightness = &brightness
		}
	}
}

func legacyPageID(name string, index int, used map[string]bool) string {
	var builder strings.Builder
	lastUnderscore := false
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			builder.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore && builder.Len() > 0 {
			builder.WriteByte('_')
			lastUnderscore = true
		}
	}
	base := strings.Trim(builder.String(), "_")
	if base == "" {
		base = fmt.Sprintf("page_%d", index+1)
	}
	return uniquePageID(base, used)
}

func uniquePageID(base string, used map[string]bool) string {
	if !used[base] {
		return base
	}
	for suffix := 2; ; suffix++ {
		candidate := fmt.Sprintf("%s_%d", base, suffix)
		if !used[candidate] {
			return candidate
		}
	}
}

func Repair(cfg *Config) RepairReport {
	report := RepairReport{}
	add := func(id string, message string, fixed bool) {
		report.Issues = append(report.Issues, RepairIssue{ID: id, Message: message, Fixed: fixed})
		if fixed {
			report.Changed = true
		}
	}
	if cfg.Update.Repository == "" || staleProjectValue(cfg.Update.Repository) || cfg.Update.Repository != "MickLesk/KioskMate" {
		cfg.Update.Repository = "MickLesk/KioskMate"
		add("update_repository", "Update repository reset to MickLesk/KioskMate.", true)
	}
	if cfg.Update.Service == "" || staleProjectValue(cfg.Update.Service) || cfg.Update.Service != "kioskmate.service" {
		cfg.Update.Service = "kioskmate.service"
		add("update_service", "Systemd user service reset to kioskmate.service.", true)
	}
	if cfg.Admin.Bind == "127.0.0.1" || cfg.Admin.Bind == "localhost" || cfg.Admin.Bind == "" {
		cfg.Admin.Bind = "0.0.0.0"
		add("admin_bind", "Admin bind address reset to 0.0.0.0 for LAN access.", true)
	}
	if cfg.Admin.Port == 0 {
		cfg.Admin.Port = 33333
		add("admin_port", "Admin port reset to 33333.", true)
	}
	if cfg.MQTT.Node == "" || staleProjectValue(cfg.MQTT.Node) {
		cfg.MQTT.Node = "kioskmate"
		add("mqtt_node", "MQTT node reset to kioskmate.", true)
	}
	if cfg.MQTT.BaseTopic == "" || staleProjectValue(cfg.MQTT.BaseTopic) {
		cfg.MQTT.BaseTopic = "kioskmate"
		add("mqtt_base_topic", "MQTT base topic reset to kioskmate.", true)
	}
	if cfg.MQTT.Version == "" || !supportedMQTTVersion(cfg.MQTT.Version) {
		cfg.MQTT.Version = "3.1.1"
		add("mqtt_version", "MQTT protocol reset to 3.1.1 because the configured version is unsupported.", true)
	}
	if cfg.Kiosk.UserDataDir == "" || staleProjectValue(cfg.Kiosk.UserDataDir) {
		cfg.Kiosk.UserDataDir = defaultBrowserDataDir()
		add("browser_profile", "Browser profile path reset to KioskMate config directory.", true)
	}
	if len(cfg.Kiosk.PageURLs()) == 0 {
		cfg.Kiosk.Pages = []KioskPage{{Name: "Home Assistant Demo", URL: "https://demo.home-assistant.io"}}
		cfg.Kiosk.URLs = cfg.Kiosk.PageURLs()
		add("kiosk_pages", "Added a default kiosk page because no active URL was configured.", true)
	}
	normalize(cfg)
	if len(report.Issues) == 0 {
		add("ok", "No repairable issues found.", false)
	}
	return report
}

func ApplyRaspberrySafeMode(cfg *Config) {
	cfg.Kiosk.BrowserPreset = "chromium-lite"
	cfg.Performance.Profile = "low-power"
	cfg.Performance.GPUMode = "auto"
	cfg.Performance.ReduceMotion = true
	cfg.Watchdog.Enabled = true
	cfg.Watchdog.CheckInterval = 10 * time.Second
	cfg.Watchdog.CPUGrace = 10 * time.Minute
	cfg.Watchdog.MaxRSSMB = 1200
	cfg.Watchdog.MaxCPUPercent = 300
	cfg.Kiosk.ExtraArgs = appendUnique(cfg.Kiosk.ExtraArgs,
		"--disable-dev-shm-usage",
		"--disable-extensions",
		"--disable-sync",
		"--metrics-recording-only",
	)
	normalize(cfg)
}

func appendUnique(values []string, additions ...string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	for _, value := range additions {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func supportedMQTTVersion(version string) bool {
	switch strings.TrimSpace(version) {
	case "3.1.1", "5.0":
		return true
	default:
		return false
	}
}

func supportedPerformanceProfile(profile string) bool {
	switch strings.TrimSpace(strings.ToLower(profile)) {
	case "quality", "balanced", "raspberry", "low-power", "minimal", "conservative":
		return true
	default:
		return false
	}
}

func NormalizeKioskTheme(theme string) string {
	switch strings.TrimSpace(strings.ToLower(theme)) {
	case "light", "dark", "force-dark":
		return strings.TrimSpace(strings.ToLower(theme))
	default:
		return "dark"
	}
}

func staleProjectValue(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return false
	}
	stale := []string{
		"touch" + "kio",
		"go." + "kiosk",
		"go-" + "kiosk",
		"go_" + "kiosk",
	}
	for _, item := range stale {
		if strings.Contains(normalized, item) {
			return true
		}
	}
	return false
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
		return "low-power"
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
