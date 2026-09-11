package admin

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MickLesk/KioskMate/internal/config"
	"github.com/MickLesk/KioskMate/internal/supervisor"
)

func TestBrowserSoakReportDownload(t *testing.T) {
	browser := supervisor.NewBrowser(&config.Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	server := NewServer(&config.Config{}, browser, nil, nil, nil, nil, "test", slog.Default())
	req := httptest.NewRequest(http.MethodGet, "/api/browser/soak-report?download=1", nil)
	rec := httptest.NewRecorder()

	server.browserSoakReport(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if disposition := rec.Header().Get("Content-Disposition"); !strings.Contains(disposition, "kioskmate-soak-report.json") {
		t.Fatalf("content disposition = %q", disposition)
	}
	var report supervisor.SoakReport
	if err := json.Unmarshal(rec.Body.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Status != "collecting" || report.RequiredSeconds != 24*60*60 || len(report.Checks) != 6 {
		t.Fatalf("report = %#v", report)
	}
}
