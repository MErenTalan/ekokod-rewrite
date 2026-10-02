//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

var analyticsEpoch = time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

// analyticsRefresh materialises a continuous aggregate over everything up to
// now, exactly as the migration 00005 header's "OPERATOR NOTE" describes for
// a backfill. It must run outside a transaction — pool.Exec's implicit
// autocommit, never inside a BEGIN — because refresh_continuous_aggregate
// cannot run inside a transaction block.
func analyticsRefresh(t *testing.T, ctx context.Context, pool *pgxpool.Pool, view string) {
	t.Helper()
	_, err := pool.Exec(ctx, `call refresh_continuous_aggregate($1, NULL, now())`, view)
	require.NoError(t, err)
}

func TestAnalyticsConsumptionHourlyAndDailyAreRealTime(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	readingRepo := postgres.NewReadingRepository(pool)
	analyticsRepo := postgres.NewAnalyticsRepository(pool)
	analyzerID := tenant.Analyzers[0].ID

	rows := []model.MeterReading{
		{AnalyzerID: analyzerID, Ts: analyticsEpoch, Kind: model.ReadingKindLoadProfile,
			ActiveImport: analyticsDecPtr("1000.0000"), MultiplierApplied: decimal.RequireFromString("1"),
			SourceProvider: model.IntegrationProviderOSOS, IngestedAt: time.Now().UTC()},
		{AnalyzerID: analyzerID, Ts: analyticsEpoch.Add(30 * time.Minute), Kind: model.ReadingKindLoadProfile,
			ActiveImport: analyticsDecPtr("1010.0000"), MultiplierApplied: decimal.RequireFromString("1"),
			SourceProvider: model.IntegrationProviderOSOS, IngestedAt: time.Now().UTC()},
	}
	_, _, err := readingRepo.BulkInsert(ctx, tenant.Scope, rows)
	require.NoError(t, err)

	// consumption_hourly and consumption_daily are materialized_only = false:
	// real-time aggregation must make this readable with no manual refresh.
	// analyzerIDs is a REQUIRED POSITIONAL parameter (empty/nil means NO
	// ROWS, fail-closed — Important finding 4), so this analyzer must be
	// named explicitly.
	hourly, err := analyticsRepo.ConsumptionHourly(ctx, tenant.Scope, []uuid.UUID{analyzerID},
		store.TimeRange{From: analyticsEpoch, To: analyticsEpoch.Add(time.Hour)})
	require.NoError(t, err)
	require.Len(t, hourly, 1)
	require.True(t, decimal.RequireFromString("10.0000").Equal(*hourly[0].ActiveConsumption))

	// consumption_daily buckets in Europe/Istanbul (UTC+3): the bucket for
	// this UTC-midnight reading STARTS the previous UTC day
	// (2025-12-31T21:00Z), so the window is widened by a day on each side
	// rather than aligned to UTC calendar days.
	daily, err := analyticsRepo.ConsumptionDaily(ctx, tenant.Scope, []uuid.UUID{analyzerID},
		store.TimeRange{From: analyticsEpoch.Add(-24 * time.Hour), To: analyticsEpoch.Add(48 * time.Hour)})
	require.NoError(t, err)
	require.Len(t, daily, 1)
}

