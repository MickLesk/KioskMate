package admin

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/MickLesk/KioskMate/internal/config"
	"github.com/MickLesk/KioskMate/internal/signals"
)

func TestConfigSaveRefusesOutdatedRevision(t *testing.T) {
	cfg, err := config.Load(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(cfg, &fakeActionBrowser{}, nil, nil, nil, nil, "test", slog.Default())
	get := func() *config.Config {
		rec := httptest.NewRecorder()
		server.config(rec, httptest.NewRequest(http.MethodGet, "/api/config", nil))
		var out config.Config
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return &out
	}
	save := func(next *config.Config, query string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(next)
		rec := httptest.NewRecorder()
		server.config(rec, httptest.NewRequest(http.MethodPost, "/api/config"+query, bytes.NewReader(body)))
		return rec
	}
	first := get()
	if first.Revision == "" {
		t.Fatal("GET /api/config must include a revision")
	}
	stale := get()
	first.Kiosk.ZoomPercent = 110
	if rec := save(first, ""); rec.Code != http.StatusOK {
		t.Fatalf("first save: %d %s", rec.Code, rec.Body.String())
	}
	stale.Kiosk.ZoomPercent = 90
	if rec := save(stale, ""); rec.Code != http.StatusConflict {
		t.Fatalf("stale save should be refused, got %d %s", rec.Code, rec.Body.String())
	}
	if rec := save(stale, "?force=1"); rec.Code != http.StatusOK {
		t.Fatalf("forced save should pass, got %d", rec.Code)
	}
	if cfg.Snapshot().Kiosk.ZoomPercent != 90 || cfg.Snapshot().Revision != "" {
		t.Fatalf("forced save not applied or revision stored: %+v", cfg.Snapshot().Kiosk.ZoomPercent)
	}
	stale.Revision = ""
	if rec := save(stale, ""); rec.Code != http.StatusOK {
		t.Fatalf("a save without revision keeps working, got %d", rec.Code)
	}
}

func TestPresenceEndpoint(t *testing.T) {
	cfg, err := config.Load(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Mutate(func(next *config.Config) error {
		next.Kiosk.Presence = config.PresenceConfig{Enabled: true, Mode: "wake", TimeoutMinutes: 2}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	server := NewServer(cfg, &fakeActionBrowser{}, nil, nil, nil, nil, "test", slog.Default())
	store := signals.New()
	server.SetSignals(store)
	rec := httptest.NewRecorder()
	server.presence(rec, httptest.NewRequest(http.MethodPost, "/api/presence", nil))
	var out struct {
		Present bool   `json:"present"`
		Source  string `json:"source"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || !out.Present || out.Source != "api" {
		t.Fatalf("presence not recorded: %s", rec.Body.String())
	}
	if last, _ := store.LastPresence(); last.IsZero() {
		t.Fatal("store did not receive the detection")
	}
}

func TestPageThumbnails(t *testing.T) {
	cfg, err := config.Load(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(cfg, &fakeActionBrowser{}, nil, nil, nil, nil, "test", slog.Default())
	id := cfg.Snapshot().Kiosk.Pages[0].PageID
	if err := server.saveThumbnail(id, []byte("jpeg")); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	server.pageThumbnails(rec, httptest.NewRequest(http.MethodGet, "/api/pages/thumbnails", nil))
	var list map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if _, ok := list[id]; !ok {
		t.Fatalf("thumbnail list misses %s: %s", id, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	server.pageThumbnails(rec, httptest.NewRequest(http.MethodGet, "/api/pages/thumbnails/"+id, nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "jpeg" || rec.Header().Get("Content-Type") != "image/jpeg" {
		t.Fatalf("thumbnail not served: %d %q", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	server.pageThumbnails(rec, httptest.NewRequest(http.MethodGet, "/api/pages/thumbnails/..%2Fconfig", nil))
	if rec.Code == http.StatusOK {
		t.Fatal("path traversal must be refused")
	}
	if _, err := os.Stat(filepath.Join(server.thumbnailDir(), id+".jpg.tmp")); err == nil {
		t.Fatal("temporary file left behind")
	}
}
