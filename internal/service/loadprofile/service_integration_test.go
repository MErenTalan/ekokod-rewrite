//go:build integration

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

var istanbul = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		panic(err)
	}
	return loc
}()

// istanbulAt builds a UTC time.Time naming the given Istanbul-local instant
// (Turkey has used a fixed +03:00 offset with no DST since 2016, so every
// fixture date here is unambiguous).
func istanbulAt(y int, m time.Month, d, hour int) time.Time {
	return time.Date(y, m, d, hour, 0, 0, 0, istanbul)
}

func newTestService(t *testing.T, calendar store.CalendarRepository, hourly loadprofile.HourlySource) *loadprofile.Service {
	t.Helper()
	svc, err := loadprofile.New(loadprofile.Deps{
		Calendar: calendar,
		Hourly:   hourly,
		Location: istanbul,
		Log:      testfixtures.DiscardLogger(),
	})
	require.NoError(t, err)
	return svc
}

func TestAVacationMovesADayIntoTheWeekendProfile(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)

	calendar := postgres.NewCalendarRepository(pool)
	analytics := postgres.NewAnalyticsRepository(pool)
	readings := postgres.NewReadingRepository(pool)
	svc := newTestService(t, calendar, analytics)

	analyzerID := tenant.Analyzers[0].ID

	// Week 2026-01-05 (Mon) .. 2026-01-11 (Sun): 5 weekdays, 2 weekend days
	// under the default {Sat, Sun} classification. One hour (0) is reported
	// per day, needing readings at hour 0 and hour 1 (R87's one-hour
	// look-ahead). Each day's hour-0 value is distinct so a day dropping out
	// of "weekday" changes its mean.
	days := []struct {
		date int
		v0   string
		v1   string
	}{
		{5, "1000.0000", "1006.0000"},  // Mon, delta 6
		{6, "2000.0000", "2010.0000"},  // Tue, delta 10
		{7, "3000.0000", "3005.0000"},  // Wed, delta 5 (this one is vacationed)
		{8, "4000.0000", "4008.0000"},  // Thu, delta 8
		{9, "5000.0000", "5009.0000"},  // Fri, delta 9
		{10, "6000.0000", "6004.0000"}, // Sat, delta 4
		{11, "7000.0000", "7007.0000"}, // Sun, delta 7
	}
	var rows []model.MeterReading
	for _, d := range days {
		rows = append(rows,
			model.MeterReading{
				AnalyzerID: analyzerID, Ts: istanbulAt(2026, time.January, d.date, 0).UTC(),
				Kind: model.ReadingKindLoadProfile, ActiveImport: decPtr(d.v0),
				MultiplierApplied: decimal.RequireFromString("1"),
				SourceProvider:    model.IntegrationProviderOSOS, IngestedAt: time.Now().UTC(),
			},
			model.MeterReading{
				AnalyzerID: analyzerID, Ts: istanbulAt(2026, time.January, d.date, 1).UTC(),
				Kind: model.ReadingKindLoadProfile, ActiveImport: decPtr(d.v1),
				MultiplierApplied: decimal.RequireFromString("1"),
				SourceProvider:    model.IntegrationProviderOSOS, IngestedAt: time.Now().UTC(),
			},
		)
	}
	_, _, err := readings.BulkInsert(ctx, tenant.Scope, rows)
	require.NoError(t, err)

	req := loadprofile.Request{
		AnalyzerIDs: []uuid.UUID{analyzerID},
		Range:       store.TimeRange{From: istanbulAt(2026, time.January, 5, 0), To: istanbulAt(2026, time.January, 12, 0)},
	}

	before, err := svc.Profiles(ctx, tenant.Scope, req)
	require.NoError(t, err)
	require.Equal(t, 5, before.Profiles["weekday"].Days)
	require.Equal(t, 2, before.Profiles["weekend"].Days)

	_, err = calendar.CreateVacation(ctx, tenant.Scope, model.CompanyVacation{
		ID: uuid.New(), CompanyID: tenant.Company.ID,
		StartDate: istanbulAt(2026, time.January, 7, 0),
		EndDate:   istanbulAt(2026, time.January, 7, 0),
	})
	require.NoError(t, err)

	after, err := svc.Profiles(ctx, tenant.Scope, req)
	require.NoError(t, err)
	require.Equal(t, 4, after.Profiles["weekday"].Days)
	require.Equal(t, 3, after.Profiles["weekend"].Days)
	require.NotEqual(t,
		before.Statistics["weekday"].Mean.String(),
		after.Statistics["weekday"].Mean.String(),
		"removing Wednesday's delta (5) from the weekday mean must change it")
}

