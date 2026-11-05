package admin

import (
	"testing"

	"github.com/MickLesk/KioskMate/internal/supervisor"
)

func TestPageVideoCheck(t *testing.T) {
	run := func(videos []supervisor.PageVideo) (string, pageVideoReport) {
		var level string
		var report pageVideoReport
		pageVideoCheck(videos, func(id, l, _ string, detail any) {
			if id != "page_video" {
				t.Fatalf("id = %s", id)
			}
			level, report = l, detail.(pageVideoReport)
		})
		return level, report
	}
	if level, report := run(nil); level != "ok" || report.Finding != "none" {
		t.Fatalf("no video: %s %#v", level, report)
	}
	level, report := run([]supervisor.PageVideo{{Kind: "webrtc", Width: 1920, Height: 1080, Playing: true}, {Kind: "mse", Width: 640, Height: 360, Playing: true}})
	if level != "warn" || report.Finding != "webrtc_large" || report.WebRTC != 1 || report.Largest != "1920x1080" {
		t.Fatalf("large WebRTC: %s %#v", level, report)
	}
	small := supervisor.PageVideo{Kind: "webrtc", Width: 640, Height: 360, Playing: true}
	if level, report := run([]supervisor.PageVideo{small}); level != "ok" || report.Finding != "ok" {
		t.Fatalf("substream: %s %#v", level, report)
	}
	if level, report := run([]supervisor.PageVideo{small, small, small, small}); level != "warn" || report.Finding != "many" {
		t.Fatalf("four streams: %s %#v", level, report)
	}
}
