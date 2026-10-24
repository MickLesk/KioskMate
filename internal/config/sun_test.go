package config

import (
	"testing"
	"time"
)

func TestSunMinutesBerlin(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Skip("time zone data unavailable")
	}
	cases := []struct {
		day       time.Time
		rise, set int
	}{
		{time.Date(2026, 6, 21, 12, 0, 0, 0, berlin), 4*60 + 43, 21*60 + 33},
		{time.Date(2026, 12, 21, 12, 0, 0, 0, berlin), 8*60 + 15, 15*60 + 54},
	}
	for _, tc := range cases {
		rise, set := SunMinutes(tc.day, 52.52, 13.405)
		if abs(rise-tc.rise) > 3 || abs(set-tc.set) > 3 {
			t.Fatalf("%s: got %d/%d, want about %d/%d", tc.day.Format("2006-01-02"), rise, set, tc.rise, tc.set)
		}
	}
}

func TestSunMinutesPolar(t *testing.T) {
	rise, set := SunMinutes(time.Date(2026, 12, 21, 12, 0, 0, 0, time.UTC), 78.2, 15.6)
	if rise != set {
		t.Fatalf("polar night should give an empty window, got %d-%d", rise, set)
	}
	rise, set = SunMinutes(time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC), 78.2, 15.6)
	if rise != 0 || set != 24*60-1 {
		t.Fatalf("midnight sun should cover the day, got %d-%d", rise, set)
	}
}

func TestResolveClock(t *testing.T) {
	day := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	var k KioskConfig
	cases := map[string]int{"07:30": 450, "sunrise": 360, "sunset-30": 1050, "sunrise+15": 375, "Sunset": 1080}
	for value, want := range cases {
		if got, ok := k.ResolveClock(value, day); !ok || got != want {
			t.Fatalf("%q: got %d %v, want %d", value, got, ok, want)
		}
	}
	for _, bad := range []string{"", "noon", "sunset+", "sunset*5", "sunrise+999", "25:00"} {
		if _, ok := k.ResolveClock(bad, day); ok {
			t.Fatalf("%q should be rejected", bad)
		}
	}
}

func TestInsertSunTimesAndCondition(t *testing.T) {
	k := KioskConfig{
		Pages: []KioskPage{{PageID: "home", URL: "http://a"}, {PageID: "cam", URL: "http://b"}},
		Inserts: []PageInsert{{
			ID: "cam", PageID: "cam", Repeat: "times", Times: []string{"sunset"}, DurationSeconds: 60,
			Condition: &InsertCondition{Topic: "motion/hall", Value: "ON"},
		}},
	}
	if err := validateInserts(k); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 3, 1, 18, 0, 30, 0, time.UTC)
	if _, ok := k.ActiveInsert(at); !ok {
		t.Fatal("unconditional view should show the sunset insert at 18:00")
	}
	values := map[string]string{}
	lookup := func(topic string) (string, bool) { value, ok := values[topic]; return value, ok }
	if _, ok := k.InsertsWhere(lookup).ActiveInsert(at); ok {
		t.Fatal("insert must not show before its topic sent anything")
	}
	values["motion/hall"] = " on "
	if _, ok := k.InsertsWhere(lookup).ActiveInsert(at); !ok {
		t.Fatal("insert should show while the condition holds")
	}
	k.Inserts[0].Condition.Negate = true
	if _, ok := k.InsertsWhere(lookup).ActiveInsert(at); ok {
		t.Fatal("negated condition should hide the insert")
	}
	if k.Inserts[0].Disabled {
		t.Fatal("InsertsWhere must not change the original config")
	}
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
