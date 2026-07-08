package supervisor

import (
	"io"
	"log/slog"
	"os/exec"
	"testing"
	"time"

	"github.com/MickLesk/KioskMate/internal/config"
)

func TestSchedulerRotationTargetsConfiguredDurations(t *testing.T) {
	cfg := schedulerTestConfig()
	cfg.Kiosk.Scheduler = config.KioskScheduler{Enabled: true, Mode: "rotation"}
	cfg.Kiosk.Rotation = []config.RotationItem{
		{Page: 0, DurationSeconds: 60},
		{Page: 1, DurationSeconds: 120},
	}
	browser := NewBrowser(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	now := time.Date(2026, 7, 7, 13, 0, 0, 0, time.Local)

	page, status := browser.schedulerTarget(now)
	if page != 0 || status.Reason != "rotation" || status.NextSwitch == nil || status.NextSwitch.Sub(now) != time.Minute {
		t.Fatalf("first rotation = page %d, status %#v", page, status)
	}

	page, status = browser.schedulerTarget(now.Add(61 * time.Second))
	if page != 1 || status.RotationIndex != 1 || status.NextSwitch == nil || status.NextSwitch.Sub(now.Add(61*time.Second)) != 2*time.Minute {
		t.Fatalf("second rotation = page %d, status %#v", page, status)
	}
}

func TestSchedulerTimeRuleOverridesHybridRotation(t *testing.T) {
	cfg := schedulerTestConfig()
	cfg.Kiosk.Scheduler = config.KioskScheduler{Enabled: true, Mode: "hybrid"}
	cfg.Kiosk.Rotation = []config.RotationItem{{Page: 0, DurationSeconds: 60}}
	cfg.Kiosk.TimeRules = []config.TimeRule{{
		Name:  "Lunch Dashboard",
		Page:  1,
		Start: "13:00",
		End:   "14:00",
		Days:  []string{"tue"},
	}}
	browser := NewBrowser(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))

	page, status := browser.schedulerTarget(time.Date(2026, 7, 7, 13, 30, 0, 0, time.Local))
	if page != 1 || status.Reason != "time" || status.ActiveRule != "Lunch Dashboard" {
		t.Fatalf("hybrid time rule = page %d, status %#v", page, status)
	}
}

func TestSchedulerOvernightRule(t *testing.T) {
	cfg := schedulerTestConfig()
	cfg.Kiosk.Scheduler = config.KioskScheduler{Enabled: true, Mode: "time"}
	cfg.Kiosk.TimeRules = []config.TimeRule{{
		Name:  "Night",
		Page:  1,
		Start: "22:00",
		End:   "06:00",
	}}
	browser := NewBrowser(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))

	for _, hour := range []int{23, 2} {
		page, status := browser.schedulerTarget(time.Date(2026, 7, 7, hour, 10, 0, 0, time.Local))
		if page != 1 || status.Reason != "time" {
			t.Fatalf("overnight hour %d = page %d, status %#v", hour, page, status)
		}
	}
}

func TestExpectedBrowserStopDoesNotRecordLastError(t *testing.T) {
	cfg := schedulerTestConfig()
	cfg.Kiosk.BrowserCommand = "go"
	browser := NewBrowser(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	cmd := exec.Command("go", "env", "-badflag")
	if err := cmd.Start(); err != nil {
		t.Skipf("go command unavailable: %v", err)
	}
	browser.mu.Lock()
	browser.cmd = cmd
	browser.stopping = true
	browser.mu.Unlock()

	browser.wait(cmd)

	status := browser.Status()
	if status.LastError != "" {
		t.Fatalf("expected no last error after intentional stop, got %q", status.LastError)
	}
	if status.LastExit == nil {
		t.Fatal("expected last exit timestamp")
	}
}

func TestUnexpectedBrowserExitRecordsLastError(t *testing.T) {
	cfg := schedulerTestConfig()
	cfg.Kiosk.BrowserCommand = "go"
	browser := NewBrowser(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	cmd := exec.Command("go", "env", "-badflag")
	if err := cmd.Start(); err != nil {
		t.Skipf("go command unavailable: %v", err)
	}
	browser.mu.Lock()
	browser.cmd = cmd
	browser.mu.Unlock()

	browser.wait(cmd)

	status := browser.Status()
	if status.LastError == "" {
		t.Fatal("expected last error after unexpected exit")
	}
}

func schedulerTestConfig() *config.Config {
	return &config.Config{
		Kiosk: config.KioskConfig{
			Pages: []config.KioskPage{
				{Name: "Main", URL: "http://ha.local/main"},
				{Name: "Calendar", URL: "http://ha.local/calendar"},
			},
		},
	}
}
