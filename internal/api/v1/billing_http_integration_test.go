//go:build integration

package v1_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/dto"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/seed"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
)

func validTariff(building uuid.UUID) map[string]any {
	return map[string]any{
		"building_id": building.String(), "name": "Ticari AG", "effective_from": "2026-01-01", "currency": "TRY",
		"energy_type": "grid_energy", "voltage_level": "lv", "user_group": "commercial", "price_type": "single_time",
		"term": "monomial", "supply_company": "incumbent", "single_time_price": "3.4547", "distribution_cost": "2.4794",
		"reactive_power_price": "3.4937", "generation_usage": "none", "vat_rate": "20",
		"taxes": []map[string]any{{"name": "BTV", "rate": "5"}},
	}
}

func TestTariffDTORequiresVatAndTaxRates(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ca := h.as(seed.E2ECompanyAdminEmail)
	body := validTariff(h.fx.BuildingA1)
	delete(body, "vat_rate")
	body["taxes"] = []map[string]any{{"name": "BTV"}}
	res := ca.do(http.MethodPost, "/tariffs", body)
	require.Equal(t, http.StatusUnprocessableEntity, res.status, string(res.body))
	var env dto.Error
	res.json(t, &env)
	require.Equal(t, []any{"required"}, env.Error.Details["vat_rate"])
	require.Equal(t, []any{"required"}, env.Error.Details["taxes[0].rate"])

	body = validTariff(h.fx.BuildingA1)
	body["term"] = "binomial"
	res = ca.do(http.MethodPost, "/tariffs", body)
	require.Equal(t, http.StatusUnprocessableEntity, res.status, string(res.body))
	res.json(t, &env)
	require.Equal(t, []any{"required"}, env.Error.Details["contracted_power_kw"], "domain validation (R128) surfaces its field codes")
}

func TestTariffLifecycle(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ca := h.as(seed.E2ECompanyAdminEmail)
	res := ca.do(http.MethodPost, "/tariffs", validTariff(h.fx.BuildingA1))
	require.Equal(t, http.StatusCreated, res.status, string(res.body))
	var created dto.Tariff
	res.json(t, &created)
	require.Equal(t, "20", created.VatRate.String())
	require.Equal(t, "kbk", created.PowerPriceSource, "unset price sources default to kbk (F4)")
	require.Len(t, created.Taxes, 1)

	var applicable dto.Tariff
	res = ca.do(http.MethodGet, "/tariffs/applicable?building_id="+h.fx.BuildingA1.String()+"&date=2026-09-01", nil)
	require.Equal(t, http.StatusOK, res.status, string(res.body))
	res.json(t, &applicable)
	require.Equal(t, created.ID, applicable.ID)

	body := validTariff(h.fx.BuildingA1)
	body["single_time_price"] = "4.1"
	res = ca.do(http.MethodPatch, "/tariffs/"+created.ID.String(), body)
	require.Equal(t, http.StatusOK, res.status, string(res.body))

	ba := h.as(seed.E2EBuildingAdminEmail)
	require.Equal(t, http.StatusOK, ba.do(http.MethodGet, "/tariffs/"+created.ID.String(), nil).status)
	require.Equal(t, http.StatusForbidden, ba.do(http.MethodPost, "/tariffs", validTariff(h.fx.BuildingA1)).status)
	require.Equal(t, http.StatusNotFound, h.as(seed.E2ECompanyBAdminEmail).do(http.MethodGet, "/tariffs/"+created.ID.String(), nil).status)
	require.Equal(t, http.StatusNoContent, ca.do(http.MethodDelete, "/tariffs/"+created.ID.String(), nil).status)
}

func (h *harness) bill(building uuid.UUID) model.Bill {
	h.t.Helper()
	b, err := postgres.NewBillRepository(h.pool).Create(h.t.Context(), store.SystemScope(h.fx.CompanyA), model.Bill{
		CompanyID: h.fx.CompanyA, BuildingID: &building, Scope: model.BillScopeBuilding, PeriodKey: "2026-08",
		PeriodStart: time.Date(2026, 8, 1, 0, 0, 0, 0, ist), PeriodEnd: time.Date(2026, 9, 1, 0, 0, 0, 0, ist), DaysInPeriod: 31,
		TotalCost: decimal.RequireFromString("12345.67"), Currency: model.CurrencyTRY, Status: model.BillStatusIssued,
		GenerationUsage: model.GenerationUsageNone, ComputedAt: h.clock.Now(), IndexStart: json.RawMessage(`{}`), IndexEnd: json.RawMessage(`{}`),
	}, nil, nil)
	require.NoError(h.t, err)
	return b
}

