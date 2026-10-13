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
