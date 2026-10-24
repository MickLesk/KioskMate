package supervisor

import (
	"testing"
	"time"

	"github.com/MickLesk/KioskMate/internal/config"
	"github.com/MickLesk/KioskMate/internal/signals"
)

func TestPresenceWakesAndBlanksDisplay(t *testing.T) {
	cfg := config.KioskConfig{
		Pages:     []config.KioskPage{{PageID: "home", URL: "http://a", DisplayMode: "schedule"}},
		Scheduler: config.KioskScheduler{Enabled: true, Mode: "time"},
		TimeRules: []config.TimeRule{{Page: 0, Start: "07:00", End: "23:00"}},
		Presence:  config.PresenceConfig{Enabled: true, Mode: "wake", TimeoutMinutes: 5},
	}
	store := signals.New()
	night := time.Date(2026, 3, 1, 2, 0, 0, 0, time.UTC)
	off := SchedulerStatus{Reason: "no active time rule"}

	if status := applyPresence(night, cfg, store, off); !scheduleWantsDisplayOff(cfg, -1, status) || status.Presence != "absent" {
		t.Fatalf("nobody there at night: display should stay off, got %+v", status)
	}
	store.NotePresence(night.Add(-2*time.Minute), "mqtt")
	status := applyPresence(night, cfg, store, off)
	if scheduleWantsDisplayOff(cfg, -1, status) {
		t.Fatal("presence should wake the display outside the schedule")
	}
	if status.NextSwitch == nil || !status.NextSwitch.Equal(night.Add(3*time.Minute)) {
		t.Fatalf("scheduler should wake when presence runs out, got %v", status.NextSwitch)
	}
	if status := applyPresence(night.Add(4*time.Minute), cfg, store, off); !scheduleWantsDisplayOff(cfg, -1, status) {
		t.Fatal("display should go off again after the timeout")
	}

	day := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	on := SchedulerStatus{Reason: "time"}
	if scheduleWantsDisplayOff(cfg, 0, applyPresence(day, cfg, store, on)) {
		t.Fatal("wake mode must not blank the display inside the schedule")
	}
	cfg.Presence.Mode = "require"
	if !scheduleWantsDisplayOff(cfg, 0, applyPresence(day, cfg, store, on)) {
		t.Fatal("require mode blanks the display when nobody is there")
	}
	store.NotePresence(day, "touch")
	if scheduleWantsDisplayOff(cfg, 0, applyPresence(day, cfg, store, on)) {
		t.Fatal("require mode keeps the display on while someone is there")
	}
}

func TestTimeRuleWithSunTimes(t *testing.T) {
	cfg := config.KioskConfig{
		Pages:     []config.KioskPage{{PageID: "a", URL: "http://a"}, {PageID: "b", URL: "http://b"}},
		TimeRules: []config.TimeRule{{Page: 1, Start: "sunset", End: "23:00"}},
	}
	if _, _, _, ok := timeRuleTarget(cfg, time.Date(2026, 3, 1, 17, 59, 0, 0, time.UTC)); ok {
		t.Fatal("rule should not run before sunset (18:00 without location)")
	}
	if page, _, _, ok := timeRuleTarget(cfg, time.Date(2026, 3, 1, 18, 1, 0, 0, time.UTC)); !ok || page != 1 {
		t.Fatal("rule should run after sunset")
	}
}
