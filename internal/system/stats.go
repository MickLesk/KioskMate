package system

import "time"

type ProcessTreeStats struct {
	PIDs       []int      `json:"pids"`
	RSSMB      uint64     `json:"rss_mb"`
	CPUPercent float64    `json:"cpu_percent"`
	Updated    time.Time  `json:"-"`
	UpdatedAt  *time.Time `json:"updated,omitempty"`

	totalTicks uint64
}
