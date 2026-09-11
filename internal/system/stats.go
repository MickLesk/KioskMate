package system

import "time"

type ProcessTreeStats struct {
	PIDs       []int                       `json:"pids"`
	RSSMB      uint64                      `json:"rss_mb"`
	CPUPercent float64                     `json:"cpu_percent"`
	Roles      map[string]ProcessRoleStats `json:"roles,omitempty"`
	Updated    time.Time                   `json:"-"`
	UpdatedAt  *time.Time                  `json:"updated,omitempty"`

	totalTicks uint64
	roleTicks  map[string]uint64
}

type ProcessRoleStats struct {
	Count      int     `json:"count"`
	RSSMB      uint64  `json:"rss_mb"`
	CPUPercent float64 `json:"cpu_percent"`
}
