//go:build integration

package admin_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/admin"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

// aggregatesIstanbul is loaded once so every test in this file evaluates
// month boundaries the same way migration 00005's consumption_monthly does
// (time_bucket('1 month', ts, 'Europe/Istanbul')).
var aggregatesIstanbul = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		panic(err)
	}
	return loc
}()

// insertLoadProfileReading writes one load_profile reading through
// ReadingRepository, matching analytics_integration_test.go's fixture style.
func insertLoadProfileReading(t *testing.T, ctx context.Context, readingRepo *postgres.ReadingRepository, scope store.Scope, analyzerID uuid.UUID, ts time.Time, activeImport string) {
	t.Helper()
	value := decimal.RequireFromString(activeImport)
	row := model.MeterReading{
		AnalyzerID: analyzerID, Ts: ts, Kind: model.ReadingKindLoadProfile,
		ActiveImport: &value, MultiplierApplied: decimal.RequireFromString("1"),
		SourceProvider: model.IntegrationProviderOSOS, IngestedAt: time.Now().UTC(),
	}
	_, _, err := readingRepo.BulkInsert(ctx, scope, []model.MeterReading{row})
	require.NoError(t, err)
}

// TestRefreshMaterialisesAClosedMonth is the refresh proof for a
// materialized_only view: consumption_monthly's watermark sits at the end of
// the last CLOSED month (migration 00005's header), so a closed month from
// roughly two years back is unambiguously below it and absent until refreshed
// explicitly — no real-time union can rescue it, unlike consumption_hourly.
func TestRefreshMaterialisesAClosedMonth(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 6001)
	analyzer := tenant.Analyzers[0]
	readingRepo := postgres.NewReadingRepository(pool)
	analyticsRepo := postgres.NewAnalyticsRepository(pool)
	repo := admin.NewAggregateRepository(pool)

	// A month about two years back, unambiguously closed relative to any
	// clock this test runs under.
	monthStart := time.Date(2024, time.March, 1, 0, 0, 0, 0, aggregatesIstanbul)
	monthEnd := monthStart.AddDate(0, 1, 0)
	monthWindow := store.TimeRange{From: monthStart, To: monthEnd}

	insertLoadProfileReading(t, ctx, readingRepo, tenant.Scope, analyzer.ID,
		time.Date(2024, time.March, 10, 12, 0, 0, 0, aggregatesIstanbul), "1000")
	insertLoadProfileReading(t, ctx, readingRepo, tenant.Scope, analyzer.ID,
		time.Date(2024, time.March, 20, 12, 0, 0, 0, aggregatesIstanbul), "1012.5")

	before, err := analyticsRepo.ConsumptionMonthly(ctx, tenant.AdminScope, []uuid.UUID{analyzer.ID}, monthWindow)
	require.NoError(t, err)
	require.Empty(t, before, "a closed month below consumption_monthly's watermark is absent, not stale")

	require.NoError(t, repo.Refresh(ctx, store.ViewConsumptionMonthly, monthWindow))

	after, err := analyticsRepo.ConsumptionMonthly(ctx, tenant.AdminScope, []uuid.UUID{analyzer.ID}, monthWindow)
	require.NoError(t, err)
	require.Len(t, after, 1)
	require.True(t, decimal.RequireFromString("12.5").Equal(*after[0].ActiveConsumption),
		"want 12.5, got %s", after[0].ActiveConsumption.String())
}

