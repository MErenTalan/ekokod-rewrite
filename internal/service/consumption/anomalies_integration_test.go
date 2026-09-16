//go:build integration

package consumption_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/lock"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/consumption"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

// anomaliesEpoch is this file's own fixed instant, distinct from
// paths_integration_test.go's pathsEpoch so a test in either file never
// accidentally shares seeded data with the other.
var anomaliesEpoch = time.Date(2026, 5, 1, 8, 0, 0, 0, time.UTC)

// anomaliesNewBilling builds a *consumption.Billing wired with every Task 8
// dependency (Locker, Analyzers) against real Postgres repositories.
// lock.NewMemory is used rather than a Redis container: Memory is a real,
// mutex-backed Locker (not a fake) and proves the same serialisation
// contract without adding a second container under memory pressure.
func anomaliesNewBilling(t *testing.T, readingRepo store.ReadingRepository, anomalyRepo store.AnomalyRepository, opsRepo store.OpsRepository, analyzerRepo store.AnalyzerRepository, locker lock.Locker, now time.Time) *consumption.Billing {
	t.Helper()
	b, err := consumption.NewBilling(consumption.BillingDeps{
		Readings:  readingRepo,
		Anomalies: anomalyRepo,
		Ops:       opsRepo,
		Clock:     clock.NewFake(now),
		Log:       testfixtures.DiscardLogger(),
		Locker:    locker,
		Analyzers: analyzerRepo,
	})
	require.NoError(t, err)
	return b
}

// TestASuspectPeriodWritesAnAnomalyAndAnOperatorMessage is Task 8's headline
// test: a negative delta with no covering reset writes exactly one
// consumption_anomalies row and one operator message, R60's shape.
func TestASuspectPeriodWritesAnAnomalyAndAnOperatorMessage(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	id := tenant.Analyzers[0].ID
	h := anomaliesEpoch

	readingRepo := pathsNewReadingRepo(pool)
	anomalyRepo := postgres.NewAnomalyRepository(pool)
	opsRepo := postgres.NewOpsRepository(pool)
	analyzerRepo := postgres.NewAnalyzerRepository(pool)

	pathsSeedReading(t, ctx, readingRepo, tenant.Scope, readingRow(id, h, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}))
	pathsSeedReading(t, ctx, readingRepo, tenant.Scope, readingRow(id, h.Add(time.Hour), model.ReadingKindLoadProfile, map[string]string{"active_import": "900"}))

	billing := anomaliesNewBilling(t, readingRepo, anomalyRepo, opsRepo, analyzerRepo, lock.NewMemory(nil), h.Add(time.Hour))

	req := consumption.SeriesRequest{AnalyzerIDs: []uuid.UUID{id}, Level: energy.Hourly, Range: store.TimeRange{From: h, To: h.Add(time.Hour)}}
	rows, err := billing.ConsumptionAndRecord(ctx, tenant.Scope, req)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Nil(t, rows[0].Values[energy.ActiveImport], "never a number")

	an, err := billing.ListAnomalies(ctx, tenant.AdminScope, consumption.AnomalyListRequest{AnalyzerIDs: []uuid.UUID{id}, Unresolved: true})
	require.NoError(t, err)
	require.Len(t, an, 1)
	require.Equal(t, "negative_delta", an[0].Reason)
	require.Equal(t, h, an[0].PeriodStart.UTC())
	require.Equal(t, h.Add(time.Hour), an[0].PeriodEnd.UTC())

	var detail map[string]any
	require.NoError(t, json.Unmarshal(an[0].Detail, &detail))
	require.Equal(t, "consumption.suspect_period", detail["code"])
	regs, ok := detail["registers"].(map[string]any)
	require.True(t, ok)
	reg, ok := regs["active_import"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "negative_delta", reg["reason"])
	require.Equal(t, "-100", reg["delta"])

	msgs, err := opsRepo.ListMessages(ctx, tenant.AdminScope, store.MessageFilter{RelatedID: &an[0].ID})
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	require.Equal(t, "system", msgs[0].Kind)
	require.Equal(t, "consumption-suspect-period", msgs[0].Category)
	require.Equal(t, "error", msgs[0].Status)
	require.NotNil(t, msgs[0].RelatedType)
	require.Equal(t, "consumption_anomaly", *msgs[0].RelatedType)
}

