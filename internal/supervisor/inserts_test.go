package supervisor

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/MickLesk/KioskMate/internal/config"
)

func insertTestBrowser(scheduler bool) *Browser {
	cfg := &config.Config{Kiosk: config.KioskConfig{
		Pages: []config.KioskPage{
			{PageID: "ha", Name: "HA", URL: "http://ha.local/main"},
			{PageID: "weather", Name: "Weather", URL: "http://ha.local/weather", DisplayMode: "insert"},
			{PageID: "calendar", Name: "Calendar", URL: "http://ha.local/calendar", DisplayMode: "insert"},
		},
		Scheduler: config.KioskScheduler{Enabled: scheduler, Mode: "time"},
		TimeRules: []config.TimeRule{{Name: "Day", Page: 0, Start: "07:00", End: "23:00"}},
		Inserts: []config.PageInsert{
			{ID: "weather", Name: "Wetter", PageID: "weather", Repeat: "interval", EveryMinutes: 5, DurationSeconds: 30},
			{ID: "calendar", PageID: "calendar", Repeat: "hourly", DurationSeconds: 60, Priority: 1},
		},
	}}
	return NewBrowser(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestSchedulerInsertsInterleaveBaseProgramme(t *testing.T) {
	browser := insertTestBrowser(true)
	day := func(clock string) time.Time {
		parsed, _ := time.ParseInLocation("15:04:05", clock, time.Local)
		return time.Date(2026, 10, 23, parsed.Hour(), parsed.Minute(), parsed.Second(), 0, time.Local)
	}
	cases := []struct {
		clock  string
		page   int
		reason string
	}{
		{"10:03:00", 0, "time"},
		{"10:05:05", 1, "insert"},
		{"10:05:31", 0, "time"},
		{"11:00:10", 2, "insert"},
		// Outside the time rule the display is off: inserts must not wake it.
		{"23:05:05", -1, "no active time rule"},
	}
	for _, tc := range cases {
		page, status := browser.schedulerTarget(day(tc.clock))
		if page != tc.page || status.Reason != tc.reason {
			t.Errorf("%s: page %d reason %q, want %d %q", tc.clock, page, status.Reason, tc.page, tc.reason)
		}
	}
	_, status := browser.schedulerTarget(day("10:03:00"))
	if status.NextSwitch == nil || !status.NextSwitch.Equal(day("10:05:00")) {
		t.Fatalf("next switch = %v, want the next insert at 10:05", status.NextSwitch)
	}
	_, status = browser.schedulerTarget(day("10:05:05"))
	if status.ActiveInsert != "Wetter" {
		t.Fatalf("active insert = %q", status.ActiveInsert)
	}
}

func TestSchedulerInsertReturnsToPreviousPageWithoutScheduler(t *testing.T) {
	browser := insertTestBrowser(false)
	browser.active = 0
	day := time.Date(2026, 10, 23, 10, 5, 5, 0, time.Local)
	if page, status := browser.schedulerTarget(day); page != 1 || status.Reason != "insert" {
		t.Fatalf("insert not shown: %d %q", page, status.Reason)
	}
	browser.active = 1
	if page, _ := browser.schedulerTarget(day.Add(30 * time.Second)); page != 0 {
		t.Fatalf("after the insert the page shown before should return, got %d", page)
	}
	if page, _ := browser.schedulerTarget(day.Add(40 * time.Second)); page != -1 {
		t.Fatalf("without a scheduler the page stays manual afterwards, got %d", page)
	}
}

func TestInsertKeepsDisplayOnInTimeMode(t *testing.T) {
	browser := insertTestBrowser(true)
	day := time.Date(2026, 10, 23, 10, 5, 5, 0, time.Local)
	page, status := browser.schedulerTarget(day)
	if page != 1 || status.Reason != "insert" {
		t.Fatalf("insert not shown: %d %q", page, status.Reason)
	}
	// Turning the display off here blanked the base page and reloaded it
	// from scratch after every insert.
	if scheduleWantsDisplayOff(browser.cfg.Snapshot().Kiosk, page, status) {
		t.Fatal("an insert on screen must not turn the display off")
	}
}
