//go:build integration

// This file is the acceptance suite for 09-implementation-plan.md §F3: one
// test per acceptance criterion, named after it, with a comment quoting the
// criterion verbatim. It lives in-package, next to the code it exercises.
//
// Wiring note: loadprofile.Service is NOT constructed by worker.Build — its
// only consumer through F5 is the HTTP layer, which does not exist until F6
// — so the plan's "drive worker.Build" rule cannot apply here. Both tests
// below instead construct *Service directly with REAL repositories
// (postgres.NewCalendarRepository, postgres.NewAnalyticsRepository,
// postgres.NewReadingRepository) over testfixtures.NewIsolatedDB and a real
// tenant from testfixtures.NewTenant — exactly the pattern
// service_integration_test.go already uses for this package's other
// real-database tests (newTestService, istanbulAt, decPtr and istanbul below
// are that file's shared helpers, reused here rather than redefined).
package loadprofile_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	domainlp "github.com/MErenTalan/ekokod-rewrite/internal/domain/loadprofile"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/loadprofile"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

// f3Reading builds one load_profile meter reading at ts with active_import
// = v, for this file's own fixtures (f3-prefixed to avoid colliding with
// another file's identically-scoped helper, per helpers_test.go).
func f3Reading(analyzerID uuid.UUID, ts time.Time, v string) model.MeterReading {
	return model.MeterReading{
		AnalyzerID: analyzerID, Ts: ts, Kind: model.ReadingKindLoadProfile,
		ActiveImport:      decPtr(v),
		MultiplierApplied: decimal.RequireFromString("1"),
		SourceProvider:    model.IntegrationProviderOSOS, IngestedAt: time.Now().UTC(),
	}
}