func TestACompanyWithNoConfiguredWeekendDaysGetsSaturdayAndSunday(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)

	calendar := postgres.NewCalendarRepository(pool)
	analytics := postgres.NewAnalyticsRepository(pool)
	readings := postgres.NewReadingRepository(pool)
	svc := newTestService(t, calendar, analytics)

	analyzerID := tenant.Analyzers[0].ID
	// 2026-01-10 is a Saturday (see the week table above).
	rows := []model.MeterReading{
		{AnalyzerID: analyzerID, Ts: istanbulAt(2026, time.January, 10, 0).UTC(),
			Kind: model.ReadingKindLoadProfile, ActiveImport: decPtr("1000.0000"),
			MultiplierApplied: decimal.RequireFromString("1"),
			SourceProvider:    model.IntegrationProviderOSOS, IngestedAt: time.Now().UTC()},
		{AnalyzerID: analyzerID, Ts: istanbulAt(2026, time.January, 10, 1).UTC(),
			Kind: model.ReadingKindLoadProfile, ActiveImport: decPtr("1005.0000"),
			MultiplierApplied: decimal.RequireFromString("1"),
			SourceProvider:    model.IntegrationProviderOSOS, IngestedAt: time.Now().UTC()},
	}
	_, _, err := readings.BulkInsert(ctx, tenant.Scope, rows)
	require.NoError(t, err)

	req := loadprofile.Request{
		AnalyzerIDs: []uuid.UUID{analyzerID},
		Range:       store.TimeRange{From: istanbulAt(2026, time.January, 10, 0), To: istanbulAt(2026, time.January, 11, 0)},
	}
	res, err := svc.Profiles(ctx, tenant.Scope, req)
	require.NoError(t, err)
	require.Equal(t, "default", res.Config.WeekendSource)
	require.Positive(t, res.Profiles["weekend"].Days)
	require.ElementsMatch(t, []time.Weekday{time.Saturday, time.Sunday}, res.Config.WeekendDays)

	// R69's copy discipline: nothing reachable from a Result can move the
	// package default.
	require.Equal(t, map[time.Weekday]bool{time.Saturday: true, time.Sunday: true}, loadprofile.DefaultWeekendDays)
}

