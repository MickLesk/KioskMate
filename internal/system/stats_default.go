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
	return ProcessTreeStats{PIDs: []int{root}, Updated: now, UpdatedAt: &now}, nil
}
