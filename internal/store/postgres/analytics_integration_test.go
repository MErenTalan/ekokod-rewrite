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
	hourly, err := analyticsRepo.ConsumptionHourly(ctx, tenant.Scope, nil,
		store.TimeRange{From: analyticsEpoch, To: analyticsEpoch.Add(time.Hour)})
	require.NoError(t, err)
	require.Len(t, hourly, 1)
	require.True(t, decimal.RequireFromString("10.0000").Equal(*hourly[0].ActiveConsumption))

	// consumption_daily buckets in Europe/Istanbul (UTC+3): the bucket for
	// this UTC-midnight reading STARTS the previous UTC day
	// (2025-12-31T21:00Z), so the window is widened by a day on each side
	// rather than aligned to UTC calendar days.
	daily, err := analyticsRepo.ConsumptionDaily(ctx, tenant.Scope, nil,
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
	// error and not zero.
	before, err := analyticsRepo.ConsumptionMonthly(ctx, tenant.Scope, nil, monthWindow)
	require.NoError(t, err)
	require.Empty(t, before)

	analyticsRefresh(t, ctx, pool, "consumption_monthly")
	analyticsRefresh(t, ctx, pool, "consumption_yearly")

	monthly, err := analyticsRepo.ConsumptionMonthly(ctx, tenant.Scope, nil, monthWindow)
	require.NoError(t, err)
	require.Len(t, monthly, 1)
	require.True(t, decimal.RequireFromString("500.0000").Equal(*monthly[0].ActiveConsumption))

	yearly, err := analyticsRepo.ConsumptionYearly(ctx, tenant.Scope, nil, yearWindow)
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
	// window as the consumption side.
	daily, err := analyticsRepo.ProductionDaily(ctx, tenant.Scope, nil,
		store.TimeRange{From: analyticsEpoch.Add(-24 * time.Hour), To: analyticsEpoch.Add(48 * time.Hour)})
	require.NoError(t, err)
	require.Len(t, daily, 1)

	monthWindow := store.TimeRange{From: analyticsEpoch.Add(-24 * time.Hour), To: analyticsEpoch.AddDate(0, 1, 0).Add(24 * time.Hour)}
	beforeRefresh, err := analyticsRepo.ProductionMonthly(ctx, tenant.Scope, nil, monthWindow)
	require.NoError(t, err)
	require.Empty(t, beforeRefresh)

	analyticsRefresh(t, ctx, pool, "plant_production_monthly")
	monthly, err := analyticsRepo.ProductionMonthly(ctx, tenant.Scope, nil, monthWindow)
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
	_, err = repo.ProductionDaily(ctx, tenant.Scope, nil, invalidRange)
	require.ErrorIs(t, err, store.ErrInvalidRange)
}

// --- isolation: the aggregates have no company_id; consumption_* join
// through analyzers, plant_production_* through power_plants --------------

func TestAnalyticsConsumptionHourlyIDsOutsideScopeContributeNoRows(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenantA := testfixtures.NewTenant(t, ctx, pool, 1)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 2)
	readingRepo := postgres.NewReadingRepository(pool)
	analyticsRepo := postgres.NewAnalyticsRepository(pool)

	row := model.MeterReading{
		AnalyzerID: tenantB.Analyzers[0].ID, Ts: analyticsEpoch, Kind: model.ReadingKindLoadProfile,
		ActiveImport: analyticsDecPtr("1.0000"), MultiplierApplied: decimal.RequireFromString("1"),
		SourceProvider: model.IntegrationProviderOSOS, IngestedAt: time.Now().UTC(),
	}
	_, _, err := readingRepo.BulkInsert(ctx, tenantB.Scope, []model.MeterReading{row})
	require.NoError(t, err)

	// Cross-tenant: tenant A's scope, tenant B's analyzer id explicitly named.
	got, err := analyticsRepo.ConsumptionHourly(ctx, tenantA.Scope, []uuid.UUID{tenantB.Analyzers[0].ID},
		store.TimeRange{From: analyticsEpoch, To: analyticsEpoch.Add(time.Hour)})
	require.NoError(t, err)
	require.Empty(t, got)

	// Even tenant A's OWN scope with no ids filter must never surface
	// tenant B's rows.
	all, err := analyticsRepo.ConsumptionHourly(ctx, tenantA.Scope, nil,
		store.TimeRange{From: analyticsEpoch, To: analyticsEpoch.Add(time.Hour)})
	require.NoError(t, err)
	require.Empty(t, all)
}

func TestAnalyticsProductionDailyIDsOutsideScopeContributeNoRows(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenantA := testfixtures.NewTenant(t, ctx, pool, 1)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 2)
	productionRepo := postgres.NewProductionRepository(pool)
	analyticsRepo := postgres.NewAnalyticsRepository(pool)

	deviceID := productionSeedDevice(t, ctx, pool, tenantB.Plants[0].ID, "ISO-DEV")
	_, _, err := productionRepo.BulkInsert(ctx, tenantB.Scope, []model.PlantProduction{
		productionRow(tenantB.Plants[0].ID, deviceID, analyticsEpoch, "1.0000"),
	})
	require.NoError(t, err)

	got, err := analyticsRepo.ProductionDaily(ctx, tenantA.Scope, []uuid.UUID{tenantB.Plants[0].ID},
		store.TimeRange{From: analyticsEpoch, To: analyticsEpoch.Add(24 * time.Hour)})
	require.NoError(t, err)
	require.Empty(t, got)
}

func analyticsDecPtr(s string) *decimal.Decimal {
	d := decimal.RequireFromString(s)
	return &d
}
