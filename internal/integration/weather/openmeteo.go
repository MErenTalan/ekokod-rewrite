// Package weather is the Open-Meteo adapter (06 §8, R291). It is pure: no
// store, no cache; internal/service/weather caches and chooses coordinates.
package weather

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/httpx"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/normalize"
)

const (
	currentFields = "temperature_2m,relative_humidity_2m,weather_code,wind_speed_10m,surface_pressure,visibility"
	dailyFields   = "weather_code,temperature_2m_max,temperature_2m_min,precipitation_probability_max,uv_index_max,shortwave_radiation_sum"
	forecastPath  = "/v1/forecast?latitude={lat}&longitude={lon}&current=" + currentFields + "&daily=" + dailyFields +
		"&timezone=Europe%2FIstanbul&forecast_days=7"
)

// Current is the conditions now; wind is km/h, visibility km, pressure hPa.
type Current struct {
	TemperatureC *decimal.Decimal
	HumidityPct  *decimal.Decimal
	WindKmh      *decimal.Decimal
	PressureHpa  *decimal.Decimal
	VisibilityKm *decimal.Decimal
	WeatherCode  *int32
}

// Day is one forecast day; ShortwaveMJ is the day's shortwave radiation sum, MJ/m².
type Day struct {
	Date             time.Time
	MinC, MaxC       *decimal.Decimal
	WeatherCode      *int32
	PrecipitationPct *decimal.Decimal
	UVIndexMax       *decimal.Decimal
	ShortwaveMJ      *decimal.Decimal
}

// Forecast is one answer: current conditions and seven days.
type Forecast struct {
	Current Current
	Days    []Day
}

// Client calls Open-Meteo through the pinned httpx pool.
type Client struct {
	http *httpx.Client
	base string
}

// New builds a client for baseURL (https, e.g. https://api.open-meteo.com).
func New(pool *httpx.Pool, baseURL string) *Client {
	return &Client{base: baseURL, http: pool.Client(httpx.ClientConfig{
		Provider: integration.ProviderWeather, LimiterKey: "weather", Every: 200 * time.Millisecond, Burst: 5,
		RequestTimeout: 10 * time.Second, MaxAttempts: 2,
	})}
}

type wire struct {
	Current map[string]json.Number `json:"current"`
	Daily   struct {
		Time          []string       `json:"time"`
		Code          []*json.Number `json:"weather_code"`
		Max           []*json.Number `json:"temperature_2m_max"`
		Min           []*json.Number `json:"temperature_2m_min"`
		Precipitation []*json.Number `json:"precipitation_probability_max"`
		UV            []*json.Number `json:"uv_index_max"`
		Shortwave     []*json.Number `json:"shortwave_radiation_sum"`
	} `json:"daily"`
}

func dec(n *json.Number) *decimal.Decimal {
	if n == nil {
		return nil
	}
	v, err := decimal.NewFromString(n.String())
	if err != nil {
		return nil
	}
	return &v
}

func code(n *json.Number) *int32 {
	if n == nil {
		return nil
	}
	v, err := n.Int64()
	if err != nil {
		return nil
	}
	c := int32(v)
	return &c
}

func at[T any](s []*T, i int) *T {
	if i < len(s) {
		return s[i]
	}
	return nil
}

// Forecast fetches current conditions and a seven-day forecast for a point.
func (c *Client) Forecast(ctx context.Context, lat, lon decimal.Decimal) (Forecast, error) {
	resp, err := c.http.Do(ctx, httpx.Request{Op: "forecast", Method: http.MethodGet, Template: c.base + forecastPath,
		Params: map[string]httpx.Param{"lat": {Value: lat.String()}, "lon": {Value: lon.String()}}})
	if err != nil {
		return Forecast{}, err
	}
	var w wire
	if err := json.Unmarshal(resp.Body, &w); err != nil {
		return Forecast{}, &integration.Error{Kind: integration.ErrMalformedPayload, Provider: integration.ProviderWeather, Op: "forecast"}
	}
	cur := func(k string) *json.Number {
		if v, ok := w.Current[k]; ok {
			return &v
		}
		return nil
	}
	out := Forecast{Current: Current{
		TemperatureC: dec(cur("temperature_2m")), HumidityPct: dec(cur("relative_humidity_2m")), WindKmh: dec(cur("wind_speed_10m")),
		PressureHpa: dec(cur("surface_pressure")), WeatherCode: code(cur("weather_code")),
	}}
	if m := dec(cur("visibility")); m != nil {
		km := m.Div(decimal.NewFromInt(1000))
		out.Current.VisibilityKm = &km
	}
	for i, day := range w.Daily.Time {
		d, err := normalize.LocalLayout("2006-01-02", day)
		if err != nil {
			return Forecast{}, &integration.Error{Kind: integration.ErrMalformedPayload, Provider: integration.ProviderWeather, Op: "forecast"}
		}
		out.Days = append(out.Days, Day{Date: d.In(normalize.Istanbul), MinC: dec(at(w.Daily.Min, i)), MaxC: dec(at(w.Daily.Max, i)),
			WeatherCode: code(at(w.Daily.Code, i)), PrecipitationPct: dec(at(w.Daily.Precipitation, i)),
			UVIndexMax: dec(at(w.Daily.UV, i)), ShortwaveMJ: dec(at(w.Daily.Shortwave, i))})
	}
	return out, nil
}
