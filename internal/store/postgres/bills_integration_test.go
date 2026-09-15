//go:build integration

package postgres_test

import (
	"context"
	"encoding/json"
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

// billFixtureRow is a minimally valid, zero-charge building bill for
// buildingID (nil for a company-level bill), for periodKey.
func billFixtureRow(companyID uuid.UUID, buildingID *uuid.UUID, analyzerID *uuid.UUID, billScope model.BillScope, periodKey string) model.Bill {
	now := time.Now().UTC()
	return model.Bill{
		CompanyID: companyID, BuildingID: buildingID, AnalyzerID: analyzerID, Scope: billScope,
		PeriodKey: periodKey, PeriodStart: tariffFixtureDate(2026, 1, 1), PeriodEnd: tariffFixtureDate(2026, 2, 1), DaysInPeriod: 31,
		IndexStart: json.RawMessage(`{}`), IndexEnd: json.RawMessage(`{}`),
		GenerationUsage: model.GenerationUsageNone,
		Status:          model.BillStatusDraft, ComputedAt: now, CreatedAt: now, UpdatedAt: now,
	}
}

func TestBillGetAndListRespectNullBuildingIDRuling(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 200)
	repo := postgres.NewBillRepository(pool)

	companyBill, err := repo.Create(ctx, tenant.AdminScope, billFixtureRow(tenant.Company.ID, nil, nil, model.BillScopeCompany, "2026-01"), nil, nil)
	require.NoError(t, err)

	buildingID := tenant.Buildings[0].ID
	buildingBill, err := repo.Create(ctx, tenant.Scope, billFixtureRow(tenant.Company.ID, &buildingID, nil, model.BillScopeBuilding, "2026-01"), nil, nil)
	require.NoError(t, err)

	// The narrow Scope can read its own building's bill…
	_, err = repo.Get(ctx, tenant.Scope, buildingBill.ID)
	require.NoError(t, err)
	// …but never the company-level bill, even within its own company.
	_, err = repo.Get(ctx, tenant.Scope, companyBill.ID)
	require.ErrorIs(t, err, store.ErrNotFound)

	list, err := repo.List(ctx, tenant.Scope, store.BillFilter{})
	require.NoError(t, err)
	for _, b := range list {
		require.NotEqual(t, companyBill.ID, b.ID, "a narrow scope's List must never surface a company-level bill")
	}

	// AllBuildings sees both.
	_, err = repo.Get(ctx, tenant.AdminScope, companyBill.ID)
	require.NoError(t, err)
}

// TestBillCreateRejectsInvisibleBuildingAnalyzerOrMember proves Create's
// whole-batch isolation rule: the building, the analyzer and every member
// analyzer must be visible to the Scope, or nothing is written.
//
// Deliberate-break proof (reverted): removing the
// requireAnalyzersVisible call from BillRepository.Create let tenant A's
// narrow Scope attach tenant A's OWN Buildings[1] analyzer (outside the
// Scope) as a member without error — see the task report.
func TestBillCreateRejectsInvisibleBuildingAnalyzerOrMember(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 210)
	repo := postgres.NewBillRepository(pool)

	otherBuilding := tenant.Buildings[1].ID
	_, err := repo.Create(ctx, tenant.Scope, billFixtureRow(tenant.Company.ID, &otherBuilding, nil, model.BillScopeBuilding, "2026-01"), nil, nil)
	require.ErrorIs(t, err, store.ErrNotFound, "building outside the scope")

	ownBuilding := tenant.Buildings[0].ID
	outsideAnalyzer := tenant.Analyzers[2].ID // under Buildings[1]
	_, err = repo.Create(ctx, tenant.Scope, billFixtureRow(tenant.Company.ID, &ownBuilding, nil, model.BillScopeBuilding, "2026-01"),
		nil, []uuid.UUID{outsideAnalyzer})
	require.ErrorIs(t, err, store.ErrNotFound, "member analyzer outside the scope")

	// A valid analyzer bill, with a member that IS visible, succeeds.
	ownAnalyzer := tenant.Analyzers[0].ID
	b, err := repo.Create(ctx, tenant.Scope, billFixtureRow(tenant.Company.ID, &ownBuilding, &ownAnalyzer, model.BillScopeAnalyzer, "2026-01"),
		nil, []uuid.UUID{ownAnalyzer})
	require.NoError(t, err)

	members, err := repo.Members(ctx, tenant.Scope, b.ID)
	require.NoError(t, err)
	require.Len(t, members, 1)
	require.Equal(t, ownAnalyzer, members[0].AnalyzerID)
}

