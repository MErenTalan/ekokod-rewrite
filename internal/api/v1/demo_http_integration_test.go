//go:build integration

package v1_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/seed"
)

func (h *harness) seedDemo() {
	h.t.Helper()
	_, err := seed.SeedDemo(h.t.Context(), h.pool, h.hasher, testPassword, h.clock.Now())
	require.NoError(h.t, err)
}

// TestDemoSeesOnlySyntheticData is 09 §F6's demo acceptance (R138).
func TestDemoSeesOnlySyntheticData(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.seedDemo()
	demo := h.as(seed.DemoEmail)
	building, analyzer := seed.DemoBuildingID.String(), seed.DemoAnalyzerIDs[0].String()
	paths := []string{
		"/auth/me", "/profile", "/buildings?include=analyzer_count,active_status", "/buildings/" + building,
		"/buildings/" + building + "/comparison", "/analyzers", "/analyzers/" + analyzer,
		"/consumption?building_id=" + building + "&granularity=daily&from=2026-09-01&to=2026-09-10",
		"/consumption/summary?analyzer_id=" + analyzer + "&granularity=daily&from=2026-09-01&to=2026-09-10",
		"/consumption/reactive-status", "/consumption/anomalies", "/generation?analyzer_id=" + analyzer + "&granularity=monthly&from=2026-06-01&to=2026-08-31",
		"/energy-balance?analyzer_id=" + analyzer + "&granularity=monthly&from=2026-06-01&to=2026-08-31",
		"/load-profile?analyzer_id=" + analyzer + "&from=2026-08-01&to=2026-08-31",
		"/load-profile/statistics?analyzer_id=" + analyzer + "&from=2026-08-01&to=2026-08-31",
		"/tariffs", "/tariffs/applicable?building_id=" + building + "&date=2026-09-01", "/bills", "/calendar/events?from=2026-09-01&to=2026-09-30", "/calendar/vacations",
	}
	forbidden := []string{h.fx.CompanyA.String(), h.fx.CompanyB.String(), h.fx.PlatformCompany.String(), h.fx.BuildingA1.String(),
		h.fx.BuildingA2.String(), h.fx.BuildingB1.String(), h.fx.AnalyzerA1.String(), h.fx.AnalyzerA2.String(), h.fx.AnalyzerB1.String(),
		"E2E Şirket", "Ekokod Platform", "A1 Fabrika"}
	for id := range h.fx.Users {
		forbidden = append(forbidden, id, h.fx.Users[id].String())
	}
	for _, path := range paths {
		res := demo.do(http.MethodGet, path, nil)
		require.Equal(t, http.StatusOK, res.status, "%s: %s", path, string(res.body))
		for _, f := range forbidden {
			require.NotContains(t, string(res.body), f, "%s leaked real data", path)
		}
	}
	require.Contains(t, string(demo.do(http.MethodGet, "/buildings", nil).body), "Demo Fabrika")
	require.Equal(t, http.StatusNotFound, demo.do(http.MethodGet, "/buildings?company_id="+h.fx.CompanyA.String(), nil).status)
	require.Equal(t, http.StatusNotFound, demo.do(http.MethodGet, "/buildings/"+h.fx.BuildingA1.String(), nil).status)
	require.Equal(t, http.StatusForbidden, demo.do(http.MethodGet, "/users", nil).status)
}

func TestDemoMutationsForbidden(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.seedDemo()
	demo := h.as(seed.DemoEmail)
	for _, tc := range []struct {
		method, path string
		body         any
	}{
		{http.MethodPost, "/buildings", map[string]any{"name": "x"}},
		{http.MethodPatch, "/profile", map[string]any{"name": "x"}},
		{http.MethodPost, "/analyzers/" + seed.DemoAnalyzerIDs[0].String() + "/refresh", map[string]any{"mode": "hourly"}},
		{http.MethodPut, "/calendar/vacations", map[string]any{"weekend_days": []int{0}}},
		{http.MethodPost, "/bills/compute", map[string]any{"scope": "company", "period": "2026-08"}},
		{http.MethodPost, "/auth/change-password", map[string]any{"current_password": testPassword, "new_password": "Baska!Sifre-99x"}},
	} {
		require.Equal(t, http.StatusForbidden, demo.do(tc.method, tc.path, tc.body).status, tc.path)
	}
}
