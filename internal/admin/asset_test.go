package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAssetServesEmbeddedJavaScript(t *testing.T) {
	server := NewServer(nil, nil, nil, nil, nil, nil, "test", nil)
	req := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	rec := httptest.NewRecorder()

	server.asset(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "application/javascript; charset=utf-8" {
		t.Fatalf("content-type = %q", got)
	}
	if rec.Body.Len() == 0 {
		t.Fatal("expected embedded asset body")
	}
}

func TestAssetRejectsNestedPaths(t *testing.T) {
	server := NewServer(nil, nil, nil, nil, nil, nil, "test", nil)
	req := httptest.NewRequest(http.MethodGet, "/assets/../server.go", nil)
	rec := httptest.NewRecorder()

	server.asset(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}
