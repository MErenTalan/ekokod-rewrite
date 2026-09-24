//go:build integration

package v1_test

import (
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/dto"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/seed"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
)

func (h *harness) carbonActivityFor(company, building uuid.UUID) string {
	h.t.Helper()
	day := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	a, err := postgres.NewCarbonRepository(h.pool).CreateActivity(h.t.Context(), store.SystemScope(company), model.CarbonActivity{
		CompanyID: company, BuildingID: building, MainCategory: "cat_waste", SubCategory: "sub_waste_disposal",
		ActivityType: "sub_waste_disposal", PeriodStart: day, PeriodEnd: day, Quantity: decimalOf("1"), Unit: "tonne",
		ConversionMultiplier: decimalOf("1"), EmissionKgco2e: decimalOf("446.2"), Scope: model.CarbonScope3,
		IsoCategory: "category_6", Status: model.CarbonStatusPending})
	require.NoError(h.t, err)
	return a.ID.String()
}

func (h *harness) carbonReportFor(company, building uuid.UUID) string {
	h.t.Helper()
	r, err := postgres.NewCarbonRepository(h.pool).CreateReport(h.t.Context(), store.SystemScope(company), model.CarbonReport{
		CompanyID: company, BuildingID: building, Name: "Sweep", ReportType: "ghg", Period: "2026-08-01/2026-08-31",
		Payload: []byte(`{"report_type":"ghg","from":"2026-08-01","to":"2026-08-31","groups":[]}`)})
	require.NoError(h.t, err)
	return r.ID.String()
}

func (h *harness) carbonFactorFor(company uuid.UUID) string {
	h.t.Helper()
	f, err := postgres.NewCarbonRepository(h.pool).UpsertFactor(h.t.Context(), store.SystemScope(company), model.EmissionFactor{
		CompanyID: &company, Key: "sweep_" + company.String(), Label: "Sweep", MainCategory: "cat_waste",
		SubCategories: []string{"sub_waste_disposal"}, BaseFactor: decimalOf("1"), BaseUnit: "tonne"})
	require.NoError(h.t, err)
	return f.ID.String()
}

// The company admin's whole flow over the real stack (R305–R313).
func TestCarbonFlowForCompanyAdmin(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	// The master catalogue is seed.Load's, not E2EFixtures'.
	_, _, err := seed.LoadEmissionFactors(t.Context(), h.pool, slog.New(slog.DiscardHandler))
	require.NoError(t, err)
	ca := h.as(seed.E2ECompanyAdminEmail)
	b := h.fx.BuildingA1.String()

	res := ca.do(http.MethodPut, "/carbon/selected-activities?building_id="+b, map[string]any{"activity_keys": []string{"sub_waste_disposal"}})
	require.Equal(t, http.StatusOK, res.status, string(res.body))

	var factors dto.EmissionFactorList
	ca.do(http.MethodGet, "/carbon/emission-factors?sub_category=sub_waste_disposal", nil).json(t, &factors)
	require.NotEmpty(t, factors.Items)
	f := factors.Items[0]

	activity := map[string]any{"building_id": b, "sub_category": "sub_waste_disposal",
		"factor_key": f.Key, "quantity": "2", "unit": f.BaseUnit, "period_start": "2026-08-01", "period_end": "2026-08-31",
		"emission_kgco2e": "1"}
	res = ca.do(http.MethodPost, "/carbon/activities", activity)
	require.Equal(t, http.StatusBadRequest, res.status, "the client can never state the emission: %s", res.body)
	delete(activity, "emission_kgco2e")
	res = ca.do(http.MethodPost, "/carbon/activities", activity)
	require.Equal(t, http.StatusCreated, res.status, string(res.body))
	var a dto.CarbonActivity
	res.json(t, &a)
	require.True(t, a.EmissionKgCO2e.Equal(f.BaseFactor.Mul(decimalOf("2")).Round(6)))
	require.Equal(t, "pending", a.Status)
	require.Equal(t, "category_6", a.IsoCategory)

	res = ca.do(http.MethodPost, "/carbon/activities/"+a.ID.String()+"/status", map[string]any{"status": "approved"})
	require.Equal(t, http.StatusOK, res.status, string(res.body))

	var o dto.CarbonOverview
	ca.do(http.MethodGet, "/carbon/overview?building_id="+b+"&year=2026", nil).json(t, &o)
	require.True(t, o.TotalKgCO2e.Equal(a.EmissionKgCO2e.Decimal), "overview reconciles with the record")

	res = ca.do(http.MethodPatch, "/carbon/emission-factors/"+f.ID.String(), map[string]any{"base_factor": "9999", "source_year": 2026})
	require.Equal(t, http.StatusOK, res.status, string(res.body))
	var again dto.Page[dto.CarbonActivity]
	ca.do(http.MethodGet, "/carbon/activities?building_id="+b, nil).json(t, &again)
	require.True(t, again.Items[0].EmissionKgCO2e.Equal(a.EmissionKgCO2e.Decimal), "acceptance: a factor change never alters a stored record")
	require.Equal(t, http.StatusNoContent, ca.do(http.MethodPost, "/carbon/emission-factors/reset", nil).status)

	res = ca.do(http.MethodPost, "/carbon/reports", map[string]any{"building_id": b, "report_type": "iso", "from": "2026-08-01", "to": "2026-08-31"})
	require.Equal(t, http.StatusCreated, res.status, string(res.body))
	var r dto.CarbonReportSummary
	res.json(t, &r)
	pdf := ca.do(http.MethodGet, "/carbon/reports/"+r.ID.String()+"/pdf", nil)
	require.Equal(t, http.StatusOK, pdf.status)
	require.Equal(t, "%PDF-", string(pdf.body[:5]))
}

func TestCarbonWritesAreCompanyAdminOnly(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ba := h.as(seed.E2EBuildingAdminEmail)
	own := h.fx.BuildingA1.String()
	res := ba.do(http.MethodPut, "/carbon/selected-activities?building_id="+own, map[string]any{"activity_keys": []string{}})
	require.Equal(t, http.StatusForbidden, res.status, string(res.body))
	require.Equal(t, http.StatusOK, ba.do(http.MethodGet, "/carbon/overview?building_id="+own, nil).status)
	require.Equal(t, http.StatusNotFound, ba.do(http.MethodGet, "/carbon/overview?building_id="+h.fx.BuildingA2.String(), nil).status)

	ca := h.as(seed.E2ECompanyAdminEmail)
	foreign := h.carbonActivityFor(h.fx.CompanyB, h.fx.BuildingB1)
	res = ca.do(http.MethodPost, "/carbon/activities/"+foreign+"/status", map[string]any{"status": "approved"})
	require.Equal(t, http.StatusNotFound, res.status, string(res.body))
	report := h.carbonReportFor(h.fx.CompanyB, h.fx.BuildingB1)
	require.Equal(t, http.StatusNotFound, ca.do(http.MethodGet, "/carbon/reports/"+report+"/pdf", nil).status)
}
