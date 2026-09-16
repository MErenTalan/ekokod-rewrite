//go:build integration

package billing_test

import (
	"context"
	"fmt"
	"math/rand/v2"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	domain "github.com/MErenTalan/ekokod-rewrite/internal/domain/billing"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/lock"
	billingsvc "github.com/MErenTalan/ekokod-rewrite/internal/service/billing"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/consumption"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/admin"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

type harness struct {
	t         *testing.T
	ctx       context.Context
	pool      *pgxpool.Pool
	tenant    testfixtures.Tenant
	loc       *time.Location
	clock     *clock.Fake
	window    energy.Window // building 0's 2026-01 period (cut-off 15)
	readings  store.ReadingRepository
	anomalies store.AnomalyRepository
	bills     store.BillRepository
	tariffs   store.TariffRepository
	ops       store.OpsRepository
	reader    *countingReader
	deps      billingsvc.Deps
	svc       *billingsvc.Service
}

func newHarness(t *testing.T, seed int64) *harness {
	t.Helper()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	loc, err := time.LoadLocation("Europe/Istanbul")
	require.NoError(t, err)
	h := &harness{t: t, ctx: ctx, pool: pool, tenant: testfixtures.NewTenant(t, ctx, pool, seed), loc: loc,
		readings: postgres.NewReadingRepository(pool), anomalies: postgres.NewAnomalyRepository(pool),
		bills: postgres.NewBillRepository(pool), tariffs: postgres.NewTariffRepository(pool), ops: postgres.NewOpsRepository(pool)}
	h.window, err = domain.Period("2026-01", 15, loc)
	require.NoError(t, err)
	h.clock = clock.NewFake(h.window.To.Add(73 * time.Hour))
	cons, err := consumption.NewBilling(consumption.BillingDeps{
		Readings: h.readings, Anomalies: h.anomalies, Ops: h.ops, Clock: h.clock, Log: testfixtures.DiscardLogger(),
		Locker: lock.NewMemory(nil), Analyzers: postgres.NewAnalyzerRepository(pool), Users: postgres.NewUserRepository(pool),
	})
	require.NoError(t, err)
	h.reader = &countingReader{inner: cons}
	h.deps = billingsvc.Deps{
		Consumption: h.reader, Buildings: postgres.NewBuildingRepository(pool), Analyzers: postgres.NewAnalyzerRepository(pool),
		Tariffs: h.tariffs, Params: postgres.NewBillingParameterRepository(pool), Prices: postgres.NewPriceRepository(pool),
		Anomalies: h.anomalies, Bills: h.bills, Ops: h.ops, Clock: h.clock, Log: testfixtures.DiscardLogger(),
	}
	h.svc, err = billingsvc.New(h.deps)
	require.NoError(t, err)
	return h
}

func dec(s string) *decimal.Decimal {
	v := decimal.RequireFromString(s)
	return &v
}

// seed writes one load_profile reading; reg values are strings, "" leaves a register nil.
func (h *harness) seed(analyzer uuid.UUID, ts time.Time, active, inductive, capacitive string) {
	h.t.Helper()
	r := model.MeterReading{AnalyzerID: analyzer, Ts: ts, Kind: model.ReadingKindLoadProfile, MultiplierApplied: decimal.NewFromInt(1),
		SourceProvider: model.IntegrationProviderOSOS, IngestedAt: ts, ActiveExport: dec("0")}
	r.ActiveImport = dec(active)
	if inductive != "" {
		r.ReactiveInductiveImport = dec(inductive)
	}
	if capacitive != "" {
		r.ReactiveCapacitiveImport = dec(capacitive)
	}
	_, _, err := h.readings.BulkInsert(h.ctx, h.tenant.AdminScope, []model.MeterReading{r})
	require.NoError(h.t, err)
}

// seedPeriod writes boundary readings so the window consumes kwh with sound reactive registers.
func (h *harness) seedPeriod(analyzer uuid.UUID, w energy.Window, kwh string) {
	h.seed(analyzer, w.From, "1000", "0", "0")
	end := decimal.RequireFromString(kwh).Add(decimal.NewFromInt(1000))
	h.seed(analyzer, w.To, end.String(), "10", "5")
}

func (h *harness) generate(sc store.Scope, scope model.BillScope, subject uuid.UUID, mutate ...func(*billingsvc.GenerateRequest)) (billingsvc.GenerateResult, error) {
	req := billingsvc.GenerateRequest{Scope: scope, SubjectID: subject, PeriodKey: "2026-01"}
	for _, m := range mutate {
		m(&req)
	}
	return h.svc.Generate(h.ctx, sc, req)
}

func (h *harness) computeCode(err error) string {
	h.t.Helper()
	var ce *billingsvc.ComputeError
	require.ErrorAs(h.t, err, &ce)
	return ce.Code
}

// ptfTariff creates a building-0 single-time PTF tariff effective before the window.
func (h *harness) ptfTariff() model.Tariff {
	h.t.Helper()
	b := h.tenant.Buildings[0].ID
	t, err := h.tariffs.Create(h.ctx, h.tenant.AdminScope, model.Tariff{
		BuildingID: &b, EffectiveFrom: time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC), Currency: model.CurrencyTRY,
		EnergyType: model.EnergyTypeGrid, VoltageLevel: model.VoltageLevelMV, UserGroup: model.UserGroupCommercial,
		PriceType: model.PriceTypeSingleTime, Term: model.TariffTermMonomial, SupplyCompany: model.SupplyCompanyPrivate,
		DistributionCost: decimal.RequireFromString("0.5"), ReactivePowerPrice: decimal.RequireFromString("1"),
		GenerationUsage: model.GenerationUsageNone, VatRate: decimal.NewFromInt(20), UsePtfYekdem: true,
		KbkEnergy: dec("1.1"), KbkReactivePower: dec("0.5"), KbkDistributionCostTlPerKwh: dec("0.8"),
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	})
	require.NoError(h.t, err)
	return t
}