// R499: months and years are rolled up from the real-time daily aggregate.
func TestAnalyticsConsumptionMonthlyAndYearlyNeedNoRefresh(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	readingRepo := postgres.NewReadingRepository(pool)
	analyticsRepo := postgres.NewAnalyticsRepository(pool)
	analyzerID := tenant.Analyzers[0].ID

	// A second, long-past year: the roll-up covers any age without a refresh.
	closedYearEpoch := time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC)

	rows := []model.MeterReading{
		{AnalyzerID: analyzerID, Ts: analyticsEpoch, Kind: model.ReadingKindLoadProfile,
			ActiveImport: analyticsDecPtr("1000.0000"), MultiplierApplied: decimal.RequireFromString("1"),
			SourceProvider: model.IntegrationProviderOSOS, IngestedAt: time.Now().UTC()},
		{AnalyzerID: analyzerID, Ts: analyticsEpoch.Add(15 * 24 * time.Hour), Kind: model.ReadingKindLoadProfile,
			ActiveImport: analyticsDecPtr("1500.0000"), MultiplierApplied: decimal.RequireFromString("1"),
			SourceProvider: model.IntegrationProviderOSOS, IngestedAt: time.Now().UTC()},
		{AnalyzerID: analyzerID, Ts: closedYearEpoch, Kind: model.ReadingKindLoadProfile,
			ActiveImport: analyticsDecPtr("1.0000"), MultiplierApplied: decimal.RequireFromString("1"),
			SourceProvider: model.IntegrationProviderOSOS, IngestedAt: time.Now().UTC()},
		{AnalyzerID: analyzerID, Ts: closedYearEpoch.Add(time.Hour), Kind: model.ReadingKindLoadProfile,
			ActiveImport: analyticsDecPtr("4.0000"), MultiplierApplied: decimal.RequireFromString("1"),
			SourceProvider: model.IntegrationProviderOSOS, IngestedAt: time.Now().UTC()},
	}
	_, _, err := readingRepo.BulkInsert(ctx, tenant.Scope, rows)
	require.NoError(t, err)

	// Widened by a day on each side for the same Europe/Istanbul reason as
	// the daily window above: a bucket that STARTS in local January may
	// start in UTC December.
	monthWindow := store.TimeRange{From: analyticsEpoch.Add(-24 * time.Hour), To: analyticsEpoch.AddDate(0, 1, 0).Add(24 * time.Hour)}
	yearWindow := store.TimeRange{From: closedYearEpoch.Add(-24 * time.Hour), To: closedYearEpoch.AddDate(1, 0, 0).Add(24 * time.Hour)}

	monthly, err := analyticsRepo.ConsumptionMonthly(ctx, tenant.Scope, []uuid.UUID{analyzerID}, monthWindow)
	require.NoError(t, err)
	require.Len(t, monthly, 1)
	require.True(t, decimal.RequireFromString("500.0000").Equal(*monthly[0].ActiveConsumption))

	yearly, err := analyticsRepo.ConsumptionYearly(ctx, tenant.Scope, []uuid.UUID{analyzerID}, yearWindow)
	require.NoError(t, err)
	require.Len(t, yearly, 1)
	require.True(t, decimal.RequireFromString("3.0000").Equal(*yearly[0].ActiveConsumption))
}

func TestAnalyticsProductionDailyIsRealTimeAndMonthlyRequiresRefresh(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	productionRepo := postgres.NewProductionRepository(pool)
	analyticsRepo := postgres.NewAnalyticsRepository(pool)
	plantID := tenant.Plants[0].ID
	deviceID := productionSeedDevice(t, ctx, pool, plantID, "ANALYTICS-DEV")

	_ = productionRepo
	_ = deviceID
	seedTotal(t, ctx, pool, plantID, analyticsEpoch, "5.0000")

	// plant_production_daily buckets in Europe/Istanbul too: same widened
	// window as the consumption side. plantIDs is named explicitly
	// (empty/nil means NO ROWS, fail-closed — Important finding 4).
	daily, err := analyticsRepo.ProductionDaily(ctx, tenant.Scope, []uuid.UUID{plantID},
		store.TimeRange{From: analyticsEpoch.Add(-24 * time.Hour), To: analyticsEpoch.Add(48 * time.Hour)})
	require.NoError(t, err)
	require.Len(t, daily, 1)

	monthWindow := store.TimeRange{From: analyticsEpoch.Add(-24 * time.Hour), To: analyticsEpoch.AddDate(0, 1, 0).Add(24 * time.Hour)}
	beforeRefresh, err := analyticsRepo.ProductionMonthly(ctx, tenant.Scope, []uuid.UUID{plantID}, monthWindow)
	require.NoError(t, err)
	require.Empty(t, beforeRefresh)

	analyticsRefresh(t, ctx, pool, "plant_production_monthly")
	monthly, err := analyticsRepo.ProductionMonthly(ctx, tenant.Scope, []uuid.UUID{plantID}, monthWindow)
	require.NoError(t, err)
	require.Len(t, monthly, 1)
}

// --- scope / range validation ----------------------------------------------

