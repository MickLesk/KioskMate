//go:build !linux && !windows

package supervisor

import (
	"os/exec"
	"strconv"
	"syscall"
)

func processGroupAttr() *syscall.SysProcAttr {
	return nil
}

func terminateProcessTree(pid int) error {
	if pid <= 0 {
		return nil
	}
	return exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(pid)).Run()
}

func adoptProcessTree(int) {}