// TestSupersedeReplacesRatherThanDeletes covers 04-data-model.md §14: a
// recomputation marks the previous bill superseded rather than deleting it,
// and both remain readable afterwards.
func TestSupersedeReplacesRatherThanDeletes(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 220)
	repo := postgres.NewBillRepository(pool)
	buildingID := tenant.Buildings[0].ID

	original, err := repo.Create(ctx, tenant.Scope,
		billFixtureRow(tenant.Company.ID, &buildingID, nil, model.BillScopeBuilding, "2026-01"),
		[]model.BillLine{{Code: "energy", Label: "Energy", Amount: billDec("100.0000")}}, nil)
	require.NoError(t, err)
	require.Equal(t, model.BillStatusDraft, original.Status)

	// A second Create for the same (scope, subject, period) is refused: the
	// unique index only permits one live bill.
	_, err = repo.Create(ctx, tenant.Scope, billFixtureRow(tenant.Company.ID, &buildingID, nil, model.BillScopeBuilding, "2026-01"), nil, nil)
	require.ErrorIs(t, err, store.ErrConflict)

	replacement, err := repo.Supersede(ctx, tenant.Scope, original.ID,
		billFixtureRow(tenant.Company.ID, &buildingID, nil, model.BillScopeBuilding, "2026-01"),
		[]model.BillLine{{Code: "energy", Label: "Energy", Amount: billDec("150.0000")}}, nil, time.Now().UTC())
	require.NoError(t, err)
	require.NotEqual(t, original.ID, replacement.ID)
	require.Equal(t, model.BillStatusDraft, replacement.Status)

	// The old bill is still there, marked superseded — not deleted.
	old, err := repo.Get(ctx, tenant.AdminScope, original.ID)
	require.NoError(t, err)
	require.Equal(t, model.BillStatusSuperseded, old.Status)
	oldLines, err := repo.Lines(ctx, tenant.AdminScope, original.ID)
	require.NoError(t, err)
	require.Len(t, oldLines, 1, "the superseded bill's own lines stay readable")

	current, err := repo.Current(ctx, tenant.Scope, model.BillScopeBuilding, buildingID, "2026-01")
	require.NoError(t, err)
	require.Equal(t, replacement.ID, current.ID)

	// Superseding a bill not visible to the Scope changes nothing.
	otherTenant := testfixtures.NewTenant(t, ctx, pool, 221)
	_, err = repo.Supersede(ctx, otherTenant.AdminScope, original.ID,
		billFixtureRow(otherTenant.Company.ID, nil, nil, model.BillScopeBuilding, "2026-01"), nil, nil, time.Now().UTC())
	require.ErrorIs(t, err, store.ErrNotFound)
	stillSuperseded, err := repo.Get(ctx, tenant.AdminScope, original.ID)
	require.NoError(t, err)
	require.Equal(t, model.BillStatusSuperseded, stillSuperseded.Status)
}