// TestRerunningTheSamePeriodDoesNotCreateASecondAnomaly is C-6's basic dedup
// proof: calling ConsumptionAndRecord twice for the identical still-suspect
// period creates only one row.
func TestRerunningTheSamePeriodDoesNotCreateASecondAnomaly(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	id := tenant.Analyzers[0].ID
	h := anomaliesEpoch

	readingRepo := pathsNewReadingRepo(pool)
	anomalyRepo := postgres.NewAnomalyRepository(pool)
	opsRepo := postgres.NewOpsRepository(pool)
	analyzerRepo := postgres.NewAnalyzerRepository(pool)

	pathsSeedReading(t, ctx, readingRepo, tenant.Scope, readingRow(id, h, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}))
	pathsSeedReading(t, ctx, readingRepo, tenant.Scope, readingRow(id, h.Add(time.Hour), model.ReadingKindLoadProfile, map[string]string{"active_import": "900"}))

	billing := anomaliesNewBilling(t, readingRepo, anomalyRepo, opsRepo, analyzerRepo, lock.NewMemory(nil), h.Add(time.Hour))
	req := consumption.SeriesRequest{AnalyzerIDs: []uuid.UUID{id}, Level: energy.Hourly, Range: store.TimeRange{From: h, To: h.Add(time.Hour)}}

	_, err := billing.ConsumptionAndRecord(ctx, tenant.Scope, req)
	require.NoError(t, err)
	_, err = billing.ConsumptionAndRecord(ctx, tenant.Scope, req)
	require.NoError(t, err)

	an, err := billing.ListAnomalies(ctx, tenant.AdminScope, consumption.AnomalyListRequest{AnalyzerIDs: []uuid.UUID{id}, Unresolved: false})
	require.NoError(t, err)
	require.Len(t, an, 1)
}

// TestConcurrentRunsForTheSamePeriodStillCreateOnlyOneAnomaly proves the
// platform/lock dedup lock actually serialises the check-then-create: four
// goroutines racing ConsumptionAndRecord for the same period still produce
// exactly one anomaly row.
func TestConcurrentRunsForTheSamePeriodStillCreateOnlyOneAnomaly(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	id := tenant.Analyzers[0].ID
	h := anomaliesEpoch

	readingRepo := pathsNewReadingRepo(pool)
	anomalyRepo := postgres.NewAnomalyRepository(pool)
	opsRepo := postgres.NewOpsRepository(pool)
	analyzerRepo := postgres.NewAnalyzerRepository(pool)

	pathsSeedReading(t, ctx, readingRepo, tenant.Scope, readingRow(id, h, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}))
	pathsSeedReading(t, ctx, readingRepo, tenant.Scope, readingRow(id, h.Add(time.Hour), model.ReadingKindLoadProfile, map[string]string{"active_import": "900"}))

	billing := anomaliesNewBilling(t, readingRepo, anomalyRepo, opsRepo, analyzerRepo, lock.NewMemory(nil), h.Add(time.Hour))
	req := consumption.SeriesRequest{AnalyzerIDs: []uuid.UUID{id}, Level: energy.Hourly, Range: store.TimeRange{From: h, To: h.Add(time.Hour)}}

	const n = 4
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := billing.ConsumptionAndRecord(ctx, tenant.Scope, req)
			errs[i] = err
		}(i)
	}
	wg.Wait()
	for _, err := range errs {
		require.NoError(t, err)
	}

	an, err := billing.ListAnomalies(ctx, tenant.AdminScope, consumption.AnomalyListRequest{AnalyzerIDs: []uuid.UUID{id}, Unresolved: false})
	require.NoError(t, err)
	require.Len(t, an, 1, "the dedup lock must serialise concurrent creation")
}

