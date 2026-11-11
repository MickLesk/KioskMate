package main

import (
	"context"
	"log/slog"
	"runtime/debug"
	"time"
)

const (
	superviseMaxRestarts = 5
	superviseBaseDelay   = 5 * time.Second
)

// supervise runs a long-lived background loop and restarts it after a panic,
// with a growing pause. A panic in a plain goroutine would take the whole
// kiosk down (display included) for a bug in, say, the MQTT integration; here
// the stack lands in the log and only that part restarts. After
// superviseMaxRestarts panics the loop stays stopped and the rest keeps going.
func supervise(ctx context.Context, logger *slog.Logger, name string, fn func(context.Context)) {
	superviseWith(ctx, logger, name, superviseBaseDelay, fn)
}

func superviseWith(ctx context.Context, logger *slog.Logger, name string, base time.Duration, fn func(context.Context)) {
	for attempt := 1; ; attempt++ {
		if !runRecovered(ctx, logger, name, fn) || ctx.Err() != nil {
			return
		}
		if attempt >= superviseMaxRestarts {
			logger.Error("background task stopped after repeated panics", "task", name, "panics", attempt)
			return
		}
		delay := base * time.Duration(1<<(attempt-1))
		logger.Warn("restarting background task after panic", "task", name, "attempt", attempt, "delay", delay.String())
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
	}
}

func runRecovered(ctx context.Context, logger *slog.Logger, name string, fn func(context.Context)) (panicked bool) {
	defer func() {
		if r := recover(); r != nil {
			panicked = true
			logger.Error("background task panicked", "task", name, "panic", r, "stack", string(debug.Stack()))
		}
	}()
	fn(ctx)
	return false
}