// TestBillChildTablesJoinThroughBillIsolation covers the four mandatory
// methods on bill_lines / bill_members / bill_hourly_detail: Lines,
// Members, HourlyDetail and ReplaceHourlyDetail must join through bills.
//
// Deliberate-break proof (reverted): replacing requireVisible's body with
// `return nil` in bills.go made tenant A's calls with tenant B's bill id
// succeed instead of returning ErrNotFound, for every one of the four
// methods below.
func TestBillChildTablesJoinThroughBillIsolation(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenantA := testfixtures.NewTenant(t, ctx, pool, 230)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 231)
	repo := postgres.NewBillRepository(pool)

	buildingB := tenantB.Buildings[0].ID
	billB, err := repo.Create(ctx, tenantB.Scope, billFixtureRow(tenantB.Company.ID, &buildingB, nil, model.BillScopeBuilding, "2026-01"),
		[]model.BillLine{{Code: "energy", Label: "Energy", Amount: billDec("10.0000")}}, nil)
	require.NoError(t, err)

	_, err = repo.Lines(ctx, tenantA.AdminScope, billB.ID)
	require.ErrorIs(t, err, store.ErrNotFound)
	_, err = repo.Members(ctx, tenantA.AdminScope, billB.ID)
	require.ErrorIs(t, err, store.ErrNotFound)
	_, err = repo.HourlyDetail(ctx, tenantA.AdminScope, billB.ID)
	require.ErrorIs(t, err, store.ErrNotFound)
	_, err = repo.ReplaceHourlyDetail(ctx, tenantA.AdminScope, billB.ID, []model.BillHourlyDetail{
		{Ts: tariffFixtureDate(2026, 1, 1), Consumption: billDec("1.0000"), PTF: billDec("2.0000"), Yekdem: billDec("0.5000"), Kbk: billDec("1.0000"), UnitPrice: billDec("3.0000"), Cost: billDec("3.0000")},
	})
	require.ErrorIs(t, err, store.ErrNotFound)

	// Tenant B, its own bill: round-trips.
	n, err := repo.ReplaceHourlyDetail(ctx, tenantB.Scope, billB.ID, []model.BillHourlyDetail{
		{Ts: tariffFixtureDate(2026, 1, 1), Consumption: billDec("1.0000"), PTF: billDec("2.0000"), Yekdem: billDec("0.5000"), Kbk: billDec("1.0000"), UnitPrice: billDec("3.0000"), Cost: billDec("3.0000")},
		{Ts: tariffFixtureDate(2026, 1, 2), Consumption: billDec("1.5000"), PTF: billDec("2.1000"), Yekdem: billDec("0.5000"), Kbk: billDec("1.0000"), UnitPrice: billDec("3.1000"), Cost: billDec("4.6500")},
	})
	require.NoError(t, err)
	require.Equal(t, int64(2), n)
	detail, err := repo.HourlyDetail(ctx, tenantB.Scope, billB.ID)
	require.NoError(t, err)
	require.Len(t, detail, 2)
}

// TestBillUpdateStatusCannotSetSuperseded pins the doc comment: superseded
// is a transition owned by Supersede alone.
func TestBillUpdateStatusCannotSetSuperseded(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 240)
	repo := postgres.NewBillRepository(pool)
	buildingID := tenant.Buildings[0].ID

	b, err := repo.Create(ctx, tenant.Scope, billFixtureRow(tenant.Company.ID, &buildingID, nil, model.BillScopeBuilding, "2026-01"), nil, nil)
	require.NoError(t, err)

	_, err = repo.UpdateStatus(ctx, tenant.Scope, b.ID, model.BillStatusSuperseded, nil, time.Now().UTC())
	require.Error(t, err)
	require.NotErrorIs(t, err, store.ErrNotFound)

	updated, err := repo.UpdateStatus(ctx, tenant.Scope, b.ID, model.BillStatusIssued, nil, time.Now().UTC())
	require.NoError(t, err)
	require.Equal(t, model.BillStatusIssued, updated.Status)
}

// TestBillCreateRejectsCrossTenantBuildingEvenWithAdminScope is Critical
// Finding 1's probe applied to bills: Scope.AllowsBuilding is an in-memory
// grant check that returns true for ANY id under AllBuildings — it cannot
// know which company owns a building — so tenant A's AdminScope must not be
// able to store tenant B's building id as a bill's building_id.
func TestBillCreateRejectsCrossTenantBuildingEvenWithAdminScope(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenantA := testfixtures.NewTenant(t, ctx, pool, 250)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 251)
	repo := postgres.NewBillRepository(pool)

	crossBuilding := tenantB.Buildings[0].ID
	_, err := repo.Create(ctx, tenantA.AdminScope, billFixtureRow(tenantA.Company.ID, &crossBuilding, nil, model.BillScopeBuilding, "2026-01"), nil, nil)
	require.ErrorIs(t, err, store.ErrNotFound, "tenant A's AdminScope must not be able to store tenant B's building id")

	// Tenant B's own Create for that building and period afterwards succeeds
	// — the CONTROLLER RULING's probe against a silent cross-tenant reservation.
	ownBuilding := tenantB.Buildings[0].ID
	created, err := repo.Create(ctx, tenantB.AdminScope, billFixtureRow(tenantB.Company.ID, &ownBuilding, nil, model.BillScopeBuilding, "2026-01"), nil, nil)
	require.NoError(t, err)
	require.Equal(t, ownBuilding, *created.BuildingID)
}

