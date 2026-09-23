//go:build integration

package v1_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/dto"
	"github.com/MErenTalan/ekokod-rewrite/internal/seed"
)

var renewablePanels = []string{"overview", "realtime", "grid-interaction", "environmental", "efficiency", "forecast", "analytics", "system-status"}

// R292/R298: every panel answers a building admin for their own analyzer,
// hides everyone else's, and refuses a malformed query.
func TestRenewablePanelsAreScoped(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ba := h.as(seed.E2EBuildingAdminEmail)
	rng := "&from=2026-09-01&to=2026-09-07"
	for _, p := range renewablePanels {
		res := ba.do(http.MethodGet, "/renewable/"+p+"?analyzer_id="+h.fx.AnalyzerA1.String()+rng, nil)
		require.Equal(t, http.StatusOK, res.status, "%s: %s", p, string(res.body))
		require.Contains(t, string(res.body), `"unavailable"`, p)
		for _, foreign := range []string{"analyzer_id=" + h.fx.AnalyzerA2.String(), "building_id=" + h.fx.BuildingA2.String(),
			"analyzer_id=" + h.fx.AnalyzerB1.String(), "building_id=" + h.fx.BuildingB1.String()} {
			res := ba.do(http.MethodGet, "/renewable/"+p+"?"+foreign+rng, nil)
			require.Equal(t, http.StatusNotFound, res.status, "%s %s", p, foreign)
		}
	}
	a1 := "analyzer_id=" + h.fx.AnalyzerA1.String()
	for query, want := range map[string]int{
		"from=2026-09-01&to=2026-09-07":                       http.StatusUnprocessableEntity, // no subject
		a1 + "&building_id=" + h.fx.BuildingA1.String() + rng: http.StatusUnprocessableEntity,
		a1 + "&from=2025-01-01&to=2026-01-02":                 http.StatusUnprocessableEntity, // 367 days
		a1 + "&from=2025-01-01&to=2026-01-01":                 http.StatusOK,                  // 366 days
		a1 + "&from=2026-09-07&to=2026-09-01":                 http.StatusUnprocessableEntity,
		a1 + rng + "&battery=1":                               http.StatusBadRequest,
	} {
		res := ba.do(http.MethodGet, "/renewable/overview?"+query, nil)
		require.Equal(t, want, res.status, "%s: %s", query, string(res.body))
	}
}

// R294: company-level; twelve months, null when there is no data.
func TestFinancialRoutes(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	for _, who := range []string{seed.E2ECompanyAdminEmail, seed.E2ECompanyReadonlyEmail} {
		c := h.as(who)
		var monthly dto.FinancialMonthly
		res := c.do(http.MethodGet, "/financial/monthly?year=2026", nil)
		require.Equal(t, http.StatusOK, res.status, string(res.body))
		res.json(t, &monthly)
		require.Len(t, monthly.Items, 12)
		require.Equal(t, 12, monthly.Coverage.Of)
		var summary dto.FinancialSummary
		res = c.do(http.MethodGet, "/financial/summary?year=2026&month=9", nil)
		require.Equal(t, http.StatusOK, res.status, string(res.body))
		res.json(t, &summary)
		require.Equal(t, 9, *summary.Month)
		require.Positive(t, summary.AnalyzerCount)
	}
	ca := h.as(seed.E2ECompanyAdminEmail)
	for _, q := range []string{"/financial/summary", "/financial/summary?year=2026&month=13", "/financial/monthly?year=1999"} {
		require.Equal(t, http.StatusUnprocessableEntity, ca.do(http.MethodGet, q, nil).status, q)
	}
	require.Equal(t, http.StatusForbidden, h.as(seed.E2EBuildingAdminEmail).do(http.MethodGet, "/financial/monthly?year=2026", nil).status)
}
