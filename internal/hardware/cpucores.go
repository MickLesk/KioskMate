package hardware

import (
	"bufio"
	"io"
	"strconv"
	"strings"
	"sync"
)

// Processor usage from /proc/stat: the share of time each core was busy
// since the previous reading. The load average used before is not a usage
// figure (it counts waiting processes, including I/O waits on the SD card).

type cpuTimes struct {
	busy, total uint64
}

var (
	cpuMu       sync.Mutex
	cpuPrevious map[string]cpuTimes
)

// parseProcStat returns the times of the "cpu" summary line and of each
// "cpuN" line, keyed by name.
func parseProcStat(r io.Reader) map[string]cpuTimes {
	out := map[string]cpuTimes{}
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 5 || !strings.HasPrefix(fields[0], "cpu") {
			continue
		}
		var times cpuTimes
		for index, field := range fields[1:] {
			value, err := strconv.ParseUint(field, 10, 64)
			if err != nil {
				break
			}
			// guest and guest_nice (8, 9) are already part of user/nice.
			if index >= 8 {
				break
			}
			times.total += value
			// idle (3) and iowait (4) are not busy time.
			if index != 3 && index != 4 {
				times.busy += value
			}
		}
		out[fields[0]] = times
	}
	return out
}

// cpuUsage compares two readings: the overall usage and one value per core
// in core order, in percent. ok is false without a previous reading.
func cpuUsage(previous, current map[string]cpuTimes) (overall float64, cores []float64, ok bool) {
	share := func(name string) (float64, bool) {
		before, had := previous[name]
		now, has := current[name]
		if !had || !has || now.total <= before.total || now.busy < before.busy {
			return 0, false
		}
		return float64(now.busy-before.busy) / float64(now.total-before.total) * 100, true
	}
	overall, ok = share("cpu")
	if !ok {
		return 0, nil, false
	}
	for index := 0; ; index++ {
		value, found := share("cpu" + strconv.Itoa(index))
		if _, exists := current["cpu"+strconv.Itoa(index)]; !exists {
			break
		}
		if !found {
			value = 0
		}
		cores = append(cores, value)
	}
	return overall, cores, true
}

// readCPUUsage reads /proc/stat-format text and returns the usage since the
// previous call, rounded to one decimal.
func readCPUUsage(r io.Reader) (any, any) {
	current := parseProcStat(r)
	cpuMu.Lock()
	previous := cpuPrevious
	cpuPrevious = current
	cpuMu.Unlock()
	overall, cores, ok := cpuUsage(previous, current)
	if !ok {
		return nil, nil
	}
	for index := range cores {
		cores[index] = round1(cores[index])
	}
	return round1(overall), cores
}
