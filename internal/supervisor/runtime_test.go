package supervisor

import (
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/MickLesk/KioskMate/internal/config"
	"github.com/MickLesk/KioskMate/internal/system"
)

func TestRecoveryDelayUsesBoundedExponentialBackoff(t *testing.T) {
	wants := []time.Duration{5 * time.Second, 10 * time.Second, 20 * time.Second, 40 * time.Second}
	for index, want := range wants {
		if got := recoveryDelay(index + 1); got != want {
			t.Fatalf("attempt %d delay = %s, want %s", index+1, got, want)
		}
	}
	if got := recoveryDelay(99); got != 10*time.Minute {
		t.Fatalf("maximum delay = %s", got)
	}
}

func TestTelemetryPersistsAndRestoresRuntimeCounters(t *testing.T) {
	cfg := schedulerTestConfig()
	cfg.Path = filepath.Join(t.TempDir(), "config.json")
	browser := NewBrowser(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	browser.mu.Lock()
	browser.startCount = 7
	browser.restartCount = 3
	browser.recordTelemetryLocked(system.ProcessTreeStats{PIDs: []int{1, 2, 3}, RSSMB: 512, CPUPercent: 88.5})
	browser.mu.Unlock()
	browser.persistRuntimeState()

	restored := NewBrowser(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	history := restored.Telemetry()
	if history.Summary.Samples != 1 || history.Summary.RSSMaximumMB != 512 || history.Summary.ProcessMaximum != 3 {
		t.Fatalf("restored telemetry = %#v", history)
	}
	if history.Summary.BrowserStartCount != 7 || history.Summary.RestartCount != 3 {
		t.Fatalf("restored counters = %#v", history.Summary)
	}
}

func TestSchedulerManualOverrideExpiresBackToWorkflow(t *testing.T) {
	cfg := schedulerTestConfig()
	cfg.Kiosk.Scheduler.Enabled = true
	cfg.Kiosk.Scheduler.Mode = "rotation"
	cfg.Kiosk.Rotation = []config.RotationItem{{Page: 0, DurationSeconds: 60}}
	browser := NewBrowser(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	now := time.Now()
	until := now.Add(time.Minute)
	browser.override = ManualOverride{Active: true, Page: 1, Until: &until}

	page, status := browser.schedulerTarget(now)
	if page != 1 || status.Mode != "override" {
		t.Fatalf("active override = page %d, %#v", page, status)
	}
	page, status = browser.schedulerTarget(now.Add(2 * time.Minute))
	if page != 0 || status.Mode != "rotation" || browser.override.Active {
		t.Fatalf("expired override = page %d, %#v, override=%#v", page, status, browser.override)
	}
}

func TestAuthGuardClassificationIncludesIPBanRecovery(t *testing.T) {
	kind, action := classifyAuthGuard("Home Assistant returned HTTP 403 Forbidden")
	if kind != "ip_ban" || action == "" {
		t.Fatalf("classification = %q, %q", kind, action)
	}
}
