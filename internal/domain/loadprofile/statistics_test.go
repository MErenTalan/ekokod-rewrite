package loadprofile_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/loadprofile"
)

// TestStatsMatchHandComputedValues pins Stats against arithmetic worked out
// by hand, not against the implementation under test.
//
// hours 0..3 = 10, 20, 30, 40; the other 20 hours are nil.
// mean = 25, max = 40 (hour 3), min = 10, range = 30,
// population variance = ((15^2)+(5^2)+(5^2)+(15^2))/4 = 125, stddev = 11.1803398874989484820...
// load factor = 25/40 = 0.625
func TestStatsMatchHandComputedValues(t *testing.T) {
	s := loadprofile.Stats(profileWith(map[int]string{0: "10", 1: "20", 2: "30", 3: "40"}))
	require.Equal(t, "25", s.Mean.String())
	require.Equal(t, "40", s.Max.String())
	require.Equal(t, "10", s.Min.String())
	require.Equal(t, 3, *s.HourOfMax)
	require.Equal(t, "30", s.Range.String())
	require.True(t, s.StdDev.StringFixed(10) == "11.1803398875", s.StdDev.String())
	require.Equal(t, "0.625", s.LoadFactor.String())
}

func TestStatsLoadFactorIsZeroWhenTheProfileIsFlatZero(t *testing.T) {
	s := loadprofile.Stats(profileWith(map[int]string{0: "0", 1: "0"}))
	require.True(t, s.LoadFactor.IsZero(), "R81: §10.3 says 0 when max = 0, unlike R54's ratios")
}

func TestStatsHourOfMaxIsTheLowestHourOnATie(t *testing.T) {
	s := loadprofile.Stats(profileWith(map[int]string{0: "5", 5: "5"}))
	require.Equal(t, 0, *s.HourOfMax, "deterministic under ties: the lowest hour wins")
}

func TestStatsIsAllNilWhenTheProfileHasNoHours(t *testing.T) {
	s := loadprofile.Stats(loadprofile.Profile{Key: "empty"})
	require.Nil(t, s.Max)
	require.Nil(t, s.Min)
	require.Nil(t, s.HourOfMax)
	require.Nil(t, s.Mean)
	require.Nil(t, s.StdDev)
	require.Nil(t, s.Range)
	require.Nil(t, s.LoadFactor)
}
