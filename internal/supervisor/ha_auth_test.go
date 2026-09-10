package supervisor

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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

func TestSuspectedAuthEvidenceRequiresCorroboration(t *testing.T) {
	browser := &Browser{authEvidenceSeen: map[string][]time.Time{}}
	evidence := authEvidence{Kind: "access_denied", Confidence: "suspected", URL: "http://ha.local:8123/dashboard", Status: http.StatusForbidden}
	now := time.Now()
	blocked, occurrences, _, _ := browser.recordAuthEvidenceLocked(evidence, now)
	if blocked || occurrences != 1 {
		t.Fatalf("first suspected signal blocked=%v occurrences=%d", blocked, occurrences)
	}
	blocked, occurrences, first, last := browser.recordAuthEvidenceLocked(evidence, now.Add(time.Second))
	if !blocked || occurrences != 2 || !last.After(first) {
		t.Fatalf("corroborated signal blocked=%v occurrences=%d first=%s last=%s", blocked, occurrences, first, last)
	}
}

func TestProbableAuthEvidenceBlocksImmediately(t *testing.T) {
	browser := &Browser{}
	evidence := authEvidence{Kind: "auth_rejected", Confidence: "probable", URL: "http://ha.local:8123/auth/token", Status: http.StatusForbidden}
	blocked, occurrences, _, _ := browser.recordAuthEvidenceLocked(evidence, time.Now())
	if !blocked || occurrences != 1 {
		t.Fatalf("probable signal blocked=%v occurrences=%d", blocked, occurrences)
	}
}

func TestAuthEvidencePrunesExpiredOrigins(t *testing.T) {
	now := time.Now()
	browser := &Browser{authEvidenceSeen: map[string][]time.Time{
		"expired": {now.Add(-time.Minute)},
	}}
	evidence := authEvidence{Kind: "access_denied", Confidence: "suspected", URL: "http://ha.local:8123/dashboard", Status: http.StatusForbidden}
	browser.recordAuthEvidenceLocked(evidence, now)
	if _, exists := browser.authEvidenceSeen["expired"]; exists {
		t.Fatal("expired authentication evidence was not removed")
	}
}

func TestAuthEvidenceIsBounded(t *testing.T) {
	now := time.Now()
	browser := &Browser{authEvidenceSeen: map[string][]time.Time{}}
	for index := 0; index < 140; index++ {
		evidence := authEvidence{Kind: fmt.Sprintf("signal-%d", index), Confidence: "suspected", URL: "http://ha.local:8123/dashboard", Status: http.StatusForbidden}
		browser.recordAuthEvidenceLocked(evidence, now.Add(time.Duration(index)*time.Millisecond))
	}
	if len(browser.authEvidenceSeen) > 128 {
		t.Fatalf("authentication evidence entries = %d, want <= 128", len(browser.authEvidenceSeen))
	}
}

func TestAuthEndpointRejectionIsProbable(t *testing.T) {
	evidence := classifyHAResponse(http.StatusForbidden, "XHR", "http://ha.local:8123/auth/token")
	if evidence.Kind != "auth_rejected" || evidence.Confidence != "probable" {
		t.Fatalf("unexpected evidence: %#v", evidence)
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
