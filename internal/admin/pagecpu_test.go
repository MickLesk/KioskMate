package admin

import (
	"testing"

	"github.com/MickLesk/KioskMate/internal/supervisor"
)

func TestPageCPUCheck(t *testing.T) {
	var got []map[string]any
	add := func(id, level, message string, detail any) {
		got = append(got, map[string]any{"id": id, "level": level, "detail": detail})
	}
	pageCPUCheck([]supervisor.PageSummary{{PageID: "few", PagesCPUAverage: 390, Samples: 3}}, 4, add)
	if len(got) != 0 {
		t.Fatalf("too few samples should not report: %#v", got)
	}
	pageCPUCheck([]supervisor.PageSummary{{PageID: "cam", Name: "Kameras", PagesCPUAverage: 200, Samples: 100}, {PageID: "ha", PagesCPUAverage: 20, Samples: 900}}, 4, add)
	if len(got) != 1 || got[0]["level"] != "warn" {
		t.Fatalf("expensive page: %#v", got)
	}
	report := got[0]["detail"].(pageCPUReport)
	if report.Page != "Kameras" || report.Share != 50 || report.Finding != "high" {
		t.Fatalf("report = %#v", report)
	}
	got = nil
	pageCPUCheck([]supervisor.PageSummary{{PageID: "ha", Name: "HA", PagesCPUAverage: 40, Samples: 100}}, 4, add)
	if len(got) != 1 || got[0]["level"] != "ok" {
		t.Fatalf("cheap page: %#v", got)
	}
}

func TestTimeZoneCheck(t *testing.T) {
	var ids []string
	add := func(id, level, message string, detail any) { ids = append(ids, id+":"+level) }
	timeZoneCheck("Europe/Berlin", add)
	timeZoneCheck("", add)
	timeZoneCheck("Mars/Olympus", add)
	if len(ids) != 1 || ids[0] != "time_zone:warn" {
		t.Fatalf("checks = %v", ids)
	}
}

func TestDiskSpaceCheck(t *testing.T) {
	cases := []struct {
		available float64
		want      string
	}{{0.1, "error"}, {0.6, "warn"}, {12, "ok"}}
	for _, c := range cases {
		var level string
		diskSpaceCheck(map[string]any{"available_gb": c.available}, func(id, l, message string, detail any) {
			if id == "disk_space" {
				level = l
			}
		})
		if level != c.want {
			t.Fatalf("available %.1f: level %q, want %q", c.available, level, c.want)
		}
	}
	diskSpaceCheck(nil, func(string, string, string, any) { t.Fatal("no check expected without data") })
}
