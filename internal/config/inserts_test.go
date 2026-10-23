package config

import (
	"strings"
	"testing"
	"time"
)

func insertKiosk(inserts ...PageInsert) KioskConfig {
	return KioskConfig{
		Pages: []KioskPage{
			{PageID: "ha", Name: "HA", URL: "http://ha.local:8123"},
			{PageID: "weather", Name: "Weather", URL: "http://ha.local:8123/weather"},
			{PageID: "calendar", Name: "Calendar", URL: "http://ha.local:8123/calendar"},
			{PageID: "off", Name: "Off", URL: "http://ha.local:8123/off", Disabled: true},
		},
		Inserts: inserts,
	}
}

func at(clock string) time.Time {
	parsed, err := time.ParseInLocation("2006-01-02 15:04:05", "2026-10-23 "+clock, time.UTC)
	if err != nil {
		panic(err)
	}
	return parsed
}

func TestActiveInsertIntervalAndHourlyPriority(t *testing.T) {
	kiosk := insertKiosk(
		PageInsert{ID: "weather", PageID: "weather", Repeat: "interval", EveryMinutes: 5, DurationSeconds: 30, Start: "07:00", End: "23:00"},
		PageInsert{ID: "calendar", PageID: "calendar", Repeat: "hourly", Minute: 0, DurationSeconds: 60, Priority: 1, Start: "07:00", End: "23:00"},
	)
	cases := []struct {
		clock string
		want  string
	}{
		{"06:55:10", ""},
		{"07:05:00", "weather"},
		{"07:05:29", "weather"},
		{"07:05:30", ""},
		{"08:00:10", "calendar"},
		{"08:00:45", "calendar"},
		{"08:01:00", ""},
		{"23:00:05", ""},
	}
	for _, tc := range cases {
		slot, ok := kiosk.ActiveInsert(at(tc.clock))
		got := ""
		if ok {
			got = slot.Insert.ID
		}
		if got != tc.want {
			t.Errorf("%s: active insert = %q, want %q", tc.clock, got, tc.want)
		}
	}
}

func TestActiveInsertSkipsDisabledPagesAndDays(t *testing.T) {
	kiosk := insertKiosk(
		PageInsert{ID: "off", PageID: "off", Repeat: "interval", EveryMinutes: 5, DurationSeconds: 30},
		PageInsert{ID: "weekend", PageID: "weather", Repeat: "interval", EveryMinutes: 5, DurationSeconds: 30, Days: []string{"sat", "sun"}},
	)
	// 2026-10-23 is a Friday.
	if slot, ok := kiosk.ActiveInsert(at("10:05:01")); ok {
		t.Fatalf("expected no insert, got %q", slot.Insert.ID)
	}
}

func TestActiveInsertTimesAcrossMidnight(t *testing.T) {
	kiosk := insertKiosk(PageInsert{ID: "late", PageID: "weather", Repeat: "times", Times: []string{"23:59"}, DurationSeconds: 120})
	next := at("23:59:00").Add(90 * time.Second) // 00:00:30 next day
	slot, ok := kiosk.ActiveInsert(next)
	if !ok || slot.Insert.ID != "late" {
		t.Fatalf("expected insert spilling past midnight to be active, got %v %v", slot, ok)
	}
}

func TestNextInsertChange(t *testing.T) {
	kiosk := insertKiosk(PageInsert{ID: "weather", PageID: "weather", Repeat: "interval", EveryMinutes: 5, DurationSeconds: 30})
	next, ok := kiosk.NextInsertChange(at("10:01:00"))
	if !ok || !next.Equal(at("10:05:00")) {
		t.Fatalf("next change = %v, want 10:05", next)
	}
	next, _ = kiosk.NextInsertChange(at("10:05:10"))
	if !next.Equal(at("10:05:30")) {
		t.Fatalf("next change = %v, want 10:05:30", next)
	}
}

func TestProgramTimelineLetsPriorityInterrupt(t *testing.T) {
	kiosk := insertKiosk(
		PageInsert{ID: "weather", Name: "Weather", PageID: "weather", Repeat: "interval", EveryMinutes: 5, DurationSeconds: 120},
		PageInsert{ID: "calendar", Name: "Calendar", PageID: "calendar", Repeat: "times", Times: []string{"10:06"}, DurationSeconds: 60, Priority: 2},
	)
	segments := kiosk.ProgramTimeline(at("10:04:00"), at("10:10:00"))
	var got []string
	for _, segment := range segments {
		got = append(got, segment.Start.Format("15:04")+"-"+segment.End.Format("15:04")+"="+segment.InsertID)
	}
	want := "10:04-10:05= 10:05-10:06=weather 10:06-10:07=calendar 10:07-10:10="
	if strings.Join(got, " ") != want {
		t.Fatalf("timeline = %s\nwant      %s", strings.Join(got, " "), want)
	}
}

func TestValidateInserts(t *testing.T) {
	cases := []struct {
		name   string
		insert PageInsert
		want   string
	}{
		{"ok", PageInsert{ID: "a", PageID: "weather", Repeat: "interval", EveryMinutes: 5, DurationSeconds: 30}, ""},
		{"page", PageInsert{ID: "a", PageID: "nope", Repeat: "interval", EveryMinutes: 5, DurationSeconds: 30}, "does not exist"},
		{"repeat", PageInsert{ID: "a", PageID: "weather", Repeat: "daily", DurationSeconds: 30}, "repeat"},
		{"longer than interval", PageInsert{ID: "a", PageID: "weather", Repeat: "interval", EveryMinutes: 1, DurationSeconds: 60}, "shorter"},
		{"times", PageInsert{ID: "a", PageID: "weather", Repeat: "times", Times: []string{"7:00"}, DurationSeconds: 30}, "HH:MM"},
		{"window", PageInsert{ID: "a", PageID: "weather", Repeat: "hourly", DurationSeconds: 30, Start: "07:00"}, "both"},
		{"duration", PageInsert{ID: "a", PageID: "weather", Repeat: "hourly", DurationSeconds: 2}, "duration"},
	}
	for _, tc := range cases {
		err := validateInserts(insertKiosk(tc.insert))
		if tc.want == "" && err != nil {
			t.Errorf("%s: unexpected error %v", tc.name, err)
		}
		if tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
			t.Errorf("%s: error = %v, want %q", tc.name, err, tc.want)
		}
	}
}
