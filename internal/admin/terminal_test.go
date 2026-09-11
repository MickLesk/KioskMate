package admin

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MickLesk/KioskMate/internal/config"
)

func TestTerminalIsDisabledByDefault(t *testing.T) {
	cfg, err := config.Load(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(cfg, nil, nil, nil, nil, nil, "test", nil)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/terminal/run", strings.NewReader(`{"command":"true"}`))

	server.terminalRun(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
}
