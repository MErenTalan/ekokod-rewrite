package renewable

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	perr "github.com/MErenTalan/ekokod-rewrite/internal/platform/errors"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// MaxRangeDays is R298's renewable range limit.
const MaxRangeDays = 366

const pageSize = 500

// Deps is what the loader reads.
type Deps struct {
	Analyzers store.AnalyzerRepository
	Buildings store.BuildingRepository
	Analytics store.AnalyticsRepository
	Bills     store.BillRepository
	Forecasts store.ForecastRepository
	Carbon    store.CarbonRepository
	Clock     clock.Clock
}

// Service loads R292's inputs for one analyzer or one building.
type Service struct{ d Deps }

// New builds the service.
func New(d Deps) *Service { return &Service{d: d} }

// Query is a panel request: exactly one subject and a range.
type Query struct {
	AnalyzerID, BuildingID *uuid.UUID
	From, To               time.Time
}

// Validate is R298: one subject, a forward range of at most 366 days.
func (q Query) Validate() error {
	if (q.AnalyzerID == nil) == (q.BuildingID == nil) {
		return perr.Validation.WithParams(map[string]any{"analyzer_id": []string{"subject_required"}})
	}
	if !q.To.After(q.From) {
		return perr.Validation.WithParams(map[string]any{"to": []string{"invalid_range"}})
	}
	if q.To.Sub(q.From) > MaxRangeDays*24*time.Hour {
		return perr.Validation.WithParams(map[string]any{"to": []string{"range_too_long"}})
	}
	return nil
}

// Load reads every input a panel may use. A subject outside the scope is
// ErrNotFound (a building admin sees only their buildings' analyzers).
func (s *Service) Load(ctx context.Context, sc store.Scope, q Query) (Inputs, error) {
	if err := q.Validate(); err != nil {
		return Inputs{}, err
	}
	now := s.d.Clock.Now()
	in := Inputs{Now: now, From: q.From, To: q.To}
	analyzers, err := s.subject(ctx, sc, q)
	if err != nil {
		return Inputs{}, err
	}
	ids := make([]uuid.UUID, len(analyzers))
	for i, a := range analyzers {
		ids[i] = a.ID
		if a.LastReadingAt != nil && (in.LastReadingAt == nil || a.LastReadingAt.After(*in.LastReadingAt)) {
			in.LastReadingAt = a.LastReadingAt
		}
	}
	if err := s.loadBills(ctx, sc, q, &in); err != nil {
		return Inputs{}, err
	}
	if in.GridFactor, in.Equivalences, err = s.factors(ctx, sc); err != nil {
		return Inputs{}, err
	}
	if len(ids) == 0 {
		return in, nil
	}
	hourFrom, hourTo := q.From, q.To
	if monthAgo := now.AddDate(0, 0, -30); monthAgo.Before(hourFrom) {
		hourFrom = monthAgo
	}
	if now.After(hourTo) {
		hourTo = now
	}
	if in.Hourly, err = s.d.Analytics.ConsumptionHourly(ctx, sc, ids, store.TimeRange{From: hourFrom, To: hourTo}); err != nil {
		return Inputs{}, err
	}
	if in.Daily, err = s.d.Analytics.ConsumptionDaily(ctx, sc, ids, store.TimeRange{From: q.From, To: q.To}); err != nil {
		return Inputs{}, err
	}
	window := store.TimeRange{From: now.AddDate(0, 0, -31), To: now.AddDate(0, 0, 29)}
	for _, id := range ids {
		rows, err := s.d.Forecasts.Range(ctx, sc, id, window)
		if err != nil {
			return Inputs{}, err
		}
		in.Forecasts = append(in.Forecasts, rows...)
	}
	return in, nil
}

func (s *Service) subject(ctx context.Context, sc store.Scope, q Query) ([]model.Analyzer, error) {
	if q.AnalyzerID != nil {
		a, err := s.d.Analyzers.Get(ctx, sc, *q.AnalyzerID)
		if err != nil {
			return nil, err
		}
		return []model.Analyzer{a}, nil
	}
	// The building itself decides visibility: an empty analyzer list cannot
	// tell "no analyzers" from "not yours".
	if _, err := s.d.Buildings.Get(ctx, sc, *q.BuildingID); err != nil {
		return nil, err
	}
	var out []model.Analyzer
	for offset := int32(0); ; offset += pageSize {
		page, err := s.d.Analyzers.List(ctx, sc, store.AnalyzerFilter{BuildingID: q.BuildingID, Page: store.Page{Limit: pageSize, Offset: offset}})
		if err != nil {
			return nil, err
		}
		out = append(out, page...)
		if len(page) < pageSize {
			break
		}
	}
	return out, nil
}

// loadBills reads the subject's bills: analyzer-scope for an analyzer,
// building-scope for a building.
func (s *Service) loadBills(ctx context.Context, sc store.Scope, q Query, in *Inputs) error {
	f := store.BillFilter{AnalyzerID: q.AnalyzerID}
	scope := model.BillScopeAnalyzer
	if q.BuildingID != nil {
		f = store.BillFilter{BuildingID: q.BuildingID}
		scope = model.BillScopeBuilding
	}
	f.BillScope = &scope
	for offset := int32(0); ; offset += pageSize {
		f.Page = store.Page{Limit: pageSize, Offset: offset}
		page, err := s.d.Bills.List(ctx, sc, f)
		if err != nil {
			return err
		}
		in.Bills = append(in.Bills, page...)
		if len(page) < pageSize {
			break
		}
	}
	for i := range in.Bills {
		if in.LatestBill == nil || in.Bills[i].PeriodKey > in.LatestBill.PeriodKey {
			in.LatestBill = &in.Bills[i]
		}
	}
	return nil
}

// factors is R262's lookup (company override first) for the grid factor and
// R293's equivalences.
func (s *Service) factors(ctx context.Context, sc store.Scope) (*Factor, map[string]Factor, error) {
	keys := []string{GridFactorKey, EquivTree, EquivCoal, EquivCar, EquivHome}
	rows, err := s.d.Carbon.ListFactors(ctx, sc, store.EmissionFactorFilter{Keys: keys, IncludePlatform: true})
	if err != nil {
		return nil, nil, err
	}
	chosen := map[string]model.EmissionFactor{}
	for _, r := range rows {
		if prev, ok := chosen[r.Key]; !ok || (prev.CompanyID == nil && r.CompanyID != nil) {
			chosen[r.Key] = r
		}
	}
	equivalences := map[string]Factor{}
	var grid *Factor
	for key, r := range chosen {
		f := Factor{Value: r.BaseFactor, Unit: r.BaseUnit, Year: r.SourceYear}
		if r.Source != nil {
			f.Source = *r.Source
		}
		if key == GridFactorKey {
			grid = &f
			continue
		}
		equivalences[key] = f
	}
	return grid, equivalences, nil
}
