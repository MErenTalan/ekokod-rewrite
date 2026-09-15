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

func TestAnalyticsConsumptionMonthlyAndYearlyRequireRefresh(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	readingRepo := postgres.NewReadingRepository(pool)
	analyticsRepo := postgres.NewAnalyticsRepository(pool)
	analyzerID := tenant.Analyzers[0].ID

	// A YEAR bucket only closes at the end of its local year, and
	// refresh_continuous_aggregate never materialises a bucket "now" is
	// still inside — the exact behaviour migration 00005's header documents
	// ("the year currently in progress is never in the materialized data").
	// analyticsEpoch (2026) is the CURRENT year as of this phase's clock, so
	// its year bucket is never closed; this test therefore uses a year
	// that is unambiguously in the past instead.
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

	// Before any refresh: materialized_only = true views have NO row for
	// this still-open bucket, per migration 00005's header — absent, not an
	// error and not zero. analyzerIDs is named explicitly (empty/nil means
	// NO ROWS, fail-closed — Important finding 4).
	before, err := analyticsRepo.ConsumptionMonthly(ctx, tenant.Scope, []uuid.UUID{analyzerID}, monthWindow)
	require.NoError(t, err)
	require.Empty(t, before)

	analyticsRefresh(t, ctx, pool, "consumption_monthly")
	analyticsRefresh(t, ctx, pool, "consumption_yearly")

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
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	productionRepo := postgres.NewProductionRepository(pool)
	analyticsRepo := postgres.NewAnalyticsRepository(pool)
	plantID := tenant.Plants[0].ID
	deviceID := productionSeedDevice(t, ctx, pool, plantID, "ANALYTICS-DEV")

	rows := []model.PlantProduction{productionRow(plantID, deviceID, analyticsEpoch, "5.0000")}
	_, _, err := productionRepo.BulkInsert(ctx, tenant.AdminScope, rows)
	require.NoError(t, err)

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

	analyticsRefresh(t, ctx, pool, "consumption_monthly")
	analyticsRefresh(t, ctx, pool, "consumption_yearly")

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
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenantA := testfixtures.NewTenant(t, ctx, pool, 1)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 2)
	productionRepo := postgres.NewProductionRepository(pool)
	analyticsRepo := postgres.NewAnalyticsRepository(pool)

	deviceA := productionSeedDevice(t, ctx, pool, tenantA.Plants[0].ID, "ISO-A")
	deviceB := productionSeedDevice(t, ctx, pool, tenantB.Plants[0].ID, "ISO-B")

	_, _, err := productionRepo.BulkInsert(ctx, tenantA.AdminScope,
		[]model.PlantProduction{productionRow(tenantA.Plants[0].ID, deviceA, analyticsEpoch, "1.0000")})
	require.NoError(t, err)
	_, _, err = productionRepo.BulkInsert(ctx, tenantB.AdminScope,
		[]model.PlantProduction{productionRow(tenantB.Plants[0].ID, deviceB, analyticsEpoch, "999.0000")})
	require.NoError(t, err)

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
	_, _, err := productionRepo.BulkInsert(ctx, tenantB.Scope, []model.PlantProduction{
		productionRow(tenantB.Plants[0].ID, deviceID, analyticsEpoch, "1.0000"),
	})
	require.NoError(t, err)

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
