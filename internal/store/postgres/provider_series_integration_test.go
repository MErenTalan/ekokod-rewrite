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

var f2seriesEpoch = time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC)

// f2seriesDecPtr parses a literal into a *decimal.Decimal.
func f2seriesDecPtr(s string) *decimal.Decimal {
	d := decimal.RequireFromString(s)
	return &d
}

// f2seriesRow builds one fixture ProviderHourlyValue for analyzerID at ts.
func f2seriesRow(analyzerID uuid.UUID, ts time.Time, consumption string) model.ProviderHourlyValue {
	return model.ProviderHourlyValue{
		AnalyzerID:        analyzerID,
		Ts:                ts,
		ActiveConsumption: f2seriesDecPtr(consumption),
		SourceProvider:    model.IntegrationProviderOSOS,
	}
}

func TestProviderHourlyUpsertIsIdempotent(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	repo := postgres.NewProviderSeriesRepository(pool)
	analyzerID := tenant.Analyzers[0].ID

	rows := []model.ProviderHourlyValue{
		f2seriesRow(analyzerID, f2seriesEpoch, "100.0000"),
		f2seriesRow(analyzerID, f2seriesEpoch.Add(time.Hour), "200.0000"),
	}

	inserted, updated, err := repo.UpsertHourly(ctx, tenant.Scope, rows)
	require.NoError(t, err)
	require.Equal(t, 2, inserted)
	require.Equal(t, 0, updated)

	// Re-ingesting the SAME rows must update, never duplicate — the count is
	// unchanged.
	inserted, updated, err = repo.UpsertHourly(ctx, tenant.Scope, rows)
	require.NoError(t, err)
	require.Equal(t, 0, inserted)
	require.Equal(t, 2, updated)

	got, err := repo.HourlyRange(ctx, tenant.Scope, analyzerID,
		store.TimeRange{From: f2seriesEpoch, To: f2seriesEpoch.Add(2 * time.Hour)})
	require.NoError(t, err)
	require.Len(t, got, 2)
}

// TestProviderHourlyRefusesDuplicateKeysInBatch pins the Global Constraints
// "batches are all-or-nothing, duplicate keys refused before the DB" rule.
func TestProviderHourlyRefusesDuplicateKeysInBatch(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	repo := postgres.NewProviderSeriesRepository(pool)
	analyzerID := tenant.Analyzers[0].ID

	rows := []model.ProviderHourlyValue{
		f2seriesRow(analyzerID, f2seriesEpoch, "100.0000"),
		f2seriesRow(analyzerID, f2seriesEpoch, "999.0000"), // same (analyzer_id, ts)
	}

	inserted, updated, err := repo.UpsertHourly(ctx, tenant.Scope, rows)
	require.ErrorIs(t, err, store.ErrConflict)
	require.Zero(t, inserted)
	require.Zero(t, updated)

	// Nothing was written.
	got, err := repo.HourlyRange(ctx, tenant.Scope, analyzerID,
		store.TimeRange{From: f2seriesEpoch, To: f2seriesEpoch.Add(time.Hour)})
	require.NoError(t, err)
	require.Empty(t, got)
}

// TestProviderHourlyAllOrNothingOnInvisibleAnalyzer proves the batch's
// visibility check is evaluated against the DISTINCT set of the batch's
// analyzers, using tenant B's AdminScope on tenant A's analyzer (F1 binding
// ruling 4: a narrow Scope would let the building predicate hide a broken
// company_id predicate). A positive control shows the OWNER's own write of
// the same shape succeeds, so the refusal above is not vacuous.
func TestProviderHourlyAllOrNothingOnInvisibleAnalyzer(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenantA := testfixtures.NewTenant(t, ctx, pool, 1)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 2)
	repo := postgres.NewProviderSeriesRepository(pool)

	analyzerID := tenantA.Analyzers[0].ID
	rows := []model.ProviderHourlyValue{f2seriesRow(analyzerID, f2seriesEpoch, "100.0000")}

	// Positive control: tenant A can write its own analyzer.
	inserted, updated, err := repo.UpsertHourly(ctx, tenantA.Scope, rows)
	require.NoError(t, err)
	require.Equal(t, 1, inserted)
	require.Zero(t, updated)

	// Tenant B, with its own AdminScope, must not be able to write onto
	// tenant A's analyzer.
	otherRows := []model.ProviderHourlyValue{f2seriesRow(analyzerID, f2seriesEpoch.Add(time.Hour), "500.0000")}
	inserted, updated, err = repo.UpsertHourly(ctx, tenantB.AdminScope, otherRows)
	require.ErrorIs(t, err, store.ErrNotFound)
	require.Zero(t, inserted)
	require.Zero(t, updated)

	// Nothing from tenant B's refused batch was written, even under tenant
	// A's own scope.
	got, err := repo.HourlyRange(ctx, tenantA.Scope, analyzerID,
		store.TimeRange{From: f2seriesEpoch, To: f2seriesEpoch.Add(2 * time.Hour)})
	require.NoError(t, err)
	require.Len(t, got, 1, "only the positive control's row may exist")
}

