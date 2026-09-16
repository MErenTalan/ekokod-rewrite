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
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
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

// TestRefreshConsumptionMaterialisesAClosedMonthBelowThePolicyWindow is
// Task 6's TestRefreshMaterialisesAClosedMonth, driven through the real
// consumption.Refresher end to end (real AdminAggregateRepository, real
// Redis locker) rather than by calling repo.Refresh directly: readings are
// inserted for a closed month two years ago, consumption_monthly is proven
// ABSENT below the policy watermark, RefreshConsumption is called over
// those readings' range, and consumption_monthly is proven present with the
// hand-computed value.
func TestRefreshConsumptionMaterialisesAClosedMonthBelowThePolicyWindow(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 6101)
	analyzer := tenant.Analyzers[0]
	readingRepo := postgres.NewReadingRepository(pool)
	analyticsRepo := postgres.NewAnalyticsRepository(pool)
	aggregateRepo := admin.NewAggregateRepository(pool)

	locker := newRedisLocker(t)
	// R100(3): a month ~two years back is older than hourly (30d), daily
	// (90d) and monthly (1y)'s own policy horizons but NEWER than yearly's
	// (5y) — so this still exercises exactly the "closed month two years
	// back must still refresh" proof the fix wave asked to keep green;
	// Enqueuer is a no-op fake because the real Redis locker here never
	// contends (single caller, single lock acquisition).
	refresher, err := consumption.NewRefresher(consumption.RefreshDeps{
		Aggregates: aggregateRepo,
		Locker:     locker,
		Enqueuer:   &fakeEnqueuer{},
		Clock:      clock.System(),
		LockTTL:    2 * time.Minute,
		Location:   testIstanbul,
		Log:        discardLog(),
	})
	require.NoError(t, err)

	// A month about two years back, unambiguously closed relative to any
	// clock this test runs under — same fixture shape as Task 6's
	// TestRefreshMaterialisesAClosedMonth.
	monthStart := time.Date(2024, time.March, 1, 0, 0, 0, 0, testIstanbul)
	monthWindow := store.TimeRange{From: monthStart, To: monthStart.AddDate(0, 1, 0)}

	firstReadingAt := time.Date(2024, time.March, 10, 12, 0, 0, 0, testIstanbul)
	secondReadingAt := time.Date(2024, time.March, 20, 12, 0, 0, 0, testIstanbul)

	insertRefreshLoadProfileReading(t, ctx, readingRepo, tenant.Scope, analyzer.ID, firstReadingAt, "1000")
	insertRefreshLoadProfileReading(t, ctx, readingRepo, tenant.Scope, analyzer.ID, secondReadingAt, "1012.5")

	before, err := analyticsRepo.ConsumptionMonthly(ctx, tenant.AdminScope, []uuid.UUID{analyzer.ID}, monthWindow)
	require.NoError(t, err)
	require.Empty(t, before, "a closed month below consumption_monthly's watermark is absent, not stale")

	payload := job.ConsumptionRefreshPayload{
		CompanyID:  tenant.Company.ID,
		AnalyzerID: analyzer.ID,
		From:       firstReadingAt,
		To:         secondReadingAt.Add(time.Minute),
	}
	require.NoError(t, refresher.RefreshConsumption(ctx, payload))

	after, err := analyticsRepo.ConsumptionMonthly(ctx, tenant.AdminScope, []uuid.UUID{analyzer.ID}, monthWindow)
	require.NoError(t, err)
	require.Len(t, after, 1)
	require.True(t, decimal.RequireFromString("12.5").Equal(*after[0].ActiveConsumption),
		"want 12.5, got %s", after[0].ActiveConsumption.String())
}
