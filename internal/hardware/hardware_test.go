package hardware

import "testing"

func TestSupportHintsOnlyDescribeUnavailableCapabilities(t *testing.T) {
	hints := supportHints(Support{DisplayStatus: true, AudioVolume: true})
	if _, exists := hints["display"]; exists {
		t.Fatal("supported display received an install hint")
	}
	if _, exists := hints["audio"]; exists {
		t.Fatal("supported audio received an install hint")
	}
	if hints["brightness"] == "" || hints["keyboard"] == "" {
		t.Fatalf("missing actionable hints: %#v", hints)
	}
}