func TestAnalyticsRepositoryRejectsInvalidScope(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	repo := postgres.NewAnalyticsRepository(pool)
	invalid := store.Scope{}
	validRange := store.TimeRange{From: analyticsEpoch, To: analyticsEpoch.Add(time.Hour)}

	_, err := repo.ConsumptionHourly(ctx, invalid, nil, validRange)
	require.ErrorIs(t, err, store.ErrInvalidScope)
	_, err = repo.ConsumptionDaily(ctx, invalid, nil, validRange)
	require.ErrorIs(t, err, store.ErrInvalidScope)
	_, err = repo.ConsumptionMonthly(ctx, invalid, nil, validRange)
	require.ErrorIs(t, err, store.ErrInvalidScope)
	_, err = repo.ConsumptionYearly(ctx, invalid, nil, validRange)
	require.ErrorIs(t, err, store.ErrInvalidScope)
	_, err = repo.ProductionDaily(ctx, invalid, nil, validRange)
	require.ErrorIs(t, err, store.ErrInvalidScope)
	_, err = repo.ProductionMonthly(ctx, invalid, nil, validRange)
	require.ErrorIs(t, err, store.ErrInvalidScope)
}

func TestAnalyticsRepositoryRejectsInvalidRange(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	repo := postgres.NewAnalyticsRepository(pool)
	invalidRange := store.TimeRange{}

	_, err := repo.ConsumptionHourly(ctx, tenant.Scope, nil, invalidRange)
	require.ErrorIs(t, err, store.ErrInvalidRange)
	_, err = repo.ConsumptionDaily(ctx, tenant.Scope, nil, invalidRange)
	require.ErrorIs(t, err, store.ErrInvalidRange)
	_, err = repo.ConsumptionMonthly(ctx, tenant.Scope, nil, invalidRange)
	require.ErrorIs(t, err, store.ErrInvalidRange)
	_, err = repo.ConsumptionYearly(ctx, tenant.Scope, nil, invalidRange)
	require.ErrorIs(t, err, store.ErrInvalidRange)
	_, err = repo.ProductionDaily(ctx, tenant.Scope, nil, invalidRange)
	require.ErrorIs(t, err, store.ErrInvalidRange)
	_, err = repo.ProductionMonthly(ctx, tenant.Scope, nil, invalidRange)
	require.ErrorIs(t, err, store.ErrInvalidRange)
}

