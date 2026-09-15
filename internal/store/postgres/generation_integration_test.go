//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

var f2genEpoch = time.Date(2026, time.February, 1, 0, 0, 0, 0, time.UTC)

func TestGenerationAnchorRoundTrip(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	repo := postgres.NewGenerationRepository(pool)

	analyzerID := tenant.Analyzers[0].ID
	anchor := model.GenerationAnchor{
		AnalyzerID:   analyzerID,
		AnchorTs:     f2genEpoch,
		ActiveExport: decimal.RequireFromString("1234.5678"),
		Source:       "initial",
	}

	err := repo.SetAnchor(ctx, tenant.Scope, anchor)
	require.NoError(t, err)

	got, err := repo.Anchor(ctx, tenant.Scope, analyzerID)
	require.NoError(t, err)
	require.Equal(t, analyzerID, got.AnalyzerID)
	require.True(t, got.AnchorTs.Equal(f2genEpoch), "anchor_ts must round-trip")
	require.True(t, got.ActiveExport.Equal(anchor.ActiveExport),
		"active_export: want %s got %s", anchor.ActiveExport, got.ActiveExport)
	require.Equal(t, "initial", got.Source)
	require.False(t, got.UpdatedAt.IsZero())

	// SetAnchor upserts on analyzer_id: a second call REPLACES, never
	// accumulates a second row.
	second := model.GenerationAnchor{
		AnalyzerID:   analyzerID,
		AnchorTs:     f2genEpoch.Add(24 * time.Hour),
		ActiveExport: decimal.RequireFromString("2000.0000"),
		Source:       "operator",
	}
	require.NoError(t, repo.SetAnchor(ctx, tenant.Scope, second))

	got, err = repo.Anchor(ctx, tenant.Scope, analyzerID)
	require.NoError(t, err)
	require.True(t, got.AnchorTs.Equal(second.AnchorTs))
	require.True(t, got.ActiveExport.Equal(second.ActiveExport))
	require.Equal(t, "operator", got.Source)
}

// TestGenerationAnchorNoAnchorYetIsNotFound is the positive control's
// complement: a visible analyzer with no anchor written yet is ErrNotFound,
// never a zero-value anchor silently returned.
func TestGenerationAnchorNoAnchorYetIsNotFound(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	repo := postgres.NewGenerationRepository(pool)

	_, err := repo.Anchor(ctx, tenant.Scope, tenant.Analyzers[0].ID)
	require.ErrorIs(t, err, store.ErrNotFound)
}

// TestGenerationAnchorIsScoped proves tenant B cannot reach tenant A's
// analyzer through EITHER method, using tenant B's AdminScope (the whole
// company) — a narrow Scope would let the building predicate hide a broken
// company_id predicate (F1 binding ruling 4). A positive control (tenant A
// reading its OWN anchor with its OWN scope) proves the query is not
// vacuously empty.
func TestGenerationAnchorIsScoped(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenantA := testfixtures.NewTenant(t, ctx, pool, 1)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 2)
	repo := postgres.NewGenerationRepository(pool)

	analyzerID := tenantA.Analyzers[0].ID
	anchor := model.GenerationAnchor{
		AnalyzerID:   analyzerID,
		AnchorTs:     f2genEpoch,
		ActiveExport: decimal.RequireFromString("500.0000"),
		Source:       "initial",
	}
	require.NoError(t, repo.SetAnchor(ctx, tenantA.Scope, anchor))

	// Positive control: tenant A reads its own anchor with its own scope.
	got, err := repo.Anchor(ctx, tenantA.Scope, analyzerID)
	require.NoError(t, err)
	require.True(t, got.ActiveExport.Equal(anchor.ActiveExport))

	// Tenant B, with its OWN AdminScope (whole-company grant), must not see
	// tenant A's analyzer at all.
	_, err = repo.Anchor(ctx, tenantB.AdminScope, analyzerID)
	require.ErrorIs(t, err, store.ErrNotFound)

	// Nor may tenant B WRITE an anchor onto tenant A's analyzer.
	err = repo.SetAnchor(ctx, tenantB.AdminScope, model.GenerationAnchor{
		AnalyzerID:   analyzerID,
		AnchorTs:     f2genEpoch,
		ActiveExport: decimal.RequireFromString("999.0000"),
		Source:       "operator",
	})
	require.ErrorIs(t, err, store.ErrNotFound)

	// Nothing was written by tenant B's attempt: tenant A's anchor is
	// unchanged.
	got, err = repo.Anchor(ctx, tenantA.Scope, analyzerID)
	require.NoError(t, err)
	require.True(t, got.ActiveExport.Equal(anchor.ActiveExport),
		"tenant B's refused write must not have changed tenant A's anchor")
}

