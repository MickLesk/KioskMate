package supervisor

import (
	"strings"
	"testing"

	"github.com/MickLesk/KioskMate/internal/config"
)

func TestOfflineFallback(t *testing.T) {
	cfg := &config.Config{Admin: config.AdminConfig{Port: 8099}}
	got, ok := offlineFallback(cfg, "http://ha.local:8123/lovelace?kiosk", "net::ERR_NAME_NOT_RESOLVED")
	if !ok || got != "http://127.0.0.1:8099/local/offline?u=http%3A%2F%2Fha.local%3A8123%2Flovelace%3Fkiosk" {
		t.Fatalf("fallback = %q %v", got, ok)
	}
	for _, c := range []struct{ target, err string }{
		{"http://ha.local/", "net::ERR_ABORTED"},
		{"http://ha.local/", ""},
		{"file:///tmp/x.html", "net::ERR_FILE_NOT_FOUND"},
		{"http://127.0.0.1:8099/local/clock", "net::ERR_CONNECTION_REFUSED"},
	} {
		if got, ok := offlineFallback(cfg, c.target, c.err); ok {
			t.Fatalf("%s %s should not fall back, got %q", c.target, c.err, got)
		}
	}
	if !strings.Contains(config.LocalPageURL(cfg.Admin, "offline"), "/local/offline") {
		t.Fatal("offline page path")
	}
}
