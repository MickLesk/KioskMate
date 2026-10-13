package config

import (
	"fmt"
	"net"
	"path/filepath"
	"strings"
)

// Local page types are rendered by KioskMate itself and served to the kiosk
// browser from the admin server under /local/<page_id>, so a Pi can show a
// clock, a photo frame or a small custom page without any external server.
const (
	SourceClock     = "clock"
	SourceSlideshow = "slideshow"
	SourceHTML      = "html"

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
}

// IsLocalSource reports whether pages of this source type are served by KioskMate.
func IsLocalSource(sourceType string) bool {
	switch sourceType {
	case SourceClock, SourceSlideshow, SourceHTML:
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
		if page.Local == nil {
			page.Local = &LocalPage{}
		}
		local := page.Local
		local.Title = strings.TrimSpace(local.Title)
		local.Folder = strings.TrimSpace(local.Folder)
		if page.SourceType != SourceClock {
			local.Hour12, local.ShowSeconds, local.HideDate = false, false, false
		}
		if page.SourceType == SourceSlideshow {
			if local.IntervalSeconds <= 0 {
				local.IntervalSeconds = 10
			}
			if local.Fit != "cover" {
				local.Fit = "contain"
			}
		} else {
			local.Folder, local.IntervalSeconds, local.Fit, local.Shuffle = "", 0, "", false
		}
		if page.SourceType != SourceHTML {
			local.HTML = ""
		}
		page.URL = LocalPageURL(cfg.Admin, page.PageID)
	}
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
	}
	if len(strings.TrimSpace(local.Title)) > 200 {
		return fmt.Errorf("kiosk page %d title is too long", index+1)
	}
	return nil
}
