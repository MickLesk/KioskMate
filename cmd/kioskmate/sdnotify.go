package main

import (
	"context"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

// systemd integration without a library: READY=1 once the Admin UI listens
// (Type=notify), and WATCHDOG=1 pings while KioskMate is responsive, so a
// hung KioskMate (deadlocked supervisor) is restarted by systemd instead of
// leaving the kiosk frozen. Both are no-ops outside systemd.

func sdNotify(state string) bool {
	socket := os.Getenv("NOTIFY_SOCKET")
	if socket == "" {
		return false
	}
	conn, err := net.DialUnix("unixgram", nil, &net.UnixAddr{Name: socket, Net: "unixgram"})
	if err != nil {
		return false
	}
	defer conn.Close()
	_, err = conn.Write([]byte(state))
	return err == nil
}

// watchdogInterval returns half the systemd watchdog timeout, or 0 when the
// watchdog is off or meant for another process (KioskMate under Cage).
func watchdogInterval() time.Duration {
	usec, err := strconv.ParseInt(strings.TrimSpace(os.Getenv("WATCHDOG_USEC")), 10, 64)
	if err != nil || usec <= 0 {
		return 0
	}
	if pid := strings.TrimSpace(os.Getenv("WATCHDOG_PID")); pid != "" && pid != strconv.Itoa(os.Getpid()) {
		return 0
	}
	return time.Duration(usec) * time.Microsecond / 2
}

// runWatchdog pings systemd while healthy() answers within the interval.
// healthy should touch the locks a hang would hold (config, supervisor).
func runWatchdog(ctx context.Context, interval time.Duration, healthy func() bool) {
	if interval <= 0 {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			answered := make(chan bool, 1)
			go func() { answered <- healthy() }()
			select {
			case ok := <-answered:
				if ok {
					sdNotify("WATCHDOG=1")
				}
			case <-time.After(interval):
			case <-ctx.Done():
				return
			}
		}
	}
}
