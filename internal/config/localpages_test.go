package config

import (
	"strings"
	"testing"
)

func TestLocalPagesGetLoopbackURLAndCleanSettings(t *testing.T) {
	cfg := &Config{Admin: AdminConfig{Port: 4000}, Kiosk: KioskConfig{Pages: []KioskPage{
		{PageID: "clock", Name: "Clock", SourceType: SourceClock, Local: &LocalPage{Folder: "/x", HTML: "<p>", Hour12: true}},
		{PageID: "photos", Name: "Photos", SourceType: SourceSlideshow, Local: &LocalPage{Folder: " /srv/photos "}},
		{PageID: "web", Name: "Web", URL: "https://example.com", Local: &LocalPage{Title: "x"}},
	}}}
	normalize(cfg)
	clock, photos, web := cfg.Kiosk.Pages[0], cfg.Kiosk.Pages[1], cfg.Kiosk.Pages[2]
	if clock.URL != "http://127.0.0.1:4000/local/clock" {
		t.Fatalf("clock URL = %q", clock.URL)
	}
	if clock.Local.Folder != "" || clock.Local.HTML != "" || !clock.Local.Hour12 {
		t.Fatalf("clock settings not cleaned: %#v", clock.Local)
	}
	if photos.Local.Folder != "/srv/photos" || photos.Local.IntervalSeconds != 10 || photos.Local.Fit != "contain" {
		t.Fatalf("slideshow defaults = %#v", photos.Local)
	}
	if web.Local != nil || web.URL != "https://example.com" {
		t.Fatalf("web page changed: %#v", web)
	}
	if err := Validate(cfg); err != nil {
		t.Fatal(err)
	}
}

func TestLocalPageURLFollowsBindAndTLS(t *testing.T) {
	cases := []struct {
		admin AdminConfig
		want  string
	}{
		{AdminConfig{Bind: "0.0.0.0", Port: 1}, "http://127.0.0.1:1/local/p"},
		{AdminConfig{Bind: "::", Port: 1}, "http://127.0.0.1:1/local/p"},
		{AdminConfig{Bind: "192.168.1.5", Port: 1}, "http://192.168.1.5:1/local/p"},
		{AdminConfig{Bind: "::1", Port: 1}, "http://[::1]:1/local/p"},
		{AdminConfig{Bind: "0.0.0.0", Port: 1, TLSCert: "c", TLSKey: "k"}, "https://127.0.0.1:1/local/p"},
	}
	for _, tc := range cases {
		if got := LocalPageURL(tc.admin, "p"); got != tc.want {
			t.Errorf("LocalPageURL(%+v) = %q, want %q", tc.admin, got, tc.want)
		}
	}
}

func TestLocalPageValidation(t *testing.T) {
	check := func(page KioskPage) error {
		cfg := &Config{Admin: AdminConfig{Port: 4000}, Kiosk: KioskConfig{Pages: []KioskPage{page}}}
		normalize(cfg)
		return Validate(cfg)
	}
	if err := check(KioskPage{Name: "S", SourceType: SourceSlideshow, Local: &LocalPage{Folder: "photos"}}); err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("relative folder error = %v", err)
	}
	if err := check(KioskPage{Name: "S", SourceType: SourceSlideshow, Local: &LocalPage{Folder: "/p", IntervalSeconds: 1}}); err == nil {
		t.Fatal("1 s interval accepted")
	}
	if err := check(KioskPage{Name: "H", SourceType: SourceHTML, Local: &LocalPage{}}); err == nil {
		t.Fatal("empty HTML accepted")
	}
	if err := check(KioskPage{Name: "H", SourceType: SourceHTML, Local: &LocalPage{HTML: strings.Repeat("x", MaxLocalHTMLBytes+1)}}); err == nil {
		t.Fatal("oversized HTML accepted")
	}
	if err := check(KioskPage{Name: "H", SourceType: SourceHTML}); err == nil {
		t.Fatal("HTML page without content accepted")
	}
}

