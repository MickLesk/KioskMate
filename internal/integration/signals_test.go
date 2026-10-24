package integration

import (
	"testing"

	"github.com/MickLesk/KioskMate/internal/config"
)

func TestPresenceDetected(t *testing.T) {
	for payload, want := range map[string]bool{"ON": true, "on": true, "detected": true, "OFF": false, "false": false, "0": false, "": false, "clear": false} {
		if got := presenceDetected(payload, ""); got != want {
			t.Fatalf("%q: got %v, want %v", payload, got, want)
		}
	}
	if presenceDetected("ON", "occupied") || !presenceDetected("Occupied", "occupied") {
		t.Fatal("a configured value must match exactly (case-insensitive)")
	}
}

func TestSignalTopics(t *testing.T) {
	kiosk := config.KioskConfig{
		Inserts: []config.PageInsert{
			{ID: "a", Condition: &config.InsertCondition{Topic: "hall/motion", Value: "ON"}},
			{ID: "b", Condition: &config.InsertCondition{Topic: "hall/motion", Value: "OFF"}},
			{ID: "c", Disabled: true, Condition: &config.InsertCondition{Topic: "off/topic", Value: "x"}},
		},
		Presence: config.PresenceConfig{Enabled: true, Topic: "hall/presence"},
	}
	got := signalTopics(kiosk)
	if len(got) != 2 || got[0] != "hall/motion" || got[1] != "hall/presence" {
		t.Fatalf("unexpected topics %v", got)
	}
}
