package admin

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/MickLesk/KioskMate/internal/config"
	"github.com/MickLesk/KioskMate/internal/supervisor"
)

func TestHealthRemainsOKWhenBrowserIsStopped(t *testing.T) {
	browser := &fakeActionBrowser{status: supervisor.Status{State: "failed", LastError: "browser unavailable"}}
	server := NewServer(nil, browser, nil, nil, nil, nil, "0.8.0-test", nil)
	recorder := httptest.NewRecorder()
	server.health(recorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	var body struct {
		Status  string `json:"status"`
		Version string `json:"version"`
		Browser struct {
			State   string `json:"state"`
			Running bool   `json:"running"`
			Ready   bool   `json:"ready"`
		} `json:"browser"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Status != "ok" || body.Version != "0.8.0-test" || body.Browser.State != "failed" || body.Browser.Running || body.Browser.Ready {
		t.Fatalf("unexpected health response: %#v", body)
	}
}

func TestHealthIncludesSafeSoakSummary(t *testing.T) {
	cfg := &config.Config{Path: filepath.Join(t.TempDir(), "config.json")}
	browser := supervisor.NewBrowser(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	server := NewServer(cfg, browser, nil, nil, nil, nil, "0.9.0-test", nil)
	recorder := httptest.NewRecorder()
	server.health(recorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	var body struct {
		Soak struct {
			Status          string `json:"status"`
			RequiredSeconds int64  `json:"required_seconds"`
			Samples         int    `json:"samples"`
		} `json:"soak"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Soak.Status != "collecting" || body.Soak.RequiredSeconds != 24*60*60 || body.Soak.Samples != 0 {
		t.Fatalf("soak summary = %#v", body.Soak)
	}
}

func TestForwardedHTTPSIsTrustedOnlyFromLoopback(t *testing.T) {
	remote := httptest.NewRequest(http.MethodGet, "http://kiosk.local", nil)
	remote.RemoteAddr = "192.0.2.10:1234"
	remote.Header.Set("X-Forwarded-Proto", "https")
	if requestIsHTTPS(remote) {
		t.Fatal("untrusted remote client controlled the HTTPS decision")
	}

	local := httptest.NewRequest(http.MethodGet, "http://kiosk.local", nil)
	local.RemoteAddr = "127.0.0.1:1234"
	local.Header.Set("X-Forwarded-Proto", "https")
	if !requestIsHTTPS(local) {
		t.Fatal("loopback reverse proxy should be allowed to report HTTPS")
	}
}

func TestClientIPUsesForwardedChainOnlyFromTrustedProxy(t *testing.T) {
	untrusted := httptest.NewRequest(http.MethodGet, "http://kiosk.local", nil)
	untrusted.RemoteAddr = "192.0.2.10:1234"
	untrusted.Header.Set("X-Forwarded-For", "198.51.100.7")
	if got := clientIP(untrusted, []string{"127.0.0.1"}); got != "192.0.2.10" {
		t.Fatalf("untrusted forwarded client = %q", got)
	}

	trusted := httptest.NewRequest(http.MethodGet, "http://kiosk.local", nil)
	trusted.RemoteAddr = "127.0.0.1:1234"
	trusted.Header.Set("X-Forwarded-For", "198.51.100.7, 10.0.0.4")
	if got := clientIP(trusted, []string{"127.0.0.1", "10.0.0.0/8"}); got != "198.51.100.7" {
		t.Fatalf("trusted forwarded client = %q", got)
	}
}