// TestRegisteringAResetMakesTheNextDerivationSound: an uncovered negative
// delta (R57/Q8: a wrap with no reset row is negative_delta-suspect, exactly
// like any other negative delta) is resolved by registering the reset the
// meter actually had; the SAME period then derives a sound number on the
// next Consumption call, per R55's before/after formula.
func TestRegisteringAResetMakesTheNextDerivationSound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	id := tenant.Analyzers[0].ID
	h := anomaliesEpoch

	readingRepo := pathsNewReadingRepo(pool)
	anomalyRepo := postgres.NewAnomalyRepository(pool)
	opsRepo := postgres.NewOpsRepository(pool)
	analyzerRepo := postgres.NewAnalyzerRepository(pool)

	// Boundaries: start=1000 at h, end=190 at h+1h (negative, uncovered).
	// A prior load_profile reading at h+20m (1040) supplies R55's
	// before-reset value once the reset row is registered at h+30m.
	pathsSeedReading(t, ctx, readingRepo, tenant.Scope, readingRow(id, h, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}))
	pathsSeedReading(t, ctx, readingRepo, tenant.Scope, readingRow(id, h.Add(20*time.Minute), model.ReadingKindLoadProfile, map[string]string{"active_import": "1040"}))
	pathsSeedReading(t, ctx, readingRepo, tenant.Scope, readingRow(id, h.Add(time.Hour), model.ReadingKindLoadProfile, map[string]string{"active_import": "190"}))

	billing := anomaliesNewBilling(t, readingRepo, anomalyRepo, opsRepo, analyzerRepo, lock.NewMemory(nil), h.Add(time.Hour))
	req := consumption.SeriesRequest{AnalyzerIDs: []uuid.UUID{id}, Level: energy.Hourly, Range: store.TimeRange{From: h, To: h.Add(time.Hour)}}

	before, err := billing.ConsumptionAndRecord(ctx, tenant.Scope, req)
	require.NoError(t, err)
	require.Nil(t, before[0].Values[energy.ActiveImport])
	require.Contains(t, before[0].Suspect, energy.ActiveImport)

	an, err := billing.ListAnomalies(ctx, tenant.AdminScope, consumption.AnomalyListRequest{AnalyzerIDs: []uuid.UUID{id}, Unresolved: true})
	require.NoError(t, err)
	require.Len(t, an, 1)

	operator := tenant.Users[model.UserRoleCompanyAdmin]
	resetTS := h.Add(30 * time.Minute)
	_, err = billing.ResolveAnomaly(ctx, tenant.Scope, an[0].ID, operator.ID, consumption.Resolution{
		Mode:       consumption.ResolveByRegisteringReset,
		ResetTS:    &resetTS,
		ResetAfter: map[energy.Register]decimal.Decimal{energy.ActiveImport: decimal.RequireFromString("100")},
	})
	require.NoError(t, err)

	after, err := billing.Consumption(ctx, tenant.Scope, req)
	require.NoError(t, err)
	require.Len(t, after, 1)
	require.NotNil(t, after[0].Values[energy.ActiveImport])
	// R55: (before_reset - reading_start) + (reading_end - after_reset)
	//    = (1040 - 1000) + (190 - 100) = 40 + 90 = 130.
	require.Equal(t, "130", after[0].Values[energy.ActiveImport].String())
	require.Empty(t, after[0].Suspect)
}

