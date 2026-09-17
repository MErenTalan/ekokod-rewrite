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
	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/crypto"
	"github.com/MErenTalan/ekokod-rewrite/internal/seed"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/admin"
)

func TestBuildingAdminSeesOnlyResponsibleBuildings(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ba := h.as(seed.E2EBuildingAdminEmail)
	var page dto.Page[dto.Building]
	res := ba.do(http.MethodGet, "/buildings", nil)
	require.Equal(t, http.StatusOK, res.status, string(res.body))
	res.json(t, &page)
	require.Len(t, page.Items, 1)
	require.Equal(t, h.fx.BuildingA1, page.Items[0].ID)

	require.Equal(t, http.StatusNotFound, ba.do(http.MethodGet, "/buildings/"+h.fx.BuildingA2.String(), nil).status)
	require.Equal(t, http.StatusNotFound, ba.do(http.MethodGet, "/buildings?company_id="+h.fx.CompanyB.String(), nil).status)
	require.Equal(t, http.StatusNotFound, ba.do(http.MethodGet, "/analyzers/"+h.fx.AnalyzerA2.String(), nil).status)
	var analyzers dto.Page[dto.Analyzer]
	ba.do(http.MethodGet, "/analyzers", nil).json(t, &analyzers)
	require.Len(t, analyzers.Items, 1)
	require.Equal(t, h.fx.AnalyzerA1, analyzers.Items[0].ID)
	require.Equal(t, http.StatusForbidden, ba.do(http.MethodGet, "/power-plants", nil).status)
}

func TestBuildingListActiveStatus(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	now := h.clock.Now()
	_, err := h.pool.Exec(t.Context(), `update analyzers set last_reading_at = $1 where id = $2`, now.Add(-6*24*time.Hour), h.fx.AnalyzerA1)
	require.NoError(t, err)
	_, err = h.pool.Exec(t.Context(), `update analyzers set last_reading_at = $1 where id = $2`, now.Add(-8*24*time.Hour), h.fx.AnalyzerA2)
	require.NoError(t, err)

	ca := h.as(seed.E2ECompanyAdminEmail)
	var page dto.Page[dto.Building]
	res := ca.do(http.MethodGet, "/buildings?include=analyzer_count,active_status", nil)
	require.Equal(t, http.StatusOK, res.status, string(res.body))
	res.json(t, &page)
	byID := map[string]dto.Building{}
	for _, b := range page.Items {
		byID[b.ID.String()] = b
	}
	a1, a2 := byID[h.fx.BuildingA1.String()], byID[h.fx.BuildingA2.String()]
	require.Equal(t, "active", *a1.ActivityStatus)
	require.Equal(t, 1, *a1.AnalyzerCount)
	require.Equal(t, 1, *a1.ActiveAnalyzerCount)
	require.Equal(t, "passive", *a2.ActivityStatus)
	require.Equal(t, 0, *a2.ActiveAnalyzerCount)

	var plain map[string]any
	res = ca.do(http.MethodGet, "/buildings", nil)
	require.NoError(t, json.Unmarshal(res.body, &plain))
	require.NotContains(t, plain["items"].([]any)[0].(map[string]any), "activity_status", "includes are opt-in")
	require.Equal(t, http.StatusBadRequest, ca.do(http.MethodGet, "/buildings?include=everything", nil).status)

	var one dto.Analyzer
	ca.do(http.MethodGet, "/analyzers/"+h.fx.AnalyzerA2.String(), nil).json(t, &one)
	require.Equal(t, "passive", one.ActivityStatus)
}

