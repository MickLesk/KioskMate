package supervisor

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestOperationsSerializeAndHonorCancellation(t *testing.T) {
	b := NewBrowser(schedulerTestConfig(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	var running atomic.Int32
	var overlap atomic.Bool
	var group sync.WaitGroup
	for range 10 {
		group.Go(func() {
			_ = b.operate(context.Background(), "test", func() error {
				if running.Add(1) != 1 {
					overlap.Store(true)
				}
				time.Sleep(time.Millisecond)
				running.Add(-1)
				return nil
			})
		})
	}
	group.Wait()
	if overlap.Load() || len(b.Operations()) != 10 {
		t.Fatal("operations ran concurrently or lost results")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if b.operate(ctx, "test", func() error { t.Fatal("canceled operation executed"); return nil }) == nil {
		t.Fatal("canceled operation succeeded")
	}
}

func TestStopInvalidatesPendingRecovery(t *testing.T) {
	b := NewBrowser(schedulerTestConfig(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	until := time.Now().Add(time.Hour)
	b.recovery.BackoffUntil = &until
	b.scheduleUnexpectedExitRecovery()
	b.mu.Lock()
	epoch := b.recoveryEpoch
	b.mu.Unlock()
	if err := b.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.manuallyStopped || b.recoveryEpoch <= epoch || b.recoveryCancel != nil {
		t.Fatal("manual stop did not cancel recovery")
	}
}

func TestBusyDashboardDoesNotRestartByDefault(t *testing.T) {
	cfg := schedulerTestConfig()
	cfg.Watchdog.MaxCPUPercent = 100
	b := NewBrowser(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	b.hotSince = time.Now().Add(-time.Hour)
	if restart, _ := b.shouldRestart(processStats(0, 350)); restart {
		t.Fatal("CPU-only recovery must be opt-in")
	}
}
