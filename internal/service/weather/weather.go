// Package weather serves the weather panel (06 §8, R291): coordinates from the
// plant or building record, a Redis cache, and explicit unavailable states.
package weather

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	isweather "github.com/MErenTalan/ekokod-rewrite/internal/integration/weather"
	perr "github.com/MErenTalan/ekokod-rewrite/internal/platform/errors"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// R291's cache lifetimes and potential bands (MJ/m² of daily shortwave radiation).
const (
	currentTTL  = 15 * time.Minute
	forecastTTL = 3 * time.Hour
)

var (
	highPotential   = decimal.NewFromInt(20)
	mediumPotential = decimal.NewFromInt(10)
)

// Provider is the weather adapter (*isweather.Client).
type Provider interface {
	Forecast(ctx context.Context, lat, lon decimal.Decimal) (isweather.Forecast, error)
}

// Cache is a byte cache with a TTL (Redis in production).
type Cache interface {
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
}

// Deps are the service inputs; a nil Provider means weather is not configured (air-gapped).
type Deps struct {
	Provider  Provider
	Cache     Cache
	Plants    store.PlantRepository
	Buildings store.BuildingRepository
}

// Service answers GET /weather.
type Service struct{ d Deps }

// New builds the service.
func New(d Deps) *Service { return &Service{d: d} }

// Day is a forecast day with its generation potential (nil when no radiation figure).
type Day struct {
	isweather.Day
	Potential *string
}

// View is the answer: never a location name, only what the record's coordinates give.
type View struct {
	Available bool
	Reason    string
	Current   *isweather.Current
	Days      []Day
}

func unavailable(reason string) View { return View{Reason: reason, Days: []Day{}} }

// For resolves the coordinates of exactly one of a plant or a building.
func (s *Service) For(ctx context.Context, sc store.Scope, plantID, buildingID *uuid.UUID) (View, error) {
	if (plantID == nil) == (buildingID == nil) {
		return View{}, perr.Validation.WithParams(map[string]any{"plant_id": []string{"exactly_one"}})
	}
	var lat, lon *decimal.Decimal
	if plantID != nil {
		if !sc.AllBuildings {
			return View{}, store.ErrNotFound
		}
		p, err := s.d.Plants.Get(ctx, sc, *plantID)
		if err != nil {
			return View{}, err
		}
		lat, lon = p.Latitude, p.Longitude
	} else {
		b, err := s.d.Buildings.Get(ctx, sc, *buildingID)
		if err != nil {
			return View{}, err
		}
		lat, lon = b.Latitude, b.Longitude
	}
	if s.d.Provider == nil {
		return unavailable("weather_not_configured"), nil
	}
	if lat == nil || lon == nil {
		return unavailable("location_not_configured"), nil
	}
	f, err := s.forecast(ctx, lat.Round(2), lon.Round(2))
	if err != nil {
		return unavailable("weather_unavailable"), nil
	}
	view := View{Available: true, Current: &f.Current, Days: make([]Day, len(f.Days))}
	for i, day := range f.Days {
		view.Days[i] = Day{Day: day, Potential: potential(day.ShortwaveMJ)}
	}
	return view, nil
}

func potential(mj *decimal.Decimal) *string {
	if mj == nil {
		return nil
	}
	p := "low"
	switch {
	case mj.GreaterThanOrEqual(highPotential):
		p = "high"
	case mj.GreaterThanOrEqual(mediumPotential):
		p = "medium"
	}
	return &p
}

// forecast reads current (15 min) and days (3 h) from the cache and asks the
// provider only when either is missing.
func (s *Service) forecast(ctx context.Context, lat, lon decimal.Decimal) (isweather.Forecast, error) {
	key := fmt.Sprintf("%s:%s", lat.StringFixed(2), lon.StringFixed(2))
	var out isweather.Forecast
	if s.d.Cache != nil {
		cur, okCur, errCur := s.d.Cache.Get(ctx, "weather:current:"+key)
		days, okDays, errDays := s.d.Cache.Get(ctx, "weather:days:"+key)
		if errCur == nil && errDays == nil && okCur && okDays &&
			json.Unmarshal(cur, &out.Current) == nil && json.Unmarshal(days, &out.Days) == nil {
			return out, nil
		}
	}
	f, err := s.d.Provider.Forecast(ctx, lat, lon)
	if err != nil {
		return isweather.Forecast{}, err
	}
	if s.d.Cache != nil {
		cur, errCur := json.Marshal(f.Current)
		days, errDays := json.Marshal(f.Days)
		if err := errors.Join(errCur, errDays); err == nil {
			// A cache that cannot write only costs a later call.
			_ = s.d.Cache.Set(ctx, "weather:current:"+key, cur, currentTTL)
			_ = s.d.Cache.Set(ctx, "weather:days:"+key, days, forecastTTL)
		}
	}
	return f, nil
}
