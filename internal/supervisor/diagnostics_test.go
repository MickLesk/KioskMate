package supervisor

import (
	"errors"
	"testing"
	"time"
)

func TestDiagnosticURLRemovesSecrets(t *testing.T) {
	got := diagnosticURL("https://user:secret@ha.example:8123/dashboard/kiosk?access_token=private#view")
	want := "https://ha.example:8123/dashboard/kiosk"
	if got != want {
		t.Fatalf("diagnosticURL() = %q, want %q", got, want)
	}
}

func TestDiagnosticArgsRedactsOnlyURLs(t *testing.T) {
	got := diagnosticArgs([]string{"--kiosk", "https://ha.example/page?token=private", "--flag=value"})
	if got[1] != "https://ha.example/page" || got[0] != "--kiosk" || got[2] != "--flag=value" {
		t.Fatalf("diagnosticArgs() = %#v", got)
	}
}

func TestClassifyProcessExit(t *testing.T) {
	tests := []struct {
		name     string
		expected bool
		reason   string
		runtime  time.Duration
		err      error
		want     string
	}{
		{name: "manual", expected: true, reason: "manual stop", want: "manual_stop"},
		{name: "short", runtime: 2 * time.Second, err: errors.New("exit status 1"), want: "startup_exit"},
		{name: "crash", runtime: time.Minute, err: errors.New("signal: killed"), want: "process_crash"},
		{name: "clean unexpected", runtime: time.Minute, want: "unexpected_exit"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := classifyProcessExit(test.expected, test.reason, test.runtime, test.err); got != test.want {
				t.Fatalf("classifyProcessExit() = %q, want %q", got, test.want)
			}
		})
	}
}
