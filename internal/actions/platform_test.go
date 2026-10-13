package actions

import (
	"strings"
	"testing"
)

func TestPlatformSupportReportsWindows(t *testing.T) {
	for _, name := range []string{"apt-update", "apt-upgrade", "restart-service", "reboot", "shutdown", "time-config", "privilege"} {
		err := platformSupport("windows", name)
		if err == nil || !strings.Contains(err.Error(), "unsupported on Windows") {
			t.Fatalf("%s on windows: err = %v", name, err)
		}
		if err := platformSupport("linux", name); err != nil {
			t.Fatalf("%s on linux: err = %v", name, err)
		}
	}
}