// TestAnalyticsConsumptionViewsIDsOutsideScopeContributeNoRowsAndEmptyIDsReturnNothing
// is Important finding 3 + 4's table-driven isolation test, covering every
// consumption view (ConsumptionHourly/Daily/Monthly/Yearly), not just
// Hourly. Monthly and Yearly need a materialised refresh over a CLOSED
// period (analyticsEpoch's own year, 2026, is still open relative to this
// phase's clock, so the yearly case uses closedYearEpoch instead, exactly
// like TestAnalyticsConsumptionMonthlyAndYearlyRequireRefresh does).
//
// Each case seeds a REAL reading for BOTH tenant A's own analyzer (must
// appear) and tenant B's foreign analyzer (must never appear), then proves
// three things in one pass:
//   - tenant B's row is non-vacuous: it reads back through tenant B's own
//     Scope (F1 final review pass B, I3 — without this, "the company
//     predicate is shadowed" cross-tenant assertion below would prove
//     nothing if tenant B's row never existed in the first place);
//   - the cross-tenant assertion is called with tenant A's ADMINSCOPE, not
//     the narrow Scope this test used before I3: under a narrow Scope the
//     building predicate alone already excludes tenant B's analyzer, so a
//     tautologised company_id predicate would hide behind it and this test
//     would still pass — naming both ids together must surface ONLY tenant
//     A's row;
//   - a nil AND an empty (non-nil) ids list return NOTHING — never "every
//     analyzer visible to scope" — even though tenant A has a real, visible
//     row in the window (Important finding 4's fail-closed ruling; this is
//     the case that would have differed under the old widen-on-empty SQL).
func TestAnalyticsConsumptionViewsIDsOutsideScopeContributeNoRowsAndEmptyIDsReturnNothing(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenantA := testfixtures.NewTenant(t, ctx, pool, 1)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 2)
	readingRepo := postgres.NewReadingRepository(pool)
	analyticsRepo := postgres.NewAnalyticsRepository(pool)

	closedYearEpoch := time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC)

	seed := func(scope store.Scope, analyzerID uuid.UUID, ts time.Time, value string) {
		t.Helper()
		row := model.MeterReading{
			AnalyzerID: analyzerID, Ts: ts, Kind: model.ReadingKindLoadProfile,
			ActiveImport: analyticsDecPtr(value), MultiplierApplied: decimal.RequireFromString("1"),
			SourceProvider: model.IntegrationProviderOSOS, IngestedAt: time.Now().UTC(),
		}
		_, _, err := readingRepo.BulkInsert(ctx, scope, []model.MeterReading{row})
		require.NoError(t, err)
	}

	seed(tenantA.Scope, tenantA.Analyzers[0].ID, analyticsEpoch, "1.0000")
	seed(tenantB.Scope, tenantB.Analyzers[0].ID, analyticsEpoch, "999.0000")
	seed(tenantA.Scope, tenantA.Analyzers[0].ID, closedYearEpoch, "1.0000")
	seed(tenantB.Scope, tenantB.Analyzers[0].ID, closedYearEpoch, "999.0000")

	hourWindow := store.TimeRange{From: analyticsEpoch, To: analyticsEpoch.Add(time.Hour)}
	dayWindow := store.TimeRange{From: analyticsEpoch.Add(-24 * time.Hour), To: analyticsEpoch.Add(48 * time.Hour)}
	monthWindow := store.TimeRange{From: analyticsEpoch.Add(-24 * time.Hour), To: analyticsEpoch.AddDate(0, 1, 0).Add(24 * time.Hour)}
	yearWindow := store.TimeRange{From: closedYearEpoch.Add(-24 * time.Hour), To: closedYearEpoch.AddDate(1, 0, 0).Add(24 * time.Hour)}

	cases := []struct {
		name string
		call func(scope store.Scope, ids []uuid.UUID) ([]model.ConsumptionBucket, error)
	}{
		{"Hourly", func(scope store.Scope, ids []uuid.UUID) ([]model.ConsumptionBucket, error) {
			return analyticsRepo.ConsumptionHourly(ctx, scope, ids, hourWindow)
		}},
		{"Daily", func(scope store.Scope, ids []uuid.UUID) ([]model.ConsumptionBucket, error) {
			return analyticsRepo.ConsumptionDaily(ctx, scope, ids, dayWindow)
		}},
		{"Monthly", func(scope store.Scope, ids []uuid.UUID) ([]model.ConsumptionBucket, error) {
			return analyticsRepo.ConsumptionMonthly(ctx, scope, ids, monthWindow)
		}},
		{"Yearly", func(scope store.Scope, ids []uuid.UUID) ([]model.ConsumptionBucket, error) {
			return analyticsRepo.ConsumptionYearly(ctx, scope, ids, yearWindow)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Self-evident non-vacuity: tenant B's row really exists in this
			// view — tenant B can read it back through its own Scope.
			selfB, err := tc.call(tenantB.Scope, []uuid.UUID{tenantB.Analyzers[0].ID})
			require.NoError(t, err)
			require.Len(t, selfB, 1, "tenant B must be able to read its own row through its own scope")

			// Cross-tenant, called with tenant A's AdminScope, not the
			// narrow Scope: under a narrow Scope the building predicate
			// alone already excludes tenant B's analyzer (its building_id
			// is never in A's building_ids), so a tautologised
			// `a.company_id = sqlc.arg(company_id)` would hide behind the
			// building predicate and this assertion would still pass.
			// AdminScope removes that cover.
			named, err := tc.call(tenantA.AdminScope, []uuid.UUID{tenantA.Analyzers[0].ID, tenantB.Analyzers[0].ID})
			require.NoError(t, err)
			require.Len(t, named, 1, "naming a foreign id alongside the caller's own must surface only the caller's own row")
			require.Equal(t, tenantA.Analyzers[0].ID, named[0].AnalyzerID)

			nilIDs, err := tc.call(tenantA.Scope, nil)
			require.NoError(t, err)
			require.Empty(t, nilIDs, "nil ids must return NOTHING, not every analyzer visible to scope")

			emptyIDs, err := tc.call(tenantA.Scope, []uuid.UUID{})
			require.NoError(t, err)
			require.Empty(t, emptyIDs, "an empty (non-nil) ids slice must also return NOTHING")
		})
	}
}

