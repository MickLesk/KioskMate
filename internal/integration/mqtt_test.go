package integration

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/MickLesk/KioskMate/internal/config"
	"github.com/MickLesk/KioskMate/internal/hardware"
	"github.com/MickLesk/KioskMate/internal/supervisor"
)

func TestMQTTConnectionStatusTracksSuccessAndAuthFailure(t *testing.T) {
	service := NewMQTTService(mqttTestConfig(t), &fakeBrowser{}, hardware.New(), nil, nil, "test", slog.New(slog.NewTextHandler(io.Discard, nil)))
	service.setConnectionState("connecting")
	service.setConnectionResult(context.DeadlineExceeded)
	status := service.ConnectionStatus()
	if status.State != "error" || status.ConsecutiveFailures != 1 || status.LastError == "" {
		t.Fatalf("error status = %#v", status)
	}
	service.setConnectionResult(nil)
	status = service.ConnectionStatus()
	if status.State != "connected" || !status.Connected || status.LastConnected == nil || status.LastPublished == nil || status.ConsecutiveFailures != 0 {
		t.Fatalf("connected status = %#v", status)
	}
	service.setConnectionResult(errors.New("mqtt connack failed: not authorized (reason=0x87)"))
	if got := service.ConnectionStatus().State; got != "auth_error" {
		t.Fatalf("auth status = %q", got)
	}
}

func TestTriggeredPageUsesCustomTopicAndPayload(t *testing.T) {
	cfg := mqttTestConfig(t)
	cfg.Kiosk.Pages = []config.KioskPage{
		{Name: "Disabled", URL: "https://disabled.test", Disabled: true},
		{Name: "Main", URL: "https://main.test"},
		{Name: "Weather", URL: "https://weather.test", DisplayMode: "mqtt", Trigger: config.KioskPageTrigger{Topic: "house/kiosk/weather", Payload: "SHOW"}},
	}
	service := NewMQTTService(cfg, &fakeBrowser{}, hardware.New(), nil, nil, "test", slog.New(slog.NewTextHandler(io.Discard, nil)))

	if page, matched, topicIsTrigger := service.triggeredPage("house/kiosk/weather", "SHOW"); !matched || page != 1 || !topicIsTrigger {
		t.Fatalf("triggered page = %d, matched=%v, topicIsTrigger=%v; want enabled page 1 matched", page, matched, topicIsTrigger)
	}
	if _, matched, topicIsTrigger := service.triggeredPage("house/kiosk/weather", "HIDE"); matched || !topicIsTrigger {
		t.Fatalf("payload mismatch should not match but must still report topicIsTrigger: matched=%v, topicIsTrigger=%v", matched, topicIsTrigger)
	}
	if _, matched, topicIsTrigger := service.triggeredPage("house/kiosk/other", "SHOW"); matched || topicIsTrigger {
		t.Fatalf("unrelated topic should not match or be flagged as a trigger topic: matched=%v, topicIsTrigger=%v", matched, topicIsTrigger)
	}
}

type fakeBrowser struct {
	active       int
	starts       int
	stops        int
	restarts     int
	reloads      int
	nexts        int
	previous     int
	resetSession int
}

func (b *fakeBrowser) Start(context.Context) error {
	b.starts++
	return nil
}

func (b *fakeBrowser) Stop(context.Context) error {
	b.stops++
	return nil
}

func (b *fakeBrowser) Restart(context.Context) error {
	b.restarts++
	return nil
}

func (b *fakeBrowser) Reload(context.Context) error {
	b.reloads++
	return nil
}

func (b *fakeBrowser) Next(context.Context) error {
	b.nexts++
	return nil
}

func (b *fakeBrowser) Previous(context.Context) error {
	b.previous++
	return nil
}

func (b *fakeBrowser) SetActive(_ context.Context, index int) error {
	b.active = index
	return nil
}

func (b *fakeBrowser) ResetSession(context.Context) error {
	b.resetSession++
	return nil
}

func (b *fakeBrowser) CaptureScreenshot(context.Context) ([]byte, error) {
	return nil, nil
}

func (b *fakeBrowser) TripAuthGuard(string) {}

