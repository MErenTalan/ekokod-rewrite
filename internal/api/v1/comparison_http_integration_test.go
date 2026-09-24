//go:build integration

package v1_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/dto"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/seed"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/admin"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
	"github.com/shopspring/decimal"
)

func (h *harness) seedSectorReadings(rates map[string]string) {
	h.t.Helper()
	ist, err := time.LoadLocation("Europe/Istanbul")
	require.NoError(h.t, err)
	from := time.Date(2026, 8, 1, 0, 0, 0, 0, ist)
	to := time.Date(2026, 9, 17, 0, 0, 0, 0, ist)
	for analyzer, rate := range rates {
		company := h.fx.CompanyA
		if analyzer == h.fx.AnalyzerB1.String() {
			company = h.fx.CompanyB
		}
		id := testfixtures.MustUUID(analyzer)
		testfixtures.InsertReadings(h.t, h.t.Context(), h.pool, company, testfixtures.HourlyReadings(id, from, to, testfixtures.Constant(rate)))
	}
}

func TestComparisonResponseLeaksNoPeerIdentity(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.clock.Set(time.Date(2026, 9, 17, 10, 0, 0, 0, time.UTC))
	_, err := admin.NewCatalogueRepository(h.pool).UpsertPlatformFactor(t.Context(), model.EmissionFactor{
		Key: "grid_electricity_tr_2022", Label: "Şebeke", MainCategory: "cat_electricity",
		BaseFactor: decimal.RequireFromString("0.469"), BaseUnit: "kWh",
	})
	require.NoError(t, err)

	ca := h.as(seed.E2ECompanyAdminEmail)
	res := ca.do(http.MethodGet, "/buildings/"+h.fx.BuildingA1.String()+"/comparison", nil)
	require.Equal(t, http.StatusOK, res.status, string(res.body))
	var small dto.BuildingComparison
	res.json(t, &small)
	require.False(t, small.Available)
	require.Equal(t, "sector_too_small", *small.Reason)

	h.seedSectorReadings(map[string]string{
		h.fx.AnalyzerA1.String(): "10", h.fx.AnalyzerA2.String(): "20", h.fx.AnalyzerB1.String(): "30",
	})
	res = ca.do(http.MethodGet, "/buildings/"+h.fx.BuildingA1.String()+"/comparison", nil)
	require.Equal(t, http.StatusOK, res.status, string(res.body))
	var cmp dto.BuildingComparison
	res.json(t, &cmp)
	require.True(t, cmp.Available)
	require.Equal(t, 3, cmp.Peers)
	require.Equal(t, "7430", cmp.MonthlyConsumption.Value.String())
	require.Equal(t, "14860", cmp.MonthlyConsumption.Average.String())
	require.Equal(t, 1, cmp.MonthlyConsumption.Rank)
	require.Equal(t, 3, cmp.MonthlyConsumption.Ranked)
	require.Equal(t, "239.666667", cmp.DailyConsumption.Value.String())
	require.Equal(t, "3484.67", cmp.CO2EmissionKg.Value.String())
	require.Equal(t, "185.75", cmp.ConsumptionPerCapita.Value.String(), "7430 / 40 personnel")
	require.Equal(t, "1.486", cmp.ConsumptionPerArea.Value.String())

	for _, secret := range []string{h.fx.BuildingB1.String(), h.fx.CompanyB.String(), h.fx.AnalyzerB1.String(), "E2E Şirket B", "B1 Ofis",
		h.fx.BuildingA2.String(), "A2 Depo"} {
		require.NotContains(t, string(res.body), secret, "the comparison must not identify a peer")
	}
	var raw map[string]any
	require.NoError(t, json.Unmarshal(res.body, &raw))
	require.NotContains(t, raw, "peers_detail")

	ba := h.as(seed.E2EBuildingAdminEmail)
	require.Equal(t, http.StatusOK, ba.do(http.MethodGet, "/buildings/"+h.fx.BuildingA1.String()+"/comparison", nil).status)
	require.Equal(t, http.StatusNotFound, ba.do(http.MethodGet, "/buildings/"+h.fx.BuildingA2.String()+"/comparison", nil).status)
	require.Equal(t, http.StatusNotFound, ca.do(http.MethodGet, "/buildings/"+h.fx.BuildingB1.String()+"/comparison", nil).status)

	require.Equal(t, http.StatusOK, ca.do(http.MethodPatch, "/buildings/"+h.fx.BuildingA2.String(), map[string]any{"sector": ""}).status)
	res = ca.do(http.MethodGet, "/buildings/"+h.fx.BuildingA2.String()+"/comparison", nil)
	require.Equal(t, http.StatusConflict, res.status)
	require.Equal(t, "building_sector_missing", res.code(t))
}