// TestRefreshMaterialisesAnHourBelowTheWatermark is the refresh proof for a
// real-time (materialized_only = false) view. consumption_hourly's real-time
// union serves raw rows for everything ABOVE its materialisation watermark,
// which in a fresh database with no refresh yet is effectively every row —
// exactly why the plan's original single-test design does not hold for this
// view (see the task's override notes). To exercise the actual production
// scenario — a late backfill landing BELOW an already-advanced watermark —
// this test seeds one recent reading (the ongoing ingestion a live company
// already has), refreshes a window that reaches to one hour ago so the
// watermark advances past it, and only THEN inserts the backfilled readings
// roughly 200 days in the past, below where the watermark now sits.
func TestRefreshMaterialisesAnHourBelowTheWatermark(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 6002)
	analyzer := tenant.Analyzers[0]
	readingRepo := postgres.NewReadingRepository(pool)
	analyticsRepo := postgres.NewAnalyticsRepository(pool)
	repo := admin.NewAggregateRepository(pool)

	now := time.Now().UTC()

	// A normal, recent reading — the ongoing ingestion a company already has
	// running — so the hypertable is not entirely empty when the watermark
	// is moved below. TimescaleDB's cagg watermark is derived from what has
	// actually been materialised; refreshing a window with no rows in it AT
	// ALL (an empty hypertable) is a true no-op that advances nothing, which
	// is why this reading exists.
	insertLoadProfileReading(t, ctx, readingRepo, tenant.Scope, analyzer.ID, now.Add(-2*time.Hour), "1")

	// Move the watermark forward over a range whose only real data is that
	// recent reading, well past the OLD bucket this test is about to
	// backfill. This advances consumption_hourly's real-time cutoff to
	// ~now-1h — the routine refresh a live system already runs.
	require.NoError(t, repo.Refresh(ctx, store.ViewConsumptionHourly,
		store.TimeRange{From: now.Add(-400 * 24 * time.Hour), To: now.Add(-time.Hour)}))

	// A late backfill ~200 days back: two readings 30 minutes apart, inside
	// one hour, landing below the watermark just advanced above.
	base := now.Add(-200 * 24 * time.Hour).Truncate(time.Hour)
	hourWindow := store.TimeRange{From: base, To: base.Add(time.Hour)}

	insertLoadProfileReading(t, ctx, readingRepo, tenant.Scope, analyzer.ID, base, "1000")
	insertLoadProfileReading(t, ctx, readingRepo, tenant.Scope, analyzer.ID, base.Add(30*time.Minute), "1012.5")

	before, err := analyticsRepo.ConsumptionHourly(ctx, tenant.AdminScope, []uuid.UUID{analyzer.ID}, hourWindow)
	require.NoError(t, err)
	require.Empty(t, before, "a backfilled hour below the watermark is absent, not stale, even on a real-time view")

	require.NoError(t, repo.Refresh(ctx, store.ViewConsumptionHourly, hourWindow))

	after, err := analyticsRepo.ConsumptionHourly(ctx, tenant.AdminScope, []uuid.UUID{analyzer.ID}, hourWindow)
	require.NoError(t, err)
	require.Len(t, after, 1)
	require.True(t, decimal.RequireFromString("12.5").Equal(*after[0].ActiveConsumption),
		"want 12.5, got %s", after[0].ActiveConsumption.String())
}

// TestRefreshRejectsAnUnknownViewBeforeTouchingTheDatabase proves the closed
// set is checked before any I/O: the pool handed to the repository is
// already closed, so any use of it errors, and a closed pool errors on any
// use — a sentinel result here proves the validation ran first.
func TestRefreshRejectsAnUnknownViewBeforeTouchingTheDatabase(t *testing.T) {
	t.Parallel()
	pool := testfixtures.NewIsolatedDB(t)
	pool.Close()
	repo := admin.NewAggregateRepository(pool)

	err := repo.Refresh(context.Background(), store.AggregateView("consumption_weekly"),
		store.TimeRange{From: time.Now().Add(-time.Hour), To: time.Now()})
	require.ErrorIs(t, err, store.ErrUnknownView)
}

// TestRefreshRejectsAnInvalidRange proves the TimeRange check runs before any
// database round trip too.
func TestRefreshRejectsAnInvalidRange(t *testing.T) {
	t.Parallel()
	pool := testfixtures.NewIsolatedDB(t)
	repo := admin.NewAggregateRepository(pool)

	err := repo.Refresh(context.Background(), store.ViewConsumptionHourly, store.TimeRange{})
	require.ErrorIs(t, err, store.ErrInvalidRange)
}

// TestRefreshReturnsTheDatabaseError proves Refresh surfaces a genuine
// database-level rejection rather than swallowing it: a one-hour window
// passes store.TimeRange.Valid() (both ends set, From before To) but
// TimescaleDB itself rejects refresh_continuous_aggregate on
// consumption_daily (bucket width one day) with "refresh window too small",
// so this exercises the CALL's own error path, not either pre-I/O check.
func TestRefreshReturnsTheDatabaseError(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	repo := admin.NewAggregateRepository(pool)

	now := time.Now().UTC()
	tooSmall := store.TimeRange{From: now.Add(-time.Hour), To: now}
	require.True(t, tooSmall.Valid(), "the window must pass Valid() so only the database rejection is exercised")

	err := repo.Refresh(ctx, store.ViewConsumptionDaily, tooSmall)
	require.Error(t, err)
	require.NotErrorIs(t, err, store.ErrUnknownView)
	require.NotErrorIs(t, err, store.ErrInvalidRange)
}

// TestRefreshWalksTheFourViewsFinestFirst pins store.ConsumptionViews' order:
// a caller (Task 11's handler) that refreshes in this order never has a
// coarser view appear to lag a finer one it depends on for its own story
// about the data (R71).
func TestRefreshWalksTheFourViewsFinestFirst(t *testing.T) {
	t.Parallel()
	require.Equal(t, []store.AggregateView{
		store.ViewConsumptionHourly,
		store.ViewConsumptionDaily,
		store.ViewConsumptionMonthly,
		store.ViewConsumptionYearly,
	}, store.ConsumptionViews())
}