func TestBillPDFServesStoredOrRenders(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	b := h.bill(h.fx.BuildingA1)
	ca := h.as(seed.E2ECompanyAdminEmail)
	res := ca.do(http.MethodGet, "/bills/"+b.ID.String()+"/pdf", nil)
	require.Equal(t, http.StatusOK, res.status, string(res.body))
	require.Equal(t, "application/pdf", res.header.Get("Content-Type"))
	require.True(t, bytes.HasPrefix(res.body, []byte("%PDF")))
	tasks := h.enq.snapshot()
	require.Len(t, tasks, 1)
	require.Equal(t, job.TypeBillingRenderPDF, tasks[0].Type())
	entries, err := os.ReadDir(h.cfg.Storage.Root)
	require.NoError(t, err)
	require.Empty(t, entries, "a GET never writes the file")

	dir := filepath.Join(h.cfg.Storage.Root, "bills", h.fx.CompanyA.String())
	require.NoError(t, os.MkdirAll(dir, 0o750))
	stored := filepath.Join(dir, b.ID.String()+".pdf")
	require.NoError(t, os.WriteFile(stored, []byte("%PDF-stored"), 0o600))
	require.NoError(t, postgres.NewBillRepository(h.pool).SetPDFPath(t.Context(), store.SystemScope(h.fx.CompanyA), b.ID, stored))
	res = ca.do(http.MethodGet, "/bills/"+b.ID.String()+"/pdf", nil)
	require.Equal(t, "%PDF-stored", string(res.body))

	outside := filepath.Join(t.TempDir(), "evil.pdf")
	require.NoError(t, os.WriteFile(outside, []byte("%PDF-outside"), 0o600))
	require.NoError(t, postgres.NewBillRepository(h.pool).SetPDFPath(t.Context(), store.SystemScope(h.fx.CompanyA), b.ID, outside))
	res = ca.do(http.MethodGet, "/bills/"+b.ID.String()+"/pdf", nil)
	require.NotEqual(t, "%PDF-outside", string(res.body), "a path outside the company's bill directory is never served")

	res = ca.do(http.MethodGet, "/bills/"+b.ID.String()+"/hourly-detail?format=xlsx", nil)
	require.Equal(t, http.StatusOK, res.status, string(res.body))
	require.Equal(t, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", res.header.Get("Content-Type"))
	var hours dto.BillHours
	ca.do(http.MethodGet, "/bills/"+b.ID.String()+"/hourly-detail", nil).json(t, &hours)
	require.Empty(t, hours.Items)
}

func TestBillsScopeAndCompute(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	a1, a2 := h.bill(h.fx.BuildingA1), h.bill(h.fx.BuildingA2)
	ba := h.as(seed.E2EBuildingAdminEmail)
	require.Equal(t, http.StatusOK, ba.do(http.MethodGet, "/bills/"+a1.ID.String(), nil).status)
	require.Equal(t, http.StatusNotFound, ba.do(http.MethodGet, "/bills/"+a2.ID.String(), nil).status)
	require.Equal(t, http.StatusNotFound, ba.do(http.MethodGet, "/bills/"+a2.ID.String()+"/pdf", nil).status)
	var page dto.Page[dto.Bill]
	ba.do(http.MethodGet, "/bills", nil).json(t, &page)
	require.Len(t, page.Items, 1)
	require.Equal(t, "12345.67", page.Items[0].TotalCost.String())
	var latest dto.Bill
	res := ba.do(http.MethodGet, "/bills/latest?scope=building&subject_id="+h.fx.BuildingA1.String(), nil)
	require.Equal(t, http.StatusOK, res.status, string(res.body))
	res.json(t, &latest)
	require.Equal(t, a1.ID, latest.ID)

	res = ba.do(http.MethodPost, "/bills/compute", map[string]any{"scope": "company", "period": "2026-08"})
	require.Equal(t, http.StatusForbidden, res.status, "a company bill needs a whole-company scope")
	require.Equal(t, http.StatusNotFound, ba.do(http.MethodPost, "/bills/compute", map[string]any{"scope": "building", "building_ids": []string{h.fx.BuildingA2.String()}, "period": "2026-08"}).status)
	res = ba.do(http.MethodPost, "/bills/compute", map[string]any{"scope": "building", "building_ids": []string{h.fx.BuildingA1.String()}, "period": "2026-08", "force": true})
	require.Equal(t, http.StatusAccepted, res.status, string(res.body))
	var accepted dto.BillComputeAccepted
	res.json(t, &accepted)
	require.Equal(t, []string{"billing.generate:building:" + h.fx.BuildingA1.String() + ":2026-08"}, accepted.JobIDs)
	p := h.enq.snapshot()
	require.Len(t, p, 1)

	ca := h.as(seed.E2ECompanyAdminEmail)
	res = ca.do(http.MethodPost, "/bills/compute", map[string]any{"scope": "company", "period": "2026-8"})
	require.Equal(t, http.StatusUnprocessableEntity, res.status)
	res = ca.do(http.MethodPost, "/bills/compute", map[string]any{"scope": "company", "period": "2026-08"})
	require.Equal(t, http.StatusAccepted, res.status, string(res.body))
	require.Equal(t, http.StatusForbidden, h.as(seed.E2EBuildingReadonlyEmail).do(http.MethodPost, "/bills/compute", map[string]any{"scope": "company", "period": "2026-08"}).status)
}

// analyzerBill writes one analyzer-scope invoice for the period, which is what
// the dashboard's rows are (R233).
func (h *harness) analyzerBill(building, analyzer uuid.UUID, period, kwh, cost string) model.Bill {
	h.t.Helper()
	price := decimal.RequireFromString("3.12")
	b, err := postgres.NewBillRepository(h.pool).Create(h.t.Context(), store.SystemScope(h.fx.CompanyA), model.Bill{
		CompanyID: h.fx.CompanyA, BuildingID: &building, AnalyzerID: &analyzer, Scope: model.BillScopeAnalyzer,
		PeriodKey: period, PeriodStart: time.Date(2026, 8, 1, 0, 0, 0, 0, ist), PeriodEnd: time.Date(2026, 9, 1, 0, 0, 0, 0, ist),
		DaysInPeriod: 31, ActiveImport: decimal.RequireFromString(kwh), ActiveExport: decimal.Zero,
		EffectiveEnergyPrice: &price, TotalCost: decimal.RequireFromString(cost), Currency: model.CurrencyTRY,
		Status: model.BillStatusIssued, GenerationUsage: model.GenerationUsageNone, ComputedAt: h.clock.Now(),
		IndexStart: json.RawMessage(`{}`), IndexEnd: json.RawMessage(`{}`),
	}, nil, nil)
	require.NoError(h.t, err)
	return b
}

func TestBillDashboardTotalsAreTheSumOfItsRows(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.analyzerBill(h.fx.BuildingA1, h.fx.AnalyzerA1, "2026-08", "120.5", "310.25")
	h.analyzerBill(h.fx.BuildingA2, h.fx.AnalyzerA2, "2026-08", "80.5", "199.75")
	h.analyzerBill(h.fx.BuildingA1, h.fx.AnalyzerA1, "2026-07", "500", "1000")

	var body dto.BillDashboard
	res := h.as(seed.E2ECompanyAdminEmail).do(http.MethodGet, "/bills/dashboard?year=2026&month=8", nil)
	require.Equal(t, http.StatusOK, res.status, string(res.body))
	res.json(t, &body)

	require.Equal(t, "2026-08", body.Period)
	require.Len(t, body.Buildings, 2, "July's bill belongs to another month")
	// 09 §F8's acceptance criterion, over the wire.
	total := decimal.Zero
	for _, b := range body.Buildings {
		rows := decimal.Zero
		for _, row := range b.Rows {
			rows = rows.Add(row.Invoice.Decimal)
		}
		require.Equal(t, rows.String(), b.TotalInvoice.String(), "building total is the sum of its rows")
		total = total.Add(rows)
	}
	require.Len(t, body.Netting, 1)
	require.Equal(t, total.String(), body.Netting[0].TotalInvoice.String())
	require.Equal(t, "net_consumption", body.Netting[0].NetStatus)
	// R235: the section exists in §7.10 and has no data source before F9.
	require.False(t, body.Plants.Available)
	require.Equal(t, "no_plant_production_source", body.Plants.Reason)
}

func TestBillDashboardIsScopedLikeEveryOtherRead(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.analyzerBill(h.fx.BuildingA1, h.fx.AnalyzerA1, "2026-08", "120", "300")
	h.analyzerBill(h.fx.BuildingA2, h.fx.AnalyzerA2, "2026-08", "80", "200")

	var body dto.BillDashboard
	h.as(seed.E2EBuildingAdminEmail).do(http.MethodGet, "/bills/dashboard?year=2026&month=8", nil).json(t, &body)
	require.Len(t, body.Buildings, 1, "a building admin sees only their own building")
	require.Equal(t, h.fx.BuildingA1, body.Buildings[0].BuildingID)

	var foreign dto.BillDashboard
	h.as(seed.E2ECompanyBAdminEmail).do(http.MethodGet, "/bills/dashboard?year=2026&month=8", nil).json(t, &foreign)
	require.Empty(t, foreign.Buildings, "another company's invoices are not visible")
}

func TestBillDashboardShowsABuildingInvoiceThatDivergesFromItsRows(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.analyzerBill(h.fx.BuildingA1, h.fx.AnalyzerA1, "2026-08", "100", "250")
	// 02 §6.11 prices the building invoice over the aggregate, so it may
	// legitimately differ. h.bill writes 12345.67 for the same period.
	h.bill(h.fx.BuildingA1)

	var body dto.BillDashboard
	h.as(seed.E2ECompanyAdminEmail).do(http.MethodGet, "/bills/dashboard?year=2026&month=8", nil).json(t, &body)
	require.Len(t, body.Buildings, 1)
	require.NotNil(t, body.Buildings[0].BuildingBill)
	require.Equal(t, "12345.67", body.Buildings[0].BuildingBill.Invoice.String())
	require.True(t, body.Buildings[0].DivergesFromRows)
	require.Equal(t, "250", body.Buildings[0].TotalInvoice.String(), "the rows' total is still the rows' total")
}

func TestBillDashboardExportRendersTheSameNumbers(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.analyzerBill(h.fx.BuildingA1, h.fx.AnalyzerA1, "2026-08", "120.5", "310.25")
	ca := h.as(seed.E2ECompanyAdminEmail)

	res := ca.do(http.MethodGet, "/bills/dashboard/export?year=2026&month=8", nil)
	require.Equal(t, http.StatusOK, res.status, string(res.body))
	require.Equal(t, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", res.header.Get("Content-Type"),
		"xlsx is the default format")
	require.Contains(t, res.header.Get("Content-Disposition"), "fatura-panosu-2026-08.xlsx")

	pdf := ca.do(http.MethodGet, "/bills/dashboard/export?year=2026&month=8&format=pdf", nil)
	require.Equal(t, http.StatusOK, pdf.status, string(pdf.body))
	require.True(t, bytes.HasPrefix(pdf.body, []byte("%PDF")))

	require.Equal(t, http.StatusUnprocessableEntity,
		ca.do(http.MethodGet, "/bills/dashboard/export?year=2026&month=8&format=csv", nil).status)
}

func TestSupersededBillPDFStaysDownloadable(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	old := h.bill(h.fx.BuildingA1)
	repo := postgres.NewBillRepository(h.pool)
	sc := store.SystemScope(h.fx.CompanyA)
	replacement := model.Bill{
		CompanyID: h.fx.CompanyA, BuildingID: &h.fx.BuildingA1, Scope: model.BillScopeBuilding, PeriodKey: "2026-08",
		PeriodStart: old.PeriodStart, PeriodEnd: old.PeriodEnd, DaysInPeriod: old.DaysInPeriod,
		TotalCost: decimal.RequireFromString("13000"), Currency: model.CurrencyTRY, Status: model.BillStatusIssued,
		GenerationUsage: model.GenerationUsageNone, ComputedAt: h.clock.Now(),
		IndexStart: json.RawMessage(`{}`), IndexEnd: json.RawMessage(`{}`),
	}
	_, err := repo.Supersede(t.Context(), sc, old.ID, replacement, nil, nil, h.clock.Now())
	require.NoError(t, err)

	ca := h.as(seed.E2ECompanyAdminEmail)
	// R252: a recomputation supersedes rather than deletes, and the invoice a
	// customer already holds must still render.
	res := ca.do(http.MethodGet, "/bills/"+old.ID.String()+"/pdf", nil)
	require.Equal(t, http.StatusOK, res.status, string(res.body))
	require.True(t, bytes.HasPrefix(res.body, []byte("%PDF")))

	var body dto.BillDashboard
	ca.do(http.MethodGet, "/bills/dashboard?year=2026&month=8", nil).json(t, &body)
	for _, b := range body.Buildings {
		if b.BuildingBill != nil {
			require.Equal(t, "13000", b.BuildingBill.Invoice.String(), "the dashboard shows the live bill, not the superseded one")
		}
	}
}
