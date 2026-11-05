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
	return collectStats(pids, stats, previous, true), nil
}

// ReadProcessStats measures a single process without its children, for
// example KioskMate itself, whose child is the browser.
func ReadProcessStats(pid int, previous ProcessTreeStats) (ProcessTreeStats, error) {
	if pid <= 0 {
		return ProcessTreeStats{}, errors.New("invalid pid")
	}
	stat, err := readStat(pid)
	if err != nil {
		return ProcessTreeStats{}, err
	}
	return collectStats([]int{pid}, map[int]procStat{pid: stat}, previous, false), nil
}

func collectStats(pids []int, stats map[int]procStat, previous ProcessTreeStats, grouped bool) ProcessTreeStats {
	now := time.Now()
	var ticks uint64
	groupTicks := map[string]uint64{}
	groupCount := map[string]int{}
	groupOf := map[int]string{}
	for _, pid := range pids {
		ticks += stats[pid].ticks
		if grouped {
			name := ProcessGroupName(readCmdline(pid))
			groupOf[pid] = name
			groupTicks[name] += stats[pid].ticks
			groupCount[name]++
		}
	}
	reusePSS := !previous.pssAt.IsZero() && now.Sub(previous.pssAt) < pssInterval && samePIDs(previous.PIDs, pids)
	rssMB, pssAt := previous.RSSMB, previous.pssAt
	groupMB := map[string]uint64{}
	if reusePSS {
		for _, group := range previous.Groups {
			groupMB[group.Name] = group.MB
		}
	} else {
		var rssKB uint64
		groupKB := map[string]uint64{}
		for _, pid := range pids {
			// Prefer PSS so Chromium shared mappings aren't counted once per process.
			kb, ok := readPSSKB(pid)
			if !ok {
				kb = stats[pid].rssPages * uint64(os.Getpagesize()) / 1024
			}
			rssKB += kb
			groupKB[groupOf[pid]] += kb
		}
		rssMB, pssAt = rssKB/1024, now
		for name, kb := range groupKB {
			groupMB[name] = kb / 1024
		}
	}
	result := ProcessTreeStats{
		PIDs:       pids,
		RSSMB:      rssMB,
		Updated:    now,
		UpdatedAt:  &now,
		totalTicks: ticks,
		pssAt:      pssAt,
		groupTicks: groupTicks,
	}
	elapsed := 0.0
	if !previous.Updated.IsZero() {
		elapsed = now.Sub(previous.Updated).Seconds()
	}
	if elapsed > 0 && ticks >= previous.totalTicks {
		result.CPUPercent = float64(ticks-previous.totalTicks) / clockTicks / elapsed * 100
	}
	if grouped {
		for name, count := range groupCount {
			group := ProcessGroup{Name: name, Processes: count, MB: groupMB[name]}
			if before, ok := previous.groupTicks[name]; ok && elapsed > 0 && groupTicks[name] >= before {
				group.CPUPercent = float64(groupTicks[name]-before) / clockTicks / elapsed * 100
			}
			result.Groups = append(result.Groups, group)
		}
		sortGroups(result.Groups)
	}
	return result
}

func readCmdline(pid int) string {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cmdline"))
	if err != nil {
		return ""
	}
	return string(data)
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

// MemoryUsagePercent is the share of memory in use (not available), or 0.
func MemoryUsagePercent() float64 {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	var total, available float64
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		value, _ := strconv.ParseFloat(fields[1], 64)
		switch fields[0] {
		case "MemTotal:":
			total = value
		case "MemAvailable:":
			available = value
		}
	}
	if total <= 0 {
		return 0
	}
	return (total - available) / total * 100
}

// ProcessorTemperature is the CPU temperature in °C, or 0 when unknown.
func ProcessorTemperature() float64 {
	entries, _ := os.ReadDir("/sys/class/thermal")
	for _, wanted := range []string{"cpu-thermal", "x86_pkg_temp", "k10temp", "TCPU", "cpu", "acpitz"} {
		for _, entry := range entries {
			dir := filepath.Join("/sys/class/thermal", entry.Name())
			kind, err := os.ReadFile(filepath.Join(dir, "type"))
			if err != nil || strings.TrimSpace(string(kind)) != wanted {
				continue
			}
			raw, err := os.ReadFile(filepath.Join(dir, "temp"))
			if err != nil {
				continue
			}
			if value, err := strconv.ParseFloat(strings.TrimSpace(string(raw)), 64); err == nil && value > 0 {
				return value / 1000
			}
		}
	}
	return 0
}
