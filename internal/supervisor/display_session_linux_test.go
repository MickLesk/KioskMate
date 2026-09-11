//go:build linux

package supervisor

import "testing"

func TestDisplaySessionExplainsMissingEnvironment(t *testing.T) {
	for _, name := range []string{"DISPLAY", "WAYLAND_DISPLAY", "XDG_SESSION_TYPE", "XDG_RUNTIME_DIR"} {
		t.Setenv(name, "")
	}
	status := displaySessionStatus()
	if status.Ready || status.Error == "" {
		t.Fatalf("display status = %#v", status)
	}
}
