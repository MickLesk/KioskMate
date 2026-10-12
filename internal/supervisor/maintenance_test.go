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
