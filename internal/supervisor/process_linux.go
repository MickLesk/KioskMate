//go:build linux

package supervisor

import (
	"errors"
	"syscall"
	"time"
)

func processGroupAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}

func terminateProcessTree(pid int) error {
	if pid <= 0 {
		return nil
	}
	_ = syscall.Kill(-pid, syscall.SIGTERM)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(-pid, 0); errors.Is(err, syscall.ESRCH) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	return nil
}
