package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOfflinePage(t *testing.T) {
	s := &Server{}
	rec := httptest.NewRecorder()
	s.renderOfflinePage(rec, httptest.NewRequest(http.MethodGet, "/local/offline?u="+"http%3A%2F%2Fha.local%3A8123%2F%3Ca%3E", nil))
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, `ha.local:8123`) || strings.Contains(body, "<a>") {
		t.Fatalf("offline page %d: %s", rec.Code, body)
	}
	if !strings.Contains(rec.Header().Get("Content-Security-Policy"), "default-src 'none'") {
		t.Fatal("missing CSP")
	}
	for _, bad := range []string{"javascript%3Aalert(1)", "", "ftp%3A%2F%2Fx", "http%3A%2F%2F"} {
		rec = httptest.NewRecorder()
		s.renderOfflinePage(rec, httptest.NewRequest(http.MethodGet, "/local/offline?u="+bad, nil))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("target %q accepted: %d", bad, rec.Code)
		}
	}
}
