package admin

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/MickLesk/KioskMate/internal/config"
)

// Data for the weather, news and Immich pages is fetched by KioskMate, not by
// the kiosk browser: the pages run in a sandbox without an origin, feeds do
// not allow cross-origin reads, and the Immich API key must stay on the
// device. Answers are cached so a page reload or several kiosk tabs do not
// hit the services again, and an outdated answer is served while a service
// is unreachable.

const (
	weatherTTL      = 15 * time.Minute
	newsTTL         = 15 * time.Minute
	immichListTTL   = 10 * time.Minute
	staleFor        = 6 * time.Hour
	maxFeedBytes    = 4 << 20
	maxWeatherBytes = 1 << 20
	maxImmichBytes  = 32 << 20
	maxNewsItems    = 40
	immichListSize  = 100
)

var (
	// KIOSKMATE_OPEN_METEO_URL points the weather pages at another server
	// (the browser tests use a local fake).
	openMeteoURL     = cmp.Or(os.Getenv("KIOSKMATE_OPEN_METEO_URL"), "https://api.open-meteo.com/v1/forecast")
	localFetchClient = &http.Client{Timeout: 15 * time.Second}
	immichAssetID    = regexp.MustCompile(`^[0-9A-Fa-f-]{8,64}$`)
)

type cachedAnswer struct {
	at   time.Time
	data any
	err  error
}

// fetchCache keeps one answer per key and lets only one request per key run.
type fetchCache struct {
	mu      sync.Mutex
	entries map[string]*cachedAnswer
	locks   map[string]*sync.Mutex
}

func (c *fetchCache) get(key string, ttl time.Duration, load func() (any, error)) (any, error) {
	c.mu.Lock()
	if c.entries == nil {
		c.entries, c.locks = map[string]*cachedAnswer{}, map[string]*sync.Mutex{}
	}
	lock := c.locks[key]
	if lock == nil {
		lock = &sync.Mutex{}
		c.locks[key] = lock
	}
	c.mu.Unlock()
	lock.Lock()
	defer lock.Unlock()
	c.mu.Lock()
	entry := c.entries[key]
	c.mu.Unlock()
	if entry != nil && time.Since(entry.at) < ttl {
		return entry.data, entry.err
	}
	data, err := load()
	if err != nil {
		// Keep showing the last good answer for a while.
		if entry != nil && entry.err == nil && time.Since(entry.at) < staleFor {
			return entry.data, nil
		}
		// Remember the failure briefly so a broken service is not hammered.
		c.mu.Lock()
		c.entries[key] = &cachedAnswer{at: time.Now().Add(-ttl + time.Minute), err: err}
		c.mu.Unlock()
		return nil, err
	}
	c.mu.Lock()
	c.entries[key] = &cachedAnswer{at: time.Now(), data: data}
	c.mu.Unlock()
	return data, nil
}

func fetchBody(ctx context.Context, method, target string, header http.Header, body io.Reader, limit int64) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return nil, "", err
	}
	for key, values := range header {
		req.Header[key] = values
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "KioskMate")
	}
	resp, err := localFetchClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, "", fmt.Errorf("%s answered %s", req.URL.Host, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, "", err
	}
	if int64(len(data)) > limit {
		return nil, "", fmt.Errorf("%s answered with more than %d MB", req.URL.Host, limit>>20)
	}
	return data, resp.Header.Get("Content-Type"), nil
}

// weatherLocation is the page's own location or the kiosk location.
func weatherLocation(cfg *config.Config, page config.KioskPage) (float64, float64, bool) {
	if page.Local != nil && page.Local.Latitude != nil && page.Local.Longitude != nil {
		return *page.Local.Latitude, *page.Local.Longitude, true
	}
	if cfg.Kiosk.Scheduler.HasLocation() {
		return cfg.Kiosk.Scheduler.Latitude, cfg.Kiosk.Scheduler.Longitude, true
	}
	return 0, 0, false
}