// TestAnalyticsProductionViewsIDsOutsideScopeContributeNoRowsAndEmptyIDsReturnNothing
// is the production-side twin of the consumption test above (Important
// finding 3 + 4), covering ProductionDaily AND ProductionMonthly.
func TestAnalyticsProductionViewsIDsOutsideScopeContributeNoRowsAndEmptyIDsReturnNothing(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenantA := testfixtures.NewTenant(t, ctx, pool, 1)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 2)
	productionRepo := postgres.NewProductionRepository(pool)
	analyticsRepo := postgres.NewAnalyticsRepository(pool)

	deviceA := productionSeedDevice(t, ctx, pool, tenantA.Plants[0].ID, "ISO-A")
	deviceB := productionSeedDevice(t, ctx, pool, tenantB.Plants[0].ID, "ISO-B")

	_, _, _ = productionRepo, deviceA, deviceB
	seedTotal(t, ctx, pool, tenantA.Plants[0].ID, analyticsEpoch, "1.0000")
	seedTotal(t, ctx, pool, tenantB.Plants[0].ID, analyticsEpoch, "999.0000")

	analyticsRefresh(t, ctx, pool, "plant_production_monthly")

	dayWindow := store.TimeRange{From: analyticsEpoch.Add(-24 * time.Hour), To: analyticsEpoch.Add(48 * time.Hour)}
	monthWindow := store.TimeRange{From: analyticsEpoch.Add(-24 * time.Hour), To: analyticsEpoch.AddDate(0, 1, 0).Add(24 * time.Hour)}

	cases := []struct {
		name string
		call func(ids []uuid.UUID) ([]model.PlantProductionBucket, error)
	}{
		{"Daily", func(ids []uuid.UUID) ([]model.PlantProductionBucket, error) {
			return analyticsRepo.ProductionDaily(ctx, tenantA.Scope, ids, dayWindow)
		}},
		{"Monthly", func(ids []uuid.UUID) ([]model.PlantProductionBucket, error) {
			return analyticsRepo.ProductionMonthly(ctx, tenantA.Scope, ids, monthWindow)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			named, err := tc.call([]uuid.UUID{tenantA.Plants[0].ID, tenantB.Plants[0].ID})
			require.NoError(t, err)
			require.Len(t, named, 1, "naming a foreign plant id alongside the caller's own must surface only the caller's own row")
			require.Equal(t, tenantA.Plants[0].ID, named[0].PlantID)

			nilIDs, err := tc.call(nil)
			require.NoError(t, err)
			require.Empty(t, nilIDs, "nil ids must return NOTHING, not every plant visible to scope")

			emptyIDs, err := tc.call([]uuid.UUID{})
			require.NoError(t, err)
			require.Empty(t, emptyIDs, "an empty (non-nil) ids slice must also return NOTHING")
		})
	}
}

// TestAnalyticsConsumptionDailyBucketsInIstanbulNotUTC is the folded-minor
// exact-window test: unlike every other test in this file, which widens its
// window by a day on each side specifically to tolerate either UTC or
// Istanbul bucket alignment, this window is EXACT — From is precisely
// 2026-01-01T00:00+03:00 (Istanbul midnight), which is 2025-12-31T21:00:00Z.
// If consumption_daily bucketed in UTC instead of Europe/Istanbul, this
// reading's bucket would be dated 2025-12-31 (its UTC calendar day) and
// c.bucket < 2025-12-31T21:00:00Z (this window's own From) would exclude it
// — this test would see zero rows instead of one.
func TestAnalyticsConsumptionDailyBucketsInIstanbulNotUTC(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	readingRepo := postgres.NewReadingRepository(pool)
	analyticsRepo := postgres.NewAnalyticsRepository(pool)
	analyzerID := tenant.Analyzers[0].ID

	istanbul, err := time.LoadLocation("Europe/Istanbul")
	require.NoError(t, err)
	dayStart := time.Date(2026, time.January, 1, 0, 0, 0, 0, istanbul) // == 2025-12-31T21:00:00Z

	row := model.MeterReading{
		AnalyzerID: analyzerID, Ts: dayStart, Kind: model.ReadingKindLoadProfile,
		ActiveImport: analyticsDecPtr("1000.0000"), MultiplierApplied: decimal.RequireFromString("1"),
		SourceProvider: model.IntegrationProviderOSOS, IngestedAt: time.Now().UTC(),
	}
	_, _, err = readingRepo.BulkInsert(ctx, tenant.Scope, []model.MeterReading{row})
	require.NoError(t, err)

	exactIstanbulDay := store.TimeRange{From: dayStart, To: dayStart.AddDate(0, 0, 1)}
	got, err := analyticsRepo.ConsumptionDaily(ctx, tenant.Scope, []uuid.UUID{analyzerID}, exactIstanbulDay)
	require.NoError(t, err)
	require.Len(t, got, 1, "the exact Istanbul-midnight window must contain this reading's bucket")
}

