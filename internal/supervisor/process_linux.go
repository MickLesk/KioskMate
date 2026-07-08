//go:build linux

package supervisor

import (
	"os/exec"
	"strconv"
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
	done := make(chan error, 1)
	go func() {
		done <- exec.Command("sh", "-c", "while kill -0 "+strconv.Itoa(pid)+" 2>/dev/null; do sleep 0.1; done").Run()
	}()
	select {
	case <-done:
		return nil
	case <-time.After(3 * time.Second):
		return syscall.Kill(-pid, syscall.SIGKILL)
	}
}
