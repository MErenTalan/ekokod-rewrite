package weather_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	isweather "github.com/MErenTalan/ekokod-rewrite/internal/integration/weather"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/weather"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

func d(s string) *decimal.Decimal { v := decimal.RequireFromString(s); return &v }

type fakeProvider struct {
	calls int
	err   error
	lat   decimal.Decimal
}

func (f *fakeProvider) Forecast(_ context.Context, lat, _ decimal.Decimal) (isweather.Forecast, error) {
	f.calls++
	f.lat = lat
	if f.err != nil {
		return isweather.Forecast{}, f.err
	}
	day := func(i int, mj string) isweather.Day {
		return isweather.Day{Date: time.Date(2026, 9, 17+i, 0, 0, 0, 0, time.UTC), ShortwaveMJ: d(mj)}
	}
	return isweather.Forecast{Current: isweather.Current{TemperatureC: d("21.4")},
		Days: []isweather.Day{day(0, "20"), day(1, "19.9"), day(2, "10"), day(3, "9.9"), {Date: time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC)}}}, nil
}

type memCache struct {
	data map[string][]byte
	ttls map[string]time.Duration
}

func newCache() *memCache {
	return &memCache{data: map[string][]byte{}, ttls: map[string]time.Duration{}}
}
func (m *memCache) Get(_ context.Context, key string) ([]byte, bool, error) {
	v, ok := m.data[key]
	return v, ok, nil
}
func (m *memCache) Set(_ context.Context, key string, v []byte, ttl time.Duration) error {
	m.data[key], m.ttls[key] = v, ttl
	return nil
}

type fakePlants struct {
	store.PlantRepository
	plant model.PowerPlant
}

func (f fakePlants) Get(_ context.Context, _ store.Scope, id uuid.UUID) (model.PowerPlant, error) {
	if id != f.plant.ID {
		return model.PowerPlant{}, store.ErrNotFound
	}
	return f.plant, nil
}

type fakeBuildings struct {
	store.BuildingRepository
	building model.Building
}

func (f fakeBuildings) Get(_ context.Context, _ store.Scope, id uuid.UUID) (model.Building, error) {
	if id != f.building.ID {
		return model.Building{}, store.ErrNotFound
	}
	return f.building, nil
}

var (
	company  = uuid.New()
	plant    = model.PowerPlant{ID: uuid.New(), CompanyID: company, Name: "Konya GES", Latitude: d("37.87123"), Longitude: d("32.48456")}
	building = model.Building{ID: uuid.New(), CompanyID: company, Name: "Ankara Ofis"}
	all      = store.SystemScope(company)
)

func svc(p weather.Provider, c weather.Cache) *weather.Service {
	return weather.New(weather.Deps{Provider: p, Cache: c, Plants: fakePlants{plant: plant}, Buildings: fakeBuildings{building: building}})
}

func TestWeatherNotConfigured(t *testing.T) {
	v, err := svc(nil, newCache()).For(context.Background(), all, &plant.ID, nil)
	require.NoError(t, err)
	require.False(t, v.Available)
	require.Equal(t, "weather_not_configured", v.Reason)
}

// 10 item 32: no coordinates means no city, never a default one.
func TestWeatherLocationNotConfiguredHasNoName(t *testing.T) {
	p := &fakeProvider{}
	v, err := svc(p, newCache()).For(context.Background(), all, nil, &building.ID)
	require.NoError(t, err)
	require.Equal(t, "location_not_configured", v.Reason)
	require.Zero(t, p.calls)
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "Ankara")
}

func TestWeatherCachesCurrentFor15Minutes(t *testing.T) {
	p, c := &fakeProvider{}, newCache()
	s := svc(p, c)
	v, err := s.For(context.Background(), all, &plant.ID, nil)
	require.NoError(t, err)
	require.True(t, v.Available)
	_, err = s.For(context.Background(), all, &plant.ID, nil)
	require.NoError(t, err)
	require.Equal(t, 1, p.calls, "the second call is served from the cache")
	require.True(t, decimal.RequireFromString("37.87").Equal(p.lat), "coordinates rounded to 2 dp")
	var ttls []time.Duration
	for _, ttl := range c.ttls {
		ttls = append(ttls, ttl)
	}
	require.ElementsMatch(t, []time.Duration{15 * time.Minute, 3 * time.Hour}, ttls)
}

func TestPotentialBands(t *testing.T) {
	v, err := svc(&fakeProvider{}, newCache()).For(context.Background(), all, &plant.ID, nil)
	require.NoError(t, err)
	var got []string
	for _, day := range v.Days {
		if day.Potential == nil {
			got = append(got, "")
		} else {
			got = append(got, *day.Potential)
		}
	}
	require.Equal(t, []string{"high", "medium", "medium", "low", ""}, got)
}

func TestProviderErrorIsUnavailable(t *testing.T) {
	v, err := svc(&fakeProvider{err: errors.New("down")}, newCache()).For(context.Background(), all, &plant.ID, nil)
	require.NoError(t, err)
	require.Equal(t, "weather_unavailable", v.Reason)
}

func TestWeatherScope(t *testing.T) {
	s := svc(&fakeProvider{}, newCache())
	buildingOnly := store.Scope{CompanyID: company, BuildingIDs: []uuid.UUID{building.ID}}
	_, err := s.For(context.Background(), buildingOnly, &plant.ID, nil)
	require.ErrorIs(t, err, store.ErrNotFound, "plants are company-level")
	other := uuid.New()
	_, err = s.For(context.Background(), all, nil, &other)
	require.ErrorIs(t, err, store.ErrNotFound)
	_, err = s.For(context.Background(), all, &plant.ID, &building.ID)
	require.Error(t, err, "exactly one of plant_id and building_id")
}
