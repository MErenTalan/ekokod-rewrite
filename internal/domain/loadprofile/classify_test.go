package loadprofile_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/loadprofile"
)

func TestClassifyUsesTheConfiguredWeekendDays(t *testing.T) {
	cfg := loadprofile.Config{WeekendDays: map[time.Weekday]bool{time.Friday: true}, Location: ist}
	require.Equal(t, loadprofile.Weekend, loadprofile.Classify(date(2025, 3, 14), cfg)) // a Friday
	require.Equal(t, loadprofile.Weekday, loadprofile.Classify(date(2025, 3, 15), cfg)) // a Saturday, but this company's weekend is Friday only
}

func TestClassifyTreatsAVacationDayAsWeekend(t *testing.T) {
	cfg := loadprofile.Config{
		WeekendDays: map[time.Weekday]bool{time.Saturday: true, time.Sunday: true},
		Vacations:   []loadprofile.DateRange{{Start: date(2025, 4, 23), End: date(2025, 4, 23)}},
		Location:    ist,
	}
	require.Equal(t, loadprofile.Weekend, loadprofile.Classify(date(2025, 4, 23), cfg), "a national holiday inside a vacation period")
	require.Equal(t, loadprofile.Weekday, loadprofile.Classify(date(2025, 4, 24), cfg))
}

func TestClassifyIncludesBothEndsOfAVacationRange(t *testing.T) {
	cfg := loadprofile.Config{
		WeekendDays: map[time.Weekday]bool{time.Saturday: true, time.Sunday: true},
		Vacations:   []loadprofile.DateRange{{Start: date(2025, 7, 1), End: date(2025, 7, 3)}},
		Location:    ist,
	}
	require.Equal(t, loadprofile.Weekend, loadprofile.Classify(date(2025, 7, 1), cfg), "range start is included")
	require.Equal(t, loadprofile.Weekend, loadprofile.Classify(date(2025, 7, 2), cfg), "range middle is included")
	require.Equal(t, loadprofile.Weekend, loadprofile.Classify(date(2025, 7, 3), cfg), "range end is included")
	require.Equal(t, loadprofile.Weekday, loadprofile.Classify(date(2025, 7, 4), cfg), "the day after the range end is not")
}

func TestSeasonOfUsesMeteorologicalMonths(t *testing.T) {
	require.Equal(t, loadprofile.Winter, loadprofile.SeasonOf(date(2025, 12, 1), ist))
	require.Equal(t, loadprofile.Winter, loadprofile.SeasonOf(date(2025, 2, 28), ist))
	require.Equal(t, loadprofile.Spring, loadprofile.SeasonOf(date(2025, 3, 1), ist), "R82: not the 21 March legacy cutoff")
	require.Equal(t, loadprofile.Spring, loadprofile.SeasonOf(date(2025, 5, 31), ist))
	require.Equal(t, loadprofile.Summer, loadprofile.SeasonOf(date(2025, 6, 1), ist))
	require.Equal(t, loadprofile.Summer, loadprofile.SeasonOf(date(2025, 8, 31), ist))
	require.Equal(t, loadprofile.Autumn, loadprofile.SeasonOf(date(2025, 9, 1), ist))
	require.Equal(t, loadprofile.Autumn, loadprofile.SeasonOf(date(2025, 11, 30), ist))
	require.Equal(t, loadprofile.Winter, loadprofile.SeasonOf(date(2025, 12, 21), ist), "R82: not the 21 December legacy cutoff")
}

func TestKeysForReturnsTheDayTypeAndTheSeasonDayTypeCombination(t *testing.T) {
	require.Equal(t, []loadprofile.Key{"weekday", "winter_weekday"}, loadprofile.KeysFor(loadprofile.Weekday, loadprofile.Winter))
	require.Equal(t, []loadprofile.Key{"weekend", "summer_weekend"}, loadprofile.KeysFor(loadprofile.Weekend, loadprofile.Summer))
}
