package hardware

import (
	"testing"
	"time"
)

func TestTouchMemoryReportsMissingDevice(t *testing.T) {
	dir := t.TempDir()
	s := New()
	s.SetStateDir(dir)
	now := time.Now()
	present := map[string]any{"touch_devices": []string{"ILITEK ILITEK-TP"}}
	s.noteTouchLocked(present, now)
	if _, missing := present["touch_devices_missing"]; missing {
		t.Fatal("present device reported missing")
	}

	// A restart: the memory comes from the file.
	s = New()
	s.SetStateDir(dir)
	gone := map[string]any{"touch_devices": []string{}}
	s.noteTouchLocked(gone, now.Add(time.Hour))
	missing, _ := gone["touch_devices_missing"].([]string)
	if len(missing) != 1 || missing[0] != "ILITEK ILITEK-TP" {
		t.Fatalf("missing = %#v", gone)
	}
	if seen, _ := gone["touch_last_seen"].(time.Time); !seen.Equal(now) && seen.Sub(now).Abs() > time.Second {
		t.Fatalf("last seen = %v", gone["touch_last_seen"])
	}
}

func TestTouchMemoryOffWithoutStateDir(t *testing.T) {
	s := New()
	system := map[string]any{"touch_devices": []string{}}
	s.noteTouchLocked(system, time.Now())
	if len(system) != 1 {
		t.Fatalf("system changed: %#v", system)
	}
}
