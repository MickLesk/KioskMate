//go:build linux

package system

import (
	"os"
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
