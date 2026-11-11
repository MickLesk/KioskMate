package main

import (
	"os"
	"runtime/debug"
	"strings"
)

// lowPowerMemoryLimit is the soft heap limit for KioskMate itself on small
// boards. KioskMate normally needs 15-30 MB; the limit only makes the Go
// garbage collector work harder before the heap grows past it, it never
// fails an allocation.
const lowPowerMemoryLimit = 48 << 20

// applyMemoryLimit sets a soft memory limit in the low-power profiles, so
// KioskMate leaves more RAM to Chromium. A GOMEMLIMIT from the environment
// always wins. It returns the limit in bytes, or 0 when nothing was set.
func applyMemoryLimit(profile string) int64 {
	if strings.TrimSpace(os.Getenv("GOMEMLIMIT")) != "" {
		return 0
	}
	switch strings.ToLower(strings.TrimSpace(profile)) {
	case "raspberry", "low-power", "minimal", "conservative":
		debug.SetMemoryLimit(lowPowerMemoryLimit)
		return lowPowerMemoryLimit
	}
	return 0
}
