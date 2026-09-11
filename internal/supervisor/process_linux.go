//go:build linux

package supervisor

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func processGroupAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}

func findProfileBrowserRoots(profile string, exclude int) []int {
	profile = filepath.Clean(strings.TrimSpace(profile))
	if profile == "." || profile == "" {
		return nil
	}
	entries, _ := filepath.Glob("/proc/[0-9]*/cmdline")
	uid := os.Getuid()
	var roots []int
	for _, entry := range entries {
		pid, err := strconv.Atoi(filepath.Base(filepath.Dir(entry)))
		if err != nil || pid == exclude {
			continue
		}
		info, err := os.Stat(filepath.Dir(entry))
		if err != nil {
			continue
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || int(stat.Uid) != uid {
			continue
		}
		data, err := os.ReadFile(entry)
		if err != nil {
			continue
		}
		args := strings.Split(strings.TrimSuffix(string(data), "\x00"), "\x00")
		hasProfile, child := false, false
		for _, arg := range args {
			if strings.HasPrefix(arg, "--type=") {
				child = true
			}
			if strings.HasPrefix(arg, "--user-data-dir=") && filepath.Clean(strings.TrimPrefix(arg, "--user-data-dir=")) == profile {
				hasProfile = true
			}
		}
		if hasProfile && !child {
			roots = append(roots, pid)
		}
	}
	return roots
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
