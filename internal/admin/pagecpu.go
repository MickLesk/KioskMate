package admin

import (
	"fmt"
	"runtime"

	"github.com/MickLesk/KioskMate/internal/supervisor"
)

// pageCPUReport names the page that costs the most CPU while on screen.
type pageCPUReport struct {
	Finding  string  `json:"finding"`
	Page     string  `json:"page,omitempty"`
	Share    float64 `json:"share_percent"`
	Cores    int     `json:"cores"`
	Measured int     `json:"samples"`
}

// pageCPUMinSamples is about ten minutes at the default watchdog interval;
// fewer samples say little about a page.
const pageCPUMinSamples = 40

func pageCPUCheck(pages []supervisor.PageSummary, cores int, add func(id, level, message string, detail any)) {
	if cores <= 0 {
		cores = runtime.NumCPU()
	}
	var top *supervisor.PageSummary
	for index := range pages {
		if pages[index].Samples >= pageCPUMinSamples {
			top = &pages[index]
			break
		}
	}
	if top == nil {
		return
	}
	report := pageCPUReport{Page: top.Name, Cores: cores, Measured: top.Samples, Share: top.PagesCPUAverage / float64(cores)}
	if report.Page == "" {
		report.Page = top.PageID
	}
	if report.Share >= 40 {
		report.Finding = "high"
		add("page_cpu", "warn", fmt.Sprintf("The page %q keeps %.0f %% of the processor busy while it is on screen. Camera streams are the usual cause: use substreams or MSE instead of WebRTC, or show fewer cameras at once.", report.Page, report.Share), report)
		return
	}
	report.Finding = "ok"
	add("page_cpu", "ok", fmt.Sprintf("The most demanding page, %q, needs %.0f %% of the processor on average.", report.Page, report.Share), report)
}
