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

type panicStatusBrowser struct {
	fakeActionBrowser
}

func (b *panicStatusBrowser) Status() supervisor.Status {
	panic("runtime browser status must not be called during fast bootstrap")
}

func TestFastStatusSkipsHardwareAndStillBootstraps(t *testing.T) {
	cfg, err := config.Load(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	actionService := actions.New(cfg)
	browser := &panicStatusBrowser{}
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
	if len(body.Hardware.System) != 0 || len(body.ProfileRecommendation) != 0 {
		t.Fatalf("fast status performed optional hardware work: %#v %#v", body.Hardware, body.ProfileRecommendation)
	}
	if body.Config["admin_addr"] == "" {
		t.Fatalf("fast status is missing local configuration: %#v", body.Config)
	}
}