// TestProviderHourlyAllOrNothingMixedBatch proves a batch mixing one visible
// and one invisible analyzer is refused WHOLESALE — the visible analyzer's
// row is not written either.
func TestProviderHourlyAllOrNothingMixedBatch(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenantA := testfixtures.NewTenant(t, ctx, pool, 1)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 2)
	repo := postgres.NewProviderSeriesRepository(pool)

	mixed := []model.ProviderHourlyValue{
		f2seriesRow(tenantA.Analyzers[0].ID, f2seriesEpoch, "100.0000"),
		f2seriesRow(tenantB.Analyzers[0].ID, f2seriesEpoch, "200.0000"),
	}

	_, _, err := repo.UpsertHourly(ctx, tenantA.Scope, mixed)
	require.ErrorIs(t, err, store.ErrNotFound)

	got, err := repo.HourlyRange(ctx, tenantA.Scope, tenantA.Analyzers[0].ID,
		store.TimeRange{From: f2seriesEpoch, To: f2seriesEpoch.Add(time.Hour)})
	require.NoError(t, err)
	require.Empty(t, got, "tenant A's OWN visible row must not have been written either")
}

func TestProviderHourlyRangeRejectsInvalidRange(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	repo := postgres.NewProviderSeriesRepository(pool)

	_, err := repo.HourlyRange(ctx, tenant.Scope, tenant.Analyzers[0].ID, store.TimeRange{})
	require.ErrorIs(t, err, store.ErrInvalidRange)

	_, err = repo.HourlyRange(ctx, tenant.Scope, tenant.Analyzers[0].ID,
		store.TimeRange{From: f2seriesEpoch.Add(time.Hour), To: f2seriesEpoch})
	require.ErrorIs(t, err, store.ErrInvalidRange, "From must be strictly before To")
}

// TestProviderHourlyRangeIsScoped is HourlyRange's own isolation proof,
// using tenant B's AdminScope on tenant A's analyzer, with a positive
// control.
func TestProviderHourlyRangeIsScoped(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenantA := testfixtures.NewTenant(t, ctx, pool, 1)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 2)
	repo := postgres.NewProviderSeriesRepository(pool)

	analyzerID := tenantA.Analyzers[0].ID
	_, _, err := repo.UpsertHourly(ctx, tenantA.Scope, []model.ProviderHourlyValue{
		f2seriesRow(analyzerID, f2seriesEpoch, "100.0000"),
	})
	require.NoError(t, err)

	got, err := repo.HourlyRange(ctx, tenantA.Scope, analyzerID,
		store.TimeRange{From: f2seriesEpoch, To: f2seriesEpoch.Add(time.Hour)})
	require.NoError(t, err)
	require.Len(t, got, 1)

	_, err = repo.HourlyRange(ctx, tenantB.AdminScope, analyzerID,
		store.TimeRange{From: f2seriesEpoch, To: f2seriesEpoch.Add(time.Hour)})
	require.ErrorIs(t, err, store.ErrNotFound)
}

func TestProviderHourlyInvalidScopeIsRejected(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	repo := postgres.NewProviderSeriesRepository(pool)

	_, _, err := repo.UpsertHourly(ctx, store.Scope{}, []model.ProviderHourlyValue{{}})
	require.ErrorIs(t, err, store.ErrInvalidScope)

	_, err = repo.HourlyRange(ctx, store.Scope{}, uuid.New(), store.TimeRange{From: f2seriesEpoch, To: f2seriesEpoch.Add(time.Hour)})
	require.ErrorIs(t, err, store.ErrInvalidScope)
}