// TestResolvingAResetMissingARequiredRegisterFailsValidation is the OVERRIDE
// task's R93 test: ResetAfter must supply a value for EVERY register both
// boundary readings report, not only the currently-suspect ones. Omitting
// t1_import (reported by both boundaries here, and NOT itself suspect) is
// ErrInvalidRequest, before any write.
func TestResolvingAResetMissingARequiredRegisterFailsValidation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	id := tenant.Analyzers[0].ID
	h := anomaliesEpoch

	readingRepo := pathsNewReadingRepo(pool)
	anomalyRepo := postgres.NewAnomalyRepository(pool)
	opsRepo := postgres.NewOpsRepository(pool)
	analyzerRepo := postgres.NewAnalyzerRepository(pool)

	// Both boundaries report BOTH active_import and t1_import. active_import
	// goes negative (suspect); t1_import stays positive (sound) but is still
	// reported by both boundaries, so R93 requires it in ResetAfter too.
	pathsSeedReading(t, ctx, readingRepo, tenant.Scope, readingRow(id, h, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000", "t1_import": "500"}))
	pathsSeedReading(t, ctx, readingRepo, tenant.Scope, readingRow(id, h.Add(time.Hour), model.ReadingKindLoadProfile, map[string]string{"active_import": "900", "t1_import": "600"}))

	billing := anomaliesNewBilling(t, readingRepo, anomalyRepo, opsRepo, analyzerRepo, lock.NewMemory(nil), h.Add(time.Hour))
	req := consumption.SeriesRequest{AnalyzerIDs: []uuid.UUID{id}, Level: energy.Hourly, Range: store.TimeRange{From: h, To: h.Add(time.Hour)}}

	_, err := billing.ConsumptionAndRecord(ctx, tenant.Scope, req)
	require.NoError(t, err)

	an, err := billing.ListAnomalies(ctx, tenant.AdminScope, consumption.AnomalyListRequest{AnalyzerIDs: []uuid.UUID{id}, Unresolved: true})
	require.NoError(t, err)
	require.Len(t, an, 1)

	rangeBefore, err := readingRepo.Range(ctx, tenant.Scope, id, store.TimeRange{From: h, To: h.Add(time.Hour + time.Minute)}, model.ReadingKindReset)
	require.NoError(t, err)
	require.Empty(t, rangeBefore)

	operator := tenant.Users[model.UserRoleCompanyAdmin]
	resetTS := h.Add(30 * time.Minute)
	_, err = billing.ResolveAnomaly(ctx, tenant.Scope, an[0].ID, operator.ID, consumption.Resolution{
		Mode:    consumption.ResolveByRegisteringReset,
		ResetTS: &resetTS,
		// t1_import deliberately omitted, even though both boundaries report it.
		ResetAfter: map[energy.Register]decimal.Decimal{energy.ActiveImport: decimal.RequireFromString("100")},
	})
	require.ErrorIs(t, err, consumption.ErrInvalidRequest)

	// Nothing was written: no reset reading, and the anomaly is still
	// unresolved.
	rangeAfter, err := readingRepo.Range(ctx, tenant.Scope, id, store.TimeRange{From: h, To: h.Add(time.Hour + time.Minute)}, model.ReadingKindReset)
	require.NoError(t, err)
	require.Empty(t, rangeAfter)

	got, err := anomalyRepo.Get(ctx, tenant.Scope, an[0].ID)
	require.NoError(t, err)
	require.Nil(t, got.ResolvedAt)
}

// TestAnOverrideMustNameASuspectRegister: an override naming a register that
// was never suspect for this anomaly is ErrInvalidRequest.
func TestAnOverrideMustNameASuspectRegister(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	id := tenant.Analyzers[0].ID
	h := anomaliesEpoch

	readingRepo := pathsNewReadingRepo(pool)
	anomalyRepo := postgres.NewAnomalyRepository(pool)
	opsRepo := postgres.NewOpsRepository(pool)
	analyzerRepo := postgres.NewAnalyzerRepository(pool)

	pathsSeedReading(t, ctx, readingRepo, tenant.Scope, readingRow(id, h, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}))
	pathsSeedReading(t, ctx, readingRepo, tenant.Scope, readingRow(id, h.Add(time.Hour), model.ReadingKindLoadProfile, map[string]string{"active_import": "900"}))

	billing := anomaliesNewBilling(t, readingRepo, anomalyRepo, opsRepo, analyzerRepo, lock.NewMemory(nil), h.Add(time.Hour))
	req := consumption.SeriesRequest{AnalyzerIDs: []uuid.UUID{id}, Level: energy.Hourly, Range: store.TimeRange{From: h, To: h.Add(time.Hour)}}

	_, err := billing.ConsumptionAndRecord(ctx, tenant.Scope, req)
	require.NoError(t, err)

	an, err := billing.ListAnomalies(ctx, tenant.AdminScope, consumption.AnomalyListRequest{AnalyzerIDs: []uuid.UUID{id}, Unresolved: true})
	require.NoError(t, err)
	require.Len(t, an, 1)

	operator := tenant.Users[model.UserRoleCompanyAdmin]
	_, err = billing.ResolveAnomaly(ctx, tenant.Scope, an[0].ID, operator.ID, consumption.Resolution{
		Mode:      consumption.ResolveByOverride,
		Overrides: map[energy.Register]decimal.Decimal{energy.T1Import: decimal.RequireFromString("50")},
	})
	require.ErrorIs(t, err, consumption.ErrInvalidRequest)

	got, err := anomalyRepo.Get(ctx, tenant.Scope, an[0].ID)
	require.NoError(t, err)
	require.Nil(t, got.ResolvedAt)
}

