package main

import (
	"math"
	"runtime/debug"
	"testing"
)

func TestApplyMemoryLimit(t *testing.T) {
	defer debug.SetMemoryLimit(math.MaxInt64)
	t.Setenv("GOMEMLIMIT", "")
	if got := applyMemoryLimit("quality"); got != 0 {
		t.Fatalf("quality profile set a limit: %d", got)
	}
	if got := applyMemoryLimit("raspberry"); got != lowPowerMemoryLimit {
		t.Fatalf("raspberry limit = %d", got)
	}
	if got := debug.SetMemoryLimit(-1); got != lowPowerMemoryLimit {
		t.Fatalf("runtime limit = %d", got)
	}
	t.Setenv("GOMEMLIMIT", "100MiB")
	if got := applyMemoryLimit("raspberry"); got != 0 {
		t.Fatalf("environment limit should win, got %d", got)
	}
}