// TestBillCreateRejectsCrossTenantTariffID is Important Finding 4's probe:
// bills.tariff_id is a stored foreign key too and must name a tariff of the
// same company, never taken as given.
func TestBillCreateRejectsCrossTenantTariffID(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenantA := testfixtures.NewTenant(t, ctx, pool, 252)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 253)
	repo := postgres.NewBillRepository(pool)

	ownBuilding := tenantA.Buildings[0].ID
	row := billFixtureRow(tenantA.Company.ID, &ownBuilding, nil, model.BillScopeBuilding, "2026-01")
	foreignTariff := tenantB.Tariffs[0].ID
	row.TariffID = &foreignTariff
	_, err := repo.Create(ctx, tenantA.AdminScope, row, nil, nil)
	require.ErrorIs(t, err, store.ErrNotFound)

	ownTariff := tenantA.Tariffs[0].ID
	row.TariffID = &ownTariff
	created, err := repo.Create(ctx, tenantA.AdminScope, row, nil, nil)
	require.NoError(t, err)
	require.Equal(t, ownTariff, *created.TariffID)
}

// TestBillCreateAndMemberInsertRejectCrossTenantAnalyzerID is F1 final
// review pass A, Important Finding 1's probe: bills.analyzer_id and
// bill_members.analyzer_id are stored foreign keys and must be refused even
// under the OTHER tenant's AdminScope (binding ruling 4) — never taken as
// given. Both are now validated in SQL, inside BillCreate's and
// BillMemberInsert's own statements (queries/bills.sql), not only by
// BillRepository's Go-side requireAnalyzersVisible pre-check, which still
// runs first as defence in depth. final-fix-A-report.md records the
// procedural proof that the SQL clauses alone — with requireAnalyzersVisible
// temporarily disabled — still refuse both cases below, and that
// tautologising either new clause with the pre-check still disabled lets the
// corresponding write through.
func TestBillCreateAndMemberInsertRejectCrossTenantAnalyzerID(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenantA := testfixtures.NewTenant(t, ctx, pool, 258)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 259)
	repo := postgres.NewBillRepository(pool)

	ownBuilding := tenantA.Buildings[0].ID
	foreignAnalyzer := tenantB.Analyzers[0].ID

	// The bill's own analyzer_id (an analyzer-scope bill) naming tenant B's
	// analyzer is refused under tenant A's AdminScope.
	row := billFixtureRow(tenantA.Company.ID, &ownBuilding, &foreignAnalyzer, model.BillScopeAnalyzer, "2026-01")
	_, err := repo.Create(ctx, tenantA.AdminScope, row, nil, nil)
	require.ErrorIs(t, err, store.ErrNotFound, "bill analyzer_id naming another tenant's analyzer")

	// A member analyzer naming tenant B's analyzer is refused too, even
	// though the bill itself has no analyzer_id and one member IS visible.
	ownAnalyzer := tenantA.Analyzers[0].ID
	buildingRow := billFixtureRow(tenantA.Company.ID, &ownBuilding, nil, model.BillScopeBuilding, "2026-02")
	_, err = repo.Create(ctx, tenantA.AdminScope, buildingRow, nil, []uuid.UUID{ownAnalyzer, foreignAnalyzer})
	require.ErrorIs(t, err, store.ErrNotFound, "bill member analyzer naming another tenant's analyzer")

	// Positive control: tenant B's own AdminScope may bill its own analyzer.
	tenantBBuilding := tenantB.Buildings[0].ID
	own := billFixtureRow(tenantB.Company.ID, &tenantBBuilding, &foreignAnalyzer, model.BillScopeAnalyzer, "2026-01")
	created, err := repo.Create(ctx, tenantB.AdminScope, own, nil, nil)
	require.NoError(t, err, "positive control: tenant B's own AdminScope must be able to bill its own analyzer")
	require.Equal(t, foreignAnalyzer, *created.AnalyzerID)
}

