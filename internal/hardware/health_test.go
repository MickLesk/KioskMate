package hardware

import (
	"slices"
	"strings"
	"testing"
)

const inputDevices64 = `I: Bus=001e Vendor=0000 Product=0000 Version=0001
N: Name="vc4-hdmi-0"
P: Phys=vc4-hdmi-0/input0
H: Handlers=kbd event0
B: PROP=20
B: EV=100017
B: KEY=ffffc000000000 3ff 0 400000320fc200 40830c900000000 0 210300 49d2c040ec00 1e378000000000 8010000010000000
B: REL=3
B: MSC=10

I: Bus=0003 Vendor=0eef Product=0005 Version=0111
N: Name="eGalax Inc. USB TouchController"
H: Handlers=mouse0 event4
B: PROP=0
B: EV=b
B: KEY=400 0 0 0 0 0
B: ABS=3

I: Bus=0018 Vendor=0000 Product=0000 Version=0000
N: Name="generic ft5x06 (79)"
H: Handlers=event5
B: PROP=2
B: EV=b
B: KEY=400 0 0 0 0 0
B: ABS=260800000000003
`

func TestTouchDevices64(t *testing.T) {
	got := touchDevices(strings.NewReader(inputDevices64), 64)
	want := []string{"eGalax Inc. USB TouchController", "generic ft5x06 (79)"}
	if !slices.Equal(got, want) {
		t.Fatalf("touch devices = %q, want %q", got, want)
	}
}

func TestTouchDevices32(t *testing.T) {
	devices := `N: Name="generic ft5x06 (79)"
B: KEY=400 0 0 0 0 0 0 0 0 0 0
B: ABS=2608000 3
`
	if got := touchDevices(strings.NewReader(devices), 32); len(got) != 1 {
		t.Fatalf("32-bit multi-touch not detected: %q", got)
	}
}

func TestTouchDevicesNoneOnHDMIOnly(t *testing.T) {
	hdmiOnly := strings.SplitN(inputDevices64, "\n\n", 2)[0]
	if got := touchDevices(strings.NewReader(hdmiOnly), 64); len(got) != 0 {
		t.Fatalf("HDMI CEC remote reported as touch: %q", got)
	}
}

func TestPowerStatus(t *testing.T) {
	status := powerStatus("0", "throttled=0x50000")
	if status["undervoltage_now"] != false || status["undervoltage_since_boot"] != true || status["throttled_since_boot"] != true {
		t.Fatalf("since-boot flags not decoded: %#v", status)
	}
	if status := powerStatus("1", ""); status["undervoltage_now"] != true {
		t.Fatalf("live alarm ignored: %#v", status)
	}
	if status := powerStatus("", "throttled=0x50005"); status["undervoltage_now"] != true || status["throttled_now"] != true {
		t.Fatalf("current bits not decoded: %#v", status)
	}
	if status := powerStatus("", ""); status != nil {
		t.Fatalf("non-Pi board should report nil, got %#v", status)
	}
}