func TestBuildingCreateUpdateDelete(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ca := h.as(seed.E2ECompanyAdminEmail)
	res := ca.do(http.MethodPost, "/buildings", map[string]any{
		"name": "Yeni Tesis", "latitude": "40.1", "longitude": "29.2", "bill_cutoff_day": 15,
		"responsible_user_id": h.fx.Users[seed.E2EBuildingAdminEmail].String(),
		"contacts":            []map[string]any{{"name": "Nöbetçi", "phone": "+90 312 000 00 00"}},
	})
	require.Equal(t, http.StatusCreated, res.status, string(res.body))
	var created dto.BuildingDetail
	res.json(t, &created)
	require.Len(t, created.Contacts, 1)
	require.EqualValues(t, 15, created.BillCutoffDay)

	res = ca.do(http.MethodPost, "/buildings", map[string]any{"name": "Yabancı", "responsible_user_id": h.fx.Users[seed.E2ECompanyBAdminEmail].String()})
	require.Equal(t, http.StatusUnprocessableEntity, res.status)
	require.Equal(t, http.StatusUnprocessableEntity, ca.do(http.MethodPost, "/buildings", map[string]any{"name": "Gün", "bill_cutoff_day": 32}).status)

	res = ca.do(http.MethodPatch, "/buildings/"+created.ID.String(), map[string]any{"contacts": []any{}, "sector": "Perakende"})
	require.Equal(t, http.StatusOK, res.status, string(res.body))
	var updated dto.BuildingDetail
	res.json(t, &updated)
	require.Empty(t, updated.Contacts)
	require.Equal(t, "Perakende", *updated.Sector)
	require.Equal(t, "Yeni Tesis", updated.Name)

	res = ca.do(http.MethodDelete, "/buildings/"+h.fx.BuildingA1.String(), nil)
	require.Equal(t, http.StatusConflict, res.status)
	require.Equal(t, "building_has_analyzers", res.code(t))
	require.Equal(t, http.StatusNoContent, ca.do(http.MethodDelete, "/buildings/"+created.ID.String(), nil).status)
	require.Equal(t, http.StatusNotFound, ca.do(http.MethodGet, "/buildings/"+created.ID.String(), nil).status)
	require.Equal(t, http.StatusNotFound, ca.do(http.MethodPatch, "/buildings/"+h.fx.BuildingB1.String(), map[string]any{"name": "x"}).status)
}

func TestAnalyzerPatchAssignableFieldsOnly(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ca := h.as(seed.E2ECompanyAdminEmail)
	path := "/analyzers/" + h.fx.AnalyzerA1.String()
	res := ca.do(http.MethodPatch, path, map[string]any{"installation_number": "X"})
	require.Equal(t, http.StatusBadRequest, res.status)
	require.Equal(t, "invalid_body", res.code(t))

	res = ca.do(http.MethodPatch, path, map[string]any{"building_id": h.fx.BuildingB1.String()})
	require.Equal(t, http.StatusNotFound, res.status, "a building of another company is not found")
	var got dto.Analyzer
	ca.do(http.MethodGet, path, nil).json(t, &got)
	require.Equal(t, h.fx.BuildingA1, *got.BuildingID, "a refused assignment changes nothing")

	res = ca.do(http.MethodPatch, path, map[string]any{"building_id": h.fx.BuildingA2.String(), "meter_multiplier": "40", "installed_power_kw": "250.5"})
	require.Equal(t, http.StatusOK, res.status, string(res.body))
	res.json(t, &got)
	require.Equal(t, h.fx.BuildingA2, *got.BuildingID)
	require.Equal(t, "40", got.MeterMultiplier.String())
	require.Equal(t, http.StatusUnprocessableEntity, ca.do(http.MethodPatch, path, map[string]any{"meter_multiplier": "0"}).status)

	res = ca.do(http.MethodPatch, path, map[string]any{"unassign_building": true})
	require.Equal(t, http.StatusOK, res.status, string(res.body))
	res.json(t, &got)
	require.Nil(t, got.BuildingID)
}

