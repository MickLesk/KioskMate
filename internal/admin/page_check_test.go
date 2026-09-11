package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestPageCheckHintHomeAssistantForbidden(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusForbidden,
		Request:    &http.Request{URL: mustURL(t, "http://homeassistant.local:8123/lovelace?kiosk")},
	}

	category, hint := pageCheckHint("http://homeassistant.local:8123/lovelace?kiosk", resp, true)
	if category != "home_assistant_forbidden" || !strings.Contains(hint, "ip_bans.yaml") {
		t.Fatalf("pageCheckHint = %q %q", category, hint)
	}
}

func TestPageCheckHintAuthRedirect(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Request:    &http.Request{URL: mustURL(t, "http://ha.local:8123/auth/authorize")},
	}

	category, _ := pageCheckHint("http://ha.local:8123/lovelace", resp, true)
	if category != "auth_redirect" {
		t.Fatalf("pageCheckHint category = %q, want auth_redirect", category)
	}
}

func TestPageProbeTargetUsesHomeAssistantManifest(t *testing.T) {
	target, scope, err := pageProbeTarget("http://user:secret@ha.example.test/dashboard/main?kiosk#view", true)
	if err != nil {
		t.Fatal(err)
	}
	if target != "http://ha.example.test/manifest.json" {
		t.Fatalf("target = %q", target)
	}
	if scope != "home_assistant_origin" {
		t.Fatalf("scope = %q", scope)
	}
}

func TestCheckHTTPPageProbesHomeAssistantOrigin(t *testing.T) {
	requested := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested = r.URL.RequestURI()
		w.Header().Set("Content-Type", "application/manifest+json")
		_, _ = w.Write([]byte(`{"name":"Home Assistant"}`))
	}))
	defer server.Close()

	result := checkHTTPPage(context.Background(), server.URL+"/dashboard/main?kiosk", "test", true)
	if result["ok"] != true {
		t.Fatalf("result = %#v", result)
	}
	if requested != "/manifest.json" {
		t.Fatalf("requested = %q", requested)
	}
	if result["probe_scope"] != "home_assistant_origin" {
		t.Fatalf("probe_scope = %#v", result["probe_scope"])
	}
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}