// TestResolvingAnotherTenantsAnomalyIsNotFound (F1 ruling 4): the cross-
// tenant attempt uses tenant A's AdminScope against tenant B's real
// anomaly, and must fail with store.ErrNotFound BEFORE any write; a
// positive control afterward proves the anomaly was real by letting tenant
// B resolve it for itself.
func TestResolvingAnotherTenantsAnomalyIsNotFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenantA := testfixtures.NewTenant(t, ctx, pool, 1)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 2)
	id := tenantB.Analyzers[0].ID
	h := anomaliesEpoch

	readingRepo := pathsNewReadingRepo(pool)
	anomalyRepo := postgres.NewAnomalyRepository(pool)
	opsRepo := postgres.NewOpsRepository(pool)
	analyzerRepo := postgres.NewAnalyzerRepository(pool)

	pathsSeedReading(t, ctx, readingRepo, tenantB.Scope, readingRow(id, h, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}))
	pathsSeedReading(t, ctx, readingRepo, tenantB.Scope, readingRow(id, h.Add(time.Hour), model.ReadingKindLoadProfile, map[string]string{"active_import": "900"}))

	billing := anomaliesNewBilling(t, readingRepo, anomalyRepo, opsRepo, analyzerRepo, lock.NewMemory(nil), h.Add(time.Hour))
	req := consumption.SeriesRequest{AnalyzerIDs: []uuid.UUID{id}, Level: energy.Hourly, Range: store.TimeRange{From: h, To: h.Add(time.Hour)}}

	_, err := billing.ConsumptionAndRecord(ctx, tenantB.Scope, req)
	require.NoError(t, err)

	an, err := billing.ListAnomalies(ctx, tenantB.AdminScope, consumption.AnomalyListRequest{AnalyzerIDs: []uuid.UUID{id}, Unresolved: true})
	require.NoError(t, err)
	require.Len(t, an, 1)

	// Cross-tenant: tenant A's AdminScope must never resolve tenant B's row.
	_, err = billing.ResolveAnomaly(ctx, tenantA.AdminScope, an[0].ID, uuid.New(), consumption.Resolution{Mode: consumption.ResolveByAccepting})
	require.ErrorIs(t, err, store.ErrNotFound)

	// Positive control: the SAME anomaly is real and tenant B can resolve it.
	operatorB := tenantB.Users[model.UserRoleCompanyAdmin]
	resolved, err := billing.ResolveAnomaly(ctx, tenantB.Scope, an[0].ID, operatorB.ID, consumption.Resolution{Mode: consumption.ResolveByAccepting})
	require.NoError(t, err)
	require.NotNil(t, resolved.ResolvedAt)
}

