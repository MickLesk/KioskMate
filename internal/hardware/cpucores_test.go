package hardware

import (
	"strings"
	"testing"
)

func TestCPUUsagePerCore(t *testing.T) {
	first := "cpu  100 0 100 800 0 0 0 0 0 0\ncpu0 50 0 50 400 0 0 0 0 0 0\ncpu1 50 0 50 400 0 0 0 0 0 0\nintr 1\n"
	// core 0 busy 90 of 100 ticks, core 1 busy 10 of 100 (5 of them iowait don't count)
	second := "cpu  200 0 100 895 5 0 0 0 0 0\ncpu0 140 0 50 410 0 0 0 0 0 0\ncpu1 60 0 50 485 5 0 0 0 0 0\n"
	cpuPrevious = nil
	if overall, cores := readCPUUsage(strings.NewReader(first)); overall != nil || cores != nil {
		t.Fatalf("first reading should be empty: %v %v", overall, cores)
	}
	overall, cores := readCPUUsage(strings.NewReader(second))
	if overall.(float64) != 50 {
		t.Fatalf("overall = %v", overall)
	}
	got := cores.([]float64)
	if len(got) != 2 || got[0] != 90 || got[1] != 10 {
		t.Fatalf("cores = %v", got)
	}
}
