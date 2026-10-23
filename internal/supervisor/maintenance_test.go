package supervisor

import (
	"io"
	"log/slog"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MickLesk/KioskMate/internal/config"
)

func runningTestBrowser(t *testing.T, started time.Time) *Browser {
	t.Helper()
	cfg := schedulerTestConfig()
	cfg.Path = filepath.Join(t.TempDir(), "config.json")
	browser := NewBrowser(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	cmd := exec.Command("true")
	if err := cmd.Start(); err != nil {
		t.Skip("cannot start helper process")
	}
	t.Cleanup(func() { _ = cmd.Wait() })
	browser.cmd = cmd
	browser.started = started
	return browser
}

func TestMaintenanceReloadEveryInterval(t *testing.T) {
	start := time.Date(2026, 10, 12, 8, 0, 0, 0, time.UTC)
	browser := runningTestBrowser(t, start)
	maintenance := config.KioskMaintenance{ReloadEveryMinutes: 60}
	if browser.maintenanceReloadDue(start.Add(59*time.Minute), maintenance) {
		t.Fatal("reload before interval")
	}
	if !browser.maintenanceReloadDue(start.Add(61*time.Minute), maintenance) {
		t.Fatal("reload not due after interval")
	}
	if browser.maintenanceReloadDue(start.Add(62*time.Minute), maintenance) {
		t.Fatal("reload repeated immediately")
	}
}

func TestMaintenanceReloadDailyOnce(t *testing.T) {
	start := time.Date(2026, 10, 12, 1, 0, 0, 0, time.UTC)
	browser := runningTestBrowser(t, start)
	maintenance := config.KioskMaintenance{ReloadAt: "03:30"}
	if browser.maintenanceReloadDue(start.Add(2*time.Hour), maintenance) {
		t.Fatal("reload before daily time")
	}
	at := time.Date(2026, 10, 12, 3, 30, 10, 0, time.UTC)
	if !browser.maintenanceReloadDue(at, maintenance) {
		t.Fatal("daily reload not due")
	}
	if browser.maintenanceReloadDue(at.Add(time.Hour), maintenance) {
		t.Fatal("daily reload ran twice")
	}
	if !browser.maintenanceReloadDue(at.Add(24*time.Hour), maintenance) {
		t.Fatal("daily reload not due the next day")
	}
}

func TestNextSchedulerWakeUsesEarlierSwitch(t *testing.T) {
	now := time.Now()
	next := now.Add(3 * time.Second)
	if wake := nextSchedulerWake(now, 15*time.Second, &next); wake > 4*time.Second {
		t.Fatalf("wake = %s", wake)
	}
	if wake := nextSchedulerWake(now, 15*time.Second, nil); wake != 15*time.Second {
		t.Fatalf("wake without switch = %s", wake)
	}
}

func TestOverlayScriptEscapesText(t *testing.T) {
	script := overlayScript(`</script><img src=x onerror=alert(1)> "quoted"`, 3*time.Second)
	if strings.Contains(script, "<img") || strings.Contains(script, "</script>") || !strings.Contains(script, `\"quoted\"`) {
		t.Fatalf("overlay text not JSON-encoded:\n%s", script)
	}
	if !strings.Contains(script, "setTimeout(() => box.remove(), 3000)") {
		t.Fatal("overlay duration missing")
	}
}

func TestMaintenanceReloadAfterRotationCycles(t *testing.T) {
	start := time.Date(2026, 10, 12, 8, 0, 0, 0, time.UTC)
	browser := runningTestBrowser(t, start)
	maintenance := config.KioskMaintenance{ReloadAfterCycles: 2}
	browser.rotationCycles = 1
	if browser.maintenanceReloadDue(start.Add(time.Minute), maintenance) {
		t.Fatal("reload after one round")
	}
	browser.rotationCycles = 2
	if !browser.maintenanceReloadDue(start.Add(2*time.Minute), maintenance) {
		t.Fatal("reload not due after two rounds")
	}
	if browser.rotationCycles != 0 {
		t.Fatalf("cycle counter not reset: %d", browser.rotationCycles)
	}
}

func TestOzonePlatformFlag(t *testing.T) {
	cases := []struct {
		name, backend, wayland, x11, want string
		extra                             []string
	}{
		// 0.8 behaviour on a Pi desktop: no switch, Chromium uses XWayland.
		{name: "desktop default", wayland: "wayland-0", x11: ":0", want: ""},
		{name: "x11 session", x11: ":0", want: ""},
		{name: "cage without xwayland", wayland: "wayland-0", want: "--ozone-platform=wayland"},
		{name: "forced wayland", backend: "wayland", wayland: "wayland-0", x11: ":0", want: "--ozone-platform=wayland"},
		{name: "forced x11", backend: "x11", wayland: "wayland-0", want: "--ozone-platform=x11"},
		{name: "auto", backend: "auto", wayland: "wayland-0", x11: ":0", want: ""},
		{name: "extra args win", backend: "wayland", wayland: "wayland-0", extra: []string{"--ozone-platform=x11"}, want: ""},
	}
	for _, tc := range cases {
		if got := ozonePlatformFlag(tc.backend, tc.extra, tc.wayland, tc.x11); got != tc.want {
			t.Errorf("%s: flag = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestEvictTabsKeepsPreloadedPages(t *testing.T) {
	browser := runningTestBrowser(t, time.Now())
	cfg := &config.Config{Kiosk: config.KioskConfig{MaxTabs: 2, Pages: []config.KioskPage{
		{URL: "https://a", Preload: true}, {URL: "https://b"}, {URL: "https://c"},
	}}}
	browser.tabs = map[int]string{0: "t0", 1: "t1", 2: "t2"}
	browser.tabURL = map[int]string{}
	browser.tabUse = []int{0, 1, 2}
	evicted := browser.evictTabsLocked(cfg, 2)
	if len(evicted) != 1 || evicted[0] != "t1" {
		t.Fatalf("evicted = %v, want [t1]", evicted)
	}
	if _, ok := browser.tabs[0]; !ok {
		t.Fatal("preloaded tab was evicted")
	}
}