// TestAcceptedLeavesTheValueNullButClearsTheBlock: an accepted resolution
// stamps resolved_at, leaves Values nil (never a number) but records
// Row.Resolution, and re-running ConsumptionAndRecord for the same,
// now-resolved period never resurrects a second row (C-6's resolved+
// unresolved dedup).
func TestAcceptedLeavesTheValueNullButClearsTheBlock(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	id := tenant.Analyzers[0].ID
	h := anomaliesEpoch

	readingRepo := pathsNewReadingRepo(pool)
	anomalyRepo := postgres.NewAnomalyRepository(pool)
	opsRepo := postgres.NewOpsRepository(pool)
	analyzerRepo := postgres.NewAnalyzerRepository(pool)

	pathsSeedReading(t, ctx, readingRepo, tenant.Scope, readingRow(id, h, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}))
	pathsSeedReading(t, ctx, readingRepo, tenant.Scope, readingRow(id, h.Add(time.Hour), model.ReadingKindLoadProfile, map[string]string{"active_import": "900"}))

	billing := anomaliesNewBilling(t, readingRepo, anomalyRepo, opsRepo, analyzerRepo, lock.NewMemory(nil), h.Add(time.Hour))
	req := consumption.SeriesRequest{AnalyzerIDs: []uuid.UUID{id}, Level: energy.Hourly, Range: store.TimeRange{From: h, To: h.Add(time.Hour)}}

	_, err := billing.ConsumptionAndRecord(ctx, tenant.Scope, req)
	require.NoError(t, err)
	an, err := billing.ListAnomalies(ctx, tenant.AdminScope, consumption.AnomalyListRequest{AnalyzerIDs: []uuid.UUID{id}, Unresolved: true})
	require.NoError(t, err)
	require.Len(t, an, 1)

	operator := tenant.Users[model.UserRoleCompanyAdmin]
	resolved, err := billing.ResolveAnomaly(ctx, tenant.Scope, an[0].ID, operator.ID, consumption.Resolution{Mode: consumption.ResolveByAccepting})
	require.NoError(t, err)
	require.NotNil(t, resolved.ResolvedAt)
	require.NotNil(t, resolved.Resolution)
	require.Equal(t, "accepted", *resolved.Resolution)

	rows, err := billing.Consumption(ctx, tenant.Scope, req)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Nil(t, rows[0].Values[energy.ActiveImport], "still no number")
	require.Equal(t, "accepted", rows[0].Resolution[energy.ActiveImport])

	// C-6: re-running ConsumptionAndRecord for the same, now-RESOLVED period
	// must not resurrect a second anomaly row.
	_, err = billing.ConsumptionAndRecord(ctx, tenant.Scope, req)
	require.NoError(t, err)
	all, err := billing.ListAnomalies(ctx, tenant.AdminScope, consumption.AnomalyListRequest{AnalyzerIDs: []uuid.UUID{id}, Unresolved: false})
	require.NoError(t, err)
	require.Len(t, all, 1, "a resolved anomaly must not be recreated")
}

// TestConsumptionSubstitutesAResolvedManualOverride is C-6's headline test:
// Billing.Consumption ITSELF (never ConsumptionAndRecord) reflects a
// resolved manual_override.
func TestConsumptionSubstitutesAResolvedManualOverride(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	id := tenant.Analyzers[0].ID
	h := anomaliesEpoch

	readingRepo := pathsNewReadingRepo(pool)
	anomalyRepo := postgres.NewAnomalyRepository(pool)
	opsRepo := postgres.NewOpsRepository(pool)
	analyzerRepo := postgres.NewAnalyzerRepository(pool)

	pathsSeedReading(t, ctx, readingRepo, tenant.Scope, readingRow(id, h, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}))
	pathsSeedReading(t, ctx, readingRepo, tenant.Scope, readingRow(id, h.Add(time.Hour), model.ReadingKindLoadProfile, map[string]string{"active_import": "900"}))

	billing := anomaliesNewBilling(t, readingRepo, anomalyRepo, opsRepo, analyzerRepo, lock.NewMemory(nil), h.Add(time.Hour))
	req := consumption.SeriesRequest{AnalyzerIDs: []uuid.UUID{id}, Level: energy.Hourly, Range: store.TimeRange{From: h, To: h.Add(time.Hour)}}

	_, err := billing.ConsumptionAndRecord(ctx, tenant.Scope, req)
	require.NoError(t, err)
	an, err := billing.ListAnomalies(ctx, tenant.AdminScope, consumption.AnomalyListRequest{AnalyzerIDs: []uuid.UUID{id}, Unresolved: true})
	require.NoError(t, err)
	require.Len(t, an, 1)

	operator := tenant.Users[model.UserRoleCompanyAdmin]
	_, err = billing.ResolveAnomaly(ctx, tenant.Scope, an[0].ID, operator.ID, consumption.Resolution{
		Mode:      consumption.ResolveByOverride,
		Overrides: map[energy.Register]decimal.Decimal{energy.ActiveImport: decimal.RequireFromString("999.5")},
	})
	require.NoError(t, err)

	rows, err := billing.Consumption(ctx, tenant.Scope, req) // NOT ConsumptionAndRecord
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.NotNil(t, rows[0].Values[energy.ActiveImport])
	require.Equal(t, "999.5", rows[0].Values[energy.ActiveImport].String())
	require.Equal(t, "manual_override", rows[0].Resolution[energy.ActiveImport])
}