// TestSupersedeRejectsReplacementNamingADifferentScopeSubjectOrPeriod is the
// folded-minor probe: the replacement must keep the replaced bill's (scope,
// subject, period) — Supersede is a recomputation of ONE bill's history, not
// a way to attach an unrelated bill's row where another one used to be.
func TestSupersedeRejectsReplacementNamingADifferentScopeSubjectOrPeriod(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 254)
	repo := postgres.NewBillRepository(pool)
	buildingID := tenant.Buildings[0].ID

	original, err := repo.Create(ctx, tenant.Scope,
		billFixtureRow(tenant.Company.ID, &buildingID, nil, model.BillScopeBuilding, "2026-01"), nil, nil)
	require.NoError(t, err)

	// A different period.
	_, err = repo.Supersede(ctx, tenant.Scope, original.ID,
		billFixtureRow(tenant.Company.ID, &buildingID, nil, model.BillScopeBuilding, "2026-02"), nil, nil, time.Now().UTC())
	require.Error(t, err)
	require.NotErrorIs(t, err, store.ErrNotFound, "a mismatch is a caller programming error, not an existence question")

	// The original must still be live (not superseded): the mismatched
	// Supersede call must have changed nothing.
	stillLive, err := repo.Get(ctx, tenant.AdminScope, original.ID)
	require.NoError(t, err)
	require.Equal(t, model.BillStatusDraft, stillLive.Status)

	// A different subject (a different building).
	otherBuilding := tenant.Buildings[1].ID
	_, err = repo.Supersede(ctx, tenant.AdminScope, original.ID,
		billFixtureRow(tenant.Company.ID, &otherBuilding, nil, model.BillScopeBuilding, "2026-01"), nil, nil, time.Now().UTC())
	require.Error(t, err)
	require.NotErrorIs(t, err, store.ErrNotFound)

	// A matching replacement (same scope, subject, period) succeeds.
	replacement, err := repo.Supersede(ctx, tenant.Scope, original.ID,
		billFixtureRow(tenant.Company.ID, &buildingID, nil, model.BillScopeBuilding, "2026-01"), nil, nil, time.Now().UTC())
	require.NoError(t, err)
	require.NotEqual(t, original.ID, replacement.ID)
}

// TestSupersedeReplacementBuildingAndMembersMustBeVisible is Important
// Finding 7's missing test: the Supersede child-visibility rule — the
// replacement's building and members follow Create's rule exactly.
func TestSupersedeReplacementBuildingAndMembersMustBeVisible(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 255)
	repo := postgres.NewBillRepository(pool)
	buildingID := tenant.Buildings[0].ID

	original, err := repo.Create(ctx, tenant.Scope,
		billFixtureRow(tenant.Company.ID, &buildingID, nil, model.BillScopeBuilding, "2026-01"), nil, nil)
	require.NoError(t, err)

	// A replacement whose member analyzer is outside the Scope is refused,
	// and the original stays live.
	outsideAnalyzer := tenant.Analyzers[2].ID // under Buildings[1]
	_, err = repo.Supersede(ctx, tenant.Scope, original.ID,
		billFixtureRow(tenant.Company.ID, &buildingID, nil, model.BillScopeBuilding, "2026-01"), nil, []uuid.UUID{outsideAnalyzer}, time.Now().UTC())
	require.ErrorIs(t, err, store.ErrNotFound)

	stillLive, err := repo.Get(ctx, tenant.AdminScope, original.ID)
	require.NoError(t, err)
	require.Equal(t, model.BillStatusDraft, stillLive.Status, "a refused Supersede must not have marked the original superseded")
}

// TestBillPageLimitsClampNegativeOffset is the folded-minor probe: OFFSET
// must not be negative.
func TestBillPageLimitsClampNegativeOffset(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 256)
	repo := postgres.NewBillRepository(pool)

	_, err := repo.List(ctx, tenant.AdminScope, store.BillFilter{Page: store.Page{Limit: 10, Offset: -1}})
	require.NoError(t, err, "a negative Offset must be clamped to 0")
}

// TestBillOfSoftDeletedBuildingIsNotReadable is the folded-minor probe.
func TestBillOfSoftDeletedBuildingIsNotReadable(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 257)
	repo := postgres.NewBillRepository(pool)
	buildingID := tenant.Buildings[0].ID

	b, err := repo.Create(ctx, tenant.AdminScope, billFixtureRow(tenant.Company.ID, &buildingID, nil, model.BillScopeBuilding, "2026-01"), nil, nil)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `update buildings set deleted_at = now() where id = $1`, buildingID)
	require.NoError(t, err)

	_, err = repo.Get(ctx, tenant.AdminScope, b.ID)
	require.ErrorIs(t, err, store.ErrNotFound)

	list, err := repo.List(ctx, tenant.AdminScope, store.BillFilter{})
	require.NoError(t, err)
	for _, r := range list {
		require.NotEqual(t, b.ID, r.ID, "a bill of a soft-deleted building must not stay readable")
	}
}

// billDec parses a fixture literal into a decimal.Decimal, panicking on a
// typo — fixture values here are constants, so a parse failure is a bug in
// the test rather than a runtime condition worth threading an error for.
func billDec(s string) decimal.Decimal { return decimal.RequireFromString(s) }
