package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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