// PROVISIONAL: revisit when F6 rules on Q4 (R70's guard test name and this
// comment both flag it — F6 line 434's acceptance wording implies a
// calendar-event-driven split that this test currently forbids).
func TestACalendarEventDoesNotChangeTheSplitUnderR70(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)

	calendar := postgres.NewCalendarRepository(pool)
	analytics := postgres.NewAnalyticsRepository(pool)
	readings := postgres.NewReadingRepository(pool)
	svc := newTestService(t, calendar, analytics)

	analyzerID := tenant.Analyzers[0].ID
	// 2026-01-08 is a Thursday: a weekday under the default calendar.
	rows := []model.MeterReading{
		{AnalyzerID: analyzerID, Ts: istanbulAt(2026, time.January, 8, 0).UTC(),
			Kind: model.ReadingKindLoadProfile, ActiveImport: decPtr("1000.0000"),
			MultiplierApplied: decimal.RequireFromString("1"),
			SourceProvider:    model.IntegrationProviderOSOS, IngestedAt: time.Now().UTC()},
		{AnalyzerID: analyzerID, Ts: istanbulAt(2026, time.January, 8, 1).UTC(),
			Kind: model.ReadingKindLoadProfile, ActiveImport: decPtr("1005.0000"),
			MultiplierApplied: decimal.RequireFromString("1"),
			SourceProvider:    model.IntegrationProviderOSOS, IngestedAt: time.Now().UTC()},
	}
	_, _, err := readings.BulkInsert(ctx, tenant.Scope, rows)
	require.NoError(t, err)

	req := loadprofile.Request{
		AnalyzerIDs: []uuid.UUID{analyzerID},
		Range:       store.TimeRange{From: istanbulAt(2026, time.January, 8, 0), To: istanbulAt(2026, time.January, 9, 0)},
	}

	before, err := svc.Profiles(ctx, tenant.Scope, req)
	require.NoError(t, err)
	require.Equal(t, 1, before.Profiles["weekday"].Days)
	require.Equal(t, 0, before.Profiles["weekend"].Days)

	adminUser := tenant.Users[model.UserRoleCompanyAdmin].ID
	_, err = calendar.CreateEvent(ctx, tenant.Scope, model.CalendarEvent{
		ID: uuid.New(), CompanyID: tenant.Company.ID, Title: "Company holiday (event, not a vacation)",
		StartsAt: istanbulAt(2026, time.January, 8, 0), EndsAt: istanbulAt(2026, time.January, 9, 0),
		AllDay: true, CreatedBy: &adminUser,
	})
	require.NoError(t, err)

	after, err := svc.Profiles(ctx, tenant.Scope, req)
	require.NoError(t, err)
	require.Equal(t, 1, after.Profiles["weekday"].Days, "a calendar_events row must never move a day into the weekend profile (R70)")
	require.Equal(t, 0, after.Profiles["weekend"].Days)
}

func TestProfilesAreTenantIsolated(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenantA := testfixtures.NewTenant(t, ctx, pool, 1)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 2)

	calendar := postgres.NewCalendarRepository(pool)
	analytics := postgres.NewAnalyticsRepository(pool)
	readings := postgres.NewReadingRepository(pool)
	svc := newTestService(t, calendar, analytics)

	analyzerA := tenantA.Analyzers[0].ID
	// Same week as the vacation test: 5 weekdays, 2 weekend days.
	days := []int{5, 6, 7, 8, 9, 10, 11}
	var rows []model.MeterReading
	for i, d := range days {
		base := decimal.NewFromInt(int64(1000 * (i + 1)))
		rows = append(rows,
			model.MeterReading{
				AnalyzerID: analyzerA, Ts: istanbulAt(2026, time.January, d, 0).UTC(),
				Kind: model.ReadingKindLoadProfile, ActiveImport: decPtrD(base),
				MultiplierApplied: decimal.RequireFromString("1"),
				SourceProvider:    model.IntegrationProviderOSOS, IngestedAt: time.Now().UTC(),
			},
			model.MeterReading{
				AnalyzerID: analyzerA, Ts: istanbulAt(2026, time.January, d, 1).UTC(),
				Kind: model.ReadingKindLoadProfile, ActiveImport: decPtrD(base.Add(decimal.NewFromInt(5))),
				MultiplierApplied: decimal.RequireFromString("1"),
				SourceProvider:    model.IntegrationProviderOSOS, IngestedAt: time.Now().UTC(),
			},
		)
	}
	_, _, err := readings.BulkInsert(ctx, tenantA.Scope, rows)
	require.NoError(t, err)

	req := loadprofile.Request{
		AnalyzerIDs: []uuid.UUID{analyzerA},
		Range:       store.TimeRange{From: istanbulAt(2026, time.January, 5, 0), To: istanbulAt(2026, time.January, 12, 0)},
	}

	// Positive control: tenant A's own AdminScope sees the data.
	before, err := svc.Profiles(ctx, tenantA.AdminScope, req)
	require.NoError(t, err)
	require.Equal(t, 5, before.Profiles["weekday"].Days)
	require.Equal(t, 2, before.Profiles["weekend"].Days)

	// Tenant B creates a vacation covering the SAME calendar date
	// (2026-01-07, a Wednesday) in tenant B's own company.
	_, err = calendar.CreateVacation(ctx, tenantB.AdminScope, model.CompanyVacation{
		ID: uuid.New(), CompanyID: tenantB.Company.ID,
		StartDate: istanbulAt(2026, time.January, 7, 0),
		EndDate:   istanbulAt(2026, time.January, 7, 0),
	})
	require.NoError(t, err)

	// Tenant A's own split must be unaffected by tenant B's vacation.
	afterA, err := svc.Profiles(ctx, tenantA.AdminScope, req)
	require.NoError(t, err)
	require.Equal(t, 5, afterA.Profiles["weekday"].Days, "tenant B's vacation must not move tenant A's split")
	require.Equal(t, 2, afterA.Profiles["weekend"].Days)

	// Tenant A's analyzer id, requested under tenant B's AdminScope, must
	// return no data: the join through analyzers.company_id excludes it.
	crossTenant, err := svc.Profiles(ctx, tenantB.AdminScope, req)
	require.NoError(t, err)
	require.Zero(t, crossTenant.Profiles["weekday"].Days)
	require.Zero(t, crossTenant.Profiles["weekend"].Days)
	require.Nil(t, crossTenant.Statistics["weekday"].Mean)
}

