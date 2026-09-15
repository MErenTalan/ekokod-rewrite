//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

var anomaliesEpoch = time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

func anomaliesFixture(analyzerID uuid.UUID) model.ConsumptionAnomaly {
	return model.ConsumptionAnomaly{
		AnalyzerID:  analyzerID,
		PeriodStart: anomaliesEpoch,
		PeriodEnd:   anomaliesEpoch.Add(24 * time.Hour),
		Reason:      "negative_delta",
	}
}

func TestAnomalyCreateGetAndResolve(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	repo := postgres.NewAnomalyRepository(pool)
	analyzerID := tenant.Analyzers[0].ID

	created, err := repo.Create(ctx, tenant.Scope, anomaliesFixture(analyzerID))
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, created.ID)
	require.Nil(t, created.ResolvedAt)

	got, err := repo.Get(ctx, tenant.Scope, created.ID)
	require.NoError(t, err)
	require.Equal(t, created.ID, got.ID)

	resolvedBy := tenant.Users[model.UserRoleCompanyAdmin].ID
	at := anomaliesEpoch.Add(48 * time.Hour)
	resolved, err := repo.Resolve(ctx, tenant.Scope, created.ID, resolvedBy, "accepted", nil, at)
	require.NoError(t, err)
	require.NotNil(t, resolved.ResolvedAt)
	require.True(t, resolved.ResolvedAt.Equal(at))
	require.NotNil(t, resolved.ResolvedBy)
	require.Equal(t, resolvedBy, *resolved.ResolvedBy)
	require.NotNil(t, resolved.Resolution)
	require.Equal(t, "accepted", *resolved.Resolution)
}

func TestAnomalyListNarrowsByAnalyzerIDsAndUnresolved(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	repo := postgres.NewAnomalyRepository(pool)

	a0, err := repo.Create(ctx, tenant.Scope, anomaliesFixture(tenant.Analyzers[0].ID))
	require.NoError(t, err)
	_, err = repo.Create(ctx, tenant.Scope, anomaliesFixture(tenant.Analyzers[1].ID))
	require.NoError(t, err)

	resolvedBy := tenant.Users[model.UserRoleCompanyAdmin].ID
	_, err = repo.Resolve(ctx, tenant.Scope, a0.ID, resolvedBy, "accepted", nil, anomaliesEpoch)
	require.NoError(t, err)

	all, err := repo.List(ctx, tenant.Scope, store.AnomalyFilter{})
	require.NoError(t, err)
	require.Len(t, all, 2)

	unresolved, err := repo.List(ctx, tenant.Scope, store.AnomalyFilter{Unresolved: true})
	require.NoError(t, err)
	require.Len(t, unresolved, 1)

	narrowed, err := repo.List(ctx, tenant.Scope, store.AnomalyFilter{AnalyzerIDs: []uuid.UUID{tenant.Analyzers[1].ID}})
	require.NoError(t, err)
	require.Len(t, narrowed, 1)
	require.Equal(t, tenant.Analyzers[1].ID, narrowed[0].AnalyzerID)
}

// --- scope / range validation ----------------------------------------------

func TestAnomalyRepositoryRejectsInvalidScope(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	repo := postgres.NewAnomalyRepository(pool)
	invalid := store.Scope{}

	_, err := repo.Get(ctx, invalid, uuid.New())
	require.ErrorIs(t, err, store.ErrInvalidScope)

	_, err = repo.List(ctx, invalid, store.AnomalyFilter{})
	require.ErrorIs(t, err, store.ErrInvalidScope)

	_, err = repo.Create(ctx, invalid, anomaliesFixture(uuid.New()))
	require.ErrorIs(t, err, store.ErrInvalidScope)

	_, err = repo.Resolve(ctx, invalid, uuid.New(), uuid.New(), "accepted", nil, anomaliesEpoch)
	require.ErrorIs(t, err, store.ErrInvalidScope)
}

func TestAnomalyListRejectsInvalidRange(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	repo := postgres.NewAnomalyRepository(pool)

	invalidRange := &store.TimeRange{}
	_, err := repo.List(ctx, tenant.Scope, store.AnomalyFilter{Range: invalidRange})
	require.ErrorIs(t, err, store.ErrInvalidRange)
}

// --- isolation: consumption_anomalies has no company_id, joins through
// analyzers ------------------------------------------------------------------

func TestAnomalyCreateRefusesInvisibleAnalyzer(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenantA := testfixtures.NewTenant(t, ctx, pool, 1)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 2)
	repo := postgres.NewAnomalyRepository(pool)

	_, err := repo.Create(ctx, tenantA.Scope, anomaliesFixture(tenantB.Analyzers[0].ID))
	require.ErrorIs(t, err, store.ErrNotFound)

	// Narrow scope: tenant A's own analyzer, outside Buildings[0].
	_, err = repo.Create(ctx, tenantA.Scope, anomaliesFixture(tenantA.Analyzers[2].ID))
	require.ErrorIs(t, err, store.ErrNotFound)
}

func TestAnomalyGetIsIsolatedToVisibleAnalyzers(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenantA := testfixtures.NewTenant(t, ctx, pool, 1)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 2)
	repo := postgres.NewAnomalyRepository(pool)

	foreign, err := repo.Create(ctx, tenantB.Scope, anomaliesFixture(tenantB.Analyzers[0].ID))
	require.NoError(t, err)

	_, err = repo.Get(ctx, tenantA.Scope, foreign.ID)
	require.ErrorIs(t, err, store.ErrNotFound)
}

func TestAnomalyListIDsOutsideScopeContributeNoRows(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenantA := testfixtures.NewTenant(t, ctx, pool, 1)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 2)
	repo := postgres.NewAnomalyRepository(pool)

	_, err := repo.Create(ctx, tenantB.Scope, anomaliesFixture(tenantB.Analyzers[0].ID))
	require.NoError(t, err)

	got, err := repo.List(ctx, tenantA.Scope, store.AnomalyFilter{AnalyzerIDs: []uuid.UUID{tenantB.Analyzers[0].ID}})
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestAnomalyResolveRequiresResolverFromTheSameCompany(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenantA := testfixtures.NewTenant(t, ctx, pool, 1)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 2)
	repo := postgres.NewAnomalyRepository(pool)

	created, err := repo.Create(ctx, tenantA.Scope, anomaliesFixture(tenantA.Analyzers[0].ID))
	require.NoError(t, err)

	foreignUser := tenantB.Users[model.UserRoleCompanyAdmin].ID
	_, err = repo.Resolve(ctx, tenantA.Scope, created.ID, foreignUser, "accepted", nil, anomaliesEpoch)
	require.ErrorIs(t, err, store.ErrNotFound)

	// The anomaly must remain unresolved: the write touched nothing.
	got, err := repo.Get(ctx, tenantA.Scope, created.ID)
	require.NoError(t, err)
	require.Nil(t, got.ResolvedAt)
}
