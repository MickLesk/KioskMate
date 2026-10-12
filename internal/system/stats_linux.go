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

// pssInterval bounds how often smaps_rollup is read. The kernel walks the page
// tables of every Chromium process for it, which is far more expensive than
// /proc/<pid>/stat; memory changes slowly enough for a one-minute cadence.
const pssInterval = time.Minute

func ReadProcessTreeStats(root int, previous ProcessTreeStats) (ProcessTreeStats, error) {
	if root <= 0 {
		return ProcessTreeStats{}, errors.New("invalid pid")
	}
	pids, stats := processTree(root)
	if len(pids) == 0 {
		return ProcessTreeStats{}, os.ErrProcessDone
	}
	now := time.Now()
	var ticks uint64
	for _, pid := range pids {
		ticks += stats[pid].ticks
	}
	reusePSS := !previous.pssAt.IsZero() && now.Sub(previous.pssAt) < pssInterval && samePIDs(previous.PIDs, pids)
	rssMB, pssAt := previous.RSSMB, previous.pssAt
	if !reusePSS {
		var rssKB uint64
		for _, pid := range pids {
			// Prefer PSS so Chromium shared mappings aren't counted once per process.
			if pss, ok := readPSSKB(pid); ok {
				rssKB += pss
				continue
			}
			rssKB += stats[pid].rssPages * uint64(os.Getpagesize()) / 1024
		}
		rssMB, pssAt = rssKB/1024, now
	}
	result := ProcessTreeStats{
		PIDs:       pids,
		RSSMB:      rssMB,
		Updated:    now,
		UpdatedAt:  &now,
		totalTicks: ticks,
		pssAt:      pssAt,
	}
	if !previous.Updated.IsZero() && ticks >= previous.totalTicks {
		elapsed := now.Sub(previous.Updated).Seconds()
		if elapsed > 0 {
			result.CPUPercent = float64(ticks-previous.totalTicks) / clockTicks / elapsed * 100
		}
	}
	return result, nil
}

func samePIDs(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

type procStat struct {
	ppid     int
	ticks    uint64
	rssPages uint64
}

// processTree returns the sorted PIDs of root and its descendants together
// with their parsed /proc/<pid>/stat, read once per call.
func processTree(root int) ([]int, map[int]procStat) {
	stats := map[int]procStat{}
	children := map[int][]int{}
	entries, _ := os.ReadDir("/proc")
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		stat, err := readStat(pid)
		if err == nil {
			stats[pid] = stat
			children[stat.ppid] = append(children[stat.ppid], pid)
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
		queue = append(queue, children[pid]...)
	}
	sort.Ints(out)
	return out, stats
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

func readPSSKB(pid int) (uint64, bool) {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "smaps_rollup"))
	if err != nil {
		return 0, false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "Pss:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return 0, false
		}
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return 0, false
		}
		return value, true
	}
	return 0, false
}
