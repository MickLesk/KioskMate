package admin

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestPageCheckHintHomeAssistantForbidden(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusForbidden,
		Request:    &http.Request{URL: mustURL(t, "http://homeassistant.local:8123/lovelace?kiosk")},
	}

	category, hint := pageCheckHint("http://homeassistant.local:8123/lovelace?kiosk", resp)
	if category != "home_assistant_forbidden" || !strings.Contains(hint, "ip_bans.yaml") {
		t.Fatalf("pageCheckHint = %q %q", category, hint)
	}
}

func TestPageCheckHintAuthRedirect(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Request:    &http.Request{URL: mustURL(t, "http://ha.local:8123/auth/authorize")},
	}

	category, _ := pageCheckHint("http://ha.local:8123/lovelace", resp)
	if category != "auth_redirect" {
		t.Fatalf("pageCheckHint category = %q, want auth_redirect", category)
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
