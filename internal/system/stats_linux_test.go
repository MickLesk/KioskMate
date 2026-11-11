//go:build linux

package system

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadPSSKBFromCurrentProcess(t *testing.T) {
	pss, ok := readPSSKB(os.Getpid())
	if !ok {
		t.Skip("smaps_rollup unavailable")
	}
	if pss == 0 {
		t.Fatal("expected non-zero PSS for current process")
	}
}

func TestProcessTreeStatsReusesRecentPSS(t *testing.T) {
	first, err := ReadProcessTreeStats(os.Getpid(), ProcessTreeStats{})
	if err != nil {
		t.Fatal(err)
	}
	if first.pssAt.IsZero() {
		t.Fatal("first sample did not record a memory reading")
	}
	second, err := ReadProcessTreeStats(os.Getpid(), first)
	if err != nil {
		t.Fatal(err)
	}
	if !second.pssAt.Equal(first.pssAt) || second.RSSMB != first.RSSMB {
		t.Fatalf("memory was re-read within %s: %v -> %v", pssInterval, first.pssAt, second.pssAt)
	}
}

func TestFindTemperatureSensor(t *testing.T) {
	root := t.TempDir()
	write := func(zone, kind, temp string) {
		dir := filepath.Join(root, zone)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		_ = os.WriteFile(filepath.Join(dir, "type"), []byte(kind+"\n"), 0o644)
		_ = os.WriteFile(filepath.Join(dir, "temp"), []byte(temp+"\n"), 0o644)
	}
	write("thermal_zone0", "acpitz", "30000")
	write("thermal_zone1", "cpu-thermal", "51234")
	path := findTemperatureSensor(root)
	if path != filepath.Join(root, "thermal_zone1", "temp") {
		t.Fatalf("sensor = %q", path)
	}
	if got := readTemperature(path); got < 51.2 || got > 51.3 {
		t.Fatalf("temperature = %v", got)
	}
	if findTemperatureSensor(filepath.Join(root, "missing")) != "" {
		t.Fatal("missing root should find nothing")
	}
}
