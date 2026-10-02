//go:build integration

package consumption_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/lock"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/consumption"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/admin"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

// insertRefreshLoadProfileReading writes one load_profile reading through
// ReadingRepository, matching aggregates_integration_test.go's fixture
// style (internal/store/postgres/admin).
func insertRefreshLoadProfileReading(t *testing.T, ctx context.Context, readingRepo *postgres.ReadingRepository, scope store.Scope, analyzerID uuid.UUID, ts time.Time, activeImport string) {
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

// newRedisLocker connects a real lock.Redis to an isolated redis container,
// mirroring the shape internal/job's integration tests build their client
// against (testfixtures.StartRedis).
func newRedisLocker(t *testing.T) lock.Locker {
	t.Helper()
	uri := testfixtures.StartRedis(t)
	opts, err := goredis.ParseURL(uri)
	require.NoError(t, err)
	client := goredis.NewClient(opts)
	t.Cleanup(func() { _ = client.Close() })
	return lock.NewRedis(client)
}

// TestRefreshConsumptionShowsRecentRowsWrittenBelowTheWatermark is R500's
// production scenario through the real Refresher (real aggregates, real
// Redis lock): a live system's watermark sits about an hour back, and a
// fetch then writes ten-day-old rows below it. They are absent from every
// chart until a refresh covers them — and ten days is INSIDE the policy
// window the refresher used to leave to the scheduled job.
func TestRefreshConsumptionShowsRecentRowsWrittenBelowTheWatermark(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 6101)
	analyzer := tenant.Analyzers[0]
	readingRepo := postgres.NewReadingRepository(pool)
	analyticsRepo := postgres.NewAnalyticsRepository(pool)
	aggregateRepo := admin.NewAggregateRepository(pool)

	refresher, err := consumption.NewRefresher(consumption.RefreshDeps{
		Aggregates: aggregateRepo,
		Locker:     newRedisLocker(t),
		Enqueuer:   &fakeEnqueuer{},
		LockTTL:    2 * time.Minute,
		Location:   testIstanbul,
		Log:        discardLog(),
	})
	require.NoError(t, err)

	now := time.Now().UTC()
	// The ongoing ingestion: readings in a closed hour and a closed day, then
	// the routine refresh that moves both watermarks past them.
	insertRefreshLoadProfileReading(t, ctx, readingRepo, tenant.Scope, analyzer.ID, now.Add(-30*time.Hour), "4990")
	insertRefreshLoadProfileReading(t, ctx, readingRepo, tenant.Scope, analyzer.ID, now.Add(-2*time.Hour), "5000")
	for _, view := range store.ConsumptionViews() {
		require.NoError(t, aggregateRepo.Refresh(ctx, view, store.TimeRange{From: now.Add(-60 * 24 * time.Hour), To: now.Add(-time.Hour)}))
	}

	day := now.Add(-10 * 24 * time.Hour).In(testIstanbul)
	day = time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, testIstanbul)
	first, second := day.Add(9*time.Hour), day.Add(15*time.Hour)
	insertRefreshLoadProfileReading(t, ctx, readingRepo, tenant.Scope, analyzer.ID, first, "1000")
	insertRefreshLoadProfileReading(t, ctx, readingRepo, tenant.Scope, analyzer.ID, second, "1012.5")
	dayWindow := store.TimeRange{From: day, To: day.AddDate(0, 0, 1)}

	before, err := analyticsRepo.ConsumptionDaily(ctx, tenant.AdminScope, []uuid.UUID{analyzer.ID}, dayWindow)
	require.NoError(t, err)
	require.Empty(t, before, "rows below the watermark are absent until a refresh covers them")

	require.NoError(t, refresher.RefreshConsumption(ctx, job.ConsumptionRefreshPayload{
		CompanyID: tenant.Company.ID, AnalyzerID: analyzer.ID, From: first, To: second.Add(time.Microsecond),
	}))

	daily, err := analyticsRepo.ConsumptionDaily(ctx, tenant.AdminScope, []uuid.UUID{analyzer.ID}, dayWindow)
	require.NoError(t, err)
	require.Len(t, daily, 1)
	require.Equal(t, "12.5", daily[0].ActiveConsumption.String())

	hourly, err := analyticsRepo.ConsumptionHourly(ctx, tenant.AdminScope, []uuid.UUID{analyzer.ID},
		store.TimeRange{From: first, To: second.Add(time.Hour)})
	require.NoError(t, err)
	require.Len(t, hourly, 2, "both readings' hours")

	// The month rolls up from the refreshed day (R499).
	month := time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, testIstanbul)
	monthly, err := analyticsRepo.ConsumptionMonthly(ctx, tenant.AdminScope, []uuid.UUID{analyzer.ID},
		store.TimeRange{From: month, To: month.AddDate(0, 1, 0)})
	require.NoError(t, err)
	require.Len(t, monthly, 1)
}