// seedMarket writes PTF for every window hour except the first skip hours, and YEKDEM for Jan and Feb 2026.
func (h *harness) seedMarket(skip int) {
	h.t.Helper()
	repo := admin.NewMarketDataRepository(h.pool)
	var prices []model.MarketPrice
	i := 0
	for ts := h.window.From; ts.Before(h.window.To); ts = ts.Add(time.Hour) {
		if i >= skip {
			prices = append(prices, model.MarketPrice{Ts: ts.UTC(), PTF: decimal.NewFromInt(int64(2000 + i%24*10)), FetchedAt: time.Now().UTC()})
		}
		i++
	}
	_, err := repo.UpsertHourlyPrices(h.ctx, prices)
	require.NoError(h.t, err)
	_, err = repo.UpsertYekdem(h.ctx, []model.YekdemMonthly{{Year: 2026, Month: 1, Value: decimal.NewFromInt(500), FetchedAt: time.Now().UTC()}, {Year: 2026, Month: 2, Value: decimal.NewFromInt(600), FetchedAt: time.Now().UTC()}})
	require.NoError(h.t, err)
}

// seedHourly writes one load_profile reading per hour over the window (1 kWh/h).
func (h *harness) seedHourly(analyzer uuid.UUID) {
	h.t.Helper()
	var rows []model.MeterReading
	v := decimal.NewFromInt(1000)
	for ts := h.window.From; !ts.After(h.window.To); ts = ts.Add(time.Hour) {
		rows = append(rows, model.MeterReading{AnalyzerID: analyzer, Ts: ts, Kind: model.ReadingKindLoadProfile, MultiplierApplied: decimal.NewFromInt(1),
			SourceProvider: model.IntegrationProviderOSOS, IngestedAt: ts, ActiveImport: dec(v.String()), ReactiveInductiveImport: dec("0"),
			ReactiveCapacitiveImport: dec("0"), ActiveExport: dec("0")})
		v = v.Add(decimal.NewFromInt(1))
	}
	_, _, err := h.readings.BulkInsert(h.ctx, h.tenant.AdminScope, rows)
	require.NoError(h.t, err)
}

type countingReader struct {
	inner  billingsvc.ConsumptionReader
	mu     sync.Mutex
	period []int
	hourly []int
}

func (c *countingReader) PeriodConsumptionAndRecord(ctx context.Context, sc store.Scope, req consumption.PeriodRequest) ([]consumption.Row, error) {
	c.mu.Lock()
	c.period = append(c.period, len(req.AnalyzerIDs))
	c.mu.Unlock()
	return c.inner.PeriodConsumptionAndRecord(ctx, sc, req)
}

func (c *countingReader) Consumption(ctx context.Context, sc store.Scope, req consumption.SeriesRequest) ([]consumption.Row, error) {
	c.mu.Lock()
	c.hourly = append(c.hourly, len(req.AnalyzerIDs))
	c.mu.Unlock()
	return c.inner.Consumption(ctx, sc, req)
}

