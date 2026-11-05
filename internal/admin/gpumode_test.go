package admin

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/MickLesk/KioskMate/internal/config"
)

func TestBrowserGPUModeSavesAndRestarts(t *testing.T) {
	cfg, err := config.Load(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	browser := &fakeActionBrowser{}
	server := NewServer(cfg, browser, nil, nil, nil, nil, "test", slog.Default())

	rec := httptest.NewRecorder()
	server.browserGPUMode(rec, httptest.NewRequest(http.MethodPost, "/api/browser/gpu-mode", bytes.NewBufferString(`{"mode":"hardware"}`)))
	if rec.Code != http.StatusOK || cfg.Snapshot().Performance.GPUMode != "hardware" {
		t.Fatalf("status %d body %s mode %s", rec.Code, rec.Body.String(), cfg.Snapshot().Performance.GPUMode)
	}
	if browser.status.LastError == "" {
		t.Fatal("the browser must be restarted with the new mode")
	}

	rec = httptest.NewRecorder()
	server.browserGPUMode(rec, httptest.NewRequest(http.MethodPost, "/api/browser/gpu-mode", bytes.NewBufferString(`{"mode":"turbo"}`)))
	if rec.Code != http.StatusBadRequest || cfg.Snapshot().Performance.GPUMode != "hardware" {
		t.Fatalf("invalid mode: status %d mode %s", rec.Code, cfg.Snapshot().Performance.GPUMode)
	}
}
