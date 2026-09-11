package supervisor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHomeAssistantBanPreflightFixtures(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		banned bool
	}{{"healthy", http.StatusOK, false}, {"forbidden", http.StatusForbidden, true}, {"unauthorized", http.StatusUnauthorized, false}} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(test.status) }))
			defer server.Close()
			banned, err := checkHomeAssistantBan(context.Background(), server.URL+"/dashboard")
			if err != nil || banned != test.banned {
				t.Fatalf("banned=%v err=%v", banned, err)
			}
		})
	}
}

func TestHomeAssistantBanPreflightHonorsContextTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err := checkHomeAssistantBan(ctx, server.URL); err == nil {
		t.Fatal("timeout was not reported")
	}
}

func TestHomeAssistantAuthClassifications(t *testing.T) {
	for _, test := range []struct {
		status   int
		resource string
		url      string
		kind     string
	}{{401, "XHR", "http://ha.local:8123/auth/token", "auth_rejected"}, {403, "Document", "http://ha.local:8123/dashboard", "access_denied"}, {403, "Image", "http://ha.local:8123/local/image.png", ""}} {
		if got := classifyHAResponse(test.status, test.resource, test.url).Kind; got != test.kind {
			t.Fatalf("classification for %s = %q, want %q", test.url, got, test.kind)
		}
	}
	if !homeAssistantInvalidGrant(`{"error":"invalid_grant"}`) || homeAssistantInvalidGrant(`{"error":"temporarily_unavailable"}`) {
		t.Fatal("invalid_grant response classification failed")
	}
}
