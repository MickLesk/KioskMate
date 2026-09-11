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

func TestProcessRoleClassifiesCurrentProcessAsRoot(t *testing.T) {
	if role := processRole(os.Getpid(), os.Getpid()); role != "browser" {
		t.Fatalf("role = %q, want browser", role)
	}
}

func TestReadProcessTreeStatsIncludesRoles(t *testing.T) {
	stats, err := ReadProcessTreeStats(os.Getpid(), ProcessTreeStats{})
	if err != nil {
		t.Fatal(err)
	}
	if stats.Roles["browser"].Count != 1 || len(stats.PIDs) == 0 {
		t.Fatalf("unexpected process roles: %#v", stats.Roles)
	}
}
