package admin

import (
	"testing"
	"time"

	"github.com/MickLesk/KioskMate/internal/events"
)

func TestEventsSinceReturnsOnlyNewEvents(t *testing.T) {
	now := time.Now()
	recent := []events.Event{{ID: "a", At: now}, {ID: "b", At: now.Add(time.Second)}, {ID: "c", At: now.Add(2 * time.Second)}}
	fresh, newest := eventsSince(recent, "")
	if len(fresh) != 3 || newest != "c" {
		t.Fatalf("first push = %v, %q", fresh, newest)
	}
	fresh, newest = eventsSince(recent, "b")
	if len(fresh) != 1 || fresh[0].ID != "c" || newest != "c" {
		t.Fatalf("incremental push = %v, %q", fresh, newest)
	}
	if fresh, _ = eventsSince(recent, "c"); len(fresh) != 0 {
		t.Fatalf("no new events expected, got %v", fresh)
	}
	if fresh, _ = eventsSince(recent, "gone"); len(fresh) != 3 {
		t.Fatalf("evicted marker should resend the window, got %v", fresh)
	}
}
