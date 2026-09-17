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
