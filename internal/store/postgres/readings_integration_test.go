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

// readingsDecPtr parses a literal into a *decimal.Decimal, panicking on a
// typo — the fixture values below are constants in this file.
func readingsDecPtr(s string) *decimal.Decimal {
	d := decimal.RequireFromString(s)
	return &d
}

// readingsRow builds one fixture MeterReading for analyzerID at ts.
func readingsRow(analyzerID uuid.UUID, ts time.Time, kind model.ReadingKind, activeImport string, multiplier string) model.MeterReading {
	return model.MeterReading{
		AnalyzerID:        analyzerID,
		Ts:                ts,
		Kind:              kind,
		ActiveImport:      readingsDecPtr(activeImport),
		MultiplierApplied: decimal.RequireFromString(multiplier),
		SourceProvider:    model.IntegrationProviderOSOS,
		IngestedAt:        time.Now().UTC(),
	}
}

var readingsEpoch = time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

func TestBulkInsertIsIdempotent(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	repo := postgres.NewReadingRepository(pool)

	analyzerID := tenant.Analyzers[0].ID
	rows := make([]model.MeterReading, 0, 100)
	for i := range 100 {
		ts := readingsEpoch.Add(time.Duration(i) * time.Hour)
		rows = append(rows, readingsRow(analyzerID, ts, model.ReadingKindLoadProfile,
			"1000.0000", "40.000000"))
	}

	inserted, updated, err := repo.BulkInsert(ctx, tenant.Scope, rows)
	require.NoError(t, err)
	require.Equal(t, 100, inserted)
	require.Equal(t, 0, updated)

	// Re-ingesting the SAME 100 rows must update, never duplicate.
	inserted, updated, err = repo.BulkInsert(ctx, tenant.Scope, rows)
	require.NoError(t, err)
	require.Equal(t, 0, inserted)
	require.Equal(t, 100, updated)

	got, err := repo.Range(ctx, tenant.Scope, analyzerID,
		store.TimeRange{From: readingsEpoch, To: readingsEpoch.Add(200 * time.Hour)},
		model.ReadingKindLoadProfile)
	require.NoError(t, err)
	require.Len(t, got, 100)
}

func TestBulkInsertMixedInsertAndUpdateCountsAreReal(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	repo := postgres.NewReadingRepository(pool)
	analyzerID := tenant.Analyzers[0].ID

	first := []model.MeterReading{
		readingsRow(analyzerID, readingsEpoch, model.ReadingKindLoadProfile, "100.0000", "1"),
		readingsRow(analyzerID, readingsEpoch.Add(time.Hour), model.ReadingKindLoadProfile, "200.0000", "1"),
	}
	inserted, updated, err := repo.BulkInsert(ctx, tenant.Scope, first)
	require.NoError(t, err)
	require.Equal(t, 2, inserted)
	require.Equal(t, 0, updated)

	// One row repeats an existing key (updated), one is brand new (inserted).
	second := []model.MeterReading{
		readingsRow(analyzerID, readingsEpoch, model.ReadingKindLoadProfile, "999.0000", "1"),
		readingsRow(analyzerID, readingsEpoch.Add(2*time.Hour), model.ReadingKindLoadProfile, "300.0000", "1"),
	}
	inserted, updated, err = repo.BulkInsert(ctx, tenant.Scope, second)
	require.NoError(t, err)
	require.Equal(t, 1, inserted)
	require.Equal(t, 1, updated)
}

func TestReadingsStoreMultipliedValues(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	repo := postgres.NewReadingRepository(pool)
	analyzerID := tenant.Analyzers[0].ID

	// tenant.Analyzers[0].MeterMultiplier is 40 (testfixtures.NewTenant); the
	// row records that multiplier applied and the ALREADY-multiplied value.
	row := readingsRow(analyzerID, readingsEpoch, model.ReadingKindLoadProfile, "40000.0000", "40.000000")
	_, _, err := repo.BulkInsert(ctx, tenant.Scope, []model.MeterReading{row})
	require.NoError(t, err)

	got, err := repo.Range(ctx, tenant.Scope, analyzerID,
		store.TimeRange{From: readingsEpoch, To: readingsEpoch.Add(time.Hour)},
		model.ReadingKindLoadProfile)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.True(t, decimal.RequireFromString("40000.0000").Equal(*got[0].ActiveImport))
	require.True(t, decimal.RequireFromString("40.000000").Equal(got[0].MultiplierApplied))
}