func (b *fakeBrowser) NoteDisplayPower(string) {}

func (b *fakeBrowser) Status() supervisor.Status {
	return supervisor.Status{Active: b.active}
}

func TestMQTTCommandsControlBrowserPages(t *testing.T) {
	cfg := mqttTestConfig(t)
	browser := &fakeBrowser{}
	service := NewMQTTService(cfg, browser, hardware.New(), nil, nil, "test", slog.New(slog.NewTextHandler(io.Discard, nil)))

	service.handleCommand(context.Background(), service.root()+"/page_name/set", "Calendar")
	if browser.active != 1 {
		t.Fatalf("page_name command active index = %d, want 1", browser.active)
	}
	service.handleCommand(context.Background(), service.root()+"/browser/set", "ON")
	service.handleCommand(context.Background(), service.root()+"/browser/set", "OFF")
	service.handleCommand(context.Background(), service.root()+"/next/execute", "")
	service.handleCommand(context.Background(), service.root()+"/previous/execute", "")
	service.handleCommand(context.Background(), service.root()+"/reset_session/execute", "")

	if browser.starts != 1 || browser.stops != 1 || browser.nexts != 1 || browser.previous != 1 || browser.resetSession != 1 {
		t.Fatalf("browser command counts = start %d stop %d next %d previous %d reset %d", browser.starts, browser.stops, browser.nexts, browser.previous, browser.resetSession)
	}
}

func TestMQTTPageEntityCommandSwitchesPage(t *testing.T) {
	cfg := mqttTestConfig(t)
	browser := &fakeBrowser{}
	service := NewMQTTService(cfg, browser, hardware.New(), nil, nil, "test", slog.New(slog.NewTextHandler(io.Discard, nil)))

	service.handleCommand(context.Background(), service.root()+"/pages/calendar/activate", "PRESS")
	if browser.active != 1 {
		t.Fatalf("page entity active index = %d, want 1", browser.active)
	}
}

func TestMQTTPageEntitiesUseStableUniqueSlugs(t *testing.T) {
	cfg := mqttTestConfig(t)
	cfg.Kiosk.Pages = append(cfg.Kiosk.Pages, config.KioskPage{Name: "Calendar", URL: "http://ha.local/calendar-2"})
	service := NewMQTTService(cfg, &fakeBrowser{}, hardware.New(), nil, nil, "test", slog.New(slog.NewTextHandler(io.Discard, nil)))

	entities := service.pageEntities()
	if len(entities) != 3 || entities[1].ID != "calendar" || entities[2].ID != "calendar_2" {
		t.Fatalf("page entity ids = %#v", entities)
	}
}

func TestMQTTDiscoveryIncludesDisplayAndBrowserSwitches(t *testing.T) {
	cfg := mqttTestConfig(t)
	service := NewMQTTService(cfg, &fakeBrowser{}, hardware.New(), nil, nil, "test", slog.New(slog.NewTextHandler(io.Discard, nil)))
	items := service.discoveryResetEntries()

	if !hasDiscoveryEntry(items, "switch", "browser") || !hasDiscoveryEntry(items, "switch", "display_power") || !hasDiscoveryEntry(items, "light", "display") || !hasDiscoveryEntry(items, "button", "restart") || !hasDiscoveryEntry(items, "binary_sensor", "auth_guard") || !hasDiscoveryEntry(items, "binary_sensor", "browser_devtools") {
		t.Fatalf("discovery entries missing browser/display controls: %#v", items)
	}
}

func TestMQTTDiscoveryIncludesUpdaterDiagnostics(t *testing.T) {
	cfg := mqttTestConfig(t)
	service := NewMQTTService(cfg, &fakeBrowser{}, hardware.New(), nil, nil, "test", slog.New(slog.NewTextHandler(io.Discard, nil)))
	items := service.discoveryResetEntries()

	for _, entry := range [][2]string{{"update", "app"}, {"button", "update_check"}, {"button", "update_rollback"}, {"binary_sensor", "update_available"}, {"binary_sensor", "update_installing"}, {"binary_sensor", "update_rollback_available"}, {"sensor", "update_checked_at"}, {"sensor", "update_error"}, {"sensor", "update_rollback_target"}} {
		if !hasDiscoveryEntry(items, entry[0], entry[1]) {
			t.Fatalf("missing updater discovery entry %s/%s", entry[0], entry[1])
		}
	}
}

