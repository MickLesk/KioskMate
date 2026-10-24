package config

import (
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

// Local page types are rendered by KioskMate itself and served to the kiosk
// browser from the admin server under /local/<page_id>, so a Pi can show a
// clock, a photo frame or a small custom page without any external server.
const (
	SourceClock     = "clock"
	SourceSlideshow = "slideshow"
	SourceHTML      = "html"
	SourceWeather   = "weather"
	SourceCountdown = "countdown"
	SourceNews      = "news"
	SourceCamera    = "camera"
	SourceImmich    = "immich"

	// MaxNewsFeeds and MaxCameraStreams bound the lists of a news or camera page.
	MaxNewsFeeds     = 5
	MaxCameraStreams = 4
	// CountdownLayout is the format of a countdown target in local time.
	CountdownLayout = "2006-01-02T15:04"

	// MaxLocalHTMLBytes bounds the custom HTML stored in the config file.
	MaxLocalHTMLBytes = 256 << 10
)

// LocalPage holds the settings of a page KioskMate renders itself.
type LocalPage struct {
	// Title is shown above the clock or as the document title.
	Title string `json:"title,omitempty"`
	// Clock
	Hour12      bool `json:"hour12,omitempty"`
	ShowSeconds bool `json:"show_seconds,omitempty"`
	HideDate    bool `json:"hide_date,omitempty"`
	// Slideshow
	Folder          string `json:"folder,omitempty"`
	IntervalSeconds int    `json:"interval_seconds,omitempty"`
	Fit             string `json:"fit,omitempty"`
	Shuffle         bool   `json:"shuffle,omitempty"`
	// Custom HTML
	HTML string `json:"html,omitempty"`
	// Weather: without coordinates the kiosk location (scheduler) is used.
	Latitude  *float64 `json:"latitude,omitempty"`
	Longitude *float64 `json:"longitude,omitempty"`
	Location  string   `json:"location,omitempty"`
	Units     string   `json:"units,omitempty"`
	// Countdown: target in local time, CountdownLayout.
	Target   string `json:"target,omitempty"`
	DoneText string `json:"done_text,omitempty"`
	CountUp  bool   `json:"count_up,omitempty"`
	// News: RSS or Atom feeds; IntervalSeconds is the time per headline.
	Feeds []string `json:"feeds,omitempty"`
	// Camera: go2rtc, MJPEG, snapshot or video URLs shown in a grid.
	Streams []string `json:"streams,omitempty"`
	// Immich photo frame; IntervalSeconds, Fit and Shuffle as for slideshows.
	// The API key never leaves the device: the Admin API reports only
	// ImmichKeyConfigured and KioskMate fetches the photos itself.
	ImmichURL           string `json:"immich_url,omitempty"`
	ImmichAPIKey        string `json:"immich_api_key,omitempty"`
	ImmichKeyConfigured bool   `json:"immich_key_configured,omitempty"`
	ImmichAlbum         string `json:"immich_album,omitempty"`
}

var immichAlbumPattern = regexp.MustCompile(`^[0-9A-Fa-f-]{8,64}$`)

// Clone returns a copy that shares no slices or pointers with l.
func (l *LocalPage) Clone() *LocalPage {
	if l == nil {
		return nil
	}
	clone := *l
	clone.Feeds = slices.Clone(l.Feeds)
	clone.Streams = slices.Clone(l.Streams)
	if l.Latitude != nil {
		value := *l.Latitude
		clone.Latitude = &value
	}
	if l.Longitude != nil {
		value := *l.Longitude
		clone.Longitude = &value
	}
	return &clone
}

// IsLocalSource reports whether pages of this source type are served by KioskMate.
func IsLocalSource(sourceType string) bool {
	switch sourceType {
	case SourceClock, SourceSlideshow, SourceHTML, SourceWeather, SourceCountdown, SourceNews, SourceCamera, SourceImmich:
		return true
	}
	return false
}

// IsLocal reports whether KioskMate renders the page itself.
func (p KioskPage) IsLocal() bool { return IsLocalSource(p.SourceType) }

// LocalPageURL is the address the kiosk browser uses for a local page. The
// host is the loopback address unless the admin server only listens on a
// specific address. With TLS the browser gets --allow-insecure-localhost so
// the admin certificate is accepted for loopback addresses.
func LocalPageURL(admin AdminConfig, pageID string) string {
	scheme := "http"
	if admin.TLSCert != "" && admin.TLSKey != "" {
		scheme = "https"
	}
	host := "127.0.0.1"
	if bind := strings.TrimSpace(admin.Bind); bind != "" {
		if ip := net.ParseIP(bind); ip == nil || !ip.IsUnspecified() {
			host = bind
		}
	}
	return fmt.Sprintf("%s://%s/local/%s", scheme, net.JoinHostPort(host, fmt.Sprint(admin.Port)), pageID)
}

func normalizeLocalPages(cfg *Config) {
	for index := range cfg.Kiosk.Pages {
		page := &cfg.Kiosk.Pages[index]
		if !page.IsLocal() {
			page.Local = nil
			continue
		}
		page.Local = normalizeLocalPage(page.SourceType, page.Local)
		page.URL = LocalPageURL(cfg.Admin, page.PageID)
	}
}

// normalizeLocalPage keeps only the settings of the page's own type.
func normalizeLocalPage(sourceType string, in *LocalPage) *LocalPage {
	if in == nil {
		in = &LocalPage{}
	}
	out := &LocalPage{Title: strings.TrimSpace(in.Title)}
	switch sourceType {
	case SourceClock:
		out.Hour12, out.ShowSeconds, out.HideDate = in.Hour12, in.ShowSeconds, in.HideDate
	case SourceSlideshow, SourceImmich:
		out.IntervalSeconds, out.Fit, out.Shuffle = in.IntervalSeconds, in.Fit, in.Shuffle
		if out.IntervalSeconds <= 0 {
			out.IntervalSeconds = 10
		}
		if out.Fit != "cover" {
			out.Fit = "contain"
		}
		if sourceType == SourceSlideshow {
			out.Folder = strings.TrimSpace(in.Folder)
		} else {
			out.ImmichURL = strings.TrimRight(strings.TrimSpace(in.ImmichURL), "/")
			out.ImmichAPIKey = strings.TrimSpace(in.ImmichAPIKey)
			out.ImmichAlbum = strings.TrimSpace(in.ImmichAlbum)
		}
	case SourceHTML:
		out.HTML = in.HTML
	case SourceWeather:
		out.Latitude, out.Longitude = in.Latitude, in.Longitude
		if out.Latitude == nil || out.Longitude == nil {
			out.Latitude, out.Longitude = nil, nil
		}
		out.Location = strings.TrimSpace(in.Location)
		out.Units = "metric"
		if in.Units == "imperial" {
			out.Units = "imperial"
		}
	case SourceCountdown:
		out.Target, out.DoneText, out.CountUp = strings.TrimSpace(in.Target), strings.TrimSpace(in.DoneText), in.CountUp
	case SourceNews:
		out.Feeds = cleanURLList(in.Feeds)
		out.IntervalSeconds = in.IntervalSeconds
		if out.IntervalSeconds <= 0 {
			out.IntervalSeconds = 12
		}
	case SourceCamera:
		out.Streams = cleanURLList(in.Streams)
	}
	return out
}

func cleanURLList(list []string) []string {
	out := make([]string, 0, len(list))
	for _, item := range list {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

func validWebURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}

func validateLocalPage(index int, page KioskPage) error {
	local := page.Local
	if local == nil {
		local = &LocalPage{}
	}
	switch page.SourceType {
	case SourceSlideshow:
		if folder := strings.TrimSpace(local.Folder); folder == "" || !filepath.IsAbs(folder) {
			return fmt.Errorf("kiosk page %d slideshow needs an absolute image folder", index+1)
		}
		if local.IntervalSeconds != 0 && (local.IntervalSeconds < 2 || local.IntervalSeconds > 86400) {
			return fmt.Errorf("kiosk page %d slideshow interval must be between 2 and 86400 seconds", index+1)
		}
	case SourceHTML:
		if strings.TrimSpace(local.HTML) == "" && !page.Disabled {
			return fmt.Errorf("kiosk page %d needs HTML content", index+1)
		}
		if len(local.HTML) > MaxLocalHTMLBytes {
			return fmt.Errorf("kiosk page %d HTML must be at most %d KB", index+1, MaxLocalHTMLBytes>>10)
		}
	case SourceWeather:
		if (local.Latitude == nil) != (local.Longitude == nil) {
			return fmt.Errorf("kiosk page %d weather needs both latitude and longitude", index+1)
		}
		if local.Latitude != nil && (*local.Latitude < -90 || *local.Latitude > 90 || *local.Longitude < -180 || *local.Longitude > 180) {
			return fmt.Errorf("kiosk page %d weather location is out of range", index+1)
		}
		if local.Units != "" && local.Units != "metric" && local.Units != "imperial" {
			return fmt.Errorf("kiosk page %d weather units must be metric or imperial", index+1)
		}
	case SourceCountdown:
		target := strings.TrimSpace(local.Target)
		if target == "" && !page.Disabled {
			return fmt.Errorf("kiosk page %d countdown needs a target date", index+1)
		}
		if target != "" {
			if _, err := time.ParseInLocation(CountdownLayout, target, time.Local); err != nil {
				return fmt.Errorf("kiosk page %d countdown target must use YYYY-MM-DDTHH:MM", index+1)
			}
		}
		if len(local.DoneText) > 200 {
			return fmt.Errorf("kiosk page %d countdown text is too long", index+1)
		}
	case SourceNews:
		feeds := cleanURLList(local.Feeds)
		if len(feeds) == 0 && !page.Disabled {
			return fmt.Errorf("kiosk page %d news needs at least one feed", index+1)
		}
		if len(feeds) > MaxNewsFeeds {
			return fmt.Errorf("kiosk page %d news allows at most %d feeds", index+1, MaxNewsFeeds)
		}
		for _, feed := range feeds {
			if !validWebURL(feed) {
				return fmt.Errorf("kiosk page %d news feed must be an http or https address", index+1)
			}
		}
		if local.IntervalSeconds != 0 && (local.IntervalSeconds < 4 || local.IntervalSeconds > 600) {
			return fmt.Errorf("kiosk page %d news interval must be between 4 and 600 seconds", index+1)
		}
	case SourceCamera:
		streams := cleanURLList(local.Streams)
		if len(streams) == 0 && !page.Disabled {
			return fmt.Errorf("kiosk page %d camera needs at least one stream", index+1)
		}
		if len(streams) > MaxCameraStreams {
			return fmt.Errorf("kiosk page %d camera allows at most %d streams", index+1, MaxCameraStreams)
		}
		for _, stream := range streams {
			if !validWebURL(stream) {
				return fmt.Errorf("kiosk page %d camera stream must be an http or https address", index+1)
			}
		}
	case SourceImmich:
		if !validWebURL(local.ImmichURL) {
			if !page.Disabled {
				return fmt.Errorf("kiosk page %d Immich needs the server address", index+1)
			}
		}
		if strings.TrimSpace(local.ImmichAPIKey) == "" && !page.Disabled {
			return fmt.Errorf("kiosk page %d Immich needs an API key", index+1)
		}
		if album := strings.TrimSpace(local.ImmichAlbum); album != "" && !immichAlbumPattern.MatchString(album) {
			return fmt.Errorf("kiosk page %d Immich album must be an album ID", index+1)
		}
		if local.IntervalSeconds != 0 && (local.IntervalSeconds < 2 || local.IntervalSeconds > 86400) {
			return fmt.Errorf("kiosk page %d Immich interval must be between 2 and 86400 seconds", index+1)
		}
	}
	if len(strings.TrimSpace(local.Title)) > 200 {
		return fmt.Errorf("kiosk page %d title is too long", index+1)
	}
	return nil
}
