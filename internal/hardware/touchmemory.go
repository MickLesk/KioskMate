package hardware

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"time"
)

// A touchscreen that worked yesterday and is missing today is a much
// stronger signal than "no touch device": the USB controller dropped out
// (often undervoltage) or the cable came loose. The service remembers the
// touch devices it has seen in a small file next to the configuration and
// reports the remembered names as touch_devices_missing when none is found.

type touchMemory struct {
	Devices  []string  `json:"devices"`
	LastSeen time.Time `json:"last_seen"`
}

// SetStateDir enables the touch memory; dir is the configuration directory.
func (s *Service) SetStateDir(dir string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stateDir = dir
	s.touch = loadTouchMemory(dir)
}

func touchMemoryPath(dir string) string {
	return filepath.Join(dir, "touch-devices.json")
}

func loadTouchMemory(dir string) touchMemory {
	var memory touchMemory
	if dir == "" {
		return memory
	}
	if data, err := os.ReadFile(touchMemoryPath(dir)); err == nil {
		_ = json.Unmarshal(data, &memory)
	}
	return memory
}

// noteTouchLocked updates the memory from the current scan and annotates
// system. It writes the file only when the set of devices changes, or once a
// day to refresh last_seen.
func (s *Service) noteTouchLocked(system map[string]any, now time.Time) {
	if system == nil || s.stateDir == "" {
		return
	}
	current, ok := system["touch_devices"].([]string)
	if !ok {
		return
	}
	delete(system, "touch_devices_missing")
	delete(system, "touch_last_seen")
	if len(current) > 0 {
		names := append([]string(nil), current...)
		sort.Strings(names)
		if !slices.Equal(names, s.touch.Devices) || now.Sub(s.touch.LastSeen) > 24*time.Hour {
			s.touch = touchMemory{Devices: names, LastSeen: now}
			if data, err := json.Marshal(s.touch); err == nil {
				_ = os.WriteFile(touchMemoryPath(s.stateDir), data, 0o600)
			}
		}
		return
	}
	if len(s.touch.Devices) > 0 {
		system["touch_devices_missing"] = append([]string(nil), s.touch.Devices...)
		system["touch_last_seen"] = s.touch.LastSeen
	}
}
