package integration

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/MickLesk/KioskMate/internal/config"
	"github.com/MickLesk/KioskMate/internal/hardware"
	"github.com/MickLesk/KioskMate/internal/supervisor"
)

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
	service.handleCommand(context.Background(), service.root()+"/next/execute", "")
	service.handleCommand(context.Background(), service.root()+"/previous/execute", "")
	service.handleCommand(context.Background(), service.root()+"/reset_session/execute", "")

	if browser.nexts != 1 || browser.previous != 1 || browser.resetSession != 1 {
		t.Fatalf("browser command counts = next %d previous %d reset %d", browser.nexts, browser.previous, browser.resetSession)
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

func TestMQTTCommandsUpdateConfigControls(t *testing.T) {
	cfg := mqttTestConfig(t)
	browser := &fakeBrowser{}
	service := NewMQTTService(cfg, browser, hardware.New(), nil, nil, "test", slog.New(slog.NewTextHandler(io.Discard, nil)))

	service.handleCommand(context.Background(), service.root()+"/scheduler_tick/set", "45")
	service.handleCommand(context.Background(), service.root()+"/performance_profile/set", "raspberry")
	service.handleCommand(context.Background(), service.root()+"/gpu_mode/set", "software")
	service.handleCommand(context.Background(), service.root()+"/reduce_motion/set", "ON")
	service.handleCommand(context.Background(), service.root()+"/isolate_page_sessions/set", "ON")
	service.handleCommand(context.Background(), service.root()+"/watchdog_enabled/set", "OFF")

	if cfg.Kiosk.Scheduler.TickInterval != 45*time.Second {
		t.Fatalf("scheduler tick = %s, want 45s", cfg.Kiosk.Scheduler.TickInterval)
	}
	if cfg.Performance.Profile != "raspberry" || cfg.Performance.GPUMode != "software" || !cfg.Performance.ReduceMotion || !cfg.Kiosk.IsolateSessions || cfg.Watchdog.Enabled {
		t.Fatalf("config controls not updated: %#v %#v", cfg.Performance, cfg.Watchdog)
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
