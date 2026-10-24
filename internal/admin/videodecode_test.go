package admin

import (
	"strings"
	"testing"
)

func TestVideoDecodeParsing(t *testing.T) {
	total, free := parseMeminfoCMA("MemTotal: 3884000 kB\nCmaTotal:         524288 kB\nCmaFree:          131072 kB\n")
	if total != 512 || free != 128 {
		t.Fatalf("cma = %d/%d", total, free)
	}
	log := "[ 12.3] cma: cma_alloc: linux,cma: alloc failed, req-size: 2025 pages, ret: -12\n[ 13.0] other\n[ 14.1] cma_alloc failed\n"
	if got := countCMAFailures(log); got != 2 {
		t.Fatalf("failures = %d", got)
	}
	gpu, overlay := parseBootConfig("# gpu_mem=64\ngpu_mem=512\n[pi4]\ndtoverlay=vc4-kms-v3d\nmax_framebuffers=2\n")
	if gpu != 512 || overlay != "vc4-kms-v3d" {
		t.Fatalf("boot config = %d %q", gpu, overlay)
	}
}

func TestVideoDecodeCheckAdvice(t *testing.T) {
	run := func(report videoDecodeReport) (string, string) {
		var level, message string
		videoDecodeCheck(report, func(_, l, m string, _ any) { level, message = l, m })
		return level, message
	}
	yes, no := true, false
	pi4 := videoDecodeReport{Model: "Raspberry Pi 4 Model B", CMATotalMB: 512, Decoders: []string{"video10: bcm2835-codec-decode"}, ChromiumH264HW: &yes}
	if level, _ := run(pi4); level != "ok" {
		t.Fatalf("healthy Pi 4 should be ok, got %s", level)
	}
	starving := pi4
	starving.CMAFailures, starving.BootConfig = 300, "/boot/firmware/config.txt"
	if level, message := run(starving); level != "error" || !strings.Contains(message, "cma-512") || !strings.Contains(message, "/boot/firmware/config.txt") {
		t.Fatalf("cma failures: %s %s", level, message)
	}
	small := pi4
	small.CMATotalMB = 64
	if level, _ := run(small); level != "warn" {
		t.Fatal("64 MB CMA should warn")
	}
	gpuMem := pi4
	gpuMem.GPUMemMB, gpuMem.KMSOverlay = 512, "vc4-kms-v3d"
	if _, message := run(gpuMem); !strings.Contains(message, "gpu_mem=512") {
		t.Fatalf("gpu_mem advice missing: %s", message)
	}
	software := pi4
	software.ChromiumH264HW = &no
	if level, _ := run(software); level != "warn" {
		t.Fatal("software decode should warn")
	}
	if _, message := run(videoDecodeReport{Model: "Raspberry Pi 5 Model B Rev 1.0"}); !strings.Contains(message, "no H.264 hardware decoder") {
		t.Fatalf("Pi 5 advice: %s", message)
	}
}
