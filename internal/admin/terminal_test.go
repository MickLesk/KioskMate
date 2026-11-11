//go:build !windows

package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MickLesk/KioskMate/internal/events"
)

func TestTerminalRunJournalsProgramOnly(t *testing.T) {
	journal, err := events.Open(filepath.Join(t.TempDir(), "events.jsonl"), 10)
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(nil, nil, nil, nil, nil, nil, "test", nil, journal)
	rec := httptest.NewRecorder()
	server.terminalRun(rec, httptest.NewRequest(http.MethodPost, "/api/terminal/run", strings.NewReader(`{"command":"sh -c 'exit 3' secret-token"}`)))
	var result struct {
		ExitCode int `json:"exit_code"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &result)
	if result.ExitCode != 3 {
		t.Fatalf("exit code = %d (%s)", result.ExitCode, rec.Body.String())
	}
	recent := journal.Recent(10)
	if len(recent) != 1 || recent[0].Details["program"] != "sh" || recent[0].Details["exit_code"] != "3" {
		t.Fatalf("journal = %#v", recent)
	}
	if strings.Contains(recent[0].Message+recent[0].Details["program"], "secret") {
		t.Fatal("arguments leaked into the journal")
	}
}
