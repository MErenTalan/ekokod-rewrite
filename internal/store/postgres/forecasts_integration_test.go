//go:build integration

package postgres_test

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
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

var forecastsEpoch = time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

func forecastsRow(analyzerID uuid.UUID, ts, generatedAt time.Time, median string) model.Forecast {
	return model.Forecast{
		AnalyzerID:   analyzerID,
		Ts:           ts,
		GeneratedAt:  generatedAt,
		HorizonHours: 24,
		Median:       decimal.RequireFromString(median),
		ModelID:      "test-model",
	}
}

func TestForecastBulkInsertIsIdempotent(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	repo := postgres.NewForecastRepository(pool)
	analyzerID := tenant.Analyzers[0].ID

	rows := []model.Forecast{forecastsRow(analyzerID, forecastsEpoch, forecastsEpoch, "10.0000")}
	inserted, updated, err := repo.BulkInsert(ctx, tenant.Scope, rows)
	require.NoError(t, err)
	require.Equal(t, 1, inserted)
	require.Equal(t, 0, updated)

	inserted, updated, err = repo.BulkInsert(ctx, tenant.Scope, rows)
	require.NoError(t, err)
	require.Equal(t, 0, inserted)
	require.Equal(t, 1, updated)
}

func TestForecastSuccessiveRunsAccumulate(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	repo := postgres.NewForecastRepository(pool)
	analyzerID := tenant.Analyzers[0].ID

	run1 := forecastsEpoch
	run2 := forecastsEpoch.Add(time.Hour)

	_, _, err := repo.BulkInsert(ctx, tenant.Scope, []model.Forecast{forecastsRow(analyzerID, forecastsEpoch, run1, "10.0000")})
	require.NoError(t, err)
	_, _, err = repo.BulkInsert(ctx, tenant.Scope, []model.Forecast{forecastsRow(analyzerID, forecastsEpoch, run2, "20.0000")})
	require.NoError(t, err)

	// generated_at is part of the key: the second run ACCUMULATES rather
	// than overwriting the first.
	all, err := repo.Range(ctx, tenant.Scope, analyzerID, store.TimeRange{From: forecastsEpoch, To: forecastsEpoch.Add(time.Hour)})
	require.NoError(t, err)
	require.Len(t, all, 2)

	latest, err := repo.LatestRun(ctx, tenant.Scope, analyzerID, store.TimeRange{From: forecastsEpoch, To: forecastsEpoch.Add(time.Hour)})
	require.NoError(t, err)
	require.Len(t, latest, 1)
	require.True(t, latest[0].GeneratedAt.Equal(run2))
	require.True(t, decimal.RequireFromString("20.0000").Equal(latest[0].Median))
}

func TestForecastLatestRunEmptyWhenNoneInWindow(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	repo := postgres.NewForecastRepository(pool)

	got, err := repo.LatestRun(ctx, tenant.Scope, tenant.Analyzers[0].ID, store.TimeRange{From: forecastsEpoch, To: forecastsEpoch.Add(time.Hour)})
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestForecastRecordGapsAndGaps(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	repo := postgres.NewForecastRepository(pool)
	analyzerID := tenant.Analyzers[0].ID

	gaps := []model.ForecastGap{{
		AnalyzerID:   analyzerID,
		GeneratedAt:  forecastsEpoch,
		GapStart:     forecastsEpoch,
		GapEnd:       forecastsEpoch.Add(3 * time.Hour),
		MissingHours: 3,
	}}
	require.NoError(t, repo.RecordGaps(ctx, tenant.Scope, gaps))

	got, err := repo.Gaps(ctx, tenant.Scope, analyzerID, forecastsEpoch)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, int32(3), got[0].MissingHours)
	require.NotEqual(t, uuid.Nil, got[0].ID)
}

// --- scope / range validation ----------------------------------------------

