package supervisor

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLogFilterFoldsRepeats(t *testing.T) {
	var out bytes.Buffer
	f := newLogFilter(&out, nil)
	now := time.Unix(1000, 0)
	f.now = func() time.Time { return now }
	_, _ = f.Write([]byte("[1:2:1109/101500.1:ERROR:gpu.cc(1)] boom\n[1:2:1109/101500.2:ERROR:gpu.cc(1)] boom\n[1:2:1109/1015"))
	_, _ = f.Write([]byte("00.3:ERROR:gpu.cc(1)] boom\nother\n"))
	got := out.String()
	want := "[1:2:1109/101500.1:ERROR:gpu.cc(1)] boom\n    (previous line repeated 2 times)\nother\n"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	out.Reset()
	_, _ = f.Write([]byte("other\n"))
	now = now.Add(11 * time.Minute)
	_, _ = f.Write([]byte("other\n"))
	f.Flush()
	if out.String() != "    (previous line repeated 2 times)\n" {
		t.Fatalf("periodic summary: %q", out.String())
	}
}

func TestLogFilterKeepsFileBelowLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "browser.log")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	f := newLogFilter(nil, file)
	f.limit = 1000
	for i := 0; i < 200; i++ {
		_, _ = f.Write([]byte(strings.Repeat("x", 20) + string(rune('a'+i%26)) + "\n"))
	}
	info, _ := os.Stat(path)
	if info.Size() > 1000 {
		t.Fatalf("log size %d over limit", info.Size())
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "older lines removed") {
		t.Fatalf("missing truncation marker: %q", data)
	}
}
