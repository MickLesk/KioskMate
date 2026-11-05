//go:build linux

package system

import (
	"os"
	"testing"
)

func TestProcessGroupName(t *testing.T) {
	cases := map[string]string{
		"/usr/lib/chromium/chromium\x00--kiosk\x00":                                      "browser",
		"/usr/lib/chromium/chromium\x00--type=renderer\x00--lang=de\x00":                 "pages",
		"/usr/lib/chromium/chromium\x00--type=gpu-process\x00":                           "graphics",
		"/usr/lib/chromium/chromium\x00--type=utility\x00--utility-sub-type=network\x00": "helpers",
		"/usr/lib/chromium/chromium\x00--type=zygote\x00":                                "browser",
		"": "browser",
	}
	for cmdline, want := range cases {
		if got := ProcessGroupName(cmdline); got != want {
			t.Errorf("ProcessGroupName(%q) = %q, want %q", cmdline, got, want)
		}
	}
}

func TestReadProcessStatsCoversOnlyOneProcess(t *testing.T) {
	stats, err := ReadProcessStats(os.Getpid(), ProcessTreeStats{})
	if err != nil {
		t.Skip("process stats unavailable:", err)
	}
	if len(stats.PIDs) != 1 || stats.PIDs[0] != os.Getpid() || len(stats.Groups) != 0 {
		t.Fatalf("stats = %#v", stats)
	}
	tree, err := ReadProcessTreeStats(os.Getpid(), ProcessTreeStats{})
	if err != nil || len(tree.Groups) == 0 {
		t.Fatalf("tree stats must be grouped: %#v %v", tree, err)
	}
}
