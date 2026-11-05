package system

import (
	"sort"
	"strings"
	"time"
)

type ProcessTreeStats struct {
	PIDs       []int      `json:"pids"`
	RSSMB      uint64     `json:"rss_mb"`
	CPUPercent float64    `json:"cpu_percent"`
	Updated    time.Time  `json:"-"`
	UpdatedAt  *time.Time `json:"updated,omitempty"`
	// Groups splits a Chromium process tree by process role, so a busy
	// page renderer (software video decoding) can be told apart from the
	// GPU process or the browser itself.
	Groups []ProcessGroup `json:"groups,omitempty"`

	totalTicks uint64
	pssAt      time.Time
	groupTicks map[string]uint64
}

// ProcessGroup is the share of one process role in a process tree.
type ProcessGroup struct {
	// Name is pages (renderers), graphics (GPU process), helpers (network,
	// audio and other utility processes) or browser (the main process and
	// anything else).
	Name       string  `json:"name"`
	Processes  int     `json:"processes"`
	CPUPercent float64 `json:"cpu_percent"`
	MB         uint64  `json:"mb"`
}

// ProcessGroupName maps a Chromium command line to its process group.
func ProcessGroupName(cmdline string) string {
	for _, arg := range strings.Split(cmdline, "\x00") {
		value, ok := strings.CutPrefix(arg, "--type=")
		if !ok {
			continue
		}
		switch value {
		case "renderer":
			return "pages"
		case "gpu-process":
			return "graphics"
		case "zygote":
			return "browser"
		default:
			return "helpers"
		}
	}
	return "browser"
}

var groupOrder = map[string]int{"pages": 0, "graphics": 1, "browser": 2, "helpers": 3}

// sortGroups orders groups as the Admin UI shows them.
func sortGroups(groups []ProcessGroup) {
	sort.Slice(groups, func(i, j int) bool { return groupOrder[groups[i].Name] < groupOrder[groups[j].Name] })
}
