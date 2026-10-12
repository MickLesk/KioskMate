package hardware

import (
	"context"
	"strings"
	"sync"
	"time"
)

// Capability probes spawn external tools (sudo, ddcutil, loginctl, wlopm).
// On a Raspberry Pi that is measurable CPU, every `sudo` call writes to the
// auth log, and `ddcutil capabilities` blocks on I2C for seconds. The answers
// only change when the device setup changes, so they are memoized and can be
// re-run explicitly through Service.Rediscover.
const (
	probeTTL       = 6 * time.Hour
	sudoProbeTTL   = 15 * time.Minute
	sessionTTL     = 10 * time.Minute
	displayCmdTTL  = 30 * time.Minute
	probeFailedTTL = 2 * time.Minute
)

type probeEntry struct {
	value string
	at    time.Time
	ttl   time.Duration
}

var (
	probeMu    sync.Mutex
	probeCache = map[string]probeEntry{}
)

// memoProbe returns the cached result of fn for key, or runs fn and caches
// its value. Empty results are retried sooner so a tool installed later or a
// session that was not ready at boot is picked up without a restart.
func memoProbe(key string, ttl time.Duration, fn func() string) string {
	probeMu.Lock()
	entry, ok := probeCache[key]
	probeMu.Unlock()
	if ok && time.Since(entry.at) < entry.ttl {
		return entry.value
	}
	value := fn()
	effective := ttl
	if value == "" && probeFailedTTL < ttl {
		effective = probeFailedTTL
	}
	probeMu.Lock()
	probeCache[key] = probeEntry{value: value, at: time.Now(), ttl: effective}
	probeMu.Unlock()
	return value
}

func resetProbes() {
	probeMu.Lock()
	probeCache = map[string]probeEntry{}
	probeMu.Unlock()
}

func sudoRights(ctx context.Context) bool {
	return memoProbe("sudo", sudoProbeTTL, func() string {
		if run(ctx, "sudo", "-n", "true") == nil {
			return "yes"
		}
		return "no"
	}) == "yes"
}

// ddcCapabilities returns the cached `ddcutil capabilities` output, or "" when
// ddcutil or passwordless sudo is unavailable.
func ddcCapabilities(ctx context.Context) string {
	if !commandExists("ddcutil") || !sudoRights(ctx) {
		return ""
	}
	return memoProbe("ddcutil-capabilities", probeTTL, func() string {
		return runText(ctx, "sudo", "ddcutil", "capabilities")
	})
}

func ddcSupports(ctx context.Context, feature string) bool {
	return strings.Contains(ddcCapabilities(ctx), "Feature: "+feature)
}
