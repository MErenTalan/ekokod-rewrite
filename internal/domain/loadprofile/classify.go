package loadprofile

import "time"

// DayType classifies a calendar date for load-profile purposes.
type DayType string

// Weekday and Weekend are the two values Classify returns.
const (
	Weekday DayType = "weekday"
	Weekend DayType = "weekend"
)

// Season is a meteorological season (R82) — never the legacy equinox-based
// cutoff.
type Season string

// Winter, Spring, Summer and Autumn are the four values SeasonOf returns,
// using meteorological (not legacy equinox-anchored) month boundaries.
const (
	Winter Season = "winter" // Dec, Jan, Feb — meteorological, per §10.3 (R82)
	Spring Season = "spring" // Mar, Apr, May
	Summer Season = "summer" // Jun, Jul, Aug
	Autumn Season = "autumn" // Sep, Oct, Nov
)

// Key is a profile identifier: "weekday", "weekend", and the eight
// "<season>_<daytype>" combinations.
type Key string

// DateRange is an inclusive [Start, End] range of calendar dates
// (company_vacations).
type DateRange struct{ Start, End time.Time }

// Config is the company's classification configuration. WeekendDays is
// applied exactly as given; the service supplies the default when the
// company configured none (R69). Vacations are inclusive date ranges.
// calendar_events never participate (R70) — there is no field here for one.
type Config struct {
	WeekendDays map[time.Weekday]bool
	Vacations   []DateRange
	Location    *time.Location
}

// calendarDate normalises t to the Y-M-D value of its calendar date in loc,
// at midnight, so two time.Time values naming the same local calendar date
// (regardless of any time-of-day component) compare and compute equal.
func calendarDate(t time.Time, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.UTC
	}
	y, m, day := t.In(loc).Date()
	return time.Date(y, m, day, 0, 0, 0, 0, loc)
}

// Classify reports whether a calendar date is a weekday or a weekend for
// this company: a configured non-working weekday, or any date inside a
// vacation period (inclusive of both ends), is Weekend (§10.3). Dates are
// compared, never instants: day is converted to cfg.Location and reduced to
// its calendar date before any comparison.
func Classify(day time.Time, cfg Config) DayType {
	cal := calendarDate(day, cfg.Location)

	if cfg.WeekendDays[cal.Weekday()] {
		return Weekend
	}

	for _, v := range cfg.Vacations {
		start := calendarDate(v.Start, cfg.Location)
		end := calendarDate(v.End, cfg.Location)
		if !cal.Before(start) && !cal.After(end) {
			return Weekend
		}
	}

	return Weekday
}

// SeasonOf returns the meteorological season of a date (R82): winter is
// December-February, spring March-May, summer June-August, autumn
// September-November.
func SeasonOf(day time.Time, loc *time.Location) Season {
	if loc == nil {
		loc = time.UTC
	}
	switch day.In(loc).Month() {
	case time.December, time.January, time.February:
		return Winter
	case time.March, time.April, time.May:
		return Spring
	case time.June, time.July, time.August:
		return Summer
	default: // September, October, November
		return Autumn
	}
}

// KeysFor returns the profile keys a day contributes to: its bare day type
// and its season+day-type combination.
func KeysFor(dt DayType, s Season) []Key {
	return []Key{Key(dt), Key(string(s) + "_" + string(dt))}
}