// TestF3LoadProfileStatisticsMatchHandComputedValues is 09 §F3's criterion:
// "Load profile statistics match hand-computed values for a fixture with
// known weekday/weekend and seasonal distribution."
//
// Six fixture days, three hourly readings each (hours 0,1,2 -> two R87
// consecutive-start deltas, hour 0 and hour 1):
//
//	Mon 2026-01-05 (winter, weekday): 1000,1004,1012 -> h0=4,  h1=8
//	Tue 2026-01-06 (winter, weekday): 2000,2008,2020 -> h0=8,  h1=12
//	Mon 2026-07-06 (summer, weekday): 5000,5008,5012 -> h0=8,  h1=4
//	Tue 2026-07-07 (summer, weekday): 6000,6012,6020 -> h0=12, h1=8
//	Sat 2026-01-10 (winter, weekend): 3000,3009,3021 -> h0=9,  h1=12
//	Sat 2026-07-11 (summer, weekend): 4000,4006,4022 -> h0=6,  h1=16
//
// (Jan 5 is Monday and Jan 10 is Saturday per service_integration_test.go's
// own dated comment; 2026-07-06/07/11 are each exactly 26 weeks -- 182 days,
// 182 mod 7 == 0 -- after 2026-01-05/06/10, so they land on the SAME
// weekdays: Monday, Tuesday and Saturday respectively.)
//
// Hand-computed per-profile hour-0/hour-1 means (h0, h1):
//
//	winter_weekday = (4+8)/2, (8+12)/2       = 6, 10
//	summer_weekday = (8+12)/2, (4+8)/2       = 10, 6
//	weekday        = (4+8+8+12)/4, (8+12+4+8)/4 = 8, 8   (the two seasons cancel out)
//	winter_weekend = 9, 12   (one day)
//	summer_weekend = 6, 16   (one day)
//	weekend        = (9+6)/2, (12+16)/2      = 7.5, 14
//
// Hand-computed statistics (population stddev, divisor n = 2 non-nil hours
// -- or n = 1 where only one day contributed):
//
//	winter_weekday: Mean=8, Max=10@h1, Min=6@h0, Range=4,
//	                Var=((6-8)^2+(10-8)^2)/2=4, StdDev=2, LoadFactor=8/10=0.8
//	summer_weekday: Mean=8, Max=10@h0, Min=6@h1, Range=4,
//	                Var=4, StdDev=2, LoadFactor=8/10=0.8
//	weekday:        Mean=8, Max=8, Min=8, Range=0, Var=0, StdDev=0,
//	                LoadFactor=8/8=1, HourOfMax=0 (flat: the two seasons'
//	                opposite hour-0/hour-1 skew cancels in the combined mean)
//	winter_weekend: Mean=10.5, Max=12@h1, Min=9@h0, Range=3,
//	                Var=((9-10.5)^2+(12-10.5)^2)/2=2.25, StdDev=1.5,
//	                LoadFactor=10.5/12=0.875
//	summer_weekend: Mean=11, Max=16@h1, Min=6@h0, Range=10,
//	                Var=((6-11)^2+(16-11)^2)/2=25, StdDev=5,
//	                LoadFactor=11/16=0.6875
func TestF3LoadProfileStatisticsMatchHandComputedValues(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)

	calendar := postgres.NewCalendarRepository(pool)
	analytics := postgres.NewAnalyticsRepository(pool)
	readings := postgres.NewReadingRepository(pool)
	svc := newTestService(t, calendar, analytics)

	analyzerID := tenant.Analyzers[0].ID

	type day struct {
		y, mo, d int
		mon      time.Month
		regs     [3]string
	}
	fixture := []day{
		{2026, 1, 5, time.January, [3]string{"1000", "1004", "1012"}},  // winter weekday
		{2026, 1, 6, time.January, [3]string{"2000", "2008", "2020"}},  // winter weekday
		{2026, 7, 6, time.July, [3]string{"5000", "5008", "5012"}},     // summer weekday
		{2026, 7, 7, time.July, [3]string{"6000", "6012", "6020"}},     // summer weekday
		{2026, 1, 10, time.January, [3]string{"3000", "3009", "3021"}}, // winter weekend
		{2026, 7, 11, time.July, [3]string{"4000", "4006", "4022"}},    // summer weekend
	}
	var rows []model.MeterReading
	for _, d := range fixture {
		for h, v := range d.regs {
			rows = append(rows, f3Reading(analyzerID, istanbulAt(d.y, d.mon, d.d, h), v))
		}
	}
	_, _, err := readings.BulkInsert(ctx, tenant.Scope, rows)
	require.NoError(t, err)

	req := loadprofile.Request{
		AnalyzerIDs: []uuid.UUID{analyzerID},
		Range:       store.TimeRange{From: istanbulAt(2026, time.January, 1, 0), To: istanbulAt(2026, time.August, 1, 0)},
	}
	res, err := svc.Profiles(ctx, tenant.Scope, req)
	require.NoError(t, err)

	check := func(key string, days int, h0, h1 string) {
		t.Helper()
		p := res.Profiles[domainlp.Key(key)]
		require.Equal(t, days, p.Days, "%s: Days", key)
		require.NotNil(t, p.Hours[0], "%s: hour 0", key)
		require.NotNil(t, p.Hours[1], "%s: hour 1", key)
		require.True(t, decimal.RequireFromString(h0).Equal(*p.Hours[0]), "%s hour0: got %s want %s", key, p.Hours[0], h0)
		require.True(t, decimal.RequireFromString(h1).Equal(*p.Hours[1]), "%s hour1: got %s want %s", key, p.Hours[1], h1)
	}
	check("winter_weekday", 2, "6", "10")
	check("summer_weekday", 2, "10", "6")
	check("weekday", 4, "8", "8")
	check("winter_weekend", 1, "9", "12")
	check("summer_weekend", 1, "6", "16")
	check("weekend", 2, "7.5", "14")

	// stdDevTolerance accounts for PowWithPrecision's iterative square root
	// (Newton's method to DivisionScale): even a perfect square's root can
	// land a handful of units below DivisionScale's last digit rather than
	// bit-exact (observed: 5.00000000000000000000000000000001 for
	// sqrt(25)) — the same reason TestStatisticsMatchTheHandComputedFixture
	// above compares its own (irrational) StdDev with a tolerance rather
	// than .Equal. Every OTHER stat here is compared exactly: only a square
	// root goes through an iterative approximation.
	stdDevTolerance := decimal.RequireFromString("0.0000000000000000001")

	checkStats := func(key string, mean, max, min, rng, stdDev, loadFactor string, hourOfMax int) {
		t.Helper()
		s := res.Statistics[domainlp.Key(key)]
		require.NotNil(t, s.Mean, "%s: Mean", key)
		require.NotNil(t, s.Max, "%s: Max", key)
		require.NotNil(t, s.Min, "%s: Min", key)
		require.NotNil(t, s.Range, "%s: Range", key)
		require.NotNil(t, s.StdDev, "%s: StdDev", key)
		require.NotNil(t, s.LoadFactor, "%s: LoadFactor", key)
		require.NotNil(t, s.HourOfMax, "%s: HourOfMax", key)
		require.True(t, decimal.RequireFromString(mean).Equal(*s.Mean), "%s Mean: got %s want %s", key, s.Mean, mean)
		require.True(t, decimal.RequireFromString(max).Equal(*s.Max), "%s Max: got %s want %s", key, s.Max, max)
		require.True(t, decimal.RequireFromString(min).Equal(*s.Min), "%s Min: got %s want %s", key, s.Min, min)
		require.True(t, decimal.RequireFromString(rng).Equal(*s.Range), "%s Range: got %s want %s", key, s.Range, rng)
		wantStdDev := decimal.RequireFromString(stdDev)
		require.True(t, s.StdDev.Sub(wantStdDev).Abs().LessThanOrEqual(stdDevTolerance),
			"%s StdDev: got %s want approximately %s", key, s.StdDev, stdDev)
		require.True(t, decimal.RequireFromString(loadFactor).Equal(*s.LoadFactor), "%s LoadFactor: got %s want %s", key, s.LoadFactor, loadFactor)
		require.Equal(t, hourOfMax, *s.HourOfMax, "%s: HourOfMax", key)
	}
	checkStats("winter_weekday", "8", "10", "6", "4", "2", "0.8", 1)
	checkStats("summer_weekday", "8", "10", "6", "4", "2", "0.8", 0)
	checkStats("weekday", "8", "8", "8", "0", "0", "1", 0)
	checkStats("winter_weekend", "10.5", "12", "9", "3", "1.5", "0.875", 1)
	checkStats("summer_weekend", "11", "16", "6", "10", "5", "0.6875", 1)
}

