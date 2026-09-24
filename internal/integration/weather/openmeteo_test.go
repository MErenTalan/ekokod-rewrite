package weather_test

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/fake"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/httpx"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/weather"
)

const body = `{
 "current": {"temperature_2m": 21.4, "relative_humidity_2m": 55, "weather_code": 2, "wind_speed_10m": 12.3, "surface_pressure": 1012.5, "visibility": 24140},
 "daily": {
  "time": ["2026-09-17", "2026-09-18"],
  "weather_code": [2, 61],
  "temperature_2m_max": [27.1, null],
  "temperature_2m_min": [15.2, 14.0],
  "precipitation_probability_max": [10, 80],
  "uv_index_max": [6.2, 3.1],
  "shortwave_radiation_sum": [21.5, 9.9]
 }
}`

func client(t *testing.T, srv *fake.Server) *weather.Client {
	t.Helper()
	pool, err := httpx.NewPool(httpx.PoolOptions{PinnedCerts: srv.Pins, Sleep: func(context.Context, time.Duration) error { return nil }})
	require.NoError(t, err)
	return weather.New(pool, srv.URL)
}

func TestForecastNormalisesUnits(t *testing.T) {
	srv := fake.NewTLSServer(t, fake.Route{Method: http.MethodGet, Path: "/v1/forecast", Respond: fake.JSON(http.StatusOK, []byte(body))})
	f, err := client(t, srv).Forecast(context.Background(), decimal.RequireFromString("37.871"), decimal.RequireFromString("32.4846"))
	require.NoError(t, err)
	require.True(t, decimal.RequireFromString("21.4").Equal(*f.Current.TemperatureC))
	require.True(t, decimal.RequireFromString("24.14").Equal(*f.Current.VisibilityKm), "metres → km")
	require.Equal(t, int32(2), *f.Current.WeatherCode)
	require.Len(t, f.Days, 2)
	require.Nil(t, f.Days[1].MaxC, "a null stays null")
	require.True(t, decimal.RequireFromString("9.9").Equal(*f.Days[1].ShortwaveMJ))
	require.Equal(t, "2026-09-18", f.Days[1].Date.Format("2006-01-02"))

	q, err := url.ParseQuery(srv.Requests()[0].RawQuery)
	require.NoError(t, err)
	require.Equal(t, "37.871", q.Get("latitude"))
	require.Equal(t, "Europe/Istanbul", q.Get("timezone"))
	require.Equal(t, "7", q.Get("forecast_days"))
	require.Contains(t, q.Get("daily"), "shortwave_radiation_sum")
	require.Contains(t, q.Get("current"), "visibility")
}

func TestForecastProviderErrorIsClassified(t *testing.T) {
	srv := fake.NewTLSServer(t, fake.Route{Method: http.MethodGet, Path: "/v1/forecast", Respond: fake.JSON(http.StatusServiceUnavailable, []byte(`{}`))})
	_, err := client(t, srv).Forecast(context.Background(), decimal.NewFromInt(37), decimal.NewFromInt(32))
	require.Error(t, err)
	require.True(t, errors.Is(err, integration.ErrUpstreamUnavailable))
}

func TestForecastMalformedBody(t *testing.T) {
	srv := fake.NewTLSServer(t, fake.Route{Method: http.MethodGet, Path: "/v1/forecast", Respond: fake.JSON(http.StatusOK, []byte(`{"daily": {"time": ["x"]}}`))})
	_, err := client(t, srv).Forecast(context.Background(), decimal.NewFromInt(37), decimal.NewFromInt(32))
	require.True(t, errors.Is(err, integration.ErrMalformedPayload))
}
