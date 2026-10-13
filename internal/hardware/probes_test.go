package hardware

import (
	"testing"
	"time"
)

func TestMemoProbeCachesValue(t *testing.T) {
	resetProbes()
	calls := 0
	probe := func() string { calls++; return "value" }
	for i := 0; i < 3; i++ {
		if got := memoProbe("test", time.Hour, probe); got != "value" {
			t.Fatalf("memoProbe = %q", got)
		}
	}
	if calls != 1 {
		t.Fatalf("probe ran %d times, want 1", calls)
	}
	resetProbes()
	memoProbe("test", time.Hour, probe)
	if calls != 2 {
		t.Fatalf("probe ran %d times after reset, want 2", calls)
	}
}

func TestMemoProbeRetriesEmptyResultSooner(t *testing.T) {
	resetProbes()
	memoProbe("empty", time.Hour, func() string { return "" })
	probeMu.Lock()
	ttl := probeCache["empty"].ttl
	probeMu.Unlock()
	if ttl != probeFailedTTL {
		t.Fatalf("empty probe ttl = %s, want %s", ttl, probeFailedTTL)
	}
}

func TestRefreshFastSystemCopiesMap(t *testing.T) {
	original := map[string]any{"disk_usage": 42, "uptime_minutes": -1}
	next := refreshFastSystem(original)
	if original["uptime_minutes"] != -1 {
		t.Fatal("refreshFastSystem mutated the cached map")
	}
	if next["disk_usage"] != 42 {
		t.Fatal("slow values were not carried over")
	}
}
