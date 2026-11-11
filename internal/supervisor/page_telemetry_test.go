package supervisor

import (
	"os/exec"
	"testing"
	"time"

	"github.com/MickLesk/KioskMate/internal/config"
	"github.com/MickLesk/KioskMate/internal/system"
)

func TestPageTelemetryCreditsPageOnScreen(t *testing.T) {
	cfg := &config.Config{Kiosk: config.KioskConfig{Pages: []config.KioskPage{
		{PageID: "ha", Name: "Dashboard", URL: "http://ha.local/"},
		{PageID: "cam", Name: "Kameras", URL: "http://cam.local/"},
	}}}
	b := NewBrowser(cfg, nil)
	b.cmd = &exec.Cmd{}
	stats := func(pages float64) system.ProcessTreeStats {
		return system.ProcessTreeStats{CPUPercent: pages + 10, RSSMB: 400, Groups: []system.ProcessGroup{{Name: "pages", CPUPercent: pages}}}
	}
	start := time.Now()
	k := cfg.Snapshot().Kiosk
	b.mu.Lock()
	b.active = 0
	b.notePageSampleLocked(start, k, stats(500))                     // switch: not measured
	b.notePageSampleLocked(start.Add(10*time.Second), k, stats(500)) // still settling
	b.notePageSampleLocked(start.Add(40*time.Second), k, stats(20))
	b.notePageSampleLocked(start.Add(55*time.Second), k, stats(40))
	b.active = 1
	b.notePageSampleLocked(start.Add(70*time.Second), k, stats(300))
	b.notePageSampleLocked(start.Add(110*time.Second), k, stats(160))
	b.idleBlanked = true
	b.notePageSampleLocked(start.Add(130*time.Second), k, stats(999))
	b.mu.Unlock()

	pages := b.PageTelemetrySummaries()
	if len(pages) != 2 {
		t.Fatalf("pages = %#v", pages)
	}
	if pages[0].PageID != "cam" || pages[0].Name != "Kameras" || pages[0].Samples != 1 || pages[0].PagesCPUAverage != 160 {
		t.Fatalf("most expensive page = %#v", pages[0])
	}
	if pages[1].PageID != "ha" || pages[1].Samples != 2 || pages[1].PagesCPUAverage != 30 || pages[1].CPUAverage != 40 {
		t.Fatalf("dashboard = %#v", pages[1])
	}
}

func TestPageTelemetryFadesAndForgetsRemovedPages(t *testing.T) {
	entry := PageTelemetry{PageID: "x", Samples: pageSampleCap, PagesCPUSum: float64(pageSampleCap) * 50}
	cfg := &config.Config{Kiosk: config.KioskConfig{Pages: []config.KioskPage{{PageID: "x", Name: "X", URL: "http://x/"}}}}
	b := NewBrowser(cfg, nil)
	b.cmd = &exec.Cmd{}
	b.pageTelemetry = map[string]*PageTelemetry{"x": &entry, "gone": {PageID: "gone", Samples: 3, PagesCPUSum: 3}}
	b.pageShownID, b.pageShownAt = "x", time.Now().Add(-time.Hour)
	b.mu.Lock()
	b.notePageSampleLocked(time.Now(), cfg.Snapshot().Kiosk, system.ProcessTreeStats{Groups: []system.ProcessGroup{{Name: "pages", CPUPercent: 50}}})
	b.mu.Unlock()
	if entry.Samples != pageSampleCap/2+1 {
		t.Fatalf("samples after halving = %d", entry.Samples)
	}
	pages := b.PageTelemetrySummaries()
	if len(pages) != 1 || pages[0].PageID != "x" || pages[0].PagesCPUAverage != 50 {
		t.Fatalf("pages = %#v", pages)
	}
}