// TestStatisticsMatchTheHandComputedFixture runs a small real fixture
// through the database: one 1-hour-metered analyzer over two weekdays
// (2026-01-05 Mon, 2026-01-06 Tue) and one weekend day (2026-01-03 Sat),
// with readings chosen so every R87 hourly delta is a small integer.
//
// Register readings (hours 0-3 Istanbul-local each day; hour 3 exists only
// to give hour 2 a successor, R87's one-hour look-ahead):
//
//	Sat 2026-01-03: 3000, 3004, 3009, 3011  -> deltas h0=4, h1=5, h2=2
//	Mon 2026-01-05: 1000, 1006, 1011, 1015  -> deltas h0=6, h1=5, h2=4
//	Tue 2026-01-06: 2000, 2010, 2017, 2021  -> deltas h0=10, h1=7, h2=4
//
// "weekday" profile (Mon+Tue, Days=2), per-hour MEAN across the two days:
//
//	hour0: (6+10)/2  = 8
//	hour1: (5+7)/2   = 6
//	hour2: (4+4)/2   = 4
//
// Statistics over the profile's non-nil hourly means [8, 6, 4] (n=3):
//
//	Max = 8 (HourOfMax = 0, the first hour reaching the max)
//	Min = 4
//	Mean = (8+6+4)/3 = 18/3 = 6
//	Population variance = ((8-6)^2 + (6-6)^2 + (4-6)^2) / 3 = (4+0+4)/3 = 8/3
//	StdDev = sqrt(8/3) = sqrt(8)/sqrt(3) = (2*sqrt(6))/3
//	       = (2 * 2.449489742783178...) / 3 = 1.632993161855452...
//	Range = Max - Min = 8 - 4 = 4
//	LoadFactor = Mean / Max = 6 / 8 = 0.75
func TestStatisticsMatchTheHandComputedFixture(t *testing.T) {
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
		date int
		regs []string // hour 0..3 register readings
	}
	fixture := []day{
		{3, []string{"3000.0000", "3004.0000", "3009.0000", "3011.0000"}}, // Sat (weekend)
		{5, []string{"1000.0000", "1006.0000", "1011.0000", "1015.0000"}}, // Mon
		{6, []string{"2000.0000", "2010.0000", "2017.0000", "2021.0000"}}, // Tue
	}
	var rows []model.MeterReading
	for _, d := range fixture {
		for h, reg := range d.regs {
			rows = append(rows, model.MeterReading{
				AnalyzerID: analyzerID, Ts: istanbulAt(2026, time.January, d.date, h).UTC(),
				Kind: model.ReadingKindLoadProfile, ActiveImport: decPtr(reg),
				MultiplierApplied: decimal.RequireFromString("1"),
				SourceProvider:    model.IntegrationProviderOSOS, IngestedAt: time.Now().UTC(),
			})
		}
	}
	_, _, err := readings.BulkInsert(ctx, tenant.Scope, rows)
	require.NoError(t, err)

	req := loadprofile.Request{
		AnalyzerIDs: []uuid.UUID{analyzerID},
		Range:       store.TimeRange{From: istanbulAt(2026, time.January, 3, 0), To: istanbulAt(2026, time.January, 7, 0)},
	}
	res, err := svc.Profiles(ctx, tenant.Scope, req)
	require.NoError(t, err)

	weekday := res.Profiles["weekday"]
	require.Equal(t, 2, weekday.Days)
	require.True(t, decimal.RequireFromString("8").Equal(*weekday.Hours[0]))
	require.True(t, decimal.RequireFromString("6").Equal(*weekday.Hours[1]))
	require.True(t, decimal.RequireFromString("4").Equal(*weekday.Hours[2]))

	stats := res.Statistics["weekday"]
	require.NotNil(t, stats.Max)
	require.NotNil(t, stats.Min)
	require.NotNil(t, stats.HourOfMax)
	require.NotNil(t, stats.Mean)
	require.NotNil(t, stats.StdDev)
	require.NotNil(t, stats.Range)
	require.NotNil(t, stats.LoadFactor)

	require.True(t, decimal.RequireFromString("8").Equal(*stats.Max), "Max: got %s", stats.Max)
	require.True(t, decimal.RequireFromString("4").Equal(*stats.Min), "Min: got %s", stats.Min)
	require.Equal(t, 0, *stats.HourOfMax)
	require.True(t, decimal.RequireFromString("6").Equal(*stats.Mean), "Mean: got %s", stats.Mean)
	require.True(t, decimal.RequireFromString("4").Equal(*stats.Range), "Range: got %s", stats.Range)
	require.True(t, decimal.RequireFromString("0.75").Equal(*stats.LoadFactor), "LoadFactor: got %s", stats.LoadFactor)

	wantStdDev := decimal.RequireFromString("1.632993161855452065")
	gotStdDev := *stats.StdDev
	tolerance := decimal.RequireFromString("0.000000000000000001")
	require.True(t, gotStdDev.Sub(wantStdDev).Abs().LessThanOrEqual(tolerance),
		"StdDev: got %s, want approximately %s", gotStdDev, wantStdDev)
}