func lineAmounts(t *testing.T, h *harness, billID uuid.UUID) map[string]string {
	t.Helper()
	lines, err := h.bills.Lines(h.ctx, h.tenant.AdminScope, billID)
	require.NoError(t, err)
	out := map[string]string{}
	for _, l := range lines {
		out[l.Code] = l.Amount.StringFixed(2)
	}
	return out
}

func TestGenerateAnalyzerIssuesBillWithLinesAndMembers(t *testing.T) {
	t.Parallel()
	h := newHarness(t, 7001)
	a := h.tenant.Analyzers[0]
	h.seedPeriod(a.ID, h.window, "500")

	res, err := h.generate(h.tenant.Scope, model.BillScopeAnalyzer, a.ID)
	require.NoError(t, err)
	require.True(t, res.Created)
	b := res.Bill
	require.Equal(t, model.BillStatusIssued, b.Status, "flag reason %v", b.FlagReason)
	require.Equal(t, a.ID, *b.AnalyzerID)
	require.Equal(t, h.tenant.Buildings[0].ID, *b.BuildingID)
	require.True(t, b.PeriodStart.Equal(h.window.From))
	require.Equal(t, int32(31), b.DaysInPeriod)
	require.Equal(t, "500", b.NetConsumption.String())
	require.Equal(t, model.CurrencyTRY, b.Currency)
	a2 := lineAmounts(t, h, b.ID)
	require.Equal(t, "1225.62", a2[model.BillLineEnergy], "500 × 2.451234")
	require.Equal(t, "493.83", a2[model.BillLineDistribution])
	require.Equal(t, "6172.84", a2[model.BillLinePower], "500 kW × 12.345678")
	require.Contains(t, a2, model.BillLineVat)
	_, _, members, err := h.svc.Get(h.ctx, h.tenant.Scope, b.ID)
	require.NoError(t, err)
	require.Len(t, members, 1)
	latest, err := h.svc.Latest(h.ctx, h.tenant.Scope, model.BillScopeAnalyzer, a.ID)
	require.NoError(t, err)
	require.Equal(t, b.ID, latest.ID)
}

func TestGenerateCutoff15UsesWindowConsumptionNotCalendarMonth(t *testing.T) {
	t.Parallel()
	h := newHarness(t, 7002)
	a := h.tenant.Analyzers[0].ID
	jan1, feb1 := time.Date(2026, 1, 1, 0, 0, 0, 0, h.loc), time.Date(2026, 2, 1, 0, 0, 0, 0, h.loc)
	h.seed(a, jan1, "100", "0", "0")
	h.seed(a, h.window.From, "500", "0", "0")
	h.seed(a, feb1, "900", "0", "0")
	h.seed(a, h.window.To, "1500", "0", "0")
	res, err := h.generate(h.tenant.Scope, model.BillScopeAnalyzer, a)
	require.NoError(t, err)
	require.Equal(t, "1000", res.Bill.NetConsumption.String(), "15 Jan → 15 Feb, not the calendar month's 800")
}

func TestGenerateBlocksOnUnresolvedAnomaly(t *testing.T) {
	t.Parallel()
	h := newHarness(t, 7003)
	a := h.tenant.Analyzers[0].ID
	h.seedPeriod(a, h.window, "500")
	_, err := h.anomalies.Create(h.ctx, h.tenant.AdminScope, model.ConsumptionAnomaly{AnalyzerID: a, PeriodStart: h.window.From.AddDate(0, 0, 3), PeriodEnd: h.window.From.AddDate(0, 0, 4), Reason: "negative_delta", Detail: []byte(`{}`)})
	require.NoError(t, err)
	_, err = h.generate(h.tenant.Scope, model.BillScopeAnalyzer, a)
	require.Equal(t, billingsvc.CodeUnresolvedAnomaly, h.computeCode(err))
	_, err = h.bills.Current(h.ctx, h.tenant.Scope, model.BillScopeAnalyzer, a, "2026-01")
	require.ErrorIs(t, err, store.ErrNotFound, "no bill persisted")
}

func TestGenerateBlocksOnAnomalyStartingBeforeWindow(t *testing.T) {
	t.Parallel()
	h := newHarness(t, 7004)
	a := h.tenant.Analyzers[0].ID
	h.seedPeriod(a, h.window, "500")
	_, err := h.anomalies.Create(h.ctx, h.tenant.AdminScope, model.ConsumptionAnomaly{AnalyzerID: a,
		PeriodStart: time.Date(2026, 1, 1, 0, 0, 0, 0, h.loc), PeriodEnd: time.Date(2026, 2, 1, 0, 0, 0, 0, h.loc), Reason: "meter_reset", Detail: []byte(`{}`)})
	require.NoError(t, err)
	_, err = h.generate(h.tenant.Scope, model.BillScopeAnalyzer, a)
	require.Equal(t, billingsvc.CodeUnresolvedAnomaly, h.computeCode(err))
}

