package loadprofile_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/loadprofile"
)

// TestBuildAndStatsMatchHandComputedValuesAcrossDayTypesAndSeasons covers
// F3 acceptance criterion 6 end to end: it runs real calendar dates through
// Build (and therefore Classify and SeasonOf), not a hand-built Profile, and
// checks Stats against arithmetic worked out by hand for three resulting
// profile keys.
//
// Fixture (all dates Europe/Istanbul, weekend = Saturday+Sunday, one
// single-day vacation on a weekday):
//
//	D1 = 2026-02-24 Tuesday   winter weekday            hour6=40  hour18=80
//	D2 = 2026-02-25 Wednesday winter, on vacation        hour6=8   hour18=16
//	D3 = 2026-02-28 Saturday  winter weekend             hour6=12  hour18=24
//	D4 = 2026-03-02 Monday    spring weekday             hour6=20  hour18=40
//	D5 = 2026-03-04 Wednesday spring weekday             hour6=30  hour18=60
//
// Weekday-of-date check (anchor: 2026-01-01 is a Thursday; offset = days
// since 2026-01-01 mod 7, Thu=0):
//
//	D1: Jan(31)+23 = offset 54, 54 mod 7 = 5 -> Thu+5 = Tuesday
//	D2: offset 55, 55 mod 7 = 6 -> Thu+6 = Wednesday
//	D3: offset 58, 58 mod 7 = 2 -> Thu+2 = Saturday
//	D4: Jan(31)+Feb(28)+1 = offset 60, 60 mod 7 = 4 -> Thu+4 = Monday
//	D5: offset 62, 62 mod 7 = 6 -> Thu+6 = Wednesday
//
// D2 (Wednesday, not a configured weekend day) is inside the single-day
// vacation [2026-02-25, 2026-02-25], so Classify reports it Weekend (R70's
// vacation-as-weekend rule), not Weekday.
//
// Only hours 6 and 18 are ever reported, so every resulting profile has
// exactly two non-nil hourly means and 22 nil hours; every Stats call below
// operates on exactly those two values, which makes the population variance
// ((high-low)/2)^2 by construction (a two-point population's variance is
// always the square of half the spread), and so every StdDev below is an
// exact integer or half-integer by hand. PowWithPrecision's Newton-iteration
// square root lands within a few units of the requested precision but is
// not bit-exact even on a perfect square (e.g. sqrt(225) comes back as
// "14.999999999999999999999999999997", not "15"), so StdDev is asserted
// with StringFixed(15) against the hand value rather than raw string
// equality — 15 digits is far inside the ~30-digit-accurate region the
// implementation actually produces.
//
// --- "weekday" profile: D1, D4, D5 (all three weekdays, winter + spring) ---
//
//	hour6:  (40+20+30)/3 = 90/3  = 30
//	hour18: (80+40+60)/3 = 180/3 = 60
//	Days      = 3 (D1, D4, D5)
//	Max       = 60 (hour 18)
//	Min       = 30 (hour 6)
//	HourOfMax = 18
//	Mean      = (30+60)/2 = 45
//	Range     = 60-30 = 30
//	StdDev    = |60-30|/2 = 15   (variance = 15^2 = 225, sqrt(225) = 15)
//	LoadFactor= 45/60 = 0.75
//
// --- "weekend" profile: D2, D3 (both winter; no spring weekend in this fixture) ---
//
//	hour6:  (8+12)/2  = 10
//	hour18: (16+24)/2 = 20
//	Days      = 2 (D2, D3)
//	Max       = 20 (hour 18)
//	Min       = 10 (hour 6)
//	HourOfMax = 18
//	Mean      = (10+20)/2 = 15
//	Range     = 20-10 = 10
//	StdDev    = |20-10|/2 = 5    (variance = 5^2 = 25, sqrt(25) = 5)
//	LoadFactor= 15/20 = 0.75
//
// --- "spring_weekday" profile: D4, D5 (the two spring weekdays only) ---
//
//	hour6:  (20+30)/2 = 25
//	hour18: (40+60)/2 = 50
//	Days      = 2 (D4, D5)
//	Max       = 50 (hour 18)
//	Min       = 25 (hour 6)
//	HourOfMax = 18
//	Mean      = (25+50)/2 = 37.5
//	Range     = 50-25 = 25
//	StdDev    = |50-25|/2 = 12.5 (variance = 12.5^2 = 156.25, sqrt(156.25) = 12.5)
//	LoadFactor= 37.5/50 = 0.75
//
// D2, the vacation weekday, must land in "weekend" and "winter_weekend", not
// in "weekday" or "winter_weekday": its values (8, 16) appear nowhere in the
// "weekday" arithmetic above (which uses only D1, D4, D5), and "winter_weekend"
// aggregates exactly D2 and D3 — the same two days as "weekend", since no
// spring weekend date is in this fixture, so the two keys hold identical values.
func TestBuildAndStatsMatchHandComputedValuesAcrossDayTypesAndSeasons(t *testing.T) {
	cfg := loadprofile.Config{
		WeekendDays: map[time.Weekday]bool{time.Saturday: true, time.Sunday: true},
		Vacations:   []loadprofile.DateRange{{Start: date(2026, 2, 25), End: date(2026, 2, 25)}},
		Location:    ist,
	}

	d1 := date(2026, 2, 24) // Tuesday, winter weekday
	d2 := date(2026, 2, 25) // Wednesday, winter, on vacation -> weekend
	d3 := date(2026, 2, 28) // Saturday, winter weekend
	d4 := date(2026, 3, 2)  // Monday, spring weekday
	d5 := date(2026, 3, 4)  // Wednesday, spring weekday

	// Sanity-check the classification the hand arithmetic above assumes,
	// before trusting Build with it.
	require.Equal(t, loadprofile.Weekday, loadprofile.Classify(d1, cfg))
	require.Equal(t, loadprofile.Weekend, loadprofile.Classify(d2, cfg), "R70: vacation weekday classifies as weekend")
	require.Equal(t, loadprofile.Weekend, loadprofile.Classify(d3, cfg))
	require.Equal(t, loadprofile.Weekday, loadprofile.Classify(d4, cfg))
	require.Equal(t, loadprofile.Weekday, loadprofile.Classify(d5, cfg))
	require.Equal(t, loadprofile.Winter, loadprofile.SeasonOf(d1, ist))
	require.Equal(t, loadprofile.Spring, loadprofile.SeasonOf(d4, ist))
	require.Equal(t, loadprofile.Spring, loadprofile.SeasonOf(d5, ist))

	got := loadprofile.Build([]loadprofile.HourValue{
		{Day: d1, Hour: 6, Value: d("40")}, {Day: d1, Hour: 18, Value: d("80")},
		{Day: d2, Hour: 6, Value: d("8")}, {Day: d2, Hour: 18, Value: d("16")},
		{Day: d3, Hour: 6, Value: d("12")}, {Day: d3, Hour: 18, Value: d("24")},
		{Day: d4, Hour: 6, Value: d("20")}, {Day: d4, Hour: 18, Value: d("40")},
		{Day: d5, Hour: 6, Value: d("30")}, {Day: d5, Hour: 18, Value: d("60")},
	}, cfg)

	// --- weekday: D1, D4, D5 ---
	weekday := got["weekday"]
	require.Equal(t, 3, weekday.Days)
	require.Equal(t, "30", weekday.Hours[6].String())
	require.Equal(t, "60", weekday.Hours[18].String())
	for h := 0; h < 24; h++ {
		if h == 6 || h == 18 {
			continue
		}
		require.Nil(t, weekday.Hours[h], "hour %d was never reported", h)
	}
	weekdayStats := loadprofile.Stats(weekday)
	require.Equal(t, "60", weekdayStats.Max.String())
	require.Equal(t, "30", weekdayStats.Min.String())
	require.Equal(t, 18, *weekdayStats.HourOfMax)
	require.Equal(t, "45", weekdayStats.Mean.String())
	// PowWithPrecision's Newton-iteration square root is accurate to (well
	// beyond) the requested precision but not bit-for-bit exact even on a
	// perfect square, so the exact hand value 15 is compared at the
	// package's own DivisionScale-adjacent precision rather than by raw
	// string equality.
	require.Equal(t, "15.000000000000000", weekdayStats.StdDev.StringFixed(15))
	require.Equal(t, "30", weekdayStats.Range.String())
	require.True(t, weekdayStats.LoadFactor.Equal(d("0.75")), weekdayStats.LoadFactor.String())

	// --- weekend: D2, D3 ---
	weekend := got["weekend"]
	require.Equal(t, 2, weekend.Days)
	require.Equal(t, "10", weekend.Hours[6].String())
	require.Equal(t, "20", weekend.Hours[18].String())
	weekendStats := loadprofile.Stats(weekend)
	require.Equal(t, "20", weekendStats.Max.String())
	require.Equal(t, "10", weekendStats.Min.String())
	require.Equal(t, 18, *weekendStats.HourOfMax)
	require.Equal(t, "15", weekendStats.Mean.String())
	require.Equal(t, "5.000000000000000", weekendStats.StdDev.StringFixed(15))
	require.Equal(t, "10", weekendStats.Range.String())
	require.True(t, weekendStats.LoadFactor.Equal(d("0.75")), weekendStats.LoadFactor.String())

	// --- spring_weekday: D4, D5 ---
	springWeekday := got["spring_weekday"]
	require.Equal(t, 2, springWeekday.Days)
	require.Equal(t, "25", springWeekday.Hours[6].String())
	require.Equal(t, "50", springWeekday.Hours[18].String())
	springWeekdayStats := loadprofile.Stats(springWeekday)
	require.True(t, springWeekdayStats.Max.Equal(d("50")), springWeekdayStats.Max.String())
	require.True(t, springWeekdayStats.Min.Equal(d("25")), springWeekdayStats.Min.String())
	require.Equal(t, 18, *springWeekdayStats.HourOfMax)
	require.True(t, springWeekdayStats.Mean.Equal(d("37.5")), springWeekdayStats.Mean.String())
	require.Equal(t, "12.500000000000000", springWeekdayStats.StdDev.StringFixed(15))
	require.True(t, springWeekdayStats.Range.Equal(d("25")), springWeekdayStats.Range.String())
	require.True(t, springWeekdayStats.LoadFactor.Equal(d("0.75")), springWeekdayStats.LoadFactor.String())

	// The vacation weekday (D2) landed in weekend/winter_weekend, not in
	// weekday/winter_weekday: winter_weekend aggregates exactly D2 and D3 —
	// the same two days as "weekend" above, since this fixture has no
	// spring weekend date, so the two keys carry identical values.
	winterWeekend := got["winter_weekend"]
	require.Equal(t, 2, winterWeekend.Days)
	require.Equal(t, "10", winterWeekend.Hours[6].String())
	require.Equal(t, "20", winterWeekend.Hours[18].String())

	winterWeekday := got["winter_weekday"]
	require.Equal(t, 1, winterWeekday.Days, "only D1; D2 must not be counted here")
	require.Equal(t, "40", winterWeekday.Hours[6].String(), "D1's value alone, not averaged with D2's 8")
	require.Equal(t, "80", winterWeekday.Hours[18].String(), "D1's value alone, not averaged with D2's 16")
}
