package admin

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// swapReport describes where the system swaps to. Swap on an SD card is slow
// (a dashboard that swaps stutters) and wears the card; zram keeps swapped
// pages compressed in RAM instead.
type swapReport struct {
	// Finding: none, zram, disk or disk_used.
	Finding string   `json:"finding"`
	TotalMB int      `json:"swap_total_mb"`
	UsedMB  int      `json:"swap_used_mb"`
	DiskMB  int      `json:"disk_swap_mb"`
	Devices []string `json:"devices,omitempty"`
}

// parseSwaps reads /proc/swaps.
func parseSwaps(text string) swapReport {
	var report swapReport
	zram := false
	for i, line := range strings.Split(text, "\n") {
		fields := strings.Fields(line)
		if i == 0 || len(fields) < 4 {
			continue
		}
		sizeKB, _ := strconv.Atoi(fields[2])
		usedKB, _ := strconv.Atoi(fields[3])
		report.TotalMB += sizeKB / 1024
		report.UsedMB += usedKB / 1024
		report.Devices = append(report.Devices, fields[0])
		if strings.Contains(fields[0], "zram") {
			zram = true
		} else {
			report.DiskMB += sizeKB / 1024
		}
	}
	switch {
	case report.TotalMB == 0:
		report.Finding = "none"
	case report.DiskMB == 0 && zram:
		report.Finding = "zram"
	case report.UsedMB >= 128:
		report.Finding = "disk_used"
	default:
		report.Finding = "disk"
	}
	return report
}

func swapCheck(report swapReport, add func(id, level, message string, detail any)) {
	switch report.Finding {
	case "zram":
		add("swap", "ok", "Swap uses zram (compressed RAM), not the SD card.", report)
	case "none":
		add("swap", "ok", "The system has no swap.", report)
	case "disk_used":
		add("swap", "warn", fmt.Sprintf("%d MB are swapped to the SD card or disk. That makes the dashboard stutter and wears the card. Set up zram instead: sudo apt install zram-tools, then sudo dphys-swapfile swapoff && sudo systemctl disable dphys-swapfile, and reboot.", report.UsedMB), report)
	default:
		add("swap", "ok", "Swap lives on the SD card or disk but is hardly used. zram (sudo apt install zram-tools) would be gentler on the card.", report)
	}
}

func collectSwap() (swapReport, bool) {
	data, err := os.ReadFile("/proc/swaps")
	if err != nil {
		return swapReport{}, false
	}
	return parseSwaps(string(data)), true
}
