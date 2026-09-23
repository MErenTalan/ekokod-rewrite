//go:build integration

package v1_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/dto"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/isolar"
	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/seed"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
)

// plantFor creates a linked plant of company with one day of totals.
func (h *harness) plantFor(company uuid.UUID, name, psID string) uuid.UUID {
	h.t.Helper()
	ctx := context.Background()
	sc := store.SystemScope(company)
	p, err := postgres.NewPlantRepository(h.pool).Create(ctx, sc, model.PowerPlant{CompanyID: company, Name: name, PlantKind: "grid", CreatedAt: h.clock.Now()})
	require.NoError(h.t, err)
	p.IsolarPsID = &psID
	_, err = postgres.NewPlantRepository(h.pool).SetIsolarLink(ctx, sc, p)
	require.NoError(h.t, err)
	day := time.Date(2026, 9, 16, 0, 0, 0, 0, ist)
	v := decimalOf("12.5")
	_, err = postgres.NewProductionTotalsRepository(h.pool).ReplaceDays(ctx, sc, p.ID, []time.Time{day},
		[]model.PlantProductionTotal{{PlantID: p.ID, Ts: day.Add(12 * time.Hour), ProductionKwh: &v, Basis: model.BasisPlantMeter}})
	require.NoError(h.t, err)
	return p.ID
}

func TestSolarRoutesHideOtherCompanysPlant(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	own := h.plantFor(h.fx.CompanyA, "Own GES", "PS-OWN").String()
	foreign := h.plantFor(h.fx.CompanyB, "Foreign GES", "PS-FOREIGN").String()
	ca := h.as(seed.E2ECompanyAdminEmail)
	for _, suffix := range []string{"/realtime", "/production?granularity=day&from=2026-09-10&to=2026-09-17", "/devices", "/alarms", "/revenue",
		"/production/export?granularity=day&from=2026-09-10&to=2026-09-17"} {
		require.Equal(t, http.StatusOK, ca.do(http.MethodGet, "/plants/"+own+suffix, nil).status, suffix)
		res := ca.do(http.MethodGet, "/plants/"+foreign+suffix, nil)
		require.Equal(t, http.StatusNotFound, res.status, "%s: %s", suffix, string(res.body))
	}
	for _, c := range []struct{ method, path string }{
		{http.MethodPost, "/plants/" + foreign + "/sync"}, {http.MethodDelete, "/plants/" + foreign + "/isolar-link"},
	} {
		require.Equal(t, http.StatusNotFound, ca.do(c.method, c.path, map[string]any{}).status, c.path)
	}
	res := ca.do(http.MethodPut, "/plants/"+foreign+"/alarm-recipients", map[string]any{"emails": []string{"a@b.test"}})
	require.Equal(t, http.StatusNotFound, res.status, string(res.body))
	require.Empty(t, h.enq.snapshot())
}

func TestBuildingAdminGets403OnPlantRoutes(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	own := h.plantFor(h.fx.CompanyA, "Own GES", "PS-OWN").String()
	ba := h.as(seed.E2EBuildingAdminEmail)
	for _, path := range []string{"/plants/" + own + "/realtime", "/plants/" + own + "/revenue"} {
		require.Equal(t, http.StatusForbidden, ba.do(http.MethodGet, path, nil).status, path)
	}
}

func TestSyncAnswers202WithWatchableJob(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	own := h.plantFor(h.fx.CompanyA, "Own GES", "PS-OWN")
	ca := h.as(seed.E2ECompanyAdminEmail)
	res := ca.do(http.MethodPost, "/plants/"+own.String()+"/sync", map[string]any{})
	require.Equal(t, http.StatusAccepted, res.status, string(res.body))
	var accepted dto.JobAccepted
	res.json(t, &accepted)
	require.Equal(t, job.SolarSyncTaskID(job.SolarSyncPayload{PlantID: own}), accepted.JobID)
	tasks := h.enq.snapshot()
	require.Len(t, tasks, 1)
	require.Equal(t, job.TypeSolarSyncPlant, tasks[0].Type())
}

func TestProductionExportIsXlsx(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	own := h.plantFor(h.fx.CompanyA, "Own GES", "PS-OWN").String()
	res := h.as(seed.E2ECompanyAdminEmail).do(http.MethodGet, "/plants/"+own+"/production/export?granularity=day&from=2026-09-10&to=2026-09-17", nil)
	require.Equal(t, http.StatusOK, res.status)
	require.Equal(t, xlsxContentType, res.header.Get("Content-Type"))
	require.Equal(t, "PK", string(res.body[:2]), "a zip container")
}

