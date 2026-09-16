package loadprofile_test

import (
	"time"

	_ "time/tzdata" // Europe/Istanbul must be loadable without system tzdata

	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/loadprofile"
)

// ist is Europe/Istanbul, loaded from the tzdata bundled by the blank
// import above so the test suite never depends on the host's system tzdata.
var ist = mustIstanbul()

func mustIstanbul() *time.Location {
	loc, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		panic(err)
	}
	return loc
}

// date builds an Istanbul-local calendar date at midnight — the shape
// loadprofile.HourValue.Day and loadprofile.DateRange bounds are documented
// to carry.
func date(y, m, d int) time.Time {
	return time.Date(y, time.Month(m), d, 0, 0, 0, 0, ist)
}

// d parses a decimal literal, panicking on a malformed literal — this is
// test fixture setup, never production arithmetic.
func d(s string) decimal.Decimal {
	v, err := decimal.NewFromString(s)
	if err != nil {
		panic(err)
	}
	return v
}

// weekdayCfg is a plain Saturday/Sunday-weekend company with no vacations,
// used by tests that only care about day-type/season bucketing.
var weekdayCfg = loadprofile.Config{
	WeekendDays: map[time.Weekday]bool{time.Saturday: true, time.Sunday: true},
	Location:    ist,
}

// profileWith builds a Profile from a sparse hour->literal map, leaving
// every other hour nil — exactly the shape Stats documents it accepts.
func profileWith(hours map[int]string) loadprofile.Profile {
	p := loadprofile.Profile{Key: "test"}
	for h, v := range hours {
		val := d(v)
		p.Hours[h] = &val
	}
	return p
}