// --- isolation: the aggregates have no company_id; consumption_* join
// through analyzers, plant_production_* through power_plants --------------

// TestAnalyticsConsumptionHourlyIDsOutsideScopeContributeNoRows (F1 final
// review pass B, I3) adds the positive control the review found missing —
// without it, `require.Empty(t, got)` also passes on a window that matches
// nothing at all — and calls the cross-tenant assertion with tenant A's
// AdminScope rather than the narrow Scope, for the same shadowing reason as
// TestAnalyticsConsumptionViewsIDsOutsideScopeContributeNoRowsAndEmptyIDsReturnNothing.
func TestAnalyticsConsumptionHourlyIDsOutsideScopeContributeNoRows(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenantA := testfixtures.NewTenant(t, ctx, pool, 1)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 2)
	readingRepo := postgres.NewReadingRepository(pool)
	analyticsRepo := postgres.NewAnalyticsRepository(pool)
	window := store.TimeRange{From: analyticsEpoch, To: analyticsEpoch.Add(time.Hour)}

	row := model.MeterReading{
		AnalyzerID: tenantB.Analyzers[0].ID, Ts: analyticsEpoch, Kind: model.ReadingKindLoadProfile,
		ActiveImport: analyticsDecPtr("1.0000"), MultiplierApplied: decimal.RequireFromString("1"),
		SourceProvider: model.IntegrationProviderOSOS, IngestedAt: time.Now().UTC(),
	}
	_, _, err := readingRepo.BulkInsert(ctx, tenantB.Scope, []model.MeterReading{row})
	require.NoError(t, err)

	// Self-evident non-vacuity: tenant B's row really exists in this view.
	selfB, err := analyticsRepo.ConsumptionHourly(ctx, tenantB.Scope, []uuid.UUID{tenantB.Analyzers[0].ID}, window)
	require.NoError(t, err)
	require.Len(t, selfB, 1)

	// Cross-tenant, called with tenant A's AdminScope, not the narrow Scope:
	// under a narrow Scope the building predicate alone already excludes
	// tenant B's analyzer, so a tautologised company_id predicate would hide
	// behind it.
	got, err := analyticsRepo.ConsumptionHourly(ctx, tenantA.AdminScope, []uuid.UUID{tenantB.Analyzers[0].ID}, window)
	require.NoError(t, err)
	require.Empty(t, got)

	// Even tenant A's OWN AdminScope with no ids filter must never surface
	// tenant B's rows.
	all, err := analyticsRepo.ConsumptionHourly(ctx, tenantA.AdminScope, nil, window)
	require.NoError(t, err)
	require.Empty(t, all)
}

// TestAnalyticsProductionDailyIDsOutsideScopeContributeNoRows (I3) adds the
// positive control the review found missing. It is NOT switched to
// AdminScope: plant_production_daily joins power_plants, which has no
// building_id at all (PlantRepository's doc), so tenant A's narrow Scope
// already applies no building predicate here to shadow behind — only the
// company_id check does the work either way.
func TestAnalyticsProductionDailyIDsOutsideScopeContributeNoRows(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenantA := testfixtures.NewTenant(t, ctx, pool, 1)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 2)
	productionRepo := postgres.NewProductionRepository(pool)
	analyticsRepo := postgres.NewAnalyticsRepository(pool)
	// Widened by a day on each side, same as
	// TestAnalyticsProductionDailyIsRealTimeAndMonthlyRequiresRefresh: the
	// bucket for a UTC-midnight reading STARTS the previous UTC day
	// (Europe/Istanbul is UTC+3), so a window that starts exactly at
	// analyticsEpoch excludes it and the positive control below would fail
	// for a reason that has nothing to do with scope.
	window := store.TimeRange{From: analyticsEpoch.Add(-24 * time.Hour), To: analyticsEpoch.Add(48 * time.Hour)}

	deviceID := productionSeedDevice(t, ctx, pool, tenantB.Plants[0].ID, "ISO-DEV")
	_, _ = productionRepo, deviceID
	seedTotal(t, ctx, pool, tenantB.Plants[0].ID, analyticsEpoch, "1.0000")

	// Self-evident non-vacuity: tenant B's row really exists in this view.
	selfB, err := analyticsRepo.ProductionDaily(ctx, tenantB.Scope, []uuid.UUID{tenantB.Plants[0].ID}, window)
	require.NoError(t, err)
	require.Len(t, selfB, 1)

	got, err := analyticsRepo.ProductionDaily(ctx, tenantA.Scope, []uuid.UUID{tenantB.Plants[0].ID}, window)
	require.NoError(t, err)
	require.Empty(t, got)
}