func TestForecastRepositoryRejectsInvalidScope(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	repo := postgres.NewForecastRepository(pool)
	invalid := store.Scope{}
	analyzerID := uuid.New()
	validRange := store.TimeRange{From: forecastsEpoch, To: forecastsEpoch.Add(time.Hour)}

	_, _, err := repo.BulkInsert(ctx, invalid, []model.Forecast{forecastsRow(analyzerID, forecastsEpoch, forecastsEpoch, "1")})
	require.ErrorIs(t, err, store.ErrInvalidScope)

	_, err = repo.Range(ctx, invalid, analyzerID, validRange)
	require.ErrorIs(t, err, store.ErrInvalidScope)

	_, err = repo.LatestRun(ctx, invalid, analyzerID, validRange)
	require.ErrorIs(t, err, store.ErrInvalidScope)

	err = repo.RecordGaps(ctx, invalid, []model.ForecastGap{{AnalyzerID: analyzerID, GeneratedAt: forecastsEpoch, GapStart: forecastsEpoch, GapEnd: forecastsEpoch, MissingHours: 1}})
	require.ErrorIs(t, err, store.ErrInvalidScope)

	_, err = repo.Gaps(ctx, invalid, analyzerID, forecastsEpoch)
	require.ErrorIs(t, err, store.ErrInvalidScope)
}

func TestForecastRepositoryRejectsInvalidRange(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	repo := postgres.NewForecastRepository(pool)
	invalidRange := store.TimeRange{}

	_, err := repo.Range(ctx, tenant.Scope, tenant.Analyzers[0].ID, invalidRange)
	require.ErrorIs(t, err, store.ErrInvalidRange)

	_, err = repo.LatestRun(ctx, tenant.Scope, tenant.Analyzers[0].ID, invalidRange)
	require.ErrorIs(t, err, store.ErrInvalidRange)
}

// --- isolation: forecasts and forecast_gaps have no company_id, join
// through analyzers ---------------------------------------------------------

func TestForecastBulkInsertRefusesWholeBatchWhenAnalyzerNotVisible(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenantA := testfixtures.NewTenant(t, ctx, pool, 1)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 2)
	repo := postgres.NewForecastRepository(pool)

	own := tenantA.Analyzers[0].ID
	foreign := tenantB.Analyzers[0].ID
	rows := []model.Forecast{
		forecastsRow(own, forecastsEpoch, forecastsEpoch, "1"),
		forecastsRow(foreign, forecastsEpoch, forecastsEpoch, "2"),
	}
	_, _, err := repo.BulkInsert(ctx, tenantA.Scope, rows)
	require.ErrorIs(t, err, store.ErrNotFound)

	got, err := repo.Range(ctx, tenantA.Scope, own, store.TimeRange{From: forecastsEpoch, To: forecastsEpoch.Add(time.Hour)})
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestForecastRangeAndLatestRunAnalyzerNotVisibleReturnsNotFound(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenantA := testfixtures.NewTenant(t, ctx, pool, 1)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 2)
	repo := postgres.NewForecastRepository(pool)
	validRange := store.TimeRange{From: forecastsEpoch, To: forecastsEpoch.Add(time.Hour)}

	_, err := repo.Range(ctx, tenantA.Scope, tenantB.Analyzers[0].ID, validRange)
	require.ErrorIs(t, err, store.ErrNotFound)

	_, err = repo.LatestRun(ctx, tenantA.Scope, tenantB.Analyzers[0].ID, validRange)
	require.ErrorIs(t, err, store.ErrNotFound)
}

func TestForecastRecordGapsRefusesWholeCallWhenAnalyzerNotVisible(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenantA := testfixtures.NewTenant(t, ctx, pool, 1)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 2)
	repo := postgres.NewForecastRepository(pool)

	own := tenantA.Analyzers[0].ID
	foreign := tenantB.Analyzers[0].ID
	gaps := []model.ForecastGap{
		{AnalyzerID: own, GeneratedAt: forecastsEpoch, GapStart: forecastsEpoch, GapEnd: forecastsEpoch.Add(time.Hour), MissingHours: 1},
		{AnalyzerID: foreign, GeneratedAt: forecastsEpoch, GapStart: forecastsEpoch, GapEnd: forecastsEpoch.Add(time.Hour), MissingHours: 1},
	}
	err := repo.RecordGaps(ctx, tenantA.Scope, gaps)
	require.ErrorIs(t, err, store.ErrNotFound)

	got, err := repo.Gaps(ctx, tenantA.Scope, own, forecastsEpoch)
	require.NoError(t, err)
	require.Empty(t, got, "the whole call must be refused: even the gap naming the caller's own analyzer")
}

func TestForecastGapsAnalyzerNotVisibleReturnsNotFound(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenantA := testfixtures.NewTenant(t, ctx, pool, 1)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 2)
	repo := postgres.NewForecastRepository(pool)

	_, err := repo.Gaps(ctx, tenantA.Scope, tenantB.Analyzers[0].ID, forecastsEpoch)
	require.ErrorIs(t, err, store.ErrNotFound)
}
