package supervisor

import (
	"context"
	"testing"
	"time"

	"github.com/MickLesk/KioskMate/internal/systemtime"
)

func TestWaitForTimeSync(t *testing.T) {
	defer func(original func(context.Context) systemtime.Status) { timeSyncReader = original }(timeSyncReader)
	calls := 0
	timeSyncReader = func(context.Context) systemtime.Status {
		calls++
		return systemtime.Status{Supported: true, NTPEnabled: true, Synchronized: calls >= 3}
	}
	if !waitForTimeSync(context.Background(), time.Minute, time.Millisecond) || calls != 3 {
		t.Fatalf("waited for sync: calls %d", calls)
	}
	calls = 0
	timeSyncReader = func(context.Context) systemtime.Status {
		calls++
		return systemtime.Status{Supported: true, NTPEnabled: true}
	}
	start := time.Now()
	waitForTimeSync(context.Background(), 20*time.Millisecond, 5*time.Millisecond)
	if time.Since(start) > time.Second {
		t.Fatal("wait not bounded")
	}
	timeSyncReader = func(context.Context) systemtime.Status { return systemtime.Status{Supported: false} }
	if waitForTimeSync(context.Background(), time.Minute, time.Millisecond) {
		t.Fatal("unsupported clock should not wait")
	}
}