// TestHourlyValuesUseConsecutiveStartIndexesNotTheBucketsOwnConsumption is
// R87's own worked example: a 1-hour meter, one reading per hour, so the
// bucket's own ActiveConsumption (last-first inside the bucket) is
// structurally ZERO for every hour — the bug this proves against.
func TestHourlyValuesUseConsecutiveStartIndexesNotTheBucketsOwnConsumption(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)

	calendar := postgres.NewCalendarRepository(pool)
	analytics := postgres.NewAnalyticsRepository(pool)
	readings := postgres.NewReadingRepository(pool)
	svc := newTestService(t, calendar, analytics)

	analyzerID := tenant.Analyzers[0].ID
	// 2026-01-05 is a Monday: a weekday.
	rows := []model.MeterReading{
		{AnalyzerID: analyzerID, Ts: istanbulAt(2026, time.January, 5, 0).UTC(),
			Kind: model.ReadingKindLoadProfile, ActiveImport: decPtr("1000.0000"),
			MultiplierApplied: decimal.RequireFromString("1"),
			SourceProvider:    model.IntegrationProviderOSOS, IngestedAt: time.Now().UTC()},
		{AnalyzerID: analyzerID, Ts: istanbulAt(2026, time.January, 5, 1).UTC(),
			Kind: model.ReadingKindLoadProfile, ActiveImport: decPtr("1010.0000"),
			MultiplierApplied: decimal.RequireFromString("1"),
			SourceProvider:    model.IntegrationProviderOSOS, IngestedAt: time.Now().UTC()},
		{AnalyzerID: analyzerID, Ts: istanbulAt(2026, time.January, 5, 2).UTC(),
			Kind: model.ReadingKindLoadProfile, ActiveImport: decPtr("1025.0000"),
			MultiplierApplied: decimal.RequireFromString("1"),
			SourceProvider:    model.IntegrationProviderOSOS, IngestedAt: time.Now().UTC()},
	}
	_, _, err := readings.BulkInsert(ctx, tenant.Scope, rows)
	require.NoError(t, err)

	req := loadprofile.Request{
		AnalyzerIDs: []uuid.UUID{analyzerID},
		Range:       store.TimeRange{From: istanbulAt(2026, time.January, 5, 0), To: istanbulAt(2026, time.January, 5, 2)},
	}
	res, err := svc.Profiles(ctx, tenant.Scope, req)
	require.NoError(t, err)

	// decimal.Decimal.String() is not used here: active_import is
	// numeric(18,4), so ActiveImportStart differences carry that same
	// fixed scale ("10.0000") rather than the plan's illustrative bare
	// "10" — value equality (.Equal), not text formatting, is what R87
	// actually promises.
	got0 := res.Profiles["weekday"].Hours[0]
	got1 := res.Profiles["weekday"].Hours[1]
	require.NotNil(t, got0)
	require.NotNil(t, got1)
	require.True(t, decimal.RequireFromString("10").Equal(*got0), "hour0: got %s, want 10 (never the bucket's own 0 active_consumption)", got0)
	require.True(t, decimal.RequireFromString("15").Equal(*got1), "hour1: got %s, want 15 (never the bucket's own 0 active_consumption)", got1)
}

