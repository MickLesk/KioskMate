package admin

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/MickLesk/KioskMate/internal/config"
)

func TestProgramTimelinePreviewsDraftInserts(t *testing.T) {
	cfg, err := config.Load(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(cfg, &fakeActionBrowser{}, nil, nil, nil, nil, "test", slog.Default())
	pages := cfg.Snapshot().Kiosk.Pages
	body, _ := json.Marshal(map[string]any{
		"pages":   pages,
		"inserts": []config.PageInsert{{ID: "w", Name: "Wetter", PageID: pages[0].PageID, Repeat: "interval", EveryMinutes: 5, DurationSeconds: 30}},
		"minutes": 60,
	})
	rec := httptest.NewRecorder()
	server.programTimeline(rec, httptest.NewRequest(http.MethodPost, "/api/program/timeline", bytes.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var result struct {
		Segments []config.ProgramSegment `json:"segments"`
		Upcoming []config.ProgramSegment `json:"upcoming"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	inserts := 0
	for _, segment := range result.Segments {
		if segment.InsertID == "w" {
			inserts++
		}
	}
	if inserts < 11 || inserts > 13 {
		t.Fatalf("expected about 12 insert runs in an hour, got %d", inserts)
	}
	if len(result.Upcoming) == 0 || result.Upcoming[0].Name != "Wetter" {
		t.Fatalf("upcoming = %#v", result.Upcoming)
	}
}
