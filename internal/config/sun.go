package config

import (
	"math"
	"strconv"
	"strings"
	"time"
)

// Schedule times may follow the sun instead of the clock: "sunrise",
// "sunset", or either with an offset in minutes such as "sunset-30" or
// "sunrise+15". They need the kiosk's location (scheduler latitude and
// longitude) and are resolved per day; without a location sunrise counts as
// 06:00 and sunset as 18:00.

const maxSunOffset = 6 * 60

// sunClock splits a sun time into its event and offset.
func sunClock(value string) (event string, offset int, ok bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	for _, name := range []string{"sunrise", "sunset"} {
		if !strings.HasPrefix(value, name) {
			continue
		}
		rest := strings.TrimSpace(value[len(name):])
		if rest == "" {
			return name, 0, true
		}
		if rest[0] != '+' && rest[0] != '-' {
			return "", 0, false
		}
		minutes, err := strconv.Atoi(strings.TrimSpace(rest[1:]))
		if err != nil || minutes < 0 || minutes > maxSunOffset {
			return "", 0, false
		}
		if rest[0] == '-' {
			minutes = -minutes
		}
		return name, minutes, true
	}
	return "", 0, false
}

// IsSunClock reports whether value is a valid sun time.
func IsSunClock(value string) bool {
	_, _, ok := sunClock(value)
	return ok
}

// validScheduleClock accepts HH:MM or a sun time.
func validScheduleClock(value string) bool {
	return validClock(value) || IsSunClock(value)
}

// HasLocation reports whether the scheduler knows where the kiosk is.
func (s KioskScheduler) HasLocation() bool {
	return s.Latitude != 0 || s.Longitude != 0
}

// ResolveClock returns the minute of the day value stands for on day: HH:MM
// as written, sun times for that day at the scheduler's location.
func (k KioskConfig) ResolveClock(value string, day time.Time) (int, bool) {
	if minute, ok := insertClock(value); ok {
		return minute, true
	}
	event, offset, ok := sunClock(value)
	if !ok {
		return 0, false
	}
	rise, set := 6*60, 18*60
	if k.Scheduler.HasLocation() {
		rise, set = SunMinutes(day, k.Scheduler.Latitude, k.Scheduler.Longitude)
	}
	minute := set
	if event == "sunrise" {
		minute = rise
	}
	minute += offset
	return min(max(minute, 0), 24*60-1), true
}

// SunMinutes returns sunrise and sunset on day as minutes in day's time zone.
// During polar night both are noon (empty window), during midnight sun the
// whole day is light.
func SunMinutes(day time.Time, latitude, longitude float64) (int, int) {
	rise, set, state := SunTimes(day, latitude, longitude)
	switch state {
	case "night":
		return 12 * 60, 12 * 60
	case "day":
		return 0, 24*60 - 1
	}
	local := func(at time.Time) int {
		at = at.In(day.Location())
		start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location())
		return min(max(int(at.Sub(start).Minutes()+0.5), 0), 24*60-1)
	}
	return local(rise), local(set)
}

// SunTimes computes sunrise and sunset on the calendar day of day with the
// sunrise equation (accurate to about a minute). state is "" normally,
// "night" when the sun stays below the horizon and "day" when it never sets.
func SunTimes(day time.Time, latitude, longitude float64) (rise, set time.Time, state string) {
	const rad = math.Pi / 180
	noon := time.Date(day.Year(), day.Month(), day.Day(), 12, 0, 0, 0, time.UTC)
	julian := float64(noon.Unix())/86400 + 2440587.5
	n := math.Round(julian - 2451545.0 + 0.0008)
	solarNoon := n - longitude/360
	anomaly := math.Mod(357.5291+0.98560028*solarNoon, 360)
	center := 1.9148*math.Sin(anomaly*rad) + 0.02*math.Sin(2*anomaly*rad) + 0.0003*math.Sin(3*anomaly*rad)
	ecliptic := math.Mod(anomaly+center+180+102.9372, 360)
	transit := 2451545.0 + solarNoon + 0.0053*math.Sin(anomaly*rad) - 0.0069*math.Sin(2*ecliptic*rad)
	declination := math.Asin(math.Sin(ecliptic*rad) * math.Sin(23.4397*rad))
	cosHour := (math.Sin(-0.833*rad) - math.Sin(latitude*rad)*math.Sin(declination)) / (math.Cos(latitude*rad) * math.Cos(declination))
	if cosHour > 1 {
		return time.Time{}, time.Time{}, "night"
	}
	if cosHour < -1 {
		return time.Time{}, time.Time{}, "day"
	}
	hour := math.Acos(cosHour) / rad
	toTime := func(j float64) time.Time {
		return time.Unix(0, int64((j-2440587.5)*86400*float64(time.Second))).In(day.Location())
	}
	return toTime(transit - hour/360), toTime(transit + hour/360), ""
}
