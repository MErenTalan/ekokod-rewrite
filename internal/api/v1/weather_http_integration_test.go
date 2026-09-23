//go:build integration

package v1_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/dto"
	"github.com/MErenTalan/ekokod-rewrite/internal/seed"
)

// The harness configures no weather provider: an air-gapped installation (R291).
func TestWeatherSaysNotConfiguredAndNeverNamesACity(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	var body dto.Weather
	res := h.as(seed.E2EBuildingAdminEmail).do(http.MethodGet, "/weather?building_id="+h.fx.BuildingA1.String(), nil)
	require.Equal(t, http.StatusOK, res.status, string(res.body))
	res.json(t, &body)
	require.False(t, body.Available)
	require.Equal(t, "weather_not_configured", body.Reason)
	require.NotContains(t, string(res.body), "Ankara")
}

func TestWeatherIsScoped(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ba := h.as(seed.E2EBuildingAdminEmail)
	require.Equal(t, http.StatusNotFound, ba.do(http.MethodGet, "/weather?building_id="+h.fx.BuildingA2.String(), nil).status)
	require.Equal(t, http.StatusNotFound, ba.do(http.MethodGet, "/weather?building_id="+h.fx.BuildingB1.String(), nil).status)
	plant := h.plantFor(h.fx.CompanyA, "A GES", "PS-W").String()
	require.Equal(t, http.StatusNotFound, ba.do(http.MethodGet, "/weather?plant_id="+plant, nil).status, "plants are company-level")
	ca := h.as(seed.E2ECompanyAdminEmail)
	require.Equal(t, http.StatusOK, ca.do(http.MethodGet, "/weather?plant_id="+plant, nil).status)
	require.Equal(t, http.StatusUnprocessableEntity, ca.do(http.MethodGet, "/weather", nil).status)
	require.Equal(t, http.StatusNotFound, ca.do(http.MethodGet, "/weather?plant_id="+uuid.NewString(), nil).status)
}
