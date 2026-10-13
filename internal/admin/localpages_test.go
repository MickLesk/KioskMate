package admin

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MickLesk/KioskMate/internal/config"
)

func localPageServer(t *testing.T, pages ...config.KioskPage) *Server {
	t.Helper()
	cfg, err := config.Load(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.Kiosk.Pages = pages
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	return NewServer(cfg, &fakeActionBrowser{}, nil, nil, nil, nil, "test", slog.Default())
}

func getLocal(server *Server, target, remote string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.RemoteAddr = remote
	rec := httptest.NewRecorder()
	server.localPage(rec, req)
	return rec
}

func TestLocalPagesOnlyServeTheDeviceAndRunSandboxed(t *testing.T) {
	server := localPageServer(t,
		config.KioskPage{PageID: "clock", Name: "Clock", SourceType: config.SourceClock, Local: &config.LocalPage{Title: "<Lobby>"}},
		config.KioskPage{PageID: "custom", Name: "Custom", SourceType: config.SourceHTML, Local: &config.LocalPage{HTML: "<h1>Hi</h1>"}},
	)
	if rec := getLocal(server, "/local/clock", "192.0.2.10:5000"); rec.Code != http.StatusForbidden {
		t.Fatalf("remote request status = %d, want 403", rec.Code)
	}
	rec := getLocal(server, "/local/clock", "127.0.0.1:5000")
	if rec.Code != http.StatusOK {
		t.Fatalf("clock status = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "&lt;Lobby&gt;") || !strings.Contains(body, `"title":"\u003cLobby\u003e"`) {
		t.Fatalf("title not escaped:\n%s", body)
	}
	if csp := rec.Header().Get("Content-Security-Policy"); !strings.HasPrefix(csp, "sandbox allow-scripts;") {
		t.Fatalf("clock CSP = %q", csp)
	}
	rec = getLocal(server, "/local/custom", "[::1]:5000")
	if rec.Body.String() != "<h1>Hi</h1>" || !strings.HasPrefix(rec.Header().Get("Content-Security-Policy"), "sandbox") {
		t.Fatalf("custom page = %d %q %q", rec.Code, rec.Body.String(), rec.Header().Get("Content-Security-Policy"))
	}
	if rec := getLocal(server, "/local/missing", "127.0.0.1:5000"); rec.Code != http.StatusNotFound {
		t.Fatalf("missing page status = %d", rec.Code)
	}
}

func TestSlideshowListsAndServesOnlyImagesInsideTheFolder(t *testing.T) {
	folder := t.TempDir()
	for name, data := range map[string]string{"b.jpg": "jpeg", "a b.PNG": "png", "notes.txt": "x", ".hidden.jpg": "x"} {
		if err := os.WriteFile(filepath.Join(folder, name), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(folder, "sub.jpg"), 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "secret.jpg")
	if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(folder, "link.jpg")); err != nil {
		t.Fatal(err)
	}
	server := localPageServer(t, config.KioskPage{PageID: "photos", Name: "Photos", SourceType: config.SourceSlideshow, Local: &config.LocalPage{Folder: folder}})

	rec := getLocal(server, "/local/photos/images", "127.0.0.1:1")
	var listing struct{ Images []string }
	if err := json.Unmarshal(rec.Body.Bytes(), &listing); err != nil {
		t.Fatal(err)
	}
	want := []string{"photos/image/a%20b.PNG", "photos/image/b.jpg", "photos/image/link.jpg"}
	if strings.Join(listing.Images, ",") != strings.Join(want, ",") {
		t.Fatalf("images = %v, want %v", listing.Images, want)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatal("listing is not readable from the sandboxed page")
	}
	rec = getLocal(server, "/local/photos/image/a%20b.PNG", "127.0.0.1:1")
	if rec.Code != http.StatusOK || rec.Body.String() != "png" || rec.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("image = %d %q %q", rec.Code, rec.Body.String(), rec.Header().Get("Content-Type"))
	}
	for _, name := range []string{"link.jpg", "notes.txt", ".hidden.jpg", "sub.jpg", "..%2Fsecret.jpg"} {
		if rec := getLocal(server, "/local/photos/image/"+name, "127.0.0.1:1"); rec.Code != http.StatusNotFound {
			t.Errorf("%s status = %d, want 404", name, rec.Code)
		}
	}
	if !strings.Contains(getLocal(server, "/local/photos", "127.0.0.1:1").Body.String(), `"photos/image/b.jpg"`) {
		t.Fatal("slideshow page does not embed the image list")
	}
}