func analyticsDecPtr(s string) *decimal.Decimal {
	d := decimal.RequireFromString(s)
	return &d
}

// TestAnalyticsHourlyMeterIsNotZero is F3 Q6 (F15q R471): a meter that reports
// once an hour has one reading per hourly bucket, so last−first inside it is
// zero; the repository differences consecutive bucket boundaries instead.
func TestAnalyticsHourlyMeterIsNotZero(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	readingRepo := postgres.NewReadingRepository(pool)
	analyticsRepo := postgres.NewAnalyticsRepository(pool)
	id := tenant.Analyzers[0].ID

	// Istanbul midnight on 2 Jan 2026, hourly for 26 hours: 1000 + 10·h.
	start := analyticsEpoch.Add(21 * time.Hour)
	var rows []model.MeterReading
	for h := 0; h <= 26; h++ {
		rows = append(rows, model.MeterReading{AnalyzerID: id, Ts: start.Add(time.Duration(h) * time.Hour),
			Kind: model.ReadingKindLoadProfile, ActiveImport: analyticsDecPtr(decimal.NewFromInt(int64(1000 + 10*h)).String()),
			MultiplierApplied: decimal.RequireFromString("1"), SourceProvider: model.IntegrationProviderOSOS, IngestedAt: time.Now().UTC()})
	}
	_, _, err := readingRepo.BulkInsert(ctx, tenant.Scope, rows)
	require.NoError(t, err)
	for _, v := range []string{"consumption_hourly", "consumption_daily"} {
		analyticsRefresh(t, ctx, pool, v)
	}

	hourly, err := analyticsRepo.ConsumptionHourly(ctx, tenant.Scope, []uuid.UUID{id},
		store.TimeRange{From: start, To: start.Add(24 * time.Hour)})
	require.NoError(t, err)
	require.Len(t, hourly, 24)
	for _, b := range hourly {
		require.Equal(t, "10", b.ActiveConsumption.String(), b.Bucket)
	}

	daily, err := analyticsRepo.ConsumptionDaily(ctx, tenant.Scope, []uuid.UUID{id},
		store.TimeRange{From: start, To: start.Add(24 * time.Hour)})
	require.NoError(t, err)
	require.Len(t, daily, 1)
	require.Equal(t, "240", daily[0].ActiveConsumption.String(), "the whole day, 00:00 to 24:00")
}

// analyticsReading is one load_profile row; a nil active is a row whose meter
// registers the gateway could not read (PM5340 sends these with generation only).
func analyticsReading(analyzerID uuid.UUID, ts time.Time, active *decimal.Decimal, export string) model.MeterReading {
	return model.MeterReading{AnalyzerID: analyzerID, Ts: ts, Kind: model.ReadingKindLoadProfile,
		ActiveImport: active, ActiveExport: analyticsDecPtr(export), MultiplierApplied: decimal.RequireFromString("1"),
		SourceProvider: model.IntegrationProviderPM5340, IngestedAt: time.Now().UTC()}
}

