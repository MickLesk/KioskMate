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

func TestConfigImportDryRunDoesNotReplaceActiveConfig(t *testing.T) {
	cfg, err := config.Load(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	before := cfg.Snapshot()
	next := cfg.Snapshot()
	next.Kiosk.Pages = append(next.Kiosk.Pages, config.KioskPage{Name: "Preview only", URL: "https://example.test/dashboard"})
	data, err := json.Marshal(next)
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(cfg, &fakeActionBrowser{}, nil, nil, nil, nil, "test", slog.Default())
	req := httptest.NewRequest(http.MethodPost, "/api/config/import?dry_run=1", bytes.NewReader(data))
	rec := httptest.NewRecorder()

	server.configImport(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var result struct {
		DryRun  bool          `json:"dry_run"`
		Summary configPreview `json:"summary"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !result.DryRun || !result.Summary.KioskChanged || !result.Summary.RequiresNavigation || result.Summary.RequiresBrowserRestart {
		t.Fatalf("unexpected preview: %#v", result)
	}
	if cfg.Snapshot().Kiosk.PageCount() != before.Kiosk.PageCount() {
		t.Fatal("dry-run changed the active configuration")
	}
}

func TestConfigChangeSummaryDetectsAdminRestart(t *testing.T) {
	current := &config.Config{}
	next := &config.Config{}
	next.Admin.Port = 4444
	preview := configChangeSummary(current, next)
	if !preview.AdminChanged || !preview.RequiresServiceRestart {
		t.Fatalf("unexpected preview: %#v", preview)
	}
}