// TestHourlyValueIsAttributedToTheBucketStartsDayAndHour proves the day/hour
// a value is attributed to comes from the bucket's START instant, not its
// end: an hour whose bucket starts at 23:00 Istanbul on one day (and whose
// successor bucket starts at 00:00 the NEXT day) must land on hour 23 of the
// first day, never hour 0 of the second.
func TestHourlyValueIsAttributedToTheBucketStartsDayAndHour(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)

	calendar := postgres.NewCalendarRepository(pool)
	analytics := postgres.NewAnalyticsRepository(pool)
	readings := postgres.NewReadingRepository(pool)
	svc := newTestService(t, calendar, analytics)

	analyzerID := tenant.Analyzers[0].ID
	// 2026-01-05 (Mon) 23:00 -> 2026-01-06 (Tue) 00:00, both weekdays, so
	// only the hour/day attribution is under test, not classification.
	rows := []model.MeterReading{
		{AnalyzerID: analyzerID, Ts: istanbulAt(2026, time.January, 5, 23).UTC(),
			Kind: model.ReadingKindLoadProfile, ActiveImport: decPtr("1000.0000"),
			MultiplierApplied: decimal.RequireFromString("1"),
			SourceProvider:    model.IntegrationProviderOSOS, IngestedAt: time.Now().UTC()},
		{AnalyzerID: analyzerID, Ts: istanbulAt(2026, time.January, 6, 0).UTC(),
			Kind: model.ReadingKindLoadProfile, ActiveImport: decPtr("1012.0000"),
			MultiplierApplied: decimal.RequireFromString("1"),
			SourceProvider:    model.IntegrationProviderOSOS, IngestedAt: time.Now().UTC()},
	}
	_, _, err := readings.BulkInsert(ctx, tenant.Scope, rows)
	require.NoError(t, err)

	req := loadprofile.Request{
		AnalyzerIDs: []uuid.UUID{analyzerID},
		Range:       store.TimeRange{From: istanbulAt(2026, time.January, 5, 23), To: istanbulAt(2026, time.January, 6, 0)},
	}
	res, err := svc.Profiles(ctx, tenant.Scope, req)
	require.NoError(t, err)

	weekday := res.Profiles["weekday"]
	require.NotNil(t, weekday.Hours[23], "the value must land on hour 23 (the bucket's START hour)")
	require.True(t, decimal.RequireFromString("12").Equal(*weekday.Hours[23]))
	for h := 0; h < 23; h++ {
		require.Nil(t, weekday.Hours[h], "hour %d must be nil: the value belongs to hour 23 of the bucket's start day, never hour 0 of the next day", h)
	}
}