func TestGenerateBlocksOnSuspectRow(t *testing.T) {
	t.Parallel()
	h := newHarness(t, 7005)
	a := h.tenant.Analyzers[0].ID
	h.seed(a, h.window.From, "1000", "0", "0")
	h.seed(a, h.window.To, "900", "0", "0") // negative delta
	_, err := h.generate(h.tenant.Scope, model.BillScopeAnalyzer, a)
	require.Equal(t, billingsvc.CodeUnresolvedAnomaly, h.computeCode(err))
}

func TestGenerateNoTariffFails(t *testing.T) {
	t.Parallel()
	h := newHarness(t, 7006)
	a := h.tenant.Analyzers[0].ID
	h.seedPeriod(a, h.window, "500")
	require.NoError(t, h.tariffs.SoftDelete(h.ctx, h.tenant.AdminScope, h.tenant.Tariffs[0].ID, time.Now()))
	_, err := h.generate(h.tenant.Scope, model.BillScopeAnalyzer, a)
	require.Equal(t, billingsvc.CodeTariffNotFound, h.computeCode(err))
}

func TestGeneratePeriodNotClosed(t *testing.T) {
	t.Parallel()
	h := newHarness(t, 7007)
	a := h.tenant.Analyzers[0].ID
	h.seedPeriod(a, h.window, "500")
	h.clock.Set(h.window.To.Add(consumption.SettleDelayMonthly - time.Minute))
	_, err := h.generate(h.tenant.Scope, model.BillScopeAnalyzer, a)
	require.Equal(t, billingsvc.CodePeriodNotClosed, h.computeCode(err))
	h.clock.Set(h.window.To.Add(consumption.SettleDelayMonthly))
	_, err = h.generate(h.tenant.Scope, model.BillScopeAnalyzer, a)
	require.NoError(t, err)
}

func TestGenerateMissingMemberRowFails(t *testing.T) {
	t.Parallel()
	h := newHarness(t, 7008)
	h.seedPeriod(h.tenant.Analyzers[0].ID, h.window, "500") // Analyzers[1] is silent
	_, err := h.generate(h.tenant.Scope, model.BillScopeBuilding, h.tenant.Buildings[0].ID)
	require.Equal(t, billingsvc.CodeNoConsumptionData, h.computeCode(err))
}

func TestGenerateBillingParametersInvalidFails(t *testing.T) {
	t.Parallel()
	h := newHarness(t, 7009)
	a := h.tenant.Analyzers[0].ID
	h.seedPeriod(a, h.window, "500")
	p, err := h.deps.Params.Effective(h.ctx, h.tenant.Scope, h.window.From)
	require.NoError(t, err)
	p.EffectiveFrom = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	p.ReactiveBands[1].MinKw = decimal.NewFromInt(31) // gap
	_, err = admin.NewCatalogueRepository(h.pool).UpsertBillingParameters(h.ctx, p)
	require.NoError(t, err)
	_, err = h.generate(h.tenant.Scope, model.BillScopeAnalyzer, a)
	require.Equal(t, billingsvc.CodeBillingParametersInvalid, h.computeCode(err))
}

func TestGeneratePTFMissingHoursFlagsInsteadOfIssuing(t *testing.T) {
	t.Parallel()
	h := newHarness(t, 7010)
	a := h.tenant.Analyzers[0].ID
	h.ptfTariff()
	h.seedHourly(a)
	h.seedMarket(48) // 48 of 744 hours without PTF: 6.45 % > 2 %
	res, err := h.generate(h.tenant.Scope, model.BillScopeAnalyzer, a)
	require.NoError(t, err)
	require.Equal(t, model.BillStatusFlagged, res.Bill.Status)
	require.Equal(t, "ptf_data_missing", *res.Bill.FlagReason)
	require.Equal(t, int32(744), *res.Bill.PtfHoursExpected)
	require.Equal(t, int32(48), *res.Bill.PtfHoursMissing)

	msgs, err := h.ops.ListMessages(h.ctx, h.tenant.AdminScope, store.MessageFilter{RelatedID: &res.Bill.ID})
	require.NoError(t, err)
	require.Len(t, msgs, 1, "TestGenerateFlaggedBillAppendsOpsMessage")
	require.Equal(t, "bill-generation", msgs[0].Category)
}