func TestSolarQueriesValidate(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	own := h.plantFor(h.fx.CompanyA, "Own GES", "PS-OWN").String()
	ca := h.as(seed.E2ECompanyAdminEmail)
	require.Equal(t, http.StatusBadRequest, ca.do(http.MethodGet, "/plants/"+own+"/devices?colour=green", nil).status)
	res := ca.do(http.MethodGet, "/plants/"+own+"/production?granularity=hour&from=2026-08-01&to=2026-09-17", nil)
	require.Equal(t, http.StatusUnprocessableEntity, res.status, "R284: hour ≤ 31 days: %s", string(res.body))
	var series dto.PlantProductionSeries
	ca.do(http.MethodGet, "/plants/"+own+"/production?granularity=day&from=2026-09-10&to=2026-09-17", nil).json(t, &series)
	require.Len(t, series.Points, 1)
	require.Equal(t, "12.5", series.Points[0].ProductionKwh.String())
	require.Equal(t, "plant_meter", *series.Points[0].Basis)
}

func TestNettingAnalyzerMustBeSameCompany(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	own := h.plantFor(h.fx.CompanyA, "Own GES", "PS-OWN").String()
	ca := h.as(seed.E2ECompanyAdminEmail)
	res := ca.do(http.MethodPatch, "/power-plants/"+own, map[string]any{"netting_analyzer_id": h.fx.AnalyzerB1.String()})
	require.Equal(t, http.StatusUnprocessableEntity, res.status, string(res.body))
	require.Contains(t, string(res.body), "netting_analyzer_id")

	res = ca.do(http.MethodPatch, "/power-plants/"+own, map[string]any{"netting_analyzer_id": h.fx.AnalyzerA1.String()})
	require.Equal(t, http.StatusOK, res.status, string(res.body))
	var p dto.PlantDetail
	res.json(t, &p)
	require.Equal(t, h.fx.AnalyzerA1, *p.NettingAnalyzerID)

	res = ca.do(http.MethodPatch, "/power-plants/"+own, map[string]any{"clear_netting_analyzer": true})
	require.Equal(t, http.StatusOK, res.status, string(res.body))
	res.json(t, &p)
	require.Nil(t, p.NettingAnalyzerID)
}

const xlsxContentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"

func decimalOf(s string) decimal.Decimal { return decimal.RequireFromString(s) }

// fakeSolar is the API's iSolar adapter in tests: an empty account.
type fakeSolar struct{}

func (fakeSolar) Plants(context.Context, integration.Credentials) ([]isolar.Plant, error) {
	return nil, nil
}
func (fakeSolar) Devices(context.Context, integration.Credentials, string) ([]isolar.Device, error) {
	return nil, nil
}
func (fakeSolar) DeviceMinuteSeries(context.Context, integration.Credentials, []string, time.Time, time.Time) ([]isolar.YieldSample, error) {
	return nil, nil
}
func (fakeSolar) PlantMinuteSeries(context.Context, integration.Credentials, string, time.Time, time.Time) ([]isolar.YieldSample, error) {
	return nil, nil
}
func (fakeSolar) PlantDailySeries(context.Context, integration.Credentials, string, time.Time, time.Time) ([]isolar.DailyYield, error) {
	return nil, nil
}
func (fakeSolar) DeviceRealtime(context.Context, integration.Credentials, int32, []string) ([]isolar.DeviceSnapshot, error) {
	return nil, nil
}
func (fakeSolar) Faults(context.Context, integration.Credentials, time.Time, time.Time) ([]isolar.Fault, error) {
	return nil, nil
}

// R290 through the real wiring: a plant with a netting analyzer adds the
// analyzer's columns to the bills dashboard (the e2e run found the API's
// solar service built without the analyzer and bill readers).
func TestBillsDashboardNamesTheNettingAnalyzer(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ctx := context.Background()
	sc := store.SystemScope(h.fx.CompanyA)
	id := h.plantFor(h.fx.CompanyA, "Netting GES", "PS-NET")
	plants := postgres.NewPlantRepository(h.pool)
	p, err := plants.Get(ctx, sc, id)
	require.NoError(t, err)
	p.NettingAnalyzerID = &h.fx.AnalyzerA1
	_, err = plants.Update(ctx, sc, p)
	require.NoError(t, err)

	res := h.as(seed.E2ECompanyAdminEmail).do(http.MethodGet, "/bills/dashboard?year=2026&month=9", nil)
	require.Equal(t, http.StatusOK, res.status, string(res.body))
	require.Contains(t, string(res.body), "Netting GES")
	require.Contains(t, string(res.body), `"installation_number":"E2E-A1"`)
}
