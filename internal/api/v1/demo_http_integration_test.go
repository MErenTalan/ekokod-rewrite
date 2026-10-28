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
	_, err := seed.Demo(h.t.Context(), h.pool, h.hasher, testPassword, h.clock.Now())
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
		"/users",
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
}

// TestDemoMutatesLikeCompanyAdminHTTP is R541–R543: a company admin's writes
// work, the shared login's own account cannot be changed or locked.
func TestDemoMutatesLikeCompanyAdminHTTP(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.seedDemo()
	demo := h.as(seed.DemoEmail)
	analyzer := seed.DemoAnalyzerIDs[0].String()
	for _, tc := range []struct {
		method, path string
		body         any
		want         int
	}{
		{http.MethodPost, "/tariffs", validTariff(seed.DemoBuildingID), http.StatusCreated},
		{http.MethodPost, "/alarms", map[string]any{
			"name": "İletişim", "type": "data_communication", "is_enabled": true, "analyzer_ids": []string{analyzer},
			"channels": []map[string]string{{"channel": "email", "target": "ops@example.com"}},
			"settings": map[string]any{"communication_threshold_hours": 6},
		}, http.StatusCreated},
		{http.MethodPatch, "/companies/" + seed.DemoCompanyID.String(), map[string]any{"sector": "Üretim"}, http.StatusOK},
		{http.MethodPost, "/users", map[string]any{"name": "Selin Koç", "email": "selin@demo-enerji.example",
			"role": "building_admin", "password": "Guclu!Parola-91"}, http.StatusCreated},
		{http.MethodPatch, "/profile", map[string]any{"name": "x"}, http.StatusForbidden},
		{http.MethodPost, "/auth/change-password", map[string]any{"current_password": testPassword, "new_password": "Baska!Sifre-99x"}, http.StatusForbidden},
		{http.MethodGet, "/auth/sessions", nil, http.StatusForbidden},
		{http.MethodPatch, "/users/" + seed.DemoUserID.String(), map[string]any{"is_active": false}, http.StatusForbidden},
		{http.MethodDelete, "/users/" + seed.DemoUserID.String(), nil, http.StatusConflict}, // cannot_modify_self comes first
	} {
		res := demo.do(tc.method, tc.path, tc.body)
		require.Equal(t, tc.want, res.status, "%s %s: %s", tc.method, tc.path, string(res.body))
	}
	var me struct {
		SessionID string `json:"session_id"`
	}
	demo.do(http.MethodGet, "/auth/me", nil).json(t, &me)
	require.Equal(t, http.StatusForbidden, demo.do(http.MethodDelete, "/auth/sessions/"+me.SessionID, nil).status)
	require.Equal(t, http.StatusForbidden, demo.do(http.MethodPost, "/auth/logout-all", nil).status)
	require.Equal(t, http.StatusOK, demo.do(http.MethodGet, "/auth/me", nil).status, "the shared session survives")
}

// TestLoginRefusesVisitorUsersOfDemoCompany is R543.
func TestLoginRefusesVisitorUsersOfDemoCompany(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.seedDemo()
	demo := h.as(seed.DemoEmail)
	res := demo.do(http.MethodPost, "/users", map[string]any{"name": "Selin Koç", "email": "selin@demo-enerji.example",
		"role": "company_admin", "password": "Guclu!Parola-91"})
	require.Equal(t, http.StatusCreated, res.status, string(res.body))

	visitor := h.client(uaChrome)
	got := visitor.do(http.MethodPost, "/auth/login", map[string]any{"email": "selin@demo-enerji.example", "password": "Guclu!Parola-91"})
	require.Equal(t, http.StatusUnauthorized, got.status, string(got.body))
	require.Contains(t, string(got.body), "invalid_credentials")
	h.as(seed.E2ECompanyAdminEmail) // a normal company's admin still logs in
}