func TestGeneratePTFHourlyPersistsHourlyDetail(t *testing.T) {
	t.Parallel()
	h := newHarness(t, 7011)
	a := h.tenant.Analyzers[0].ID
	h.ptfTariff()
	h.seedHourly(a)
	h.seedMarket(0)
	res, err := h.generate(h.tenant.Scope, model.BillScopeAnalyzer, a)
	require.NoError(t, err)
	require.Equal(t, model.BillStatusIssued, res.Bill.Status, "%v", res.Bill.FlagReason)
	require.True(t, res.Bill.PtfYekdemUsed)
	require.Equal(t, "744", res.Bill.NetConsumption.String())
	detail, err := h.svc.HourlyDetail(h.ctx, h.tenant.Scope, res.Bill.ID)
	require.NoError(t, err)
	require.Len(t, detail, 744)
	require.Equal(t, int32(744), *res.Bill.PtfHoursMatched)

	// M-9: a live bill that lost its hourly rows gets them back on a non-force Generate.
	_, err = h.bills.ReplaceHourlyDetail(h.ctx, h.tenant.Scope, res.Bill.ID, nil)
	require.NoError(t, err)
	again, err := h.generate(h.tenant.Scope, model.BillScopeAnalyzer, a)
	require.NoError(t, err)
	require.False(t, again.Created)
	detail, err = h.svc.HourlyDetail(h.ctx, h.tenant.Scope, res.Bill.ID)
	require.NoError(t, err)
	require.Len(t, detail, 744)
}

func TestGenerateIsIdempotentWithoutForce(t *testing.T) {
	t.Parallel()
	h := newHarness(t, 7012)
	a := h.tenant.Analyzers[0].ID
	h.seedPeriod(a, h.window, "500")
	first, err := h.generate(h.tenant.Scope, model.BillScopeAnalyzer, a)
	require.NoError(t, err)
	second, err := h.generate(h.tenant.Scope, model.BillScopeAnalyzer, a)
	require.NoError(t, err)
	require.False(t, second.Created)
	require.Equal(t, first.Bill.ID, second.Bill.ID)
}

func TestGenerateForceSupersedes(t *testing.T) {
	t.Parallel()
	h := newHarness(t, 7013)
	a := h.tenant.Analyzers[0].ID
	h.seedPeriod(a, h.window, "500")
	first, err := h.generate(h.tenant.Scope, model.BillScopeAnalyzer, a)
	require.NoError(t, err)
	forced, err := h.generate(h.tenant.Scope, model.BillScopeAnalyzer, a, func(r *billingsvc.GenerateRequest) { r.Force = true })
	require.NoError(t, err)
	require.True(t, forced.Created)
	require.Equal(t, first.Bill.ID, *forced.Superseded)
	old, err := h.bills.Get(h.ctx, h.tenant.Scope, first.Bill.ID)
	require.NoError(t, err)
	require.Equal(t, model.BillStatusSuperseded, old.Status)
}

// conflictingBills hides the live bill from the first Current call, as a
// concurrent Generate that has not committed yet would.
type conflictingBills struct {
	store.BillRepository
	hidden bool
}

func (c *conflictingBills) Current(ctx context.Context, s store.Scope, scope model.BillScope, subject uuid.UUID, key string) (model.Bill, error) {
	if !c.hidden {
		c.hidden = true
		return model.Bill{}, store.ErrNotFound
	}
	return c.BillRepository.Current(ctx, s, scope, subject, key)
}

func TestGenerateConcurrentNonForceReturnsExistingOnConflict(t *testing.T) {
	t.Parallel()
	h := newHarness(t, 7014)
	a := h.tenant.Analyzers[0].ID
	h.seedPeriod(a, h.window, "500")
	first, err := h.generate(h.tenant.Scope, model.BillScopeAnalyzer, a)
	require.NoError(t, err)
	deps := h.deps
	deps.Bills = &conflictingBills{BillRepository: h.bills}
	svc, err := billingsvc.New(deps)
	require.NoError(t, err)
	res, err := svc.Generate(h.ctx, h.tenant.Scope, billingsvc.GenerateRequest{Scope: model.BillScopeAnalyzer, SubjectID: a, PeriodKey: "2026-01"})
	require.NoError(t, err)
	require.False(t, res.Created)
	require.Equal(t, first.Bill.ID, res.Bill.ID)
}

