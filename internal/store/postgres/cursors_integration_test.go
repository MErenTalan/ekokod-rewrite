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

// TestCursorListEmptyIDsReturnsNothingAndNonEmptyNarrows proves
// CursorRepository.List's analyzerIDs is a REQUIRED POSITIONAL parameter
// (controller ruling, task-10 fix round 1): nil or an empty slice means NO
// ROWS, fail-closed, identical to Scope.BuildingIDs — never "every analyzer
// visible to the Scope". A non-empty list still narrows further, and ids
// outside the Scope still contribute no rows (covered separately by
// TestCursorListIDsOutsideScopeContributeNoRows).
func TestCursorListEmptyIDsReturnsNothingAndNonEmptyNarrows(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	repo := postgres.NewCursorRepository(pool)

	for _, a := range tenant.Analyzers[:2] { // both under Buildings[0]
		require.NoError(t, repo.RecordSuccess(ctx, tenant.Scope, a.ID, model.ReadingKindLoadProfile, cursorsEpoch, cursorsEpoch))
	}

	nilIDs, err := repo.List(ctx, tenant.Scope, nil)
	require.NoError(t, err)
	require.Empty(t, nilIDs, "nil analyzerIDs must return NO rows, not every visible one")

	emptyIDs, err := repo.List(ctx, tenant.Scope, []uuid.UUID{})
	require.NoError(t, err)
	require.Empty(t, emptyIDs, "an empty (non-nil) analyzerIDs must also return NO rows")

	narrowed, err := repo.List(ctx, tenant.Scope, []uuid.UUID{tenant.Analyzers[0].ID})
	require.NoError(t, err)
	require.Len(t, narrowed, 1)
	require.Equal(t, tenant.Analyzers[0].ID, narrowed[0].AnalyzerID)

	both, err := repo.List(ctx, tenant.Scope, []uuid.UUID{tenant.Analyzers[0].ID, tenant.Analyzers[1].ID})
	require.NoError(t, err)
	require.Len(t, both, 2)
}

// TestCursorRecordSuccessLastTsNeverMovesBackwards proves the folded-minor
// fix to CursorRecordSuccess (`greatest(ingestion_cursors.last_ts,
// excluded.last_ts)`): an out-of-order success call, whose lastTs is BEHIND
// what is already stored, must not regress the high-water mark.
func TestCursorRecordSuccessLastTsNeverMovesBackwards(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	repo := postgres.NewCursorRepository(pool)
	analyzerID := tenant.Analyzers[0].ID

	later := cursorsEpoch.Add(24 * time.Hour)
	require.NoError(t, repo.RecordSuccess(ctx, tenant.Scope, analyzerID, model.ReadingKindLoadProfile, later, cursorsEpoch))

	got, err := repo.Get(ctx, tenant.Scope, analyzerID, model.ReadingKindLoadProfile)
	require.NoError(t, err)
	require.True(t, got.LastTs.Equal(later))

	// An out-of-order call reports an EARLIER lastTs (e.g. a delayed retry of
	// an older ingestion window landing after a newer one already succeeded).
	require.NoError(t, repo.RecordSuccess(ctx, tenant.Scope, analyzerID, model.ReadingKindLoadProfile, cursorsEpoch, cursorsEpoch.Add(time.Minute)))

	got, err = repo.Get(ctx, tenant.Scope, analyzerID, model.ReadingKindLoadProfile)
	require.NoError(t, err)
	require.True(t, got.LastTs.Equal(later), "last_ts must stay at the later instant, never regress")
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

// TestCursorListIDsOutsideScopeContributeNoRows is List's isolation proof
// (F1 final review pass B, I3): a positive control (tenant B lists its own
// cursor) and a cross-tenant assertion called with tenant A's AdminScope,
// not the narrow Scope — under a narrow Scope the building predicate alone
// already excludes tenant B's analyzer, so a tautologised company_id
// predicate in CursorList would hide behind it and this test would still
// pass.
func TestCursorListIDsOutsideScopeContributeNoRows(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenantA := testfixtures.NewTenant(t, ctx, pool, 1)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 2)
	repo := postgres.NewCursorRepository(pool)

	require.NoError(t, repo.RecordSuccess(ctx, tenantB.Scope, tenantB.Analyzers[0].ID, model.ReadingKindLoadProfile, cursorsEpoch, cursorsEpoch))

	// Self-evident non-vacuity: tenant B can list its own cursor.
	selfB, err := repo.List(ctx, tenantB.Scope, []uuid.UUID{tenantB.Analyzers[0].ID})
	require.NoError(t, err)
	require.Len(t, selfB, 1)

	// Cross-tenant, called with tenant A's AdminScope.
	got, err := repo.List(ctx, tenantA.AdminScope, []uuid.UUID{tenantB.Analyzers[0].ID})
	require.NoError(t, err)
	require.Empty(t, got)
}

// TestCursorRecordSuccessAndFailureRefuseInvisibleAnalyzer is the write
// isolation proof for both cursor writes (I3). The cross-tenant assertion is
// called with tenant A's AdminScope, not the narrow Scope: under a narrow
// Scope the building predicate alone already excludes tenant B's analyzer,
// so a tautologised company_id predicate in CursorRecordSuccess /
// CursorRecordFailure would hide behind it and this test would still pass.
// AdminScope removes that cover, leaving only the company_id check standing
// between tenant A and tenant B's analyzer — and "nothing changed" is
// proved directly, by reading the row back through tenant B's own Scope.
func TestCursorRecordSuccessAndFailureRefuseInvisibleAnalyzer(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenantA := testfixtures.NewTenant(t, ctx, pool, 1)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 2)
	repo := postgres.NewCursorRepository(pool)

	err := repo.RecordSuccess(ctx, tenantA.AdminScope, tenantB.Analyzers[0].ID, model.ReadingKindLoadProfile, cursorsEpoch, cursorsEpoch)
	require.ErrorIs(t, err, store.ErrNotFound)

	err = repo.RecordFailure(ctx, tenantA.AdminScope, tenantB.Analyzers[0].ID, model.ReadingKindLoadProfile, "x", cursorsEpoch)
	require.ErrorIs(t, err, store.ErrNotFound)

	// Nothing was created for tenant B either — the write touched no row.
	_, err = repo.Get(ctx, tenantB.Scope, tenantB.Analyzers[0].ID, model.ReadingKindLoadProfile)
	require.ErrorIs(t, err, store.ErrNotFound)

	// Narrow scope: tenant A's own analyzer, outside Buildings[0], is also
	// refused under the narrow Scope...
	err = repo.RecordSuccess(ctx, tenantA.Scope, tenantA.Analyzers[2].ID, model.ReadingKindLoadProfile, cursorsEpoch, cursorsEpoch)
	require.ErrorIs(t, err, store.ErrNotFound)

	// ...and reachable under AdminScope — confirming the ErrNotFound above is
	// the Scope's doing, not a fixture mistake.
	require.NoError(t, repo.RecordSuccess(ctx, tenantA.AdminScope, tenantA.Analyzers[2].ID, model.ReadingKindLoadProfile, cursorsEpoch, cursorsEpoch))
	got, err := repo.Get(ctx, tenantA.AdminScope, tenantA.Analyzers[2].ID, model.ReadingKindLoadProfile)
	require.NoError(t, err)
	require.NotNil(t, got.LastTs)
}