func TestBoundaryReadingsYieldsNoRowWhenAReadingIsMissing(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	repo := postgres.NewReadingRepository(pool)
	analyzerID := tenant.Analyzers[0].ID

	// Only an END reading exists; nothing at or before readingsEpoch.
	end := readingsRow(analyzerID, readingsEpoch.Add(48*time.Hour), model.ReadingKindLoadProfile, "500.0000", "1")
	_, _, err := repo.BulkInsert(ctx, tenant.Scope, []model.MeterReading{end})
	require.NoError(t, err)

	start, endReading, err := repo.BoundaryReadings(ctx, tenant.Scope, analyzerID,
		model.ReadingKindLoadProfile, readingsEpoch, readingsEpoch.Add(48*time.Hour))
	require.NoError(t, err)
	require.Nil(t, start, "no reading at or before `start`: must be nil, not a zero-valued struct")
	require.NotNil(t, endReading)
	require.True(t, decimal.RequireFromString("500.0000").Equal(*endReading.ActiveImport))
}

func TestBoundaryReadingsDistinguishesKindsAtTheSameInstant(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	repo := postgres.NewReadingRepository(pool)
	analyzerID := tenant.Analyzers[0].ID

	loadProfile := readingsRow(analyzerID, readingsEpoch, model.ReadingKindLoadProfile, "111.0000", "1")
	billing := readingsRow(analyzerID, readingsEpoch, model.ReadingKindBilling, "222.0000", "1")
	_, _, err := repo.BulkInsert(ctx, tenant.Scope, []model.MeterReading{loadProfile, billing})
	require.NoError(t, err)

	_, lp, err := repo.BoundaryReadings(ctx, tenant.Scope, analyzerID, model.ReadingKindLoadProfile, readingsEpoch, readingsEpoch)
	require.NoError(t, err)
	require.NotNil(t, lp)
	require.True(t, decimal.RequireFromString("111.0000").Equal(*lp.ActiveImport))

	_, b, err := repo.BoundaryReadings(ctx, tenant.Scope, analyzerID, model.ReadingKindBilling, readingsEpoch, readingsEpoch)
	require.NoError(t, err)
	require.NotNil(t, b)
	require.True(t, decimal.RequireFromString("222.0000").Equal(*b.ActiveImport))
}

func TestReadingLatestReturnsNilWhenNoneInRange(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	repo := postgres.NewReadingRepository(pool)
	analyzerID := tenant.Analyzers[0].ID

	got, err := repo.Latest(ctx, tenant.Scope, analyzerID,
		store.TimeRange{From: readingsEpoch, To: readingsEpoch.Add(time.Hour)},
		model.ReadingKindLoadProfile)
	require.NoError(t, err)
	require.Nil(t, got)
}

// --- scope validation, before any database round trip ----------------------

func TestReadingRepositoryRejectsInvalidScope(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	repo := postgres.NewReadingRepository(pool)
	invalid := store.Scope{}
	analyzerID := uuid.New()
	validRange := store.TimeRange{From: readingsEpoch, To: readingsEpoch.Add(time.Hour)}

	_, _, err := repo.BulkInsert(ctx, invalid, []model.MeterReading{readingsRow(analyzerID, readingsEpoch, model.ReadingKindLoadProfile, "1", "1")})
	require.ErrorIs(t, err, store.ErrInvalidScope)

	_, err = repo.Range(ctx, invalid, analyzerID, validRange, model.ReadingKindLoadProfile)
	require.ErrorIs(t, err, store.ErrInvalidScope)

	_, _, err = repo.BoundaryReadings(ctx, invalid, analyzerID, model.ReadingKindLoadProfile, readingsEpoch, readingsEpoch)
	require.ErrorIs(t, err, store.ErrInvalidScope)

	_, err = repo.Latest(ctx, invalid, analyzerID, validRange, model.ReadingKindLoadProfile)
	require.ErrorIs(t, err, store.ErrInvalidScope)
}

func TestReadingRepositoryRejectsInvalidRange(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	repo := postgres.NewReadingRepository(pool)
	analyzerID := tenant.Analyzers[0].ID
	invalidRange := store.TimeRange{} // zero From/To

	_, err := repo.Range(ctx, tenant.Scope, analyzerID, invalidRange, model.ReadingKindLoadProfile)
	require.ErrorIs(t, err, store.ErrInvalidRange)

	_, err = repo.Latest(ctx, tenant.Scope, analyzerID, invalidRange, model.ReadingKindLoadProfile)
	require.ErrorIs(t, err, store.ErrInvalidRange)
}