// TestResolvingAnF3PeriodAlsoResolvesTheOverlappingF2IngestionAnomaly is
// I-5: resolving the F3 (billing-path) anomaly also resolves F2's own
// ingestion-time negative_delta row for the same analyzer nested inside the
// F3 period, with the SAME resolution and resolver.
func TestResolvingAnF3PeriodAlsoResolvesTheOverlappingF2IngestionAnomaly(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	id := tenant.Analyzers[0].ID
	h := anomaliesEpoch

	readingRepo := pathsNewReadingRepo(pool)
	anomalyRepo := postgres.NewAnomalyRepository(pool)
	opsRepo := postgres.NewOpsRepository(pool)
	analyzerRepo := postgres.NewAnalyzerRepository(pool)

	pathsSeedReading(t, ctx, readingRepo, tenant.Scope, readingRow(id, h, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}))
	pathsSeedReading(t, ctx, readingRepo, tenant.Scope, readingRow(id, h.Add(20*time.Minute), model.ReadingKindLoadProfile, map[string]string{"active_import": "1040"}))
	pathsSeedReading(t, ctx, readingRepo, tenant.Scope, readingRow(id, h.Add(time.Hour), model.ReadingKindLoadProfile, map[string]string{"active_import": "190"}))

	billing := anomaliesNewBilling(t, readingRepo, anomalyRepo, opsRepo, analyzerRepo, lock.NewMemory(nil), h.Add(time.Hour))
	req := consumption.SeriesRequest{AnalyzerIDs: []uuid.UUID{id}, Level: energy.Hourly, Range: store.TimeRange{From: h, To: h.Add(time.Hour)}}

	_, err := billing.ConsumptionAndRecord(ctx, tenant.Scope, req)
	require.NoError(t, err)
	f3, err := billing.ListAnomalies(ctx, tenant.AdminScope, consumption.AnomalyListRequest{AnalyzerIDs: []uuid.UUID{id}, Unresolved: true})
	require.NoError(t, err)
	require.Len(t, f3, 1)
	f3ID := f3[0].ID

	// F2's own ingestion-time negative_delta row, nested inside the F3
	// period, exactly as internal/ingest/fetch.go writes one (one row per
	// affected register, {"register","kind"} detail shape).
	f2Anomaly, err := anomalyRepo.Create(ctx, tenant.Scope, model.ConsumptionAnomaly{
		AnalyzerID:  id,
		PeriodStart: h.Add(15 * time.Minute),
		PeriodEnd:   h.Add(25 * time.Minute),
		Reason:      "negative_delta",
		Detail:      []byte(`{"register":"active_import","kind":"load_profile"}`),
	})
	require.NoError(t, err)

	// A second analyzer's overlapping-in-time negative_delta row must never
	// be touched (guard (b): the F2 overlap resolution never crosses
	// analyzers).
	otherID := tenant.Analyzers[1].ID
	otherF2, err := anomalyRepo.Create(ctx, tenant.Scope, model.ConsumptionAnomaly{
		AnalyzerID:  otherID,
		PeriodStart: h.Add(15 * time.Minute),
		PeriodEnd:   h.Add(25 * time.Minute),
		Reason:      "negative_delta",
		Detail:      []byte(`{"register":"active_import","kind":"load_profile"}`),
	})
	require.NoError(t, err)

	operator := tenant.Users[model.UserRoleCompanyAdmin]
	resetTS := h.Add(30 * time.Minute)
	_, err = billing.ResolveAnomaly(ctx, tenant.Scope, f3ID, operator.ID, consumption.Resolution{
		Mode:       consumption.ResolveByRegisteringReset,
		ResetTS:    &resetTS,
		ResetAfter: map[energy.Register]decimal.Decimal{energy.ActiveImport: decimal.RequireFromString("100")},
	})
	require.NoError(t, err)

	gotF2, err := anomalyRepo.Get(ctx, tenant.Scope, f2Anomaly.ID)
	require.NoError(t, err)
	require.NotNil(t, gotF2.ResolvedAt, "F2's ingestion-time row for the same event must be resolved too")
	require.NotNil(t, gotF2.Resolution)
	require.Equal(t, "reset_registered", *gotF2.Resolution)

	gotOther, err := anomalyRepo.Get(ctx, tenant.Scope, otherF2.ID)
	require.NoError(t, err)
	require.Nil(t, gotOther.ResolvedAt, "another analyzer's row must never be touched")
}