func TestAnalyzerRefreshEnqueues(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ca := h.as(seed.E2ECompanyAdminEmail)
	path := "/analyzers/" + h.fx.AnalyzerA1.String() + "/refresh"
	res := ca.do(http.MethodPost, path, map[string]any{"mode": "hourly"})
	require.Equal(t, http.StatusConflict, res.status)
	require.Equal(t, "integration_not_configured", res.code(t))

	ctx := t.Context()
	_, err := admin.NewCatalogueRepository(h.pool).UpsertIntegrationDefinitions(ctx, []model.IntegrationDefinition{
		{Provider: model.IntegrationProviderOSOS, Subtype: "Baskent", Endpoints: json.RawMessage(`{}`)},
	})
	require.NoError(t, err)
	cipher, err := crypto.NewCipher(h.cfg.Security.EncryptionKey)
	require.NoError(t, err)
	integrations := postgres.NewIntegrationRepository(h.pool, cipher)
	def, err := integrations.Definition(ctx, store.SystemScope(h.fx.CompanyA), model.IntegrationProviderOSOS, "Baskent")
	require.NoError(t, err)
	user := "osos-user"
	_, err = integrations.UpsertCredential(ctx, store.SystemScope(h.fx.CompanyA), model.IntegrationCredential{
		CompanyID: h.fx.CompanyA, DefinitionID: def.ID, Username: &user, IsActive: true,
	}, []byte("secret"), nil)
	require.NoError(t, err)

	res = ca.do(http.MethodPost, path, map[string]any{"mode": "hourly"})
	require.Equal(t, http.StatusAccepted, res.status, string(res.body))
	var accepted dto.JobAccepted
	res.json(t, &accepted)
	require.NotEmpty(t, accepted.JobID)
	tasks := h.enq.snapshot()
	require.Len(t, tasks, 1)
	require.Equal(t, job.TypeIntegrationRefreshAnalyzer, tasks[0].Type())
	p, err := job.DecodeRefreshAnalyzer(tasks[0])
	require.NoError(t, err)
	require.Equal(t, job.RefreshAnalyzerPayload{CompanyID: h.fx.CompanyA, CredentialID: p.CredentialID, AnalyzerID: h.fx.AnalyzerA1, Mode: "hourly"}, p)

	res = ca.do(http.MethodPost, path, map[string]any{"mode": "hourly"})
	require.Equal(t, http.StatusConflict, res.status)
	require.Equal(t, "refresh_in_progress", res.code(t))
	require.Equal(t, http.StatusUnprocessableEntity, ca.do(http.MethodPost, path, map[string]any{"mode": "weekly"}).status)

	ba := h.as(seed.E2EBuildingAdminEmail)
	require.Equal(t, http.StatusAccepted, ba.do(http.MethodPost, path, map[string]any{"mode": "energy"}).status)
	require.Equal(t, http.StatusNotFound, ba.do(http.MethodPost, "/analyzers/"+h.fx.AnalyzerA2.String()+"/refresh", map[string]any{"mode": "energy"}).status)
	require.Equal(t, http.StatusForbidden, h.as(seed.E2EBuildingReadonlyEmail).do(http.MethodPost, path, map[string]any{"mode": "energy"}).status)
}

func TestPlantLifecycleAndMonthlyTargets(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ca := h.as(seed.E2ECompanyAdminEmail)
	targets := make([]string, 12)
	for i := range targets {
		targets[i] = "1500.5"
	}
	res := ca.do(http.MethodPost, "/power-plants", map[string]any{"name": "Çatı GES", "monthly_targets": targets[:11]})
	require.Equal(t, http.StatusUnprocessableEntity, res.status)

	res = ca.do(http.MethodPost, "/power-plants", map[string]any{
		"name": "Çatı GES", "plant_kind": "rooftop", "orientation": "s", "installation_date": "2025-05-01",
		"monthly_targets": targets, "alarm_recipients": []string{"Teknik@Example.com"},
	})
	require.Equal(t, http.StatusCreated, res.status, string(res.body))
	var plant dto.PlantDetail
	res.json(t, &plant)
	require.Len(t, plant.MonthlyTargets, 12)
	require.Equal(t, "1500.5", plant.MonthlyTargets[0].String())
	require.Equal(t, []string{"teknik@example.com"}, plant.AlarmRecipients)
	require.Equal(t, "2025-05-01", plant.InstallationDate.Format("2006-01-02"))

	res = ca.do(http.MethodPatch, "/power-plants/"+plant.ID.String(), map[string]any{"orientation": "up"})
	require.Equal(t, http.StatusUnprocessableEntity, res.status)
	cr := h.as(seed.E2ECompanyReadonlyEmail)
	require.Equal(t, http.StatusOK, cr.do(http.MethodGet, "/power-plants/"+plant.ID.String(), nil).status)
	require.Equal(t, http.StatusNotFound, h.as(seed.E2ECompanyBAdminEmail).do(http.MethodGet, "/power-plants/"+plant.ID.String(), nil).status)
	require.Equal(t, http.StatusNoContent, ca.do(http.MethodDelete, "/power-plants/"+plant.ID.String(), nil).status)
}