// TestGenerationAnchorRefusesNarrowScopeOnOtherBuilding: tenant.Scope grants
// Buildings[0] only (testfixtures.NewTenant's documented shape).
// Analyzers[2] belongs to Buildings[1] — same company, different building —
// so this is the SAME-COMPANY narrow-scope proof F1's binding ruling 4
// requires alongside the cross-tenant one above.
func TestGenerationAnchorRefusesNarrowScopeOnOtherBuilding(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	repo := postgres.NewGenerationRepository(pool)

	require.GreaterOrEqual(t, len(tenant.Analyzers), 3)
	otherBuildingAnalyzer := tenant.Analyzers[2].ID
	require.NotEqual(t, tenant.Buildings[0].ID, *tenant.Analyzers[2].BuildingID,
		"the fixture premise: Analyzers[2] must be under a DIFFERENT building than the narrow Scope grants")

	// Positive control: the SAME narrow scope can write and read its OWN
	// building's analyzer.
	ownAnalyzer := tenant.Analyzers[0].ID
	require.NoError(t, repo.SetAnchor(ctx, tenant.Scope, model.GenerationAnchor{
		AnalyzerID:   ownAnalyzer,
		AnchorTs:     f2genEpoch,
		ActiveExport: decimal.RequireFromString("10.0000"),
		Source:       "initial",
	}))
	_, err := repo.Anchor(ctx, tenant.Scope, ownAnalyzer)
	require.NoError(t, err)

	// Write with AdminScope so the analyzer HAS an anchor, then prove the
	// narrow scope cannot read or overwrite it.
	require.NoError(t, repo.SetAnchor(ctx, tenant.AdminScope, model.GenerationAnchor{
		AnalyzerID:   otherBuildingAnalyzer,
		AnchorTs:     f2genEpoch,
		ActiveExport: decimal.RequireFromString("20.0000"),
		Source:       "initial",
	}))

	_, err = repo.Anchor(ctx, tenant.Scope, otherBuildingAnalyzer)
	require.ErrorIs(t, err, store.ErrNotFound)

	err = repo.SetAnchor(ctx, tenant.Scope, model.GenerationAnchor{
		AnalyzerID:   otherBuildingAnalyzer,
		AnchorTs:     f2genEpoch,
		ActiveExport: decimal.RequireFromString("30.0000"),
		Source:       "operator",
	})
	require.ErrorIs(t, err, store.ErrNotFound)
}

// TestGenerationAnchorInvalidScopeIsRejected pins the before-any-I/O
// contract every repository method shares.
func TestGenerationAnchorInvalidScopeIsRejected(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	repo := postgres.NewGenerationRepository(pool)

	_, err := repo.Anchor(ctx, store.Scope{}, testfixtures.NewTenant(t, ctx, pool, 1).Analyzers[0].ID)
	require.ErrorIs(t, err, store.ErrInvalidScope)

	err = repo.SetAnchor(ctx, store.Scope{}, model.GenerationAnchor{})
	require.ErrorIs(t, err, store.ErrInvalidScope)
}
