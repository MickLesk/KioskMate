//go:build !linux

package system

import (
	"errors"
	"time"
)

func ReadProcessTreeStats(root int, previous ProcessTreeStats) (ProcessTreeStats, error) {
	if root <= 0 {
		return ProcessTreeStats{}, errors.New("invalid pid")
	}
	now := time.Now()
	return ProcessTreeStats{PIDs: []int{root}, Roles: map[string]ProcessRoleStats{"browser": {Count: 1}}, Updated: now, UpdatedAt: &now}, nil
}