// TestTwoRegistersSuspectForTheSameReasonWriteOneAnomalyRow is guard (c)'s
// permanent proof: a period with TWO registers suspect for the SAME reason
// writes exactly ONE anomaly row (R58/C-4: one row per reason, listing every
// affected register — never one row per register).
func TestTwoRegistersSuspectForTheSameReasonWriteOneAnomalyRow(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	id := tenant.Analyzers[0].ID
	h := anomaliesEpoch

	readingRepo := pathsNewReadingRepo(pool)
	anomalyRepo := postgres.NewAnomalyRepository(pool)
	opsRepo := postgres.NewOpsRepository(pool)
	analyzerRepo := postgres.NewAnalyzerRepository(pool)

	pathsSeedReading(t, ctx, readingRepo, tenant.Scope, readingRow(id, h, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000", "t1_import": "500"}))
	pathsSeedReading(t, ctx, readingRepo, tenant.Scope, readingRow(id, h.Add(time.Hour), model.ReadingKindLoadProfile, map[string]string{"active_import": "900", "t1_import": "400"}))

	billing := anomaliesNewBilling(t, readingRepo, anomalyRepo, opsRepo, analyzerRepo, lock.NewMemory(nil), h.Add(time.Hour))
	req := consumption.SeriesRequest{AnalyzerIDs: []uuid.UUID{id}, Level: energy.Hourly, Range: store.TimeRange{From: h, To: h.Add(time.Hour)}}

	rows, err := billing.ConsumptionAndRecord(ctx, tenant.Scope, req)
	require.NoError(t, err)
	require.Contains(t, rows[0].Suspect, energy.ActiveImport)
	require.Contains(t, rows[0].Suspect, energy.T1Import)
	require.Equal(t, energy.ReasonNegativeDelta, rows[0].Suspect[energy.ActiveImport].Reason)
	require.Equal(t, energy.ReasonNegativeDelta, rows[0].Suspect[energy.T1Import].Reason)

	an, err := billing.ListAnomalies(ctx, tenant.AdminScope, consumption.AnomalyListRequest{AnalyzerIDs: []uuid.UUID{id}, Unresolved: true})
	require.NoError(t, err)
	require.Len(t, an, 1, "one row per REASON, not one row per register")

	var detail map[string]any
	require.NoError(t, json.Unmarshal(an[0].Detail, &detail))
	regs := detail["registers"].(map[string]any)
	require.Contains(t, regs, "active_import")
	require.Contains(t, regs, "t1_import")
}