func TestGenerateRecomputesFlaggedButNeverForcesIssued(t *testing.T) {
	t.Parallel()
	h := newHarness(t, 7015)
	a := h.tenant.Analyzers[0].ID
	h.seed(a, h.window.From, "1000", "", "")
	h.seed(a, h.window.To, "1500", "", "") // no reactive registers → reactive_data_missing
	flagged, err := h.generate(h.tenant.Scope, model.BillScopeAnalyzer, a)
	require.NoError(t, err)
	require.Equal(t, model.BillStatusFlagged, flagged.Bill.Status)

	h.seedPeriod(a, h.window, "500") // the data arrives
	plain, err := h.generate(h.tenant.Scope, model.BillScopeAnalyzer, a)
	require.NoError(t, err)
	require.False(t, plain.Created, "without RecomputeFlagged the flagged bill stays")
	recompute := func(r *billingsvc.GenerateRequest) { r.RecomputeFlagged = true }
	fixed, err := h.generate(h.tenant.Scope, model.BillScopeAnalyzer, a, recompute)
	require.NoError(t, err)
	require.True(t, fixed.Created)
	require.Equal(t, model.BillStatusIssued, fixed.Bill.Status)

	again, err := h.generate(h.tenant.Scope, model.BillScopeAnalyzer, a, recompute)
	require.NoError(t, err)
	require.False(t, again.Created, "an issued bill is never recomputed without Force")
	require.Equal(t, fixed.Bill.ID, again.Bill.ID)
}

func TestGenerateBuildingWith51AnalyzersChunks(t *testing.T) {
	t.Parallel()
	h := newHarness(t, 7016)
	b := h.tenant.Buildings[0].ID
	h.ptfTariff()
	h.seedMarket(0)
	ids := []uuid.UUID{h.tenant.Analyzers[0].ID, h.tenant.Analyzers[1].ID}
	for i := range 49 {
		id := uuid.New()
		_, err := h.pool.Exec(h.ctx, `insert into analyzers (id, company_id, building_id, provider, provider_subtype, installation_number, meter_multiplier, is_active, installed_power_kw, created_at, updated_at)
			select $1, company_id, building_id, provider, provider_subtype, installation_number || $2, meter_multiplier, is_active, installed_power_kw, created_at, updated_at from analyzers where id = $3`,
			id, fmt.Sprintf("-%d", i), ids[0])
		require.NoError(t, err)
		ids = append(ids, id)
	}
	for _, id := range ids {
		h.seedPeriod(id, h.window, "100")
	}
	res, err := h.generate(h.tenant.AdminScope, model.BillScopeBuilding, b)
	require.NoError(t, err)
	require.Equal(t, []int{50, 1}, h.reader.period)
	require.Equal(t, []int{50, 1}, h.reader.hourly)
	_, _, members, err := h.svc.Get(h.ctx, h.tenant.AdminScope, res.Bill.ID)
	require.NoError(t, err)
	require.Len(t, members, 51)
	require.Equal(t, "5100", res.Bill.NetConsumption.String())
}

func TestGenerateBuildingAppliesTariffOnceToAggregate(t *testing.T) {
	t.Parallel()
	h := newHarness(t, 7017)
	for _, a := range h.tenant.Analyzers[:2] {
		h.seedPeriod(a.ID, h.window, "400")
	}
	res, err := h.generate(h.tenant.Scope, model.BillScopeBuilding, h.tenant.Buildings[0].ID)
	require.NoError(t, err)
	require.Equal(t, "800", res.Bill.NetConsumption.String())
	amounts := lineAmounts(t, h, res.Bill.ID)
	require.Equal(t, "1960.99", amounts[model.BillLineEnergy], "800 × 2.451234 once")
	require.Equal(t, "6172.84", amounts[model.BillLinePower], "one contracted power charge, not one per analyzer")
	require.Nil(t, res.Bill.MaxDemandKw, "R111: no coincident peak for two members")
	require.Equal(t, h.tenant.Buildings[0].ID, *res.Bill.BuildingID)
	require.Nil(t, res.Bill.AnalyzerID)
	msgs, err := h.ops.ListMessages(h.ctx, h.tenant.AdminScope, store.MessageFilter{RelatedID: &res.Bill.ID})
	require.NoError(t, err)
	require.Len(t, msgs, 1, "M-12: demand unavailable on a multi-analyzer binomial building")
}

