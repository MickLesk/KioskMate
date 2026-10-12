package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAssetServesEmbeddedJavaScript(t *testing.T) {
	server := NewServer(nil, nil, nil, nil, nil, nil, "test", nil)
	req := httptest.NewRequest(http.MethodGet, "/assets/app-core.js", nil)
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
	if strings.Contains(body, "__KIOSKMATE_ASSET_VERSION__") || !strings.Contains(body, `<script type="module" src="/assets/main.js?v=0.7.1">`) {
		t.Fatalf("index does not load the versioned module entry point: %s", body)
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
	var app strings.Builder
	for _, name := range []string{
		"app-core.js", "view-dashboard.js", "view-kiosk.js", "view-mqtt.js", "view-system.js", "view-settings.js",
		"actions-core.js", "actions-kiosk.js", "actions-mqtt.js", "actions-system.js", "actions-settings.js", "renderers.js", "main.js",
	} {
		asset, err := content.ReadFile("web/assets/" + name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		app.Write(asset)
	}
	i18n, err := content.ReadFile("web/assets/i18n-de.json")
	if err != nil {
		t.Fatal(err)
	}

	for _, marker := range []string{"dirtyViews", "confirmDiscardChanges", "renderDayPicker", "validatePages", "validateScheduler", "validateMQTT", "renderKioskStorybook", "renderKioskFlow", "renderPageWizard", "synchronizeKioskWorkflow", "stateBanner", "readinessItem", "filteredLogs", "formatEvents", "nav-mobile-toggle", "state.auth.config", "Promise.allSettled", "renderFatal", "auth-error"} {
		if !strings.Contains(app.String(), marker) {
			t.Errorf("embedded Admin assets missing %q", marker)
		}
	}
	for _, marker := range []string{"allChangesSaved", "validationPageUrl", "dayShort_mon", "kioskSequence", "finishAndStart", "mqttReadiness", "noJobsYet", "logEvents", "navigationMenu"} {
		if !strings.Contains(string(i18n), marker) {
			t.Errorf("embedded i18n-de.json missing %q", marker)
		}
	}
}
