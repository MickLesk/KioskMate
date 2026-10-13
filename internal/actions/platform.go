package actions

import (
	"fmt"
	"runtime"
)

// System actions are built on sudo/su, apt, systemd and timedatectl. On
// Windows they are reported as unsupported instead of failing with a missing
// command or a confusing sudo error.
func checkPlatform(name string) error {
	return platformSupport(runtime.GOOS, name)
}

func platformSupport(goos string, name string) error {
	if goos != "windows" {
		return nil
	}
	switch name {
	case "privilege":
		return fmt.Errorf("sudo and su privileges are unsupported on Windows")
	default:
		return fmt.Errorf("system action %s is unsupported on Windows", name)
	}
}