// companyFixture gives building 1 cut-off 1 and a dearer tariff, and seeds every analyzer.
func (h *harness) companyFixture() energy.Window {
	h.t.Helper()
	b1 := h.tenant.Buildings[1]
	_, err := h.pool.Exec(h.ctx, `update buildings set bill_cutoff_day = 1 where id = $1`, b1.ID)
	require.NoError(h.t, err)
	tar := h.tenant.Tariffs[1]
	tar.SingleTimePrice = dec("3")
	_, err = h.tariffs.Update(h.ctx, h.tenant.AdminScope, tar)
	require.NoError(h.t, err)
	w1, err := domain.Period("2026-01", 1, h.loc)
	require.NoError(h.t, err)
	for _, a := range h.tenant.Analyzers {
		if *a.BuildingID == b1.ID {
			h.seedPeriod(a.ID, w1, "300")
		} else {
			h.seedPeriod(a.ID, h.window, "400")
		}
	}
	return w1
}

func TestGenerateCompanyUsesEachBuildingsOwnTariffAndWindow(t *testing.T) {
	t.Parallel()
	h := newHarness(t, 7018)
	w1 := h.companyFixture()
	company, err := h.generate(h.tenant.AdminScope, model.BillScopeCompany, h.tenant.Company.ID)
	require.NoError(t, err)
	b0, err := h.generate(h.tenant.AdminScope, model.BillScopeBuilding, h.tenant.Buildings[0].ID)
	require.NoError(t, err)
	b1, err := h.generate(h.tenant.AdminScope, model.BillScopeBuilding, h.tenant.Buildings[1].ID)
	require.NoError(t, err)

	c := company.Bill
	require.Nil(t, c.TariffID)
	require.Nil(t, c.BuildingID)
	require.True(t, c.PeriodStart.Equal(w1.From), "hull starts at building 1's Jan 1")
	require.True(t, c.PeriodEnd.Equal(h.window.To), "hull ends at building 0's Feb 15")
	require.True(t, c.TotalCost.Equal(b0.Bill.TotalCost.Add(b1.Bill.TotalCost)), "R115: company = Σ buildings")
	require.True(t, c.VatCost.Equal(b0.Bill.VatCost.Add(b1.Bill.VatCost)))
	require.Equal(t, "1400", c.NetConsumption.String())
	lines, err := h.bills.Lines(h.ctx, h.tenant.AdminScope, c.ID)
	require.NoError(t, err)
	for _, l := range lines {
		if l.Code == model.BillLineEnergy {
			require.Nil(t, l.UnitPrice, "different building prices merge to nil (M-7)")
			require.Equal(t, "3760.99", l.Amount.StringFixed(2), "800 × 2.451234 + 600 × 3")
		}
	}
}

func TestGenerateCompanySkipsAnalyzerlessBuilding(t *testing.T) {
	t.Parallel()
	h := newHarness(t, 7019)
	h.companyFixture()
	_, err := h.pool.Exec(h.ctx, `update analyzers set deleted_at = now() where building_id = $1`, h.tenant.Buildings[1].ID)
	require.NoError(t, err)
	res, err := h.generate(h.tenant.AdminScope, model.BillScopeCompany, h.tenant.Company.ID)
	require.NoError(t, err)
	_, _, members, err := h.svc.Get(h.ctx, h.tenant.AdminScope, res.Bill.ID)
	require.NoError(t, err)
	require.Len(t, members, 2)
	require.Equal(t, "800", res.Bill.NetConsumption.String())
}

func TestGenerateCompanyRefusesBuildingRestrictedScope(t *testing.T) {
	t.Parallel()
	h := newHarness(t, 7020)
	h.companyFixture()
	_, err := h.generate(h.tenant.Scope, model.BillScopeCompany, h.tenant.Company.ID)
	require.ErrorIs(t, err, billingsvc.ErrInvalidRequest)
	_, err = h.generate(h.tenant.AdminScope, model.BillScopeCompany, uuid.New())
	require.ErrorIs(t, err, billingsvc.ErrInvalidRequest)
	res, err := h.generate(h.tenant.AdminScope, model.BillScopeCompany, h.tenant.Company.ID)
	require.NoError(t, err, "positive control")
	require.Equal(t, model.BillScopeCompany, res.Bill.Scope)
}

