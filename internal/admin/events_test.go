package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/MickLesk/KioskMate/internal/events"
)

func TestEventJournalEndpointReturnsRecentSafeEvents(t *testing.T) {
	journal, err := events.Open(filepath.Join(t.TempDir(), "events.jsonl"), 10)
	if err != nil {
		t.Fatal(err)
	}
	journal.Record("system", "maintenance", "ok", "job completed", map[string]string{
		"job_id":   "42",
		"password": "must not be exposed",
	})

	server := NewServer(nil, nil, nil, nil, nil, nil, "test", nil, journal)
	req := httptest.NewRequest(http.MethodGet, "/api/events?limit=10", nil)
	rec := httptest.NewRecorder()
	server.eventJournal(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var result struct {
		Events []events.Event `json:"events"`
		Path   string         `json:"path"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Events) != 1 || result.Events[0].Message != "job completed" {
		t.Fatalf("events = %#v", result.Events)
	}
	if _, ok := result.Events[0].Details["password"]; ok {
		t.Fatal("sensitive detail returned by event endpoint")
	}
	if result.Path == "" {
		t.Fatal("event journal path missing")
	}
}
