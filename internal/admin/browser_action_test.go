package admin

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MickLesk/KioskMate/internal/config"
	"github.com/MickLesk/KioskMate/internal/hardware"
	"github.com/MickLesk/KioskMate/internal/supervisor"
)

type fakeActionBrowser struct {
	status     supervisor.Status
	guardTrips []string
}

func (b *fakeActionBrowser) Start(context.Context) error {
	b.status.LastError = "chromium exited during startup"
	return nil
}

func (b *fakeActionBrowser) Stop(context.Context) error {
	b.status.Running = false
	return nil
}

func (b *fakeActionBrowser) Restart(ctx context.Context) error {
	return b.Start(ctx)
}

func (b *fakeActionBrowser) Next(ctx context.Context) error {
	return b.Start(ctx)
}

func (b *fakeActionBrowser) Previous(ctx context.Context) error {
	return b.Start(ctx)
}

func (b *fakeActionBrowser) ResetSession(ctx context.Context) error {
	return b.Start(ctx)
}

func (b *fakeActionBrowser) Reload(ctx context.Context) error {
	return b.Start(ctx)
}

func (b *fakeActionBrowser) SetActive(context.Context, int) error {
	return nil
}

func (b *fakeActionBrowser) CaptureScreenshot(context.Context) ([]byte, error) {
	return nil, nil
}

func (b *fakeActionBrowser) TripAuthGuard(reason string) {
	b.guardTrips = append(b.guardTrips, reason)
}

func (b *fakeActionBrowser) NoteDisplayPower(string) {}

func (b *fakeActionBrowser) Status() supervisor.Status {
	return b.status
}

func TestBrowserActionReportsFailedStartupWithDiagnostics(t *testing.T) {
	cfg := &config.Config{Path: filepath.Join(t.TempDir(), "config.json")}
	browser := &fakeActionBrowser{status: supervisor.Status{Command: "chromium", URL: "http://ha.local"}}
	server := NewServer(cfg, browser, nil, nil, nil, nil, "test", slog.Default())

	req := httptest.NewRequest(http.MethodPost, "/api/browser/start", nil)
	rec := httptest.NewRecorder()
	server.browserAction("start").ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["error"] != "chromium exited during startup" {
		t.Fatalf("error = %#v", body["error"])
	}
	if _, ok := body["browser_log"].([]any); !ok {
		t.Fatalf("browser_log missing in %#v", body)
	}
}

func TestBrowserDoctorReportsStoppedBrowser(t *testing.T) {
	cfg := &config.Config{
		Path: filepath.Join(t.TempDir(), "config.json"),
		Kiosk: config.KioskConfig{
			UserDataDir: filepath.Join(t.TempDir(), "Browser"),
		},
	}
	browser := &fakeActionBrowser{status: supervisor.Status{Command: "go", URL: ""}}
	server := NewServer(cfg, browser, nil, nil, nil, hardware.New(), "test", slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/api/browser/doctor", nil)
	rec := httptest.NewRecorder()
	server.browserDoctor(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if _, ok := body["checks"].([]any); !ok {
		t.Fatalf("checks missing: %#v", body)
	}
}

func TestBrowserPageCheckDoesNotTripAuthGuard(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/manifest.json" {
			t.Errorf("path = %q, want /manifest.json", r.URL.Path)
		}
		w.WriteHeader(http.StatusForbidden)
	}))
	defer upstream.Close()
	target := upstream.URL + "/dashboard/main?kiosk"
	cfg := &config.Config{
		Path: filepath.Join(t.TempDir(), "config.json"),
		Kiosk: config.KioskConfig{Pages: []config.KioskPage{{
			PageID: "home", Name: "Home", URL: target, SourceType: "home_assistant",
		}}},
	}
	browser := &fakeActionBrowser{}
	server := NewServer(cfg, browser, nil, nil, nil, hardware.New(), "test", slog.Default())
	req := httptest.NewRequest(http.MethodPost, "/api/browser/check-page", strings.NewReader(`{"url":"`+target+`"}`))
	rec := httptest.NewRecorder()

	server.browserCheckPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if len(browser.guardTrips) != 0 {
		t.Fatalf("diagnostic check tripped auth guard: %#v", browser.guardTrips)
	}
}

func TestBrowserSnapshotReportsCaptureDuration(t *testing.T) {
	cfg := &config.Config{
		Path: filepath.Join(t.TempDir(), "config.json"),
		Kiosk: config.KioskConfig{Pages: []config.KioskPage{{
			PageID: "home", Name: "Home", URL: "http://ha.local:8123/dashboard", SourceType: "home_assistant",
		}}},
	}
	browser := &fakeActionBrowser{status: supervisor.Status{
		Running: true, DevTools: true, URL: "http://ha.local:8123/dashboard",
	}}
	server := NewServer(cfg, browser, nil, nil, nil, hardware.New(), "test", slog.Default())
	req := httptest.NewRequest(http.MethodGet, "/api/browser/snapshot?refresh=1", nil)
	rec := httptest.NewRecorder()

	server.browserSnapshot(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("X-KioskMate-Snapshot-Duration-Ms") == "" {
		t.Fatal("snapshot duration header is missing")
	}
	if rec.Header().Get("X-KioskMate-Snapshot-Time") == "" {
		t.Fatal("snapshot time header is missing")
	}
}

func TestPublicConfigRedactsSecrets(t *testing.T) {
	cfg := &config.Config{
		Admin: config.AdminConfig{Token: "admin-token", PasswordHash: "hash"},
		MQTT:  config.MQTTConfig{Password: "mqtt-password"},
	}
	public := publicConfig(cfg)
	if public.Admin.Token != "" || public.Admin.PasswordHash != "" || public.MQTT.Password != "" {
		t.Fatalf("public config leaked secrets: %#v", public)
	}
	if cfg.Admin.Token == "" || cfg.MQTT.Password == "" {
		t.Fatal("redaction modified the live config")
	}
}