func TestGenerateIsolatesTenants(t *testing.T) {
	t.Parallel()
	h := newHarness(t, 7021)
	other := testfixtures.NewTenant(t, h.ctx, h.pool, 7121)
	a := h.tenant.Analyzers[0].ID
	h.seedPeriod(a, h.window, "500")
	_, err := h.generate(other.AdminScope, model.BillScopeAnalyzer, a)
	require.ErrorIs(t, err, store.ErrNotFound)
	_, err = h.generate(other.AdminScope, model.BillScopeBuilding, h.tenant.Buildings[0].ID)
	require.ErrorIs(t, err, store.ErrNotFound)
	res, err := h.generate(h.tenant.AdminScope, model.BillScopeAnalyzer, a)
	require.NoError(t, err, "positive control")
	_, _, _, err = h.svc.Get(h.ctx, other.AdminScope, res.Bill.ID)
	require.ErrorIs(t, err, store.ErrNotFound)
}

// TestGenerateMatchesDomainComputeOverRandomTariffs is Task 7's end-to-end
// brute force: Generate must persist exactly what billing.Compute prices for
// the same tariff, parameters and consumption.
func TestGenerateMatchesDomainComputeOverRandomTariffs(t *testing.T) {
	t.Parallel()
	h := newHarness(t, 7022)
	a := h.tenant.Analyzers[0]
	h.seed(a.ID, h.window.From, "1000", "0", "0")
	h.seed(a.ID, h.window.To, "2734.567", "812.25", "40.5")
	params, err := h.deps.Params.Effective(h.ctx, h.tenant.Scope, h.window.From)
	require.NoError(t, err)
	rng := rand.New(rand.NewPCG(20260917, 7))
	pick := func(n int) int { return rng.IntN(n) }
	money := func(maxUnits int64) *decimal.Decimal {
		return dec(decimal.New(rng.Int64N(maxUnits*1_000_000), -6).String())
	}
	n := 200
	if testing.Short() {
		n = 20
	}
	for i := range n {
		tar := h.tenant.Tariffs[0]
		tar.UserGroup = model.DistributionUserGroups()[pick(9)]
		tar.VoltageLevel = model.VoltageLevels()[pick(2)]
		tar.SupplyCompany = model.SupplyCompanies()[pick(2)]
		tar.Term = model.TariffTerms()[pick(2)]
		tar.SingleTimePrice, tar.OverusePrice = money(5), money(7)
		tar.DistributionCost, tar.ReactivePowerPrice, tar.VatRate = *money(2), *money(3), *dec("20")
		tar.ContractedPowerKw, tar.PowerUnitPrice = nil, nil
		if tar.Term == model.TariffTermBinomial {
			tar.ContractedPowerKw, tar.PowerUnitPrice = dec("300"), money(50)
		}
		_, err := h.tariffs.Update(h.ctx, h.tenant.AdminScope, tar)
		require.NoError(t, err)
		taxes := []model.TariffTax{{Name: "BTV", Rate: *money(10)}}
		_, err = h.tariffs.ReplaceTaxes(h.ctx, h.tenant.AdminScope, tar.ID, taxes)
		require.NoError(t, err)

		res, err := h.generate(h.tenant.Scope, model.BillScopeAnalyzer, a.ID, func(r *billingsvc.GenerateRequest) { r.Force = true })
		require.NoError(t, err, "scenario %d", i)

		stored, err := h.tariffs.Get(h.ctx, h.tenant.AdminScope, tar.ID)
		require.NoError(t, err)
		storedTaxes, err := h.tariffs.Taxes(h.ctx, h.tenant.AdminScope, tar.ID)
		require.NoError(t, err)
		want, err := domain.Compute(domain.Input{
			PeriodKey: "2026-01", Period: h.window, Days: 31, Tariff: stored, Taxes: storedTaxes, Params: params,
			Quantities: domain.Quantities{ActiveImport: dec("1734.567"), ReactiveInductive: dec("812.25"), ReactiveCapacitive: dec("40.5"),
				ActiveExport: dec("0")},
			InstalledPowerKw: a.InstalledPowerKw,
		})
		require.NoError(t, err)
		require.True(t, want.TotalCost.Equal(res.Bill.TotalCost), "scenario %d: total %s vs %s", i, want.TotalCost, res.Bill.TotalCost)
		require.True(t, want.VatBase.Equal(res.Bill.VatBase), "scenario %d", i)
		got := lineAmounts(t, h, res.Bill.ID)
		require.Len(t, got, len(want.Lines), "scenario %d", i)
		for _, l := range want.Lines {
			require.Equal(t, l.Amount.StringFixed(2), got[l.Code], "scenario %d line %s", i, l.Code)
		}
	}
}
