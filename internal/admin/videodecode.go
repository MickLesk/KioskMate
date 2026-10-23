package admin

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Hardware video decoding on a Raspberry Pi needs enough contiguous memory
// (CMA) for the decoder and a V4L2 decoder device; when either is missing
// Chromium falls back to software decoding and camera streams cost 100 % CPU
// or more. videoDecodeReport collects what is needed to tell which.

type videoDecodeReport struct {
	// Finding names the result for the Admin UI: ok, pi5, cma_failures,
	// cma_small, gpu_mem, software or no_decoder.
	Finding        string   `json:"finding"`
	Model          string   `json:"model"`
	CMATotalMB     int      `json:"cma_total_mb"`
	CMAFreeMB      int      `json:"cma_free_mb"`
	CMAFailures    int      `json:"cma_failures"`
	LogReadable    bool     `json:"kernel_log_readable"`
	Decoders       []string `json:"v4l2_decoders,omitempty"`
	BootConfig     string   `json:"boot_config,omitempty"`
	GPUMemMB       int      `json:"gpu_mem_mb,omitempty"`
	KMSOverlay     string   `json:"kms_overlay,omitempty"`
	ChromiumH264HW *bool    `json:"chromium_h264_hw,omitempty"`
}

func parseMeminfoCMA(text string) (totalMB, freeMB int) {
	scanner := bufio.NewScanner(strings.NewReader(text))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		value, err := strconv.Atoi(fields[1])
		if err != nil {
			continue
		}
		switch fields[0] {
		case "CmaTotal:":
			totalMB = value / 1024
		case "CmaFree:":
			freeMB = value / 1024
		}
	}
	return totalMB, freeMB
}

func countCMAFailures(log string) int {
	count := 0
	for _, line := range strings.Split(log, "\n") {
		if strings.Contains(line, "cma_alloc") && strings.Contains(line, "fail") {
			count++
		}
	}
	return count
}

// parseBootConfig reads gpu_mem and the vc4 KMS overlay line from a
// Raspberry Pi config.txt (ignoring comments; conditional sections are read
// as if they applied).
func parseBootConfig(text string) (gpuMem int, overlay string) {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		switch {
		case key == "gpu_mem":
			gpuMem, _ = strconv.Atoi(value)
		case key == "dtoverlay" && strings.HasPrefix(value, "vc4-kms-v3d"):
			overlay = value
		}
	}
	return gpuMem, overlay
}

func collectVideoDecode(ctx context.Context, model string) videoDecodeReport {
	report := videoDecodeReport{Model: model}
	if data, err := os.ReadFile("/proc/meminfo"); err == nil {
		report.CMATotalMB, report.CMAFreeMB = parseMeminfoCMA(string(data))
	}
	if log, ok := kernelLog(ctx); ok {
		report.LogReadable = true
		report.CMAFailures = countCMAFailures(log)
	}
	names, _ := filepath.Glob("/sys/class/video4linux/video*/name")
	for _, path := range names {
		data, err := os.ReadFile(path)
		name := strings.TrimSpace(string(data))
		if err == nil && (strings.Contains(name, "decode") || strings.Contains(name, "rpivid") || strings.Contains(name, "hevc")) {
			report.Decoders = append(report.Decoders, filepath.Base(filepath.Dir(path))+": "+name)
		}
	}
	for _, path := range []string{"/boot/firmware/config.txt", "/boot/config.txt"} {
		if data, err := os.ReadFile(path); err == nil {
			report.BootConfig = path
			report.GPUMemMB, report.KMSOverlay = parseBootConfig(string(data))
			break
		}
	}
	return report
}

// kernelLog returns the kernel messages of this boot, if readable.
func kernelLog(ctx context.Context) (string, bool) {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	for _, command := range [][]string{{"dmesg"}, {"journalctl", "-k", "-b", "--no-pager", "-q"}} {
		if _, err := exec.LookPath(command[0]); err != nil {
			continue
		}
		out, err := exec.CommandContext(ctx, command[0], command[1:]...).Output()
		if err == nil && len(out) > 0 {
			return string(out), true
		}
	}
	return "", false
}

// videoDecodeCheck turns the report into one Doctor check with a concrete fix.
func videoDecodeCheck(report videoDecodeReport, add func(id, level, message string, detail any)) {
	pi5 := strings.Contains(report.Model, "Raspberry Pi 5")
	switch {
	case pi5:
		report.Finding = "pi5"
		add("video_decode", "warn", "The Raspberry Pi 5 has no H.264 hardware decoder (only HEVC). H.264 camera streams are always decoded by the CPU; use a lower resolution or substream, or HEVC where the camera supports it.", report)
	case report.CMAFailures > 0:
		report.Finding = "cma_failures"
		add("video_decode", "error", fmt.Sprintf("The kernel failed %d times to reserve video memory (cma_alloc). The hardware video decoder starves and Chromium decodes in software, which costs a lot of CPU. In %s set dtoverlay=vc4-kms-v3d,cma-512 and remove gpu_mem, then reboot.", report.CMAFailures, firstNonEmpty(report.BootConfig, "config.txt")), report)
	case report.CMATotalMB > 0 && report.CMATotalMB < 256:
		report.Finding = "cma_small"
		add("video_decode", "warn", fmt.Sprintf("Only %d MB contiguous video memory (CMA). Camera streams and 4K screens need more: set dtoverlay=vc4-kms-v3d,cma-512 in %s and reboot.", report.CMATotalMB, firstNonEmpty(report.BootConfig, "config.txt")), report)
	case report.GPUMemMB >= 256 && report.KMSOverlay != "":
		report.Finding = "gpu_mem"
		add("video_decode", "warn", fmt.Sprintf("gpu_mem=%d reserves memory for the legacy firmware stack that the KMS driver does not use. Remove gpu_mem from %s and size CMA with dtoverlay=vc4-kms-v3d,cma-512 instead.", report.GPUMemMB, firstNonEmpty(report.BootConfig, "config.txt")), report)
	case report.ChromiumH264HW != nil && !*report.ChromiumH264HW:
		report.Finding = "software"
		add("video_decode", "warn", "Chromium reports no hardware H.264 decoder, so video streams are decoded by the CPU. Check that the Raspberry Pi OS Chromium package is used and that /dev/video10 (bcm2835-codec) exists.", report)
	case len(report.Decoders) == 0:
		report.Finding = "no_decoder"
		add("video_decode", "warn", "No V4L2 video decoder device was found. Without it Chromium decodes video in software.", report)
	default:
		report.Finding = "ok"
		add("video_decode", "ok", "Hardware video decoding is available and video memory looks sufficient.", report)
	}
}
