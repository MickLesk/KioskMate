package supervisor

import (
	"sort"
	"time"

	"github.com/MickLesk/KioskMate/internal/config"
	"github.com/MickLesk/KioskMate/internal/system"
)

// CPU per page: every watchdog sample is credited to the page on screen, so
// the telemetry can say which page (usually a camera dashboard) costs the
// most. Samples right after a page switch are skipped because loading a page
// always costs CPU, and nothing is credited while the display is blanked.

const (
	// pageSettle is how long a freshly shown page is not measured.
	pageSettle = 30 * time.Second
	// pageSampleCap halves the sums once a page has this many samples
	// (about a day at the default 15 s interval), so old load fades out.
	pageSampleCap = 5760
	// pageTelemetryMax limits how many pages are kept.
	pageTelemetryMax = 40
)

// PageTelemetry holds running sums for one page.
type PageTelemetry struct {
	PageID      string    `json:"page_id"`
	Name        string    `json:"name"`
	Samples     int       `json:"samples"`
	CPUSum      float64   `json:"cpu_sum"`
	PagesCPUSum float64   `json:"pages_cpu_sum"`
	RSSSum      float64   `json:"rss_sum"`
	LastSeen    time.Time `json:"last_seen"`
}

// PageSummary is the averaged view of PageTelemetry.
type PageSummary struct {
	PageID          string    `json:"page_id"`
	Name            string    `json:"name"`
	Samples         int       `json:"samples"`
	CPUAverage      float64   `json:"cpu_average"`
	PagesCPUAverage float64   `json:"pages_cpu_average"`
	RSSAverageMB    float64   `json:"rss_average_mb"`
	LastSeen        time.Time `json:"last_seen"`
}

func (p PageTelemetry) summary() PageSummary {
	out := PageSummary{PageID: p.PageID, Name: p.Name, Samples: p.Samples, LastSeen: p.LastSeen}
	if p.Samples > 0 {
		n := float64(p.Samples)
		out.CPUAverage, out.PagesCPUAverage, out.RSSAverageMB = p.CPUSum/n, p.PagesCPUSum/n, p.RSSSum/n
	}
	return out
}

// notePageSampleLocked credits one process tree sample to the page on screen.
func (b *Browser) notePageSampleLocked(now time.Time, cfg config.KioskConfig, stats system.ProcessTreeStats) {
	if b.idleBlanked || b.cmd == nil {
		return
	}
	id := cfg.EnabledPageID(b.active)
	if id == "" {
		return
	}
	if id != b.pageShownID {
		b.pageShownID, b.pageShownAt = id, now
		return
	}
	if now.Sub(b.pageShownAt) < pageSettle {
		return
	}
	if b.pageTelemetry == nil {
		b.pageTelemetry = map[string]*PageTelemetry{}
	}
	entry := b.pageTelemetry[id]
	if entry == nil {
		if len(b.pageTelemetry) >= pageTelemetryMax {
			b.dropOldestPageLocked()
		}
		entry = &PageTelemetry{PageID: id}
		b.pageTelemetry[id] = entry
	}
	entry.Name = cfg.PageNameByID(id)
	if entry.Samples >= pageSampleCap {
		entry.Samples /= 2
		entry.CPUSum /= 2
		entry.PagesCPUSum /= 2
		entry.RSSSum /= 2
	}
	entry.Samples++
	entry.CPUSum += stats.CPUPercent
	for _, group := range stats.Groups {
		if group.Name == "pages" {
			entry.PagesCPUSum += group.CPUPercent
		}
	}
	entry.RSSSum += float64(stats.RSSMB)
	entry.LastSeen = now
}

func (b *Browser) dropOldestPageLocked() {
	var oldest string
	for id, entry := range b.pageTelemetry {
		if oldest == "" || entry.LastSeen.Before(b.pageTelemetry[oldest].LastSeen) {
			oldest = id
		}
	}
	delete(b.pageTelemetry, oldest)
}

// pageSummariesLocked returns the pages, most expensive first. Pages that
// no longer exist in the configuration are left out.
func (b *Browser) pageSummariesLocked(cfg config.KioskConfig) []PageSummary {
	var out []PageSummary
	for id, entry := range b.pageTelemetry {
		if entry.Samples == 0 {
			continue
		}
		summary := entry.summary()
		if name := cfg.PageNameByID(id); name != "" {
			summary.Name = name
		} else if !pageExists(cfg, id) {
			continue
		}
		out = append(out, summary)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].PagesCPUAverage != out[j].PagesCPUAverage {
			return out[i].PagesCPUAverage > out[j].PagesCPUAverage
		}
		return out[i].PageID < out[j].PageID
	})
	return out
}

func pageExists(cfg config.KioskConfig, id string) bool {
	for _, page := range cfg.Pages {
		if page.PageID == id {
			return true
		}
	}
	return false
}

// PageTelemetrySummaries returns the CPU per page, most expensive first.
func (b *Browser) PageTelemetrySummaries() []PageSummary {
	cfg := b.cfg.Snapshot().Kiosk
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.pageSummariesLocked(cfg)
}

func pageTelemetryList(pages map[string]*PageTelemetry) []PageTelemetry {
	out := make([]PageTelemetry, 0, len(pages))
	for _, entry := range pages {
		out = append(out, *entry)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PageID < out[j].PageID })
	return out
}
