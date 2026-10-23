package config

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

// PageInsert shows a page for a short time on a fixed rhythm on top of the
// base programme (rotation and time rules): "every 5 minutes show the
// weather page for 30 seconds" or "at the full hour show today's calendar for
// one minute". The base programme keeps running underneath and takes over
// again when the insert ends.
//
// Repeat types:
//   - "interval": every EveryMinutes minutes, counted from midnight, so
//     every 5 minutes means :00, :05, :10 ...
//   - "hourly": once an hour at Minute past the hour
//   - "times": at each HH:MM in Times
//
// Start/End limit the insert to a daily window (overnight windows work), Days
// to weekdays. When inserts overlap the one with the higher Priority wins,
// equal priorities go to the insert listed first.
type PageInsert struct {
	ID              string   `json:"id"`
	Name            string   `json:"name,omitempty"`
	PageID          string   `json:"page_id"`
	Repeat          string   `json:"repeat"`
	EveryMinutes    int      `json:"every_minutes,omitempty"`
	Minute          int      `json:"minute,omitempty"`
	Times           []string `json:"times,omitempty"`
	DurationSeconds int      `json:"duration_seconds"`
	Start           string   `json:"start,omitempty"`
	End             string   `json:"end,omitempty"`
	Days            []string `json:"days,omitempty"`
	Priority        int      `json:"priority,omitempty"`
	Disabled        bool     `json:"disabled,omitempty"`
}

// InsertSlot is one run of an insert.
type InsertSlot struct {
	Insert PageInsert `json:"insert"`
	Index  int        `json:"index"`
	Start  time.Time  `json:"start"`
	End    time.Time  `json:"end"`
}

// ProgramSegment is a stretch of time in which either one insert or the base
// programme (InsertID empty) is on screen.
type ProgramSegment struct {
	Start    time.Time `json:"start"`
	End      time.Time `json:"end"`
	InsertID string    `json:"insert_id,omitempty"`
	Name     string    `json:"name,omitempty"`
	PageID   string    `json:"page_id,omitempty"`
	Priority int       `json:"priority,omitempty"`
}

const maxInsertDuration = time.Hour

var insertRepeats = []string{"interval", "hourly", "times"}

func cloneInserts(inserts []PageInsert) []PageInsert {
	out := slices.Clone(inserts)
	for i := range out {
		out[i].Times = slices.Clone(inserts[i].Times)
		out[i].Days = slices.Clone(inserts[i].Days)
	}
	return out
}

func validateInserts(kiosk KioskConfig) error {
	pageIDs := map[string]bool{}
	for _, page := range kiosk.Pages {
		pageIDs[page.PageID] = true
	}
	seen := map[string]bool{}
	for index, insert := range kiosk.Inserts {
		label := strings.TrimSpace(insert.Name)
		if label == "" {
			label = "insert " + strconv.Itoa(index+1)
		}
		fail := func(format string, args ...any) error {
			return fmt.Errorf("insert %s: %s", label, fmt.Sprintf(format, args...))
		}
		if !validRuleID(insert.ID) {
			return fail("id must be 1-64 letters, digits, '-' or '_'")
		}
		if seen[insert.ID] {
			return fail("id %q is used twice", insert.ID)
		}
		seen[insert.ID] = true
		if !pageIDs[insert.PageID] {
			return fail("page %q does not exist", insert.PageID)
		}
		if !slices.Contains(insertRepeats, insert.Repeat) {
			return fail("repeat must be interval, hourly or times")
		}
		switch insert.Repeat {
		case "interval":
			if insert.EveryMinutes < 1 || insert.EveryMinutes > 24*60 {
				return fail("interval must be between 1 minute and 24 hours")
			}
		case "hourly":
			if insert.Minute < 0 || insert.Minute > 59 {
				return fail("minute must be between 0 and 59")
			}
		case "times":
			if len(insert.Times) == 0 {
				return fail("needs at least one time")
			}
			for _, at := range insert.Times {
				if !validClock(at) {
					return fail("time %q must use HH:MM", at)
				}
			}
		}
		if insert.DurationSeconds < 5 || time.Duration(insert.DurationSeconds)*time.Second > maxInsertDuration {
			return fail("duration must be between 5 seconds and 1 hour")
		}
		if (insert.Start == "") != (insert.End == "") {
			return fail("window needs both start and end")
		}
		if insert.Start != "" && (!validClock(insert.Start) || !validClock(insert.End)) {
			return fail("window must use HH:MM")
		}
		if insert.Repeat == "interval" && time.Duration(insert.DurationSeconds)*time.Second >= time.Duration(insert.EveryMinutes)*time.Minute {
			return fail("duration must be shorter than the interval")
		}
	}
	return nil
}

// slotMinutes lists the minutes of the day at which an insert starts,
// ignoring days and window.
func (p PageInsert) slotMinutes() []int {
	var out []int
	switch p.Repeat {
	case "interval":
		if p.EveryMinutes <= 0 {
			return nil
		}
		for minute := 0; minute < 24*60; minute += p.EveryMinutes {
			out = append(out, minute)
		}
	case "hourly":
		if p.Minute < 0 || p.Minute > 59 {
			return nil
		}
		for hour := 0; hour < 24; hour++ {
			out = append(out, hour*60+p.Minute)
		}
	case "times":
		for _, at := range p.Times {
			if minute, ok := insertClock(at); ok {
				out = append(out, minute)
			}
		}
		sort.Ints(out)
		out = slices.Compact(out)
	}
	return out
}

