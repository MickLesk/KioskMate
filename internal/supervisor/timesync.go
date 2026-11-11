package supervisor

import (
	"context"
	"time"

	"github.com/MickLesk/KioskMate/internal/systemtime"
)

// A Raspberry Pi has no battery-backed clock: after a power cut it boots
// with the time of the last shutdown (fake-hwclock) until NTP answers. A
// schedule evaluated with that time can switch the display off in the
// middle of the day or show the night page. The scheduler therefore waits
// for the clock to be synchronized, at most timeSyncWait after start.

const (
	timeSyncWait = 2 * time.Minute
	timeSyncPoll = 5 * time.Second
)

// timeSyncReader is replaced in tests.
var timeSyncReader = func(ctx context.Context) systemtime.Status {
	return systemtime.Read(ctx, "")
}

// waitForTimeSync returns once the system clock is synchronized, NTP is not
// in use or cannot be queried, the wait is over, or ctx ends. It reports
// whether it waited at all.
func waitForTimeSync(ctx context.Context, wait, poll time.Duration) bool {
	deadline := time.Now().Add(wait)
	waited := false
	for {
		readCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		status := timeSyncReader(readCtx)
		cancel()
		if !status.Supported || !status.NTPEnabled || status.Synchronized || status.Error != "" {
			return waited
		}
		if !time.Now().Before(deadline) {
			return waited
		}
		waited = true
		select {
		case <-ctx.Done():
			return waited
		case <-time.After(poll):
		}
	}
}