func (s *Server) weatherData(ctx context.Context, cfg *config.Config, page config.KioskPage) (any, error) {
	lat, lon, ok := weatherLocation(cfg, page)
	if !ok {
		return nil, errors.New("no_location")
	}
	query := url.Values{}
	query.Set("latitude", strconv.FormatFloat(lat, 'f', 4, 64))
	query.Set("longitude", strconv.FormatFloat(lon, 'f', 4, 64))
	query.Set("current", "temperature_2m,apparent_temperature,relative_humidity_2m,weather_code,wind_speed_10m,is_day")
	query.Set("hourly", "temperature_2m,precipitation_probability,weather_code,is_day")
	query.Set("daily", "weather_code,temperature_2m_max,temperature_2m_min,precipitation_probability_max,sunrise,sunset")
	query.Set("timezone", "auto")
	query.Set("forecast_days", "6")
	if page.Local != nil && page.Local.Units == "imperial" {
		query.Set("temperature_unit", "fahrenheit")
		query.Set("wind_speed_unit", "mph")
	}
	target := openMeteoURL + "?" + query.Encode()
	return s.feeds.get("weather "+target, weatherTTL, func() (any, error) {
		data, _, err := fetchBody(ctx, http.MethodGet, target, nil, nil, maxWeatherBytes)
		if err != nil {
			return nil, err
		}
		var parsed map[string]any
		if err := json.Unmarshal(data, &parsed); err != nil {
			return nil, fmt.Errorf("weather service: %w", err)
		}
		return json.RawMessage(data), nil
	})
}

// NewsItem is one headline of a news page.
type NewsItem struct {
	Title     string    `json:"title"`
	Summary   string    `json:"summary,omitempty"`
	Source    string    `json:"source,omitempty"`
	Published time.Time `json:"published,omitzero"`
}

func (s *Server) newsData(ctx context.Context, page config.KioskPage) ([]NewsItem, error) {
	items := []NewsItem{}
	var lastErr error
	for _, feed := range page.Local.Feeds {
		data, err := s.feeds.get("news "+feed, newsTTL, func() (any, error) {
			body, _, err := fetchBody(ctx, http.MethodGet, feed, http.Header{"Accept": {"application/rss+xml, application/atom+xml, application/xml, text/xml"}}, nil, maxFeedBytes)
			if err != nil {
				return nil, err
			}
			return parseFeed(body)
		})
		if err != nil {
			lastErr = err
			continue
		}
		items = append(items, data.([]NewsItem)...)
	}
	if len(items) == 0 && lastErr != nil {
		return nil, lastErr
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].Published.After(items[j].Published) })
	if len(items) > maxNewsItems {
		items = items[:maxNewsItems]
	}
	return items, nil
}

type feedDocument struct {
	XMLName xml.Name
	// RSS 2.0 and RSS 1.0 (RDF)
	Channel struct {
		Title string     `xml:"title"`
		Items []feedItem `xml:"item"`
	} `xml:"channel"`
	RDFItems []feedItem `xml:"item"`
	// Atom
	Title   string     `xml:"title"`
	Entries []feedItem `xml:"entry"`
}

type feedItem struct {
	Title       string `xml:"title"`
	Description string `xml:"description"`
	Summary     string `xml:"summary"`
	Content     string `xml:"content"`
	PubDate     string `xml:"pubDate"`
	Date        string `xml:"date"`
	Published   string `xml:"published"`
	Updated     string `xml:"updated"`
}

var htmlTags = regexp.MustCompile(`(?s)<[^>]*>`)

func plainText(value string, limit int) string {
	text := html.UnescapeString(htmlTags.ReplaceAllString(value, " "))
	text = strings.Join(strings.Fields(text), " ")
	if runes := []rune(text); len(runes) > limit {
		text = strings.TrimSpace(string(runes[:limit-1])) + "…"
	}
	return text
}

func parseFeedTime(values ...string) time.Time {
	layouts := []string{time.RFC1123Z, time.RFC1123, time.RFC3339, "Mon, 2 Jan 2006 15:04:05 -0700", "Mon, 2 Jan 2006 15:04:05 MST", "2 Jan 2006 15:04:05 -0700", "2006-01-02T15:04:05Z0700", "2006-01-02"}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		for _, layout := range layouts {
			if at, err := time.Parse(layout, value); err == nil {
				return at
			}
		}
	}
	return time.Time{}
}

