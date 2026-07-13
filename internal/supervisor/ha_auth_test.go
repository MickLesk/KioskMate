package supervisor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHomeAssistantBanPreflightUsesPublicManifest(t *testing.T) {
	requestedPath := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	banned, err := checkHomeAssistantBan(context.Background(), server.URL+"/dashboard?kiosk")
	if err != nil {
		t.Fatal(err)
	}
	if !banned || requestedPath != "/manifest.json" {
		t.Fatalf("banned=%v path=%q", banned, requestedPath)
	}
}

func TestHomeAssistantBanPreflightAllowsHealthyHost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	banned, err := checkHomeAssistantBan(context.Background(), server.URL)
	if err != nil || banned {
		t.Fatalf("banned=%v error=%v", banned, err)
	}
}
