//go:build integration

package v1_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/dto"
	"github.com/MErenTalan/ekokod-rewrite/internal/ml"
	"github.com/MErenTalan/ekokod-rewrite/internal/seed"
)

// fakeMLService stands in for the ML service; err set means unreachable.
type fakeMLService struct{ err error }

func (f *fakeMLService) Forecast(_ context.Context, req ml.ForecastRequest) (ml.ForecastResponse, error) {
	if f.err != nil {
		return ml.ForecastResponse{}, f.err
	}
	last := time.Date(2026, 9, 17, 9, 0, 0, 0, time.UTC)
	if n := len(req.History); n > 0 {
		last = req.History[n-1].Ts
	}
	res := ml.ForecastResponse{Status: "ok", ModelID: "seasonal_naive", ModelVersion: "1.0.0", UsedCovariates: []string{"day_type"},
		Gaps: []ml.Gap{{Start: last.Add(-5 * time.Hour), End: last.Add(-4 * time.Hour), MissingHours: 2}}}
	for k := range req.Horizon {
		res.Timestamps = append(res.Timestamps, last.Add(time.Duration(k+1)*time.Hour))
		res.Median = append(res.Median, 5)
		res.P10 = append(res.P10, 4)
		res.P90 = append(res.P90, 6)
	}
	return res, nil
}

func (f *fakeMLService) Anomaly(_ context.Context, req ml.AnomalyRequest) (ml.AnomalyResponse, error) {
	if f.err != nil {
		return ml.AnomalyResponse{}, f.err
	}
	score, expected := 7.5, 3.0
	return ml.AnomalyResponse{IsAnomaly: req.Actual > 10, Score: &score, Expected: &expected, Method: "robust_zscore_same_hour_of_week", ModelID: "m", ModelVersion: "1"}, nil
}

// TestForecastDegradesWhenTheServiceIsDown is 09 §F13's graceful degradation over HTTP (R371).
func TestForecastDegradesWhenTheServiceIsDown(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ca := h.as(seed.E2ECompanyAdminEmail)
	run := ca.do(http.MethodPost, "/forecast/run", map[string]any{"analyzer_id": h.fx.AnalyzerA1.String(), "horizon_hours": 24})
	require.Equal(t, http.StatusServiceUnavailable, run.status, string(run.body))
	require.Contains(t, string(run.body), "forecast_unavailable")

	read := ca.do(http.MethodGet, "/forecast?analyzer_id="+h.fx.AnalyzerA1.String()+"&from=2026-09-17T00:00:00Z&to=2026-09-18T00:00:00Z", nil)
	require.Equal(t, http.StatusOK, read.status, string(read.body))
	var f dto.Forecast
	read.json(t, &f)
	require.Equal(t, "none", f.Status, "stored forecasts stay readable; none exist yet")

	check := ca.do(http.MethodPost, "/anomaly/check", map[string]any{"analyzer_id": h.fx.AnalyzerA1.String(), "ts": "2026-09-17T08:00:00Z", "actual": "4"})
	require.Equal(t, http.StatusOK, check.status)
	var a dto.AnomalyCheck
	check.json(t, &a)
	require.Equal(t, dto.AnomalyCheck{Available: false, Reason: "ml_service_unavailable"}, a)
}

func TestForecastRunReadAndScope(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.ml.err = nil
	ca := h.as(seed.E2ECompanyAdminEmail)
	run := ca.do(http.MethodPost, "/forecast/run", map[string]any{"analyzer_id": h.fx.AnalyzerA1.String(), "horizon_hours": 24})
	require.Equal(t, http.StatusOK, run.status, string(run.body))
	var ran dto.Forecast
	run.json(t, &ran)
	require.Equal(t, "ok", ran.Status)
	require.Len(t, ran.Points, 24)
	require.Len(t, ran.Gaps, 1)
	require.Equal(t, "seasonal_naive", *ran.ModelID)

	from, to := ran.Points[0].Ts.UTC().Format(time.RFC3339), ran.Points[23].Ts.Add(time.Hour).UTC().Format(time.RFC3339)
	q := "/forecast?analyzer_id=" + h.fx.AnalyzerA1.String() + "&from=" + from + "&to=" + to
	cr := h.as(seed.E2ECompanyReadonlyEmail)
	read := cr.do(http.MethodGet, q, nil)
	require.Equal(t, http.StatusOK, read.status, string(read.body))
	var stored dto.Forecast
	read.json(t, &stored)
	require.Equal(t, "ok", stored.Status)
	require.Len(t, stored.Points, 24)
	require.Len(t, stored.Gaps, 1, "the run's gaps come back with it")
	require.Equal(t, ran.GeneratedAt.Unix(), stored.GeneratedAt.Unix())

	require.Equal(t, http.StatusForbidden, cr.do(http.MethodPost, "/forecast/run", map[string]any{"analyzer_id": h.fx.AnalyzerA1.String(), "horizon_hours": 24}).status)
	foreign := map[string]any{"analyzer_id": h.fx.AnalyzerB1.String(), "horizon_hours": 24}
	require.Equal(t, http.StatusNotFound, ca.do(http.MethodPost, "/forecast/run", foreign).status)
	require.Equal(t, http.StatusNotFound, ca.do(http.MethodGet, "/forecast?analyzer_id="+h.fx.AnalyzerB1.String()+"&from="+from+"&to="+to, nil).status)
	require.Equal(t, http.StatusNotFound, ca.do(http.MethodPost, "/anomaly/check", map[string]any{"analyzer_id": h.fx.AnalyzerB1.String(), "ts": "2026-09-17T08:00:00Z", "actual": "4"}).status)
	require.Equal(t, http.StatusUnprocessableEntity, ca.do(http.MethodPost, "/forecast/run", map[string]any{"analyzer_id": h.fx.AnalyzerA1.String(), "horizon_hours": 745}).status)

	check := ca.do(http.MethodPost, "/anomaly/check", map[string]any{"analyzer_id": h.fx.AnalyzerA1.String(), "ts": "2026-09-17T08:00:00Z", "actual": "40"})
	require.Equal(t, http.StatusOK, check.status, string(check.body))
	var a dto.AnomalyCheck
	check.json(t, &a)
	require.True(t, a.Available)
	require.True(t, *a.IsAnomaly)
	require.Equal(t, "robust_zscore_same_hour_of_week", a.Method)
	require.Equal(t, "40", a.Actual.String())
}
