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

var cursorsEpoch = time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

func TestCursorRecordSuccessThenGet(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	repo := postgres.NewCursorRepository(pool)
	analyzerID := tenant.Analyzers[0].ID

	err := repo.RecordSuccess(ctx, tenant.Scope, analyzerID, model.ReadingKindLoadProfile, cursorsEpoch, cursorsEpoch.Add(time.Minute))
	require.NoError(t, err)

	got, err := repo.Get(ctx, tenant.Scope, analyzerID, model.ReadingKindLoadProfile)
	require.NoError(t, err)
	require.NotNil(t, got.LastTs)
	require.True(t, got.LastTs.Equal(cursorsEpoch))
	require.NotNil(t, got.LastSuccessAt)
	require.Zero(t, got.ConsecutiveFailures)
}

func TestCursorRecordFailureIncrementsConsecutiveFailures(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	repo := postgres.NewCursorRepository(pool)
	analyzerID := tenant.Analyzers[0].ID

	require.NoError(t, repo.RecordFailure(ctx, tenant.Scope, analyzerID, model.ReadingKindLoadProfile, "boom 1", cursorsEpoch))
	require.NoError(t, repo.RecordFailure(ctx, tenant.Scope, analyzerID, model.ReadingKindLoadProfile, "boom 2", cursorsEpoch.Add(time.Minute)))

	got, err := repo.Get(ctx, tenant.Scope, analyzerID, model.ReadingKindLoadProfile)
	require.NoError(t, err)
	require.EqualValues(t, 2, got.ConsecutiveFailures)
	require.NotNil(t, got.LastError)
	require.Equal(t, "boom 2", *got.LastError)

	// A subsequent success resets the counter — it is part of recording a
	// success, not a separate call a caller can forget.
	require.NoError(t, repo.RecordSuccess(ctx, tenant.Scope, analyzerID, model.ReadingKindLoadProfile, cursorsEpoch, cursorsEpoch.Add(2*time.Minute)))
	got, err = repo.Get(ctx, tenant.Scope, analyzerID, model.ReadingKindLoadProfile)
	require.NoError(t, err)
	require.Zero(t, got.ConsecutiveFailures)
}

func TestCursorGetWithNoCursorYetReturnsNotFound(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	repo := postgres.NewCursorRepository(pool)

	_, err := repo.Get(ctx, tenant.Scope, tenant.Analyzers[0].ID, model.ReadingKindLoadProfile)
	require.ErrorIs(t, err, store.ErrNotFound)
}

func TestCursorListNarrowsByAnalyzerIDsAndDefaultsToEveryVisibleAnalyzer(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	repo := postgres.NewCursorRepository(pool)

	for _, a := range tenant.Analyzers[:2] { // both under Buildings[0]
		require.NoError(t, repo.RecordSuccess(ctx, tenant.Scope, a.ID, model.ReadingKindLoadProfile, cursorsEpoch, cursorsEpoch))
	}

	all, err := repo.List(ctx, tenant.Scope, nil)
	require.NoError(t, err)
	require.Len(t, all, 2)

	narrowed, err := repo.List(ctx, tenant.Scope, []uuid.UUID{tenant.Analyzers[0].ID})
	require.NoError(t, err)
	require.Len(t, narrowed, 1)
	require.Equal(t, tenant.Analyzers[0].ID, narrowed[0].AnalyzerID)
}

// --- scope validation -------------------------------------------------------

func TestCursorRepositoryRejectsInvalidScope(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	repo := postgres.NewCursorRepository(pool)
	invalid := store.Scope{}
	analyzerID := uuid.New()

	_, err := repo.Get(ctx, invalid, analyzerID, model.ReadingKindLoadProfile)
	require.ErrorIs(t, err, store.ErrInvalidScope)

	_, err = repo.List(ctx, invalid, nil)
	require.ErrorIs(t, err, store.ErrInvalidScope)

	err = repo.RecordSuccess(ctx, invalid, analyzerID, model.ReadingKindLoadProfile, cursorsEpoch, cursorsEpoch)
	require.ErrorIs(t, err, store.ErrInvalidScope)

	err = repo.RecordFailure(ctx, invalid, analyzerID, model.ReadingKindLoadProfile, "x", cursorsEpoch)
	require.ErrorIs(t, err, store.ErrInvalidScope)
}

// --- isolation: ingestion_cursors has no company_id, joins through analyzers

func TestCursorGetIsIsolatedToVisibleAnalyzers(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenantA := testfixtures.NewTenant(t, ctx, pool, 1)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 2)
	repo := postgres.NewCursorRepository(pool)

	require.NoError(t, repo.RecordSuccess(ctx, tenantB.Scope, tenantB.Analyzers[0].ID, model.ReadingKindLoadProfile, cursorsEpoch, cursorsEpoch))

	// Cross-tenant.
	_, err := repo.Get(ctx, tenantA.Scope, tenantB.Analyzers[0].ID, model.ReadingKindLoadProfile)
	require.ErrorIs(t, err, store.ErrNotFound)

	// Narrow scope: tenant A's own analyzer, outside Buildings[0].
	require.NoError(t, repo.RecordSuccess(ctx, tenantA.AdminScope, tenantA.Analyzers[2].ID, model.ReadingKindLoadProfile, cursorsEpoch, cursorsEpoch))
	_, err = repo.Get(ctx, tenantA.Scope, tenantA.Analyzers[2].ID, model.ReadingKindLoadProfile)
	require.ErrorIs(t, err, store.ErrNotFound)

	got, err := repo.Get(ctx, tenantA.AdminScope, tenantA.Analyzers[2].ID, model.ReadingKindLoadProfile)
	require.NoError(t, err)
	require.NotNil(t, got.LastTs)
}

func TestCursorListIDsOutsideScopeContributeNoRows(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenantA := testfixtures.NewTenant(t, ctx, pool, 1)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 2)
	repo := postgres.NewCursorRepository(pool)

	require.NoError(t, repo.RecordSuccess(ctx, tenantB.Scope, tenantB.Analyzers[0].ID, model.ReadingKindLoadProfile, cursorsEpoch, cursorsEpoch))

	got, err := repo.List(ctx, tenantA.Scope, []uuid.UUID{tenantB.Analyzers[0].ID})
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestCursorRecordSuccessAndFailureRefuseInvisibleAnalyzer(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenantA := testfixtures.NewTenant(t, ctx, pool, 1)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 2)
	repo := postgres.NewCursorRepository(pool)

	err := repo.RecordSuccess(ctx, tenantA.Scope, tenantB.Analyzers[0].ID, model.ReadingKindLoadProfile, cursorsEpoch, cursorsEpoch)
	require.ErrorIs(t, err, store.ErrNotFound)

	err = repo.RecordFailure(ctx, tenantA.Scope, tenantB.Analyzers[0].ID, model.ReadingKindLoadProfile, "x", cursorsEpoch)
	require.ErrorIs(t, err, store.ErrNotFound)

	// Nothing was created for tenant B either — the write touched no row.
	_, err = repo.Get(ctx, tenantB.Scope, tenantB.Analyzers[0].ID, model.ReadingKindLoadProfile)
	require.ErrorIs(t, err, store.ErrNotFound)
}