func TestLocalPageValidatesBeforeNormalize(t *testing.T) {
	// The Admin UI posts local pages without a URL; Validate runs before normalize there.
	cfg := &Config{Admin: AdminConfig{Port: 4000}, Kiosk: KioskConfig{Pages: []KioskPage{{Name: "Clock", SourceType: SourceClock}}}}
	if err := Validate(cfg); err != nil {
		t.Fatal(err)
	}
}

func TestNewLocalPageTypesValidateAndClean(t *testing.T) {
	check := func(page KioskPage) error {
		cfg := &Config{Admin: AdminConfig{Port: 4000}, Kiosk: KioskConfig{Pages: []KioskPage{page}}}
		normalize(cfg)
		return Validate(cfg)
	}
	lat := 95.0
	bad := []KioskPage{
		{Name: "W", SourceType: SourceWeather, Local: &LocalPage{Latitude: &lat, Longitude: &lat}},
		{Name: "C", SourceType: SourceCountdown, Local: &LocalPage{}},
		{Name: "C", SourceType: SourceCountdown, Local: &LocalPage{Target: "24.12.2026"}},
		{Name: "N", SourceType: SourceNews, Local: &LocalPage{}},
		{Name: "N", SourceType: SourceNews, Local: &LocalPage{Feeds: []string{"ftp://x/feed"}}},
		{Name: "N", SourceType: SourceNews, Local: &LocalPage{Feeds: []string{"https://a", "https://b", "https://c", "https://d", "https://e", "https://f"}}},
		{Name: "K", SourceType: SourceCamera, Local: &LocalPage{Streams: []string{"rtsp://cam/1"}}},
		{Name: "I", SourceType: SourceImmich, Local: &LocalPage{ImmichURL: "https://photos"}},
		{Name: "I", SourceType: SourceImmich, Local: &LocalPage{ImmichURL: "https://photos", ImmichAPIKey: "k", ImmichAlbum: "../x"}},
	}
	for _, page := range bad {
		if err := check(page); err == nil {
			t.Errorf("accepted %s %#v", page.SourceType, page.Local)
		}
	}
	good := []KioskPage{
		{Name: "W", SourceType: SourceWeather},
		{Name: "C", SourceType: SourceCountdown, Local: &LocalPage{Target: "2026-12-24T18:00"}},
		{Name: "N", SourceType: SourceNews, Local: &LocalPage{Feeds: []string{" https://www.tagesschau.de/xml/rss2 ", ""}}},
		{Name: "K", SourceType: SourceCamera, Local: &LocalPage{Streams: []string{"http://go2rtc:1984/stream.html?src=door"}}},
		{Name: "I", SourceType: SourceImmich, Local: &LocalPage{ImmichURL: "https://photos/", ImmichAPIKey: "k", Folder: "/x"}},
	}
	for _, page := range good {
		if err := check(page); err != nil {
			t.Errorf("%s rejected: %v", page.SourceType, err)
		}
	}
	cfg := &Config{Admin: AdminConfig{Port: 4000}, Kiosk: KioskConfig{Pages: good}}
	normalize(cfg)
	if news := cfg.Kiosk.Pages[2].Local; len(news.Feeds) != 1 || news.IntervalSeconds != 12 {
		t.Fatalf("news defaults = %#v", news)
	}
	if immich := cfg.Kiosk.Pages[4].Local; immich.Folder != "" || immich.ImmichURL != "https://photos" || immich.IntervalSeconds != 10 {
		t.Fatalf("immich cleaned = %#v", immich)
	}
	if weather := cfg.Kiosk.Pages[0].Local; weather.Units != "metric" {
		t.Fatalf("weather defaults = %#v", weather)
	}
	snapshot := cfg.Snapshot()
	snapshot.Kiosk.Pages[2].Local.Feeds[0] = "changed"
	if cfg.Kiosk.Pages[2].Local.Feeds[0] == "changed" {
		t.Fatal("snapshot shares local page settings")
	}
}
