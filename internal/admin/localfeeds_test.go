package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/MickLesk/KioskMate/internal/config"
)

const rssFeed = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel><title>Tagesschau</title>
<item><title>Older &amp; calmer</title><description>&lt;p&gt;Some &lt;b&gt;text&lt;/b&gt;&lt;/p&gt;</description><pubDate>Fri, 23 Oct 2026 08:00:00 +0200</pubDate></item>
<item><title>Newest</title><pubDate>Sat, 24 Oct 2026 07:30:00 +0200</pubDate></item>
<item><title></title></item>
</channel></rss>`

const atomFeed = `<?xml version="1.0" encoding="utf-8"?>
<feed xmlns="http://www.w3.org/2005/Atom"><title>Heise</title>
<entry><title>Atom entry</title><summary>Short summary</summary><updated>2026-10-24T04:00:00Z</updated></entry>
</feed>`

func TestParseFeedReadsRSSAndAtom(t *testing.T) {
	items, err := parseFeed([]byte(rssFeed))
	if err != nil || len(items) != 2 {
		t.Fatalf("rss items = %#v, %v", items, err)
	}
	if items[0].Title != "Older & calmer" || items[0].Summary != "Some text" || items[0].Source != "Tagesschau" || items[0].Published.IsZero() {
		t.Fatalf("rss item = %#v", items[0])
	}
	items, err = parseFeed([]byte(atomFeed))
	if err != nil || len(items) != 1 || items[0].Source != "Heise" || items[0].Summary != "Short summary" || items[0].Published.IsZero() {
		t.Fatalf("atom items = %#v, %v", items, err)
	}
	if _, err := parseFeed([]byte("<html><body>nope</body></html>")); err == nil {
		t.Fatal("HTML accepted as feed")
	}
}

func TestNewsPageMergesFeedsNewestFirst(t *testing.T) {
	var hits atomic.Int32
	feeds := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path == "/atom" {
			_, _ = w.Write([]byte(atomFeed))
			return
		}
		_, _ = w.Write([]byte(rssFeed))
	}))
	defer feeds.Close()
	server := localPageServer(t, config.KioskPage{PageID: "news", Name: "News", SourceType: config.SourceNews, Local: &config.LocalPage{Feeds: []string{feeds.URL + "/rss", feeds.URL + "/atom"}}})
	for range 2 {
		rec := getLocal(server, "/local/news/data", "127.0.0.1:1")
		var items []NewsItem
		if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &items) != nil || len(items) != 3 {
			t.Fatalf("news = %d %s", rec.Code, rec.Body.String())
		}
		if items[0].Title != "Newest" || items[1].Title != "Atom entry" || rec.Header().Get("Access-Control-Allow-Origin") != "*" {
			t.Fatalf("order = %q %q", items[0].Title, items[1].Title)
		}
	}
	if hits.Load() != 2 {
		t.Fatalf("feeds fetched %d times, want 2 (cached)", hits.Load())
	}
	if rec := getLocal(server, "/local/news", "127.0.0.1:1"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Als Nächstes") {
		t.Fatalf("news page = %d", rec.Code)
	}
}

func TestWeatherPageUsesKioskLocation(t *testing.T) {
	var query string
	weather := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"current":{"time":"2026-10-24T09:00","temperature_2m":11.2,"weather_code":3}}`))
	}))
	defer weather.Close()
	defer func(old string) { openMeteoURL = old }(openMeteoURL)
	openMeteoURL = weather.URL

	server := localPageServer(t, config.KioskPage{PageID: "wx", Name: "Wetter", SourceType: config.SourceWeather, Local: &config.LocalPage{}})
	rec := getLocal(server, "/local/wx/data", "127.0.0.1:1")
	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "no_location") {
		t.Fatalf("without location = %d %s", rec.Code, rec.Body.String())
	}
	if err := server.cfg.Mutate(func(cfg *config.Config) error {
		cfg.Kiosk.Scheduler.Latitude, cfg.Kiosk.Scheduler.Longitude = 52.52, 13.405
		cfg.Kiosk.Pages[0].Local.Units = "imperial"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	rec = getLocal(server, "/local/wx/data", "127.0.0.1:1")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"temperature_2m":11.2`) {
		t.Fatalf("weather = %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(query, "latitude=52.5200") || !strings.Contains(query, "temperature_unit=fahrenheit") {
		t.Fatalf("query = %s", query)
	}
	if rec := getLocal(server, "/local/wx", "127.0.0.1:1"); rec.Code != http.StatusOK || !strings.Contains(rec.Header().Get("Content-Security-Policy"), "sandbox allow-scripts;") {
		t.Fatalf("weather page = %d", rec.Code)
	}
}

func TestImmichPageKeepsTheKeyOnTheDevice(t *testing.T) {
	const key = "secret-immich-key"
	const photo = "0b1f6a0e-1111-2222-3333-444455556666"
	immich := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != key {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/albums/aaaaaaaa-bbbb":
			_, _ = w.Write([]byte(`{"assets":[{"id":"` + photo + `","type":"IMAGE"},{"id":"9b1f6a0e-0000-0000-0000-000000000000","type":"VIDEO"}]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/search/random":
			_, _ = w.Write([]byte(`[{"id":"` + photo + `","type":"IMAGE"}]`))
		case r.URL.Path == "/api/assets/"+photo+"/thumbnail" && r.URL.Query().Get("size") == "preview":
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte{0xff, 0xd8, 0xff, 0xe0})
		default:
			http.NotFound(w, r)
		}
	}))
	defer immich.Close()
	server := localPageServer(t, config.KioskPage{PageID: "photos", Name: "Fotos", SourceType: config.SourceImmich, Local: &config.LocalPage{ImmichURL: immich.URL + "/", ImmichAPIKey: key, ImmichAlbum: "aaaaaaaa-bbbb"}})

	rec := getLocal(server, "/local/photos/images", "127.0.0.1:1")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"photos/image/`+photo+`"`) || strings.Contains(rec.Body.String(), "9b1f6a0e") {
		t.Fatalf("album list = %d %s", rec.Code, rec.Body.String())
	}
	rec = getLocal(server, "/local/photos/image/"+photo, "127.0.0.1:1")
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/jpeg" {
		t.Fatalf("photo = %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	if rec := getLocal(server, "/local/photos/image/..%2f..%2fapi", "127.0.0.1:1"); rec.Code != http.StatusNotFound {
		t.Fatalf("bad asset id = %d", rec.Code)
	}
	page := getLocal(server, "/local/photos", "127.0.0.1:1").Body.String()
	if strings.Contains(page, key) {
		t.Fatal("page contains the API key")
	}

	public := publicConfig(server.cfg)
	if public.Kiosk.Pages[0].Local.ImmichAPIKey != "" || !public.Kiosk.Pages[0].Local.ImmichKeyConfigured {
		t.Fatalf("public config = %#v", public.Kiosk.Pages[0].Local)
	}
	if server.cfg.Snapshot().Kiosk.Pages[0].Local.ImmichAPIKey != key {
		t.Fatal("public config changed the stored key")
	}
	data, _ := json.Marshal(public)
	next, err := server.decodeConfig(data)
	if err != nil {
		t.Fatal(err)
	}
	if next.Kiosk.Pages[0].Local.ImmichAPIKey != key {
		t.Fatal("saving from the Admin UI dropped the key")
	}
	if redacted, _ := json.Marshal(redactSecrets(server.cfg)); strings.Contains(string(redacted), key) {
		t.Fatal("diagnostics contain the key")
	}

	if err := server.cfg.Mutate(func(cfg *config.Config) error {
		cfg.Kiosk.Pages[0].Local.ImmichAlbum = ""
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if rec := getLocal(server, "/local/photos/images", "127.0.0.1:1"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), photo) {
		t.Fatalf("random list = %d %s", rec.Code, rec.Body.String())
	}
}

func TestCountdownAndCameraPagesRender(t *testing.T) {
	server := localPageServer(t,
		config.KioskPage{PageID: "xmas", Name: "Weihnachten", SourceType: config.SourceCountdown, Local: &config.LocalPage{Target: "2026-12-24T18:00", DoneText: "Frohe Weihnachten"}},
		config.KioskPage{PageID: "cam", Name: "Kamera", SourceType: config.SourceCamera, Local: &config.LocalPage{Streams: []string{"http://go2rtc.local:1984/stream.html?src=door"}}},
	)
	rec := getLocal(server, "/local/xmas", "127.0.0.1:1")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"target":"2026-12-24T18:00"`) {
		t.Fatalf("countdown = %d", rec.Code)
	}
	rec = getLocal(server, "/local/cam", "127.0.0.1:1")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "go2rtc.local") || !strings.Contains(rec.Header().Get("Content-Security-Policy"), "media-src *") {
		t.Fatalf("camera = %d %q", rec.Code, rec.Header().Get("Content-Security-Policy"))
	}
}
