package main

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestSDNotifyAndWatchdog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notify")
	conn, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Name: path, Net: "unixgram"})
	if err != nil {
		t.Skip("unixgram unavailable:", err)
	}
	defer conn.Close()
	t.Setenv("NOTIFY_SOCKET", path)
	if !sdNotify("READY=1") {
		t.Fatal("notify failed")
	}
	buf := make([]byte, 64)
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, _ := conn.Read(buf)
	if string(buf[:n]) != "READY=1" {
		t.Fatalf("got %q", buf[:n])
	}

	t.Setenv("WATCHDOG_USEC", "40000")
	t.Setenv("WATCHDOG_PID", strconv.Itoa(os.Getpid()))
	if watchdogInterval() != 20*time.Millisecond {
		t.Fatalf("interval = %v", watchdogInterval())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	go runWatchdog(ctx, watchdogInterval(), func() bool { return true })
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, _ = conn.Read(buf)
	if string(buf[:n]) != "WATCHDOG=1" {
		t.Fatalf("got %q", buf[:n])
	}
	t.Setenv("WATCHDOG_PID", "1")
	if watchdogInterval() != 0 {
		t.Fatal("watchdog for another pid must be off")
	}
}