// R498: a register the meter did not report must not blank its bucket — the
// bucket's first and last are the first and last REPORTED values.
func TestAnalyticsNullRegistersDoNotBlankTheBucket(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	id := tenant.Analyzers[0].ID
	ist := time.FixedZone("TRT", 3*3600)
	day := time.Date(2026, time.January, 27, 0, 0, 0, 0, ist)

	rows := []model.MeterReading{
		analyticsReading(id, day, analyticsDecPtr("9000.0000"), "10"),
		analyticsReading(id, day.Add(17*time.Hour), analyticsDecPtr("9121.0000"), "14"),
		analyticsReading(id, day.Add(17*time.Hour+15*time.Minute), nil, "15"), // meter unreadable from here on
		analyticsReading(id, day.Add(24*time.Hour), nil, "16"),
		analyticsReading(id, day.Add(30*time.Hour), nil, "20"),
	}
	testfixtures.InsertReadings(t, ctx, pool, tenant.Scope.CompanyID, rows)
	repo := postgres.NewAnalyticsRepository(pool)

	daily, err := repo.ConsumptionDaily(ctx, tenant.Scope, []uuid.UUID{id}, store.TimeRange{From: day, To: day.Add(48 * time.Hour)})
	require.NoError(t, err)
	require.Len(t, daily, 2)
	require.NotNil(t, daily[0].ActiveConsumption, "the day has two reported indexes")
	require.Equal(t, "121", daily[0].ActiveConsumption.String())
	require.Equal(t, "9121", daily[0].ActiveIndex.String(), "the closing index is the last reported one")
	require.Equal(t, "6", daily[0].ActiveGeneration.String(), "export still differences to the next day's first reading")
	require.Nil(t, daily[1].ActiveConsumption, "a day with no reported index has no consumption")

	monthly, err := repo.ConsumptionMonthly(ctx, tenant.Scope, []uuid.UUID{id},
		store.TimeRange{From: time.Date(2026, time.January, 1, 0, 0, 0, 0, ist), To: time.Date(2026, time.February, 1, 0, 0, 0, 0, ist)})
	require.NoError(t, err)
	require.Len(t, monthly, 1)
	require.Equal(t, "121", monthly[0].ActiveConsumption.String())
}

// R499: months and years are rolled up from consumption_daily at query time,
// so they need no refresh, include the meter's first month, and never absorb
// a gap between two months into the later one.
func TestAnalyticsMonthlyAndYearlyRollUpFromDaily(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	id := tenant.Analyzers[0].ID
	ist := time.FixedZone("TRT", 3*3600)
	at := func(month time.Month, d, h int) time.Time { return time.Date(2025, month, d, h, 0, 0, 0, ist) }

	rows := []model.MeterReading{
		// The meter's first month starts mid-month.
		analyticsReading(id, at(time.March, 21, 3), analyticsDecPtr("100"), "0"),
		analyticsReading(id, at(time.March, 31, 23), analyticsDecPtr("180"), "0"),
		// April is whole: readings on both edges.
		analyticsReading(id, at(time.April, 1, 0), analyticsDecPtr("181"), "0"),
		analyticsReading(id, at(time.April, 15, 0), analyticsDecPtr("250"), "0"),
		analyticsReading(id, at(time.May, 1, 0), analyticsDecPtr("300"), "0"),
		analyticsReading(id, at(time.May, 10, 0), analyticsDecPtr("340"), "0"),
		// Nothing until late August: the register kept counting meanwhile.
		analyticsReading(id, at(time.August, 27, 0), analyticsDecPtr("5000"), "0"),
		analyticsReading(id, at(time.August, 31, 0), analyticsDecPtr("5040"), "0"),
	}
	testfixtures.InsertReadings(t, ctx, pool, tenant.Scope.CompanyID, rows)
	repo := postgres.NewAnalyticsRepository(pool)

	monthly, err := repo.ConsumptionMonthly(ctx, tenant.Scope, []uuid.UUID{id},
		store.TimeRange{From: at(time.January, 1, 0), To: at(time.December, 1, 0)})
	require.NoError(t, err)
	got := map[time.Month]string{}
	closed := map[time.Month]bool{}
	for _, b := range monthly {
		m := b.Bucket.In(ist).Month()
		require.NotNil(t, b.ActiveConsumption, "%s", m)
		got[m], closed[m] = b.ActiveConsumption.String(), b.ClosedByBoundary
	}
	require.Equal(t, map[time.Month]string{
		time.March:  "81",  // 181 − 100: runs to April's first reading, which sits on the edge
		time.April:  "119", // 300 − 181
		time.May:    "40",  // 340 − 300: no reading on June's edge, so its own last
		time.August: "40",  // 5040 − 5000: the gap since May is nobody's consumption
	}, got)
	require.Equal(t, map[time.Month]bool{time.March: true, time.April: true, time.May: false, time.August: false}, closed)
	require.EqualValues(t, 2, monthly[0].ReadingCount)

	yearly, err := repo.ConsumptionYearly(ctx, tenant.Scope, []uuid.UUID{id},
		store.TimeRange{From: at(time.January, 1, 0), To: at(time.December, 31, 0)})
	require.NoError(t, err)
	require.Len(t, yearly, 1)
	require.Equal(t, "4940", yearly[0].ActiveConsumption.String(), "a year is its last index minus its first")
	require.False(t, yearly[0].ClosedByBoundary)
}