func hasDiscoveryEntry(items [][2]string, component string, object string) bool {
	for _, item := range items {
		if item[0] == component && item[1] == object {
			return true
		}
	}
	return false
}

func TestMQTTCommandsUpdateConfigControls(t *testing.T) {
	cfg := mqttTestConfig(t)
	browser := &fakeBrowser{}
	service := NewMQTTService(cfg, browser, hardware.New(), nil, nil, "test", slog.New(slog.NewTextHandler(io.Discard, nil)))

	service.handleCommand(context.Background(), service.root()+"/scheduler_tick/set", "45")
	service.handleCommand(context.Background(), service.root()+"/performance_profile/set", "raspberry")
	service.handleCommand(context.Background(), service.root()+"/gpu_mode/set", "software")
	service.handleCommand(context.Background(), service.root()+"/reduce_motion/set", "ON")
	service.handleCommand(context.Background(), service.root()+"/isolate_page_sessions/set", "ON")
	service.handleCommand(context.Background(), service.root()+"/kiosk_theme/set", "dark")
	service.handleCommand(context.Background(), service.root()+"/watchdog_enabled/set", "OFF")

	if cfg.Kiosk.Scheduler.TickInterval != 45*time.Second {
		t.Fatalf("scheduler tick = %s, want 45s", cfg.Kiosk.Scheduler.TickInterval)
	}
	if cfg.Performance.Profile != "raspberry" || cfg.Performance.GPUMode != "software" || !cfg.Performance.ReduceMotion || !cfg.Kiosk.IsolateSessions || cfg.Watchdog.Enabled {
		t.Fatalf("config controls not updated: %#v %#v", cfg.Performance, cfg.Watchdog)
	}
	if cfg.Kiosk.Theme != "dark" {
		t.Fatalf("kiosk theme = %q, want dark", cfg.Kiosk.Theme)
	}
}

func TestMQTTConnectionKeyTracksPageTriggers(t *testing.T) {
	cfg := mqttTestConfig(t)
	service := NewMQTTService(cfg, &fakeBrowser{}, hardware.New(), nil, nil, "test", slog.New(slog.NewTextHandler(io.Discard, nil)))
	before := service.connectionKey()
	cfg.Kiosk.Pages = append(cfg.Kiosk.Pages, config.KioskPage{
		Name:        "Trigger",
		URL:         "http://ha.local/trigger",
		DisplayMode: "mqtt",
		Trigger:     config.KioskPageTrigger{Topic: "home/show", Payload: "ON"},
	})
	after := service.connectionKey()
	if before == after {
		t.Fatal("connection key should change when MQTT page triggers are added")
	}
}

func TestMQTTInvalidPageURLReportsError(t *testing.T) {
	cfg := mqttTestConfig(t)
	before := append([]config.KioskPage(nil), cfg.Kiosk.Pages...)
	service := NewMQTTService(cfg, &fakeBrowser{}, hardware.New(), nil, nil, "test", slog.New(slog.NewTextHandler(io.Discard, nil)))
	service.handleCommand(context.Background(), service.root()+"/page_url/set", "ftp://bad.example")
	if len(cfg.Kiosk.Pages) != len(before) || cfg.Kiosk.Pages[0].URL != before[0].URL {
		t.Fatalf("invalid page url mutated config: %#v", cfg.Kiosk.Pages)
	}
}

func TestAbsolutePageIndexSkipsDisabled(t *testing.T) {
	pages := []config.KioskPage{
		{Name: "Disabled", URL: "https://disabled.test", Disabled: true},
		{Name: "Main", URL: "https://main.test"},
		{Name: "Weather", URL: "https://weather.test"},
	}
	if got := absolutePageIndex(pages, 0); got != 1 {
		t.Fatalf("enabled 0 -> absolute %d, want 1", got)
	}
	if got := absolutePageIndex(pages, 1); got != 2 {
		t.Fatalf("enabled 1 -> absolute %d, want 2", got)
	}
	if got := absolutePageIndex(pages, 2); got != -1 {
		t.Fatalf("enabled 2 -> absolute %d, want -1", got)
	}
}