func parseFeed(data []byte) ([]NewsItem, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	decoder.Strict = false
	decoder.CharsetReader = func(_ string, input io.Reader) (io.Reader, error) { return input, nil }
	var doc feedDocument
	if err := decoder.Decode(&doc); err != nil {
		return nil, fmt.Errorf("feed is not RSS or Atom: %w", err)
	}
	source := doc.Channel.Title
	entries := doc.Channel.Items
	if len(entries) == 0 {
		entries = doc.RDFItems
	}
	if len(entries) == 0 {
		entries, source = doc.Entries, doc.Title
	}
	if len(entries) == 0 && doc.XMLName.Local != "rss" && doc.XMLName.Local != "feed" && doc.XMLName.Local != "RDF" {
		return nil, errors.New("feed is not RSS or Atom")
	}
	source = plainText(source, 80)
	items := make([]NewsItem, 0, len(entries))
	for _, entry := range entries {
		title := plainText(entry.Title, 240)
		if title == "" {
			continue
		}
		summary := entry.Description
		if summary == "" {
			summary = entry.Summary
		}
		if summary == "" {
			summary = entry.Content
		}
		summary = plainText(summary, 320)
		if summary == title {
			summary = ""
		}
		items = append(items, NewsItem{Title: title, Summary: summary, Source: source, Published: parseFeedTime(entry.PubDate, entry.Published, entry.Updated, entry.Date)})
	}
	return items, nil
}

func immichHeader(page config.KioskPage) http.Header {
	return http.Header{"X-Api-Key": {page.Local.ImmichAPIKey}, "Accept": {"application/json"}}
}

// immichAssets lists the photo IDs of the album, or random photos.
func (s *Server) immichAssets(ctx context.Context, page config.KioskPage) ([]string, error) {
	local := page.Local
	if local.ImmichURL == "" || local.ImmichAPIKey == "" {
		return nil, errors.New("not_configured")
	}
	key := "immich " + page.PageID + " " + local.ImmichURL + " " + local.ImmichAlbum
	data, err := s.feeds.get(key, immichListTTL, func() (any, error) {
		type asset struct {
			ID   string `json:"id"`
			Type string `json:"type"`
		}
		var assets []asset
		if local.ImmichAlbum != "" {
			body, _, err := fetchBody(ctx, http.MethodGet, local.ImmichURL+"/api/albums/"+url.PathEscape(local.ImmichAlbum), immichHeader(page), nil, maxImmichBytes)
			if err != nil {
				return nil, err
			}
			var album struct {
				Assets []asset `json:"assets"`
			}
			if err := json.Unmarshal(body, &album); err != nil {
				return nil, fmt.Errorf("immich album: %w", err)
			}
			assets = album.Assets
		} else {
			header := immichHeader(page)
			header.Set("Content-Type", "application/json")
			request := fmt.Sprintf(`{"size":%d,"type":"IMAGE"}`, immichListSize)
			body, _, err := fetchBody(ctx, http.MethodPost, local.ImmichURL+"/api/search/random", header, strings.NewReader(request), maxImmichBytes)
			if err != nil {
				return nil, err
			}
			if err := json.Unmarshal(body, &assets); err != nil {
				return nil, fmt.Errorf("immich random photos: %w", err)
			}
		}
		ids := make([]string, 0, len(assets))
		for _, item := range assets {
			if (item.Type == "" || item.Type == "IMAGE") && immichAssetID.MatchString(item.ID) {
				ids = append(ids, item.ID)
			}
		}
		return ids, nil
	})
	if err != nil {
		return nil, err
	}
	return data.([]string), nil
}

func (s *Server) serveImmichAsset(w http.ResponseWriter, r *http.Request, page config.KioskPage, id string) {
	if !immichAssetID.MatchString(id) || page.Local.ImmichURL == "" || page.Local.ImmichAPIKey == "" {
		http.NotFound(w, r)
		return
	}
	target := page.Local.ImmichURL + "/api/assets/" + url.PathEscape(id) + "/thumbnail?size=preview"
	header := immichHeader(page)
	header.Set("Accept", "image/*")
	data, contentType, err := fetchBody(r.Context(), http.MethodGet, target, header, nil, maxImmichBytes)
	if err != nil {
		http.Error(w, "photo not available", http.StatusBadGateway)
		return
	}
	if !strings.HasPrefix(contentType, "image/") {
		contentType = http.DetectContentType(data)
		if !strings.HasPrefix(contentType, "image/") {
			http.Error(w, "not an image", http.StatusBadGateway)
			return
		}
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(data)
}