// TestNegativeDeltaContributesNothingNotZero proves a negative R87
// difference (a register reset without a documented multiplier change)
// contributes NOTHING to the profile — never decimal.Zero, which would be
// indistinguishable from a measured zero.
func TestNegativeDeltaContributesNothingNotZero(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)

	calendar := postgres.NewCalendarRepository(pool)
	analytics := postgres.NewAnalyticsRepository(pool)
	readings := postgres.NewReadingRepository(pool)
	svc := newTestService(t, calendar, analytics)

	analyzerID := tenant.Analyzers[0].ID
	// 2026-01-05 (Mon): hour0->hour1 resets DOWNWARD (1000 -> 5), a negative
	// delta; hour1->hour2 is a normal positive delta (5 -> 9).
	rows := []model.MeterReading{
		{AnalyzerID: analyzerID, Ts: istanbulAt(2026, time.January, 5, 0).UTC(),
			Kind: model.ReadingKindLoadProfile, ActiveImport: decPtr("1000.0000"),
			MultiplierApplied: decimal.RequireFromString("1"),
			SourceProvider:    model.IntegrationProviderOSOS, IngestedAt: time.Now().UTC()},
		{AnalyzerID: analyzerID, Ts: istanbulAt(2026, time.January, 5, 1).UTC(),
			Kind: model.ReadingKindLoadProfile, ActiveImport: decPtr("5.0000"),
			MultiplierApplied: decimal.RequireFromString("1"),
			SourceProvider:    model.IntegrationProviderOSOS, IngestedAt: time.Now().UTC()},
		{AnalyzerID: analyzerID, Ts: istanbulAt(2026, time.January, 5, 2).UTC(),
			Kind: model.ReadingKindLoadProfile, ActiveImport: decPtr("9.0000"),
			MultiplierApplied: decimal.RequireFromString("1"),
			SourceProvider:    model.IntegrationProviderOSOS, IngestedAt: time.Now().UTC()},
	}
	_, _, err := readings.BulkInsert(ctx, tenant.Scope, rows)
	require.NoError(t, err)

	req := loadprofile.Request{
		AnalyzerIDs: []uuid.UUID{analyzerID},
		Range:       store.TimeRange{From: istanbulAt(2026, time.January, 5, 0), To: istanbulAt(2026, time.January, 5, 2)},
	}
	res, err := svc.Profiles(ctx, tenant.Scope, req)
	require.NoError(t, err)

	weekday := res.Profiles["weekday"]
	require.Nil(t, weekday.Hours[0], "a negative delta must contribute nothing, never a zero hour")
	require.NotNil(t, weekday.Hours[1])
	require.True(t, decimal.RequireFromString("4").Equal(*weekday.Hours[1]))
}

