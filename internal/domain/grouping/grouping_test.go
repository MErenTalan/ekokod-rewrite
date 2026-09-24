package grouping_test

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/grouping"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/loadprofile"
)

var ist = mustLoad()

func mustLoad() *time.Location {
	loc, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		panic(err)
	}
	return loc
}

func d(v string) *decimal.Decimal {
	x := decimal.RequireFromString(v)
	return &x
}

func day(date string, active string) grouping.Day {
	t, err := time.ParseInLocation("2006-01-02", date, ist)
	if err != nil {
		panic(err)
	}
	return grouping.Day{Date: t, Active: d(active)}
}

func cfg(weekend []time.Weekday, vacations ...grouping.Range) loadprofile.Config {
	days := map[time.Weekday]bool{}
	for _, w := range weekend {
		days[w] = true
	}
	out := loadprofile.Config{WeekendDays: days, Location: ist}
	for _, v := range vacations {
		out.Vacations = append(out.Vacations, loadprofile.DateRange{Start: v.Start, End: v.End})
	}
	return out
}

func date(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02", s, ist)
	if err != nil {
		panic(err)
	}
	return t
}

var weekendSatSun = []time.Weekday{time.Saturday, time.Sunday}

func keysOf(buckets []grouping.Bucket) []string {
	out := make([]string, 0, len(buckets))
	for _, b := range buckets {
		out = append(out, b.Key)
	}
	return out
}

func TestGroupDaily(t *testing.T) {
	t.Parallel()
	got := grouping.Group([]grouping.Day{day("2026-03-09", "10"), day("2026-03-10", "20")}, grouping.Daily, cfg(weekendSatSun))
	require.Equal(t, []string{"2026-03-09", "2026-03-10"}, keysOf(got))
	require.Equal(t, "10", got[0].Active.String())
	require.Equal(t, 1, got[0].Days)
}

func TestGroupWeekStartsMonday(t *testing.T) {
	t.Parallel()
	// 8 March 2026 is a Sunday: it belongs to the week that started Monday 2 March.
	got := grouping.Group([]grouping.Day{day("2026-03-08", "10"), day("2026-03-09", "20")}, grouping.Week, cfg(weekendSatSun))
	require.Equal(t, []string{"2026-03-02", "2026-03-09"}, keysOf(got))
	require.Equal(t, "10", got[0].Active.String())
	require.Equal(t, "20", got[1].Active.String())
}

func TestGroupDayTypeUsesCompanyConfig(t *testing.T) {
	t.Parallel()
	// Weekend days Friday+Saturday, and Wednesday 11 March is a vacation day (R137).
	c := cfg([]time.Weekday{time.Friday, time.Saturday}, grouping.Range{Start: date("2026-03-11"), End: date("2026-03-11")})
	got := grouping.Group([]grouping.Day{
		day("2026-03-11", "5"),  // Wednesday, vacation → weekend
		day("2026-03-12", "7"),  // Thursday → weekday
		day("2026-03-13", "11"), // Friday → weekend
		day("2026-03-15", "13"), // Sunday → weekday (not configured as a weekend day)
	}, grouping.DayType, c)
	require.Equal(t, []string{"weekday", "weekend"}, keysOf(got))
	require.Equal(t, "20", got[0].Active.String())
	require.Equal(t, 2, got[0].Days)
	require.Equal(t, "16", got[1].Active.String())
	require.Equal(t, 2, got[1].Days)
}

func TestGroupSeasonDecemberStartsWinter(t *testing.T) {
	t.Parallel()
	got := grouping.Group([]grouping.Day{
		day("2026-01-15", "10"), day("2025-12-15", "20"), day("2026-03-01", "30"),
	}, grouping.Season, cfg(weekendSatSun))
	require.Equal(t, []string{"winter-2025", "spring-2026"}, keysOf(got), "December starts the winter of its own year")
	require.Equal(t, "30", got[0].Active.String())
	require.Equal(t, 2, got[0].Days)
}

func TestGroupSeasonDayTypeKeys(t *testing.T) {
	t.Parallel()
	got := grouping.Group([]grouping.Day{
		day("2026-07-04", "10"), // Saturday
		day("2026-07-06", "20"), // Monday
	}, grouping.SeasonDayType, cfg(weekendSatSun))
	require.Equal(t, []string{"summer-2026-weekday", "summer-2026-weekend"}, keysOf(got))
}

func TestGroupSumsNonNilAndMarksPartial(t *testing.T) {
	t.Parallel()
	a := day("2026-03-09", "10")
	a.Inductive, a.Capacitive = d("2"), d("1")
	b := day("2026-03-10", "20")
	b.Capacitive = d("3")
	got := grouping.Group([]grouping.Day{a, b}, grouping.Week, cfg(weekendSatSun))
	require.Len(t, got, 1)
	require.Equal(t, "30", got[0].Active.String())
	require.Equal(t, "2", got[0].Inductive.String(), "a nil day contributes nothing, it does not zero the sum")
	require.Equal(t, "4", got[0].Capacitive.String())
	require.True(t, got[0].Partial, "a missing register makes the group partial")

	full := day("2026-03-09", "10")
	full.Inductive, full.Capacitive = d("2"), d("1")
	clean := grouping.Group([]grouping.Day{full}, grouping.Daily, cfg(weekendSatSun))
	require.False(t, clean[0].Partial, "a day with every register is complete")

	flagged := day("2026-03-09", "10")
	flagged.Partial = true
	flagged.Inductive, flagged.Capacitive = d("1"), d("1")
	require.True(t, grouping.Group([]grouping.Day{flagged}, grouping.Daily, cfg(weekendSatSun))[0].Partial)

	empty := grouping.Group([]grouping.Day{{Date: date("2026-03-09")}}, grouping.Daily, cfg(weekendSatSun))
	require.Nil(t, empty[0].Active, "no value at all stays nil")
}

func TestStatsOverBuckets(t *testing.T) {
	t.Parallel()
	buckets := grouping.Group([]grouping.Day{
		day("2026-03-02", "1"), day("2026-03-09", "1"), day("2026-03-16", "2"),
	}, grouping.Week, cfg(weekendSatSun))
	stats := grouping.Stats(buckets)
	require.Equal(t, "4", stats.Total.String())
	require.Equal(t, "1.333", stats.Average.String(), "three groups with a value, rounded to 3 dp")
	require.Equal(t, "2026-03-16", stats.Peak.Key)
	require.Equal(t, "2", stats.Peak.Value.String())
	require.Equal(t, "2026-03-02", stats.Valley.Key, "the first group wins a tie")
	require.Equal(t, "1", stats.Valley.Value.String())

	none := grouping.Stats(grouping.Group([]grouping.Day{{Date: date("2026-03-09")}}, grouping.Daily, cfg(weekendSatSun)))
	require.Nil(t, none.Total)
	require.Nil(t, none.Average)
	require.Nil(t, none.Peak)
	require.Nil(t, none.Valley)
}

func TestValidRejectsUnknownGrouping(t *testing.T) {
	t.Parallel()
	for _, by := range []grouping.By{grouping.Daily, grouping.Week, grouping.DayType, grouping.Season, grouping.SeasonDayType} {
		require.True(t, grouping.Valid(by), by)
	}
	require.False(t, grouping.Valid("monthly"))
	require.False(t, grouping.Valid(""))
}
