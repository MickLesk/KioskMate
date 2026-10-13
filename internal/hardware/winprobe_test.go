package hardware

import (
	"strings"
	"testing"
)

func TestParseWindowsProbeWithBrightness(t *testing.T) {
	probe, err := parseWindowsProbe("\ufeff" + `{"manufacturer":" LENOVO ","model":"20XY","serial":"PF123","brightness":70,"brightness_methods":1,"battery":88}` + "\r\n")
	if err != nil {
		t.Fatal(err)
	}
	if probe.Manufacturer != "LENOVO" || probe.Model != "20XY" || probe.Serial != "PF123" {
		t.Fatalf("device = %#v", probe)
	}
	if !probe.brightnessSupported() || *probe.Brightness != 70 || probe.Battery == nil || *probe.Battery != 88 {
		t.Fatalf("probe = %#v", probe)
	}
}

func TestParseWindowsProbeWithoutWMIBrightness(t *testing.T) {
	probe, err := parseWindowsProbe(`{"manufacturer":"","model":"","serial":"","brightness":null,"brightness_methods":0,"battery":null}`)
	if err != nil {
		t.Fatal(err)
	}
	if probe.brightnessSupported() || probe.Battery != nil {
		t.Fatalf("probe = %#v", probe)
	}
	probe, err = parseWindowsProbe(`{"brightness":250,"brightness_methods":1,"battery":-1}`)
	if err != nil {
		t.Fatal(err)
	}
	if probe.brightnessSupported() || probe.Battery != nil {
		t.Fatalf("out-of-range values kept: %#v", probe)
	}
	if _, err := parseWindowsProbe("  "); err == nil {
		t.Fatal("empty output accepted")
	}
	if _, err := parseWindowsProbe("Get-CimInstance : error"); err == nil {
		t.Fatal("non-JSON output accepted")
	}
}

func TestWindowsBrightnessScript(t *testing.T) {
	script := windowsBrightnessScript(42)
	for _, want := range []string{"WmiMonitorBrightnessMethods", "WmiSetBrightness", "Brightness = [byte]42 "} {
		if !strings.Contains(script, want) {
			t.Fatalf("script lacks %q:\n%s", want, script)
		}
	}
	args := powershellArgs(script)
	if args[len(args)-2] != "-Command" || args[len(args)-1] != script {
		t.Fatalf("args = %#v", args)
	}
}
