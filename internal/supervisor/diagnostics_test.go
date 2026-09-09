package supervisor

import "testing"

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
