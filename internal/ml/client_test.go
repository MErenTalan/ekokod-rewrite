package ml_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/ml"
)

const key = "shared-secret-0123456789"

func forecastBody() ml.ForecastResponse {
	return ml.ForecastResponse{Status: "ok", ModelID: "random_forest", ModelVersion: "1.0.0",
		Timestamps: []time.Time{time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}, Median: []float64{5}, P10: []float64{4}, P90: []float64{6},
		Gaps: []ml.Gap{{Start: time.Date(2026, 8, 1, 3, 0, 0, 0, time.UTC), End: time.Date(2026, 8, 1, 5, 0, 0, 0, time.UTC), MissingHours: 3}}}
}

func client(url string, timeout time.Duration) *ml.Client {
	return ml.New(url, key, timeout, ml.WithBackoff(time.Millisecond, 2*time.Millisecond))
}

func TestForecastSendsTheContractAndTheSecret(t *testing.T) {
	var got ml.ForecastRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/forecast", r.URL.Path)
		require.Equal(t, "Bearer "+key, r.Header.Get("Authorization"))
		require.NoError(t, json.NewDecoder(r.Body).Decode(&got))
		_ = json.NewEncoder(w).Encode(forecastBody())
	}))
	defer srv.Close()
	v := 3.5
	res, err := client(srv.URL, time.Second).Forecast(context.Background(), ml.ForecastRequest{
		SeriesID: "a", Granularity: "hour", Horizon: 24,
		History:    []ml.Point{{Ts: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), Value: &v}, {Ts: time.Date(2026, 8, 1, 1, 0, 0, 0, time.UTC)}},
		Covariates: ml.Covariates{DayType: []ml.DayType{{Date: "2026-08-01", Type: "weekend"}}, Vacation: []ml.Vacation{{Date: "2026-08-03", Vacation: true}}},
	})
	require.NoError(t, err)
	require.Equal(t, "random_forest", res.ModelID)
	require.Equal(t, 3, res.Gaps[0].MissingHours)
	require.Equal(t, "a", got.SeriesID)
	require.Nil(t, got.History[1].Value, "a nil value travels as null")
	require.Equal(t, "weekend", got.Covariates.DayType[0].Type)
}

// TestGracefulDegradation is 09 §F13's named check: every way the service can be
// missing becomes ErrUnavailable, fast and without panics.
func TestGracefulDegradation(t *testing.T) {
	ctx := context.Background()

	t.Run("unreachable", func(t *testing.T) {
		srv := httptest.NewServer(http.NotFoundHandler())
		url := srv.URL
		srv.Close()
		start := time.Now()
		_, err := client(url, time.Second).Forecast(ctx, ml.ForecastRequest{})
		require.ErrorIs(t, err, ml.ErrUnavailable)
		require.Less(t, time.Since(start), 2*time.Second)
	})

	t.Run("retries 503 then succeeds", func(t *testing.T) {
		var calls atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			if calls.Add(1) < 3 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			_ = json.NewEncoder(w).Encode(forecastBody())
		}))
		defer srv.Close()
		_, err := client(srv.URL, time.Second).Forecast(ctx, ml.ForecastRequest{})
		require.NoError(t, err)
		require.EqualValues(t, 3, calls.Load())
	})

	t.Run("gives up after two retries", func(t *testing.T) {
		var calls atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			calls.Add(1)
			w.WriteHeader(http.StatusBadGateway)
		}))
		defer srv.Close()
		_, err := client(srv.URL, time.Second).Forecast(ctx, ml.ForecastRequest{})
		require.ErrorIs(t, err, ml.ErrUnavailable)
		require.EqualValues(t, 3, calls.Load())
	})

	for _, status := range []int{http.StatusInternalServerError, http.StatusUnauthorized} {
		t.Run(http.StatusText(status)+" is not retried", func(t *testing.T) {
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.WriteHeader(status)
			}))
			defer srv.Close()
			_, err := client(srv.URL, time.Second).Forecast(ctx, ml.ForecastRequest{})
			require.ErrorIs(t, err, ml.ErrUnavailable)
			require.EqualValues(t, 1, calls.Load())
		})
	}

	t.Run("slow answer times out", func(t *testing.T) {
		release := make(chan struct{})
		srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			select {
			case <-release:
			case <-r.Context().Done():
			}
		}))
		defer srv.Close()
		defer close(release)
		_, err := client(srv.URL, 50*time.Millisecond).Forecast(ctx, ml.ForecastRequest{})
		require.ErrorIs(t, err, ml.ErrUnavailable)
	})

	t.Run("garbage body", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("<html>")) }))
		defer srv.Close()
		_, err := client(srv.URL, time.Second).Forecast(ctx, ml.ForecastRequest{})
		require.ErrorIs(t, err, ml.ErrUnavailable)
	})

	t.Run("unconfigured", func(t *testing.T) {
		_, err := ml.New("", key, time.Second).Forecast(ctx, ml.ForecastRequest{})
		require.ErrorIs(t, err, ml.ErrUnavailable)
	})
}

func TestRejectedRequestIsNotUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"detail":"unknown model"}`))
	}))
	defer srv.Close()
	_, err := client(srv.URL, time.Second).Forecast(context.Background(), ml.ForecastRequest{})
	require.ErrorIs(t, err, ml.ErrRejected)
	require.False(t, errors.Is(err, ml.ErrUnavailable))
}

func TestAnomalyAndModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/anomaly":
			var req ml.AnomalyRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			require.InDelta(t, 80.0, req.Actual, 1e-9)
			_, _ = w.Write([]byte(`{"is_anomaly":true,"score":9.1,"expected":10,"lower":4.8,"upper":15.2,"method":"robust_zscore_same_hour_of_week","model_id":"m","model_version":"1"}`))
		case "/v1/models":
			require.Equal(t, http.MethodGet, r.Method)
			_, _ = w.Write([]byte(`{"items":[{"id":"seasonal_naive","version":"1.0.0","available":true,"trained_at":null,"default":false}]}`))
		}
	}))
	defer srv.Close()
	c := client(srv.URL, time.Second)
	a, err := c.Anomaly(context.Background(), ml.AnomalyRequest{SeriesID: "a", Ts: time.Now().UTC(), Actual: 80})
	require.NoError(t, err)
	require.True(t, a.IsAnomaly)
	require.Equal(t, "robust_zscore_same_hour_of_week", a.Method)
	models, err := c.Models(context.Background())
	require.NoError(t, err)
	require.Equal(t, "seasonal_naive", models[0].ID)
}
