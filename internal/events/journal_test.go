package events

import (
	"path/filepath"
	"testing"
	"time"
)

func TestJournalPersistsBoundedEvents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	j, err := Open(path, 2)
	if err != nil {
		t.Fatal(err)
	}
	j.Record("browser", "start", "ok", "browser started", map[string]string{"pid": "42"})
	j.Record("mqtt", "connect", "error", "not authorized", map[string]string{"password": "must-not-be-stored"})
	j.Record("browser", "reload", "ok", "page reloaded", nil)

	items := j.Recent(0)
	if len(items) != 2 || items[0].Action != "connect" || items[1].Action != "reload" {
		t.Fatalf("items = %#v", items)
	}
	if _, ok := items[0].Details["password"]; ok {
		t.Fatal("sensitive event detail was retained")
	}
	reloaded, err := Open(path, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.Recent(2); len(got) != 2 || got[1].Message != "page reloaded" {
		t.Fatalf("reloaded = %#v", got)
	}
}

func TestJournalRecordDuration(t *testing.T) {
	j, err := Open(filepath.Join(t.TempDir(), "events.jsonl"), 10)
	if err != nil {
		t.Fatal(err)
	}
	event := j.RecordDuration("update", "install", "ok", "installed", 1500*time.Millisecond, nil)
	items := j.Recent(1)
	if event.Duration != 1500 || len(items) != 1 || items[0].Duration != 1500 {
		t.Fatalf("event = %#v items = %#v", event, items)
	}
}