func TestInvalidRequestRejectsZeroOrMultipleAnalyzerIDs(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)

	calendar := postgres.NewCalendarRepository(pool)
	analytics := postgres.NewAnalyticsRepository(pool)
	svc := newTestService(t, calendar, analytics)

	validRange := store.TimeRange{From: istanbulAt(2026, time.January, 5, 0), To: istanbulAt(2026, time.January, 6, 0)}

	_, err := svc.Profiles(ctx, tenant.Scope, loadprofile.Request{AnalyzerIDs: nil, Range: validRange})
	require.ErrorIs(t, err, loadprofile.ErrInvalidRequest, "zero analyzer ids must be rejected before any I/O")

	_, err = svc.Profiles(ctx, tenant.Scope, loadprofile.Request{
		AnalyzerIDs: []uuid.UUID{tenant.Analyzers[0].ID, tenant.Analyzers[1].ID},
		Range:       validRange,
	})
	require.ErrorIs(t, err, loadprofile.ErrInvalidRequest, "two analyzer ids must be rejected: /load-profile takes exactly one")
}

func TestInvalidRequestRejectsUnknownKey(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)

	calendar := postgres.NewCalendarRepository(pool)
	analytics := postgres.NewAnalyticsRepository(pool)
	svc := newTestService(t, calendar, analytics)

	_, err := svc.Profiles(ctx, tenant.Scope, loadprofile.Request{
		AnalyzerIDs: []uuid.UUID{tenant.Analyzers[0].ID},
		Range:       store.TimeRange{From: istanbulAt(2026, time.January, 5, 0), To: istanbulAt(2026, time.January, 6, 0)},
		Keys:        []domainlp.Key{"not_a_real_key"},
	})
	require.ErrorIs(t, err, loadprofile.ErrInvalidRequest)
}

func TestRequestedKeysNarrowResultAndFillMissingWithEmptyProfile(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)

	calendar := postgres.NewCalendarRepository(pool)
	analytics := postgres.NewAnalyticsRepository(pool)
	readings := postgres.NewReadingRepository(pool)
	svc := newTestService(t, calendar, analytics)

	analyzerID := tenant.Analyzers[0].ID
	// 2026-01-05 is a Monday, winter: contributes to "weekday" and
	// "winter_weekday" only.
	rows := []model.MeterReading{
		{AnalyzerID: analyzerID, Ts: istanbulAt(2026, time.January, 5, 0).UTC(),
			Kind: model.ReadingKindLoadProfile, ActiveImport: decPtr("1000.0000"),
			MultiplierApplied: decimal.RequireFromString("1"),
			SourceProvider:    model.IntegrationProviderOSOS, IngestedAt: time.Now().UTC()},
		{AnalyzerID: analyzerID, Ts: istanbulAt(2026, time.January, 5, 1).UTC(),
			Kind: model.ReadingKindLoadProfile, ActiveImport: decPtr("1006.0000"),
			MultiplierApplied: decimal.RequireFromString("1"),
			SourceProvider:    model.IntegrationProviderOSOS, IngestedAt: time.Now().UTC()},
	}
	_, _, err := readings.BulkInsert(ctx, tenant.Scope, rows)
	require.NoError(t, err)

	res, err := svc.Profiles(ctx, tenant.Scope, loadprofile.Request{
		AnalyzerIDs: []uuid.UUID{analyzerID},
		Range:       store.TimeRange{From: istanbulAt(2026, time.January, 5, 0), To: istanbulAt(2026, time.January, 6, 0)},
		Keys:        []domainlp.Key{"weekday", "weekend"},
	})
	require.NoError(t, err)

	require.Len(t, res.Profiles, 2)
	require.Len(t, res.Statistics, 2)
	require.Equal(t, 1, res.Profiles["weekday"].Days)
	require.NotNil(t, res.Statistics["weekday"].Mean)

	// "weekend" was requested but has no data: present, empty, nil stats.
	require.Equal(t, 0, res.Profiles["weekend"].Days)
	for _, h := range res.Profiles["weekend"].Hours {
		require.Nil(t, h)
	}
	require.Nil(t, res.Statistics["weekend"].Mean)
	require.Nil(t, res.Statistics["weekend"].Max)
}

func decPtr(s string) *decimal.Decimal {
	d := decimal.RequireFromString(s)
	return &d
}

func decPtrD(d decimal.Decimal) *decimal.Decimal { return &d }
