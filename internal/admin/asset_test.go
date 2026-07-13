package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
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
	if got := rec.Header().Get("Cache-Control"); !strings.Contains(got, "no-store") {
		t.Fatalf("cache-control = %q", got)
	}
}

func TestIndexUsesVersionedAssetsAndVisibleBootstrap(t *testing.T) {
	server := NewServer(nil, nil, nil, nil, nil, nil, "0.7.1", nil)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	server.index(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "__KIOSKMATE_ASSET_VERSION__") || !strings.Contains(body, "app.js?v=0.7.1") {
		t.Fatalf("index does not contain versioned assets: %s", body)
	}
	if !strings.Contains(body, "Loading control panel") {
		t.Fatal("index is missing visible bootstrap state")
	}
	if got := rec.Header().Get("Cache-Control"); !strings.Contains(got, "no-store") {
		t.Fatalf("cache-control = %q", got)
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

func TestEmbeddedAdminUIContainsInteractionContracts(t *testing.T) {
	app, err := content.ReadFile("web/assets/app.js")
	if err != nil {
		t.Fatal(err)
	}
	i18n, err := content.ReadFile("web/assets/i18n.js")
	if err != nil {
		t.Fatal(err)
	}

	for _, marker := range []string{"dirtyViews", "confirmDiscardChanges", "renderDayPicker", "validatePages", "validateScheduler", "validateMQTT", "renderKioskStorybook", "renderKioskFlow", "renderPageWizard", "synchronizeKioskWorkflow", "stateBanner", "readinessItem", "filteredLogs", "nav-mobile-toggle", "/api/status?fast=1", "Promise.allSettled", "renderFatal", "auth-error"} {
		if !strings.Contains(string(app), marker) {
			t.Errorf("embedded app.js missing %q", marker)
		}
	}
	for _, marker := range []string{"allChangesSaved", "validationPageUrl", "dayShort_mon", "kioskSequence", "finishAndStart", "mqttReadiness", "noJobsYet", "navigationMenu"} {
		if !strings.Contains(string(i18n), marker) {
			t.Errorf("embedded i18n.js missing %q", marker)
		}
	}
}
