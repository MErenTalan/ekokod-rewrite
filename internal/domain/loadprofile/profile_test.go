package loadprofile_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/loadprofile"
)

func TestBuildAveragesEachHourAcrossMatchingDays(t *testing.T) {
	// Two weekdays: hour 9 = 10 and 20 -> mean 15. Hour 10 reported once -> 7.
	got := loadprofile.Build([]loadprofile.HourValue{
		{Day: date(2025, 3, 10), Hour: 9, Value: d("10")},
		{Day: date(2025, 3, 11), Hour: 9, Value: d("20")},
		{Day: date(2025, 3, 11), Hour: 10, Value: d("7")},
	}, weekdayCfg)
	p := got["weekday"]
	require.Equal(t, "15", p.Hours[9].String())
	require.Equal(t, "7", p.Hours[10].String())
	require.Nil(t, p.Hours[0], "an hour no day reported stays nil")
	require.Equal(t, 2, p.Days)
}

func TestBuildPutsADayInBothItsDayTypeAndItsSeasonProfile(t *testing.T) {
	got := loadprofile.Build([]loadprofile.HourValue{{Day: date(2025, 1, 8), Hour: 3, Value: d("5")}}, weekdayCfg)
	require.Contains(t, got, loadprofile.Key("weekday"))
	require.Contains(t, got, loadprofile.Key("winter_weekday"))
	require.NotContains(t, got, loadprofile.Key("weekend"))
}

func TestBuildTreatsAReportedZeroAsAValueNotAnAbsence(t *testing.T) {
	// R68: an idle building that reported zero is decimal.Zero, distinct from
	// an hour nobody reported at all, which stays nil.
	got := loadprofile.Build([]loadprofile.HourValue{
		{Day: date(2025, 3, 10), Hour: 4, Value: d("0")},
	}, weekdayCfg)
	p := got["weekday"]
	require.NotNil(t, p.Hours[4])
	require.True(t, p.Hours[4].IsZero())
	require.Nil(t, p.Hours[5])
}

func TestBuildKeepsProfilesSeparateAcrossDayTypes(t *testing.T) {
	// A weekday and a weekend day reporting the same hour must not mix into
	// one average.
	got := loadprofile.Build([]loadprofile.HourValue{
		{Day: date(2025, 3, 10), Hour: 9, Value: d("10")},  // Monday: weekday
		{Day: date(2025, 3, 15), Hour: 9, Value: d("100")}, // Saturday: weekend
	}, weekdayCfg)
	require.Equal(t, "10", got["weekday"].Hours[9].String())
	require.Equal(t, "100", got["weekend"].Hours[9].String())
}