func TestMQTTPageURLUpdatesEnabledPageNotDisabled(t *testing.T) {
	cfg := mqttTestConfig(t)
	cfg.Kiosk.Pages = []config.KioskPage{
		{Name: "Disabled", URL: "https://disabled.test", Disabled: true},
		{Name: "Main", URL: "https://main.test"},
	}
	browser := &fakeBrowser{active: 0}
	service := NewMQTTService(cfg, browser, hardware.New(), nil, nil, "test", slog.New(slog.NewTextHandler(io.Discard, nil)))
	service.handleCommand(context.Background(), service.root()+"/page_url/set", "https://updated.test")
	if cfg.Kiosk.Pages[0].URL != "https://disabled.test" {
		t.Fatalf("disabled page mutated: %#v", cfg.Kiosk.Pages[0])
	}
	if cfg.Kiosk.Pages[1].URL != "https://updated.test" {
		t.Fatalf("enabled page URL = %q, want updated", cfg.Kiosk.Pages[1].URL)
	}
}

func TestLegacyRpiNodeUsesSerialSuffix(t *testing.T) {
	if got := legacyRpiNode("100000001221af22"); got != "rpi_21AF22" {
		t.Fatalf("legacyRpiNode = %q", got)
	}
}

func TestParseDiscoveryTopic(t *testing.T) {
	component, object, ok := parseDiscoveryTopic("homeassistant/sensor/kioskmate/version/config")
	if !ok || component != "sensor" || object != "version" {
		t.Fatalf("parseDiscoveryTopic = %q %q %v", component, object, ok)
	}
}

func TestCheckPageHealth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	health := checkPageHealth(server.URL)
	if !health.OK || health.StatusCode != http.StatusNoContent || health.Error != "" {
		t.Fatalf("health = %#v", health)
	}

	health = checkPageHealth("ftp://example.invalid")
	if health.OK || health.Error == "" {
		t.Fatalf("unsupported health = %#v", health)
	}
}

func TestHomeAssistantHealthCheckUsesPublicManifest(t *testing.T) {
	got := safeHealthCheckURL("http://homeassistant.local:8123/dashboard-kiosk/main?kiosk")
	if got != "http://homeassistant.local:8123/manifest.json" {
		t.Fatalf("safe health URL = %q", got)
	}
}

func TestRefreshOnePageHealthRotates(t *testing.T) {
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer first.Close()
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer second.Close()

	service := NewMQTTService(mqttTestConfig(t), &fakeBrowser{}, hardware.New(), nil, nil, "test", slog.New(slog.NewTextHandler(io.Discard, nil)))
	pages := []pageEntity{{ID: "first", URL: first.URL}, {ID: "second", URL: second.URL}}

	service.refreshOnePageHealth(pages)
	if !service.health["first"].OK || !service.health["second"].Checked.IsZero() {
		t.Fatalf("first rotation health = %#v", service.health)
	}
	service.refreshOnePageHealth(pages)
	if service.health["second"].OK || service.health["second"].StatusCode != http.StatusForbidden {
		t.Fatalf("second rotation health = %#v", service.health)
	}
}

func mqttTestConfig(t *testing.T) *config.Config {
	t.Helper()
	return &config.Config{
		Path: filepath.Join(t.TempDir(), "config.json"),
		Kiosk: config.KioskConfig{
			Pages: []config.KioskPage{
				{Name: "Main", URL: "http://ha.local/main"},
				{Name: "Calendar", URL: "http://ha.local/calendar"},
			},
			Scheduler: config.KioskScheduler{Mode: "rotation", TickInterval: 15 * time.Second},
		},
		Performance: config.PerfConfig{Profile: "balanced", GPUMode: "auto"},
		Watchdog:    config.WatchdogConfig{Enabled: true},
		MQTT:        config.MQTTConfig{Node: "kioskmate_test", Discovery: "homeassistant", Version: "3.1.1"},
	}
}
