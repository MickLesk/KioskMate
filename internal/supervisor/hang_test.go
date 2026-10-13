package supervisor

import (
	"context"
	"testing"
	"time"
)

type probeCommander struct{ err error }

func (p probeCommander) command(context.Context, string, any, any) error { return p.err }

func TestRendererResponds(t *testing.T) {
	if !rendererResponds(context.Background(), probeCommander{}) {
		t.Fatal("answering page reported as hung")
	}
	if rendererResponds(context.Background(), probeCommander{err: context.DeadlineExceeded}) {
		t.Fatal("silent page reported as responding")
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if !rendererResponds(cancelled, probeCommander{err: context.Canceled}) {
		t.Fatal("shutdown counted as a hang")
	}
}

func TestRendererHangRebootsOnlyWhenBudgetExhaustedAndAllowed(t *testing.T) {
	for _, allowed := range []bool{false, true} {
		browser := runningTestBrowser(t, time.Now())
		browser.cfg.Watchdog.RebootOnHang = allowed
		now := time.Now()
		for range 6 {
			browser.recoveryRuns = append(browser.recoveryRuns, now)
		}
		rebooted := 0
		browser.SetRebootHandler(func(context.Context) error { rebooted++; return nil })
		browser.handleRendererHang()
		browser.handleRendererHang()
		want := 0
		if allowed {
			want = 1
		}
		if rebooted != want {
			t.Fatalf("allowed=%v: reboots = %d, want %d", allowed, rebooted, want)
		}
	}
}
