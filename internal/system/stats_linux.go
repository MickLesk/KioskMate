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
	var rssKB uint64
	var ticks uint64
	roleRSSKB := map[string]uint64{}
	roleTicks := map[string]uint64{}
	roleCounts := map[string]int{}
	for _, pid := range pids {
		stat, err := readStat(pid)
		if err != nil {
			continue
		}
		ticks += stat.ticks
		role := processRole(pid, root)
		roleTicks[role] += stat.ticks
		roleCounts[role]++
		// Prefer PSS so Chromium shared mappings aren't counted once per process.
		if pss, ok := readPSSKB(pid); ok {
			rssKB += pss
			roleRSSKB[role] += pss
			continue
		}
		resident := stat.rssPages * uint64(os.Getpagesize()) / 1024
		rssKB += resident
		roleRSSKB[role] += resident
	}
	now := time.Now()
	stats := ProcessTreeStats{
		PIDs:       pids,
		RSSMB:      rssKB / 1024,
		Updated:    now,
		UpdatedAt:  &now,
		totalTicks: ticks,
		roleTicks:  roleTicks,
		Roles:      map[string]ProcessRoleStats{},
	}
	for role, count := range roleCounts {
		stats.Roles[role] = ProcessRoleStats{Count: count, RSSMB: roleRSSKB[role] / 1024}
	}
	if !previous.Updated.IsZero() && ticks >= previous.totalTicks {
		elapsed := now.Sub(previous.Updated).Seconds()
		if elapsed > 0 {
			stats.CPUPercent = float64(ticks-previous.totalTicks) / clockTicks / elapsed * 100
			for role, currentTicks := range roleTicks {
				roleStats := stats.Roles[role]
				if previousTicks := previous.roleTicks[role]; currentTicks >= previousTicks {
					roleStats.CPUPercent = float64(currentTicks-previousTicks) / clockTicks / elapsed * 100
				}
				stats.Roles[role] = roleStats
			}
		}
	}
	return stats, nil
}

func processRole(pid, root int) string {
	if pid == root {
		return "browser"
	}
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cmdline"))
	if err != nil {
		return "other"
	}
	command := strings.ReplaceAll(string(data), "\x00", " ")
	switch {
	case strings.Contains(command, "--type=renderer"):
		return "renderer"
	case strings.Contains(command, "--type=gpu-process"):
		return "gpu"
	case strings.Contains(command, "--type=utility"):
		return "utility"
	case strings.Contains(command, "--type=zygote"):
		return "zygote"
	case strings.Contains(command, "crashpad_handler"):
		return "crashpad"
	default:
		return "other"
	}
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