// TestF3VacationMovesADayToTheWeekendProfile is 09 §F3's criterion:
// "Vacation days configured on the company move a date from weekday to
// weekend in the profile split."
//
// Week of 2026-02-02 (Mon) .. 2026-02-08 (Sun): 5 weekdays, 2 weekend days
// under the default {Sat, Sun} classification. Each weekday's hour-0 delta
// is distinct so removing one from the weekday mean is detectable:
//
//	Mon Feb 2: 1000 -> 1009 (delta 9)
//	Tue Feb 3: 2000 -> 2011 (delta 11)   <- vacationed after the first read
//	Wed Feb 4: 3000 -> 3013 (delta 13)
//	Thu Feb 5: 4000 -> 4015 (delta 15)
//	Fri Feb 6: 5000 -> 5017 (delta 17)
//	Sat Feb 7: 6000 -> 6019 (delta 19)
//	Sun Feb 8: 7000 -> 7021 (delta 21)
//
// Before the vacation: weekday Days=5, weekday hour-0 mean =
// (9+11+13+15+17)/5 = 65/5 = 13. After registering Feb 3 as a company
// vacation (CalendarRepository.CreateVacation, R70's only classification
// input besides company_weekend_days): weekday Days=4, weekend Days=3,
// weekday hour-0 mean = (9+13+15+17)/4 = 54/4 = 13.5 -- both the day count
// AND the mean move, hand-computed, never merely "changed".
func TestF3VacationMovesADayToTheWeekendProfile(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)

	calendar := postgres.NewCalendarRepository(pool)
	analytics := postgres.NewAnalyticsRepository(pool)
	readings := postgres.NewReadingRepository(pool)
	svc := newTestService(t, calendar, analytics)

	analyzerID := tenant.Analyzers[0].ID

	days := []struct {
		date int
		v0   string
		v1   string
	}{
		{2, "1000", "1009"},
		{3, "2000", "2011"},
		{4, "3000", "3013"},
		{5, "4000", "4015"},
		{6, "5000", "5017"},
		{7, "6000", "6019"},
		{8, "7000", "7021"},
	}
	var rows []model.MeterReading
	for _, d := range days {
		rows = append(rows,
			f3Reading(analyzerID, istanbulAt(2026, time.February, d.date, 0), d.v0),
			f3Reading(analyzerID, istanbulAt(2026, time.February, d.date, 1), d.v1),
		)
	}
	_, _, err := readings.BulkInsert(ctx, tenant.Scope, rows)
	require.NoError(t, err)

	req := loadprofile.Request{
		AnalyzerIDs: []uuid.UUID{analyzerID},
		Range:       store.TimeRange{From: istanbulAt(2026, time.February, 2, 0), To: istanbulAt(2026, time.February, 9, 0)},
	}

	before, err := svc.Profiles(ctx, tenant.Scope, req)
	require.NoError(t, err)
	require.Equal(t, 5, before.Profiles["weekday"].Days)
	require.Equal(t, 2, before.Profiles["weekend"].Days)
	require.NotNil(t, before.Profiles["weekday"].Hours[0])
	require.True(t, decimal.RequireFromString("13").Equal(*before.Profiles["weekday"].Hours[0]),
		"before: got %s want 13", before.Profiles["weekday"].Hours[0])

	_, err = calendar.CreateVacation(ctx, tenant.Scope, model.CompanyVacation{
		ID: uuid.New(), CompanyID: tenant.Company.ID,
		StartDate: istanbulAt(2026, time.February, 3, 0),
		EndDate:   istanbulAt(2026, time.February, 3, 0),
	})
	require.NoError(t, err)

	after, err := svc.Profiles(ctx, tenant.Scope, req)
	require.NoError(t, err)
	require.Equal(t, 4, after.Profiles["weekday"].Days, "Tuesday moved out of weekday")
	require.Equal(t, 3, after.Profiles["weekend"].Days, "Tuesday moved into weekend")
	require.NotNil(t, after.Profiles["weekday"].Hours[0])
	require.True(t, decimal.RequireFromString("13.5").Equal(*after.Profiles["weekday"].Hours[0]),
		"after: got %s want 13.5", after.Profiles["weekday"].Hours[0])
}
