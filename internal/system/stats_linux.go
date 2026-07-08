//go:build linux

package system

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const clockTicks = 100

func ReadProcessTreeStats(root int, previous ProcessTreeStats) (ProcessTreeStats, error) {
	if root <= 0 {
		return ProcessTreeStats{}, errors.New("invalid pid")
	}
	pids := processTree(root)
	if len(pids) == 0 {
		return ProcessTreeStats{}, os.ErrProcessDone
	}
	var rssPages uint64
	var ticks uint64
	for _, pid := range pids {
		stat, err := readStat(pid)
		if err != nil {
			continue
		}
		rssPages += stat.rssPages
		ticks += stat.ticks
	}
	now := time.Now()
	stats := ProcessTreeStats{
		PIDs:       pids,
		RSSMB:      rssPages * uint64(os.Getpagesize()) / 1024 / 1024,
		Updated:    now,
		UpdatedAt:  &now,
		totalTicks: ticks,
	}
	if !previous.Updated.IsZero() && ticks >= previous.totalTicks {
		elapsed := now.Sub(previous.Updated).Seconds()
		if elapsed > 0 {
			stats.CPUPercent = float64(ticks-previous.totalTicks) / clockTicks / elapsed * 100
		}
	}
	return stats, nil
}

type procStat struct {
	ppid     int
	ticks    uint64
	rssPages uint64
}

func processTree(root int) []int {
	stats := map[int]procStat{}
	entries, _ := filepath.Glob("/proc/[0-9]*/stat")
	for _, file := range entries {
		pid, err := strconv.Atoi(filepath.Base(filepath.Dir(file)))
		if err != nil {
			continue
		}
		stat, err := readStat(pid)
		if err == nil {
			stats[pid] = stat
		}
	}
	var out []int
	queue := []int{root}
	seen := map[int]bool{}
	for len(queue) > 0 {
		pid := queue[0]
		queue = queue[1:]
		if seen[pid] {
			continue
		}
		seen[pid] = true
		if _, ok := stats[pid]; ok {
			out = append(out, pid)
		}
		for child, stat := range stats {
			if stat.ppid == pid {
				queue = append(queue, child)
			}
		}
	}
	sort.Ints(out)
	return out
}

func readStat(pid int) (procStat, error) {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return procStat{}, err
	}
	text := string(data)
	end := strings.LastIndex(text, ")")
	if end < 0 || end+2 >= len(text) {
		return procStat{}, errors.New("invalid proc stat")
	}
	fields := strings.Fields(text[end+2:])
	if len(fields) < 22 {
		return procStat{}, errors.New("short proc stat")
	}
	ppid, _ := strconv.Atoi(fields[1])
	utime, _ := strconv.ParseUint(fields[11], 10, 64)
	stime, _ := strconv.ParseUint(fields[12], 10, 64)
	rss, _ := strconv.ParseUint(fields[21], 10, 64)
	return procStat{ppid: ppid, ticks: utime + stime, rssPages: rss}, nil
}
