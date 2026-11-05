package admin

import "testing"

func TestParseSwaps(t *testing.T) {
	header := "Filename\t\t\t\tType\t\tSize\t\tUsed\t\tPriority\n"
	cases := []struct {
		text, finding string
		used          int
	}{
		{header, "none", 0},
		{header + "/dev/zram0 partition 1048572 204800 100\n", "zram", 200},
		{header + "/var/swap file 524284 409600 -2\n", "disk_used", 400},
		{header + "/var/swap file 524284 10240 -2\n", "disk", 10},
	}
	for _, c := range cases {
		report := parseSwaps(c.text)
		if report.Finding != c.finding || report.UsedMB != c.used {
			t.Errorf("%q: %#v", c.text, report)
		}
	}
	var level string
	swapCheck(parseSwaps(cases[2].text), func(_, l, _ string, _ any) { level = l })
	if level != "warn" {
		t.Fatalf("level = %s", level)
	}
}
