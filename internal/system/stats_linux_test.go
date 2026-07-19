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
