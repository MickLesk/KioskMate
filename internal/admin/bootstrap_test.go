package admin

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/MickLesk/KioskMate/internal/actions"
	"github.com/MickLesk/KioskMate/internal/config"
	"github.com/MickLesk/KioskMate/internal/hardware"
	"github.com/MickLesk/KioskMate/internal/supervisor"
	"github.com/MickLesk/KioskMate/internal/updater"
)

func TestFastStatusReturnsBrowserWithoutHardware(t *testing.T) {
	cfg, err := config.Load(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	actionService := actions.New(cfg)
	browser := &fakeActionBrowser{status: supervisor.Status{Running: true, PID: 4242, Command: "chromium"}}
	server := NewServer(cfg, browser, nil, updater.New(cfg, "0.7.2", actionService), actionService, hardware.New(), "0.7.2", slog.Default())
	req := httptest.NewRequest(http.MethodGet, "/api/status?fast=1", nil)
	rec := httptest.NewRecorder()

	server.status(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Browser               supervisor.Status `json:"browser"`
		Hardware              hardware.Status   `json:"hardware"`
		ProfileRecommendation map[string]any    `json:"profile_recommendation"`
		Config                map[string]any    `json:"config"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.Browser.Running || body.Browser.PID != 4242 {
		t.Fatalf("fast status missing browser runtime: %#v", body.Browser)
	}
	if len(body.Hardware.System) != 0 || len(body.ProfileRecommendation) != 0 {
		t.Fatalf("fast status performed optional hardware work: %#v %#v", body.Hardware, body.ProfileRecommendation)
	}
	if body.Config["admin_addr"] == "" {
		t.Fatalf("fast status is missing local configuration: %#v", body.Config)
	}
}
