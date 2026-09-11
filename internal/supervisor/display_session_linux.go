//go:build linux

package supervisor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func displaySessionStatus() DisplaySessionStatus {
	runtimeDir := strings.TrimSpace(os.Getenv("XDG_RUNTIME_DIR"))
	wayland := strings.TrimSpace(os.Getenv("WAYLAND_DISPLAY"))
	if wayland == "" && strings.EqualFold(os.Getenv("XDG_SESSION_TYPE"), "wayland") && runtimeDir != "" {
		matches, _ := filepath.Glob(filepath.Join(runtimeDir, "wayland-*"))
		for _, match := range matches {
			if !strings.HasSuffix(match, ".lock") {
				wayland = filepath.Base(match)
				break
			}
		}
	}
	if wayland != "" {
		path := wayland
		if !filepath.IsAbs(path) {
			path = filepath.Join(runtimeDir, wayland)
		}
		if info, err := os.Stat(path); err == nil && info.Mode()&os.ModeSocket != 0 {
			return DisplaySessionStatus{Ready: true, Type: "wayland", Endpoint: path}
		}
		return DisplaySessionStatus{Type: "wayland", Endpoint: path, Error: "Wayland socket is not ready"}
	}
	if display := strings.TrimSpace(os.Getenv("DISPLAY")); display != "" {
		number := strings.TrimPrefix(strings.SplitN(display, ".", 2)[0], ":")
		path := filepath.Join("/tmp/.X11-unix", "X"+number)
		if info, err := os.Stat(path); err == nil && info.Mode()&os.ModeSocket != 0 {
			return DisplaySessionStatus{Ready: true, Type: "x11", Endpoint: display}
		}
		return DisplaySessionStatus{Type: "x11", Endpoint: display, Error: "X11 socket is not ready"}
	}
	return DisplaySessionStatus{Error: "no graphical display environment was imported into the user service"}
}

func waitForDisplaySession(ctx context.Context) (DisplaySessionStatus, error) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		status := displaySessionStatus()
		if status.Ready {
			return status, nil
		}
		select {
		case <-ctx.Done():
			return status, fmt.Errorf("display session not ready (%s): %w", status.Error, ctx.Err())
		case <-ticker.C:
		}
	}
}
