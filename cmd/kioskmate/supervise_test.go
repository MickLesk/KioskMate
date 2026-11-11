package main

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"
)

func TestSuperviseRestartsAfterPanic(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	runs := 0
	superviseWith(context.Background(), logger, "test", time.Millisecond, func(context.Context) {
		runs++
		if runs < 3 {
			panic("boom")
		}
	})
	if runs != 3 {
		t.Fatalf("runs = %d, want 3", runs)
	}
}

func TestSuperviseGivesUp(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	runs := 0
	superviseWith(context.Background(), logger, "test", time.Millisecond, func(context.Context) {
		runs++
		panic("always")
	})
	if runs != superviseMaxRestarts {
		t.Fatalf("runs = %d, want %d", runs, superviseMaxRestarts)
	}
}

func TestSuperviseStopsOnNormalReturn(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	runs := 0
	superviseWith(context.Background(), logger, "test", time.Millisecond, func(context.Context) { runs++ })
	if runs != 1 {
		t.Fatalf("runs = %d, want 1", runs)
	}
}
