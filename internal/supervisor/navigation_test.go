package supervisor

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/MickLesk/KioskMate/internal/config"
)

func TestNavigationLifecycleRecordsReadyTimings(t *testing.T) {
	browser := NewBrowser(&config.Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	browser.devTools = true
	browser.beginNavigation("https://example.test/dashboard?secret=hidden")
	browser.mu.Lock()
	started := time.Now().Add(-250 * time.Millisecond)
	browser.navigation.Started = &started
	browser.mu.Unlock()

	browser.completeNavigation(false)
	browser.completeNavigation(true)
	status := browser.Status().Navigation
	if status.State != "ready" || status.Loaded == nil || status.FirstFrame == nil || status.LoadDurationMS < 200 || status.FirstFrameMS < 200 {
		t.Fatalf("navigation status = %#v", status)
	}
	if status.URL != "https://example.test/dashboard" {
		t.Fatalf("navigation URL leaked query: %q", status.URL)
	}
}