// allows reports whether a run starting at minute on weekday is inside the
// insert's days and window.
func (p PageInsert) allows(weekday time.Weekday, minute int) bool {
	if !weekdayMatches(p.Days, weekday) {
		return false
	}
	if p.Start == "" {
		return true
	}
	start, ok1 := insertClock(p.Start)
	end, ok2 := insertClock(p.End)
	if !ok1 || !ok2 || start == end {
		return ok1 && ok2
	}
	if start < end {
		return minute >= start && minute < end
	}
	return minute >= start || minute < end
}

func (p PageInsert) duration() time.Duration {
	d := time.Duration(p.DurationSeconds) * time.Second
	if d <= 0 {
		return 0
	}
	return min(d, maxInsertDuration)
}

// InsertSlots returns every run of the enabled inserts that overlaps
// [from, to), ordered by start. Inserts whose page is disabled or missing are
// skipped.
func (k KioskConfig) InsertSlots(from, to time.Time) []InsertSlot {
	if !to.After(from) {
		return nil
	}
	var out []InsertSlot
	loc := from.Location()
	first := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, -1)
	for index, insert := range k.Inserts {
		if insert.Disabled || k.EnabledPageIndex(insert.PageID) < 0 {
			continue
		}
		duration := insert.duration()
		if duration <= 0 {
			continue
		}
		minutes := insert.slotMinutes()
		for day := first; day.Before(to); day = day.AddDate(0, 0, 1) {
			for _, minute := range minutes {
				start := time.Date(day.Year(), day.Month(), day.Day(), minute/60, minute%60, 0, 0, loc)
				end := start.Add(duration)
				if !end.After(from) || !start.Before(to) {
					continue
				}
				if !insert.allows(start.Weekday(), minute) {
					continue
				}
				out = append(out, InsertSlot{Insert: insert, Index: index, Start: start, End: end})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Start.Equal(out[j].Start) {
			return out[i].Index < out[j].Index
		}
		return out[i].Start.Before(out[j].Start)
	})
	return out
}

// wins reports whether slot a beats slot b when both are running.
func (a InsertSlot) wins(b InsertSlot) bool {
	if a.Insert.Priority != b.Insert.Priority {
		return a.Insert.Priority > b.Insert.Priority
	}
	return a.Index < b.Index
}

// ActiveInsert returns the insert run on screen at now. The run ends at End,
// or earlier when a higher-priority insert starts (see NextInsertChange).
func (k KioskConfig) ActiveInsert(now time.Time) (InsertSlot, bool) {
	var best InsertSlot
	found := false
	for _, slot := range k.InsertSlots(now, now.Add(time.Second)) {
		if now.Before(slot.Start) || !now.Before(slot.End) {
			continue
		}
		if !found || slot.wins(best) {
			best, found = slot, true
		}
	}
	return best, found
}

// NextInsertChange returns the next moment after now at which an insert
// starts or ends, within the next two days.
func (k KioskConfig) NextInsertChange(now time.Time) (time.Time, bool) {
	var next time.Time
	for _, slot := range k.InsertSlots(now, now.Add(48*time.Hour)) {
		for _, at := range []time.Time{slot.Start, slot.End} {
			if at.After(now) && (next.IsZero() || at.Before(next)) {
				next = at
			}
		}
	}
	return next, !next.IsZero()
}

// ProgramTimeline splits [from, to) into segments showing which insert, if
// any, is on screen. Gaps belong to the base programme.
func (k KioskConfig) ProgramTimeline(from, to time.Time) []ProgramSegment {
	slots := k.InsertSlots(from, to)
	cuts := []time.Time{from, to}
	for _, slot := range slots {
		if slot.Start.After(from) && slot.Start.Before(to) {
			cuts = append(cuts, slot.Start)
		}
		if slot.End.After(from) && slot.End.Before(to) {
			cuts = append(cuts, slot.End)
		}
	}
	sort.Slice(cuts, func(i, j int) bool { return cuts[i].Before(cuts[j]) })
	cuts = slices.CompactFunc(cuts, func(a, b time.Time) bool { return a.Equal(b) })
	var out []ProgramSegment
	for i := 0; i+1 < len(cuts); i++ {
		start, end := cuts[i], cuts[i+1]
		var best InsertSlot
		found := false
		for _, slot := range slots {
			if start.Before(slot.Start) || !start.Before(slot.End) {
				continue
			}
			if !found || slot.wins(best) {
				best, found = slot, true
			}
		}
		segment := ProgramSegment{Start: start, End: end}
		if found {
			segment.InsertID = best.Insert.ID
			segment.Name = best.Insert.Name
			segment.PageID = best.Insert.PageID
			segment.Priority = best.Insert.Priority
		}
		if n := len(out); n > 0 && out[n-1].InsertID == segment.InsertID && out[n-1].End.Equal(start) {
			out[n-1].End = end
			continue
		}
		out = append(out, segment)
	}
	return out
}

func insertClock(value string) (int, bool) {
	parts := strings.Split(strings.TrimSpace(value), ":")
	if len(parts) != 2 {
		return 0, false
	}
	hour, err1 := strconv.Atoi(parts[0])
	minute, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, false
	}
	return hour*60 + minute, true
}

func weekdayMatches(days []string, weekday time.Weekday) bool {
	if len(days) == 0 {
		return true
	}
	long := strings.ToLower(weekday.String())
	for _, day := range days {
		day = strings.ToLower(strings.TrimSpace(day))
		if day == long[:3] || day == long {
			return true
		}
	}
	return false
}