// --- isolation: meter_readings has no company_id, joins through analyzers --

func TestReadingBulkInsertRefusesWholeBatchWhenAnalyzerNotVisible(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenantA := testfixtures.NewTenant(t, ctx, pool, 1)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 2)
	repo := postgres.NewReadingRepository(pool)

	ownAnalyzer := tenantA.Analyzers[0].ID
	foreignAnalyzer := tenantB.Analyzers[0].ID

	rows := []model.MeterReading{
		readingsRow(ownAnalyzer, readingsEpoch, model.ReadingKindLoadProfile, "1.0000", "1"),
		readingsRow(foreignAnalyzer, readingsEpoch, model.ReadingKindLoadProfile, "2.0000", "1"),
	}
	_, _, err := repo.BulkInsert(ctx, tenantA.Scope, rows)
	require.ErrorIs(t, err, store.ErrNotFound)

	// The WHOLE batch is refused: even the row naming tenant A's own
	// analyzer must not have been written.
	got, err := repo.Range(ctx, tenantA.Scope, ownAnalyzer,
		store.TimeRange{From: readingsEpoch, To: readingsEpoch.Add(time.Hour)},
		model.ReadingKindLoadProfile)
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestReadingBulkInsertRefusesForAnalyzerOutsideNarrowScope(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	repo := postgres.NewReadingRepository(pool)

	// tenant.Scope covers Buildings[0] only; Analyzers[2]/[3] belong to
	// Buildings[1].
	outsideAnalyzer := tenant.Analyzers[2].ID
	rows := []model.MeterReading{readingsRow(outsideAnalyzer, readingsEpoch, model.ReadingKindLoadProfile, "1.0000", "1")}

	_, _, err := repo.BulkInsert(ctx, tenant.Scope, rows)
	require.ErrorIs(t, err, store.ErrNotFound)

	// The wider AdminScope (AllBuildings) reaches the very same analyzer.
	inserted, _, err := repo.BulkInsert(ctx, tenant.AdminScope, rows)
	require.NoError(t, err)
	require.Equal(t, 1, inserted)
}

func TestReadingRangeAnalyzerNotVisibleReturnsNotFound(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenantA := testfixtures.NewTenant(t, ctx, pool, 1)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 2)
	repo := postgres.NewReadingRepository(pool)
	validRange := store.TimeRange{From: readingsEpoch, To: readingsEpoch.Add(time.Hour)}

	// Cross-tenant: tenant A's scope, tenant B's analyzer.
	_, err := repo.Range(ctx, tenantA.Scope, tenantB.Analyzers[0].ID, validRange, model.ReadingKindLoadProfile)
	require.ErrorIs(t, err, store.ErrNotFound)

	// Narrow scope: tenant A's own analyzer, but outside Buildings[0].
	_, err = repo.Range(ctx, tenantA.Scope, tenantA.Analyzers[2].ID, validRange, model.ReadingKindLoadProfile)
	require.ErrorIs(t, err, store.ErrNotFound)

	// The same analyzer IS visible under AllBuildings.
	got, err := repo.Range(ctx, tenantA.AdminScope, tenantA.Analyzers[2].ID, validRange, model.ReadingKindLoadProfile)
	require.NoError(t, err)
	require.Empty(t, got) // visible but no data: an empty slice, not an error
}

func TestReadingBoundaryReadingsAnalyzerNotVisibleReturnsNotFound(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenantA := testfixtures.NewTenant(t, ctx, pool, 1)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 2)
	repo := postgres.NewReadingRepository(pool)

	_, _, err := repo.BoundaryReadings(ctx, tenantA.Scope, tenantB.Analyzers[0].ID, model.ReadingKindLoadProfile, readingsEpoch, readingsEpoch)
	require.ErrorIs(t, err, store.ErrNotFound)
}

func TestReadingLatestAnalyzerNotVisibleReturnsNotFound(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenantA := testfixtures.NewTenant(t, ctx, pool, 1)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 2)
	repo := postgres.NewReadingRepository(pool)
	validRange := store.TimeRange{From: readingsEpoch, To: readingsEpoch.Add(time.Hour)}

	_, err := repo.Latest(ctx, tenantA.Scope, tenantB.Analyzers[0].ID, validRange, model.ReadingKindLoadProfile)
	require.ErrorIs(t, err, store.ErrNotFound)
}
