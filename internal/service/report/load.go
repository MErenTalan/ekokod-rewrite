package report

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	domain "github.com/MErenTalan/ekokod-rewrite/internal/domain/report"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/assets"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/consumption"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// maxList is one page large enough for a building's analyzers or bills.
const maxList = 500

type loaded struct {
	buildings []domain.BuildingInput
	plants    []domain.PlantInput
	factor    *domain.GridFactor
}

// years is every calendar year a report reads: the previous year for the
// monthly comparison, two more for the yearly history (R271).
func years(r Request, p period) []int {
	if r.Type == domain.TypeMonthly {
		return []int{p.Year - 1, p.Year}
	}
	return []int{p.Year - 2, p.Year - 1, p.Year}
}

func (s *Service) load(ctx context.Context, sc store.Scope, r Request, p period) (loaded, error) {
	var out loaded
	ys := years(r, p)
	for _, id := range r.BuildingIDs {
		b, err := s.building(ctx, sc, id, r, p, ys)
		if err != nil {
			return out, err
		}
		out.buildings = append(out.buildings, b)
	}
	plants, err := s.plants(ctx, sc, r, p, ys)
	if err != nil {
		return out, err
	}
	out.plants = plants
	if r.Type == domain.TypeYearly {
		if out.factor, err = s.gridFactor(ctx, sc); err != nil {
			return out, err
		}
	}
	return out, nil
}

func (s *Service) building(ctx context.Context, sc store.Scope, id uuid.UUID, r Request, p period, ys []int) (domain.BuildingInput, error) {
	b, err := s.d.Buildings.Get(ctx, sc, id)
	if err != nil {
		return domain.BuildingInput{}, err
	}
	in := domain.BuildingInput{ID: b.ID, Name: b.Name,
		Consumption: map[int]domain.MonthSeries{}, Export: map[int]domain.MonthSeries{},
		Partial: map[int][12]bool{}, Bills: map[int][12]*domain.Bill{}}

	analyzers, err := s.d.Analyzers.List(ctx, sc, store.AnalyzerFilter{BuildingID: &id, Page: store.Page{Limit: maxList}})
	if err != nil {
		return in, err
	}
	ids := make([]uuid.UUID, len(analyzers))
	for i, a := range analyzers {
		ids[i] = a.ID
	}
	if err := s.consumption(ctx, sc, ids, r, p, ys, &in); err != nil {
		return in, err
	}

	bills, err := s.d.Bills.List(ctx, sc, store.BillFilter{BuildingID: &id, BillScope: ptr(model.BillScopeBuilding), Page: store.Page{Limit: maxList}})
	if err != nil {
		return in, err
	}
	for _, bill := range bills {
		t, err := time.Parse("2006-01", bill.PeriodKey)
		if err != nil {
			continue
		}
		if !slices.Contains(ys, t.Year()) {
			continue
		}
		series := in.Bills[t.Year()]
		series[t.Month()-1] = billInput(bill)
		in.Bills[t.Year()] = series
	}

	// The tariff in force at the period's last instant names the tariff.
	switch t, err := s.d.Tariffs.Effective(ctx, sc, id, p.end().Add(-time.Nanosecond)); {
	case err == nil:
		in.TariffName = t.Name
	case !errors.Is(err, store.ErrNotFound):
		return in, err
	}
	return in, nil
}

// consumption reads the monthly aggregates year by year and in chunks of
// the analytics path's analyzer limit (R254).
func (s *Service) consumption(ctx context.Context, sc store.Scope, ids []uuid.UUID, r Request, p period, ys []int, in *domain.BuildingInput) error {
	for _, y := range ys {
		from := time.Date(y, 1, 1, 0, 0, 0, 0, istanbul)
		for start := 0; start < len(ids); start += consumption.MaxAnalyzersPerRequest {
			chunk := ids[start:min(start+consumption.MaxAnalyzersPerRequest, len(ids))]
			rows, err := s.d.Consumption.Consumption(ctx, sc, consumption.SeriesRequest{
				AnalyzerIDs: chunk, Level: energy.Monthly, Range: store.TimeRange{From: from, To: from.AddDate(1, 0, 0)},
			})
			if err != nil {
				return err
			}
			for _, row := range rows {
				local := row.Window.From.In(istanbul)
				if local.Year() != y {
					continue
				}
				m := int(local.Month())
				add(in.Consumption, y, m, row.Values[energy.ActiveImport])
				add(in.Export, y, m, row.Values[energy.ActiveExport])
				if row.Partial {
					flags := in.Partial[y]
					flags[m-1] = true
					in.Partial[y] = flags
				}
				if r.Type == domain.TypeMonthly && y == p.Year && m == p.Month {
					in.T1 = sum(in.T1, row.Values[energy.T1Import])
					in.T2 = sum(in.T2, row.Values[energy.T2Import])
					in.T3 = sum(in.T3, row.Values[energy.T3Import])
					in.Inductive = sum(in.Inductive, row.Values[energy.ReactiveInductiveImport])
					in.Capacitive = sum(in.Capacitive, row.Values[energy.ReactiveCapacitiveImport])
				}
			}
		}
	}
	return nil
}

// plants resolves the selection (R269) and reads production, feed-in and targets.
func (s *Service) plants(ctx context.Context, sc store.Scope, r Request, p period, ys []int) ([]domain.PlantInput, error) {
	var list []model.PowerPlant
	if len(r.PlantIDs) > 0 {
		for _, id := range r.PlantIDs {
			pl, err := s.d.Plants.Get(ctx, sc, id)
			if err != nil {
				return nil, err
			}
			list = append(list, pl)
		}
	} else {
		f := store.PlantFilter{Page: store.Page{Limit: MaxPlants}}
		if r.Selection != domain.SelectionAll {
			kind := r.Selection
			f.PlantKind = &kind
		}
		var err error
		if list, err = s.d.Plants.List(ctx, sc, f); err != nil {
			return nil, err
		}
	}
	if len(list) == 0 {
		return nil, nil
	}
	ids := make([]uuid.UUID, len(list))
	for i, pl := range list {
		ids[i] = pl.ID
	}
	from := time.Date(ys[0], 1, 1, 0, 0, 0, 0, istanbul)
	buckets, err := s.d.Analytics.ProductionMonthly(ctx, sc, ids, store.TimeRange{From: from, To: time.Date(p.Year+1, 1, 1, 0, 0, 0, 0, istanbul)})
	if err != nil {
		return nil, err
	}
	production := map[uuid.UUID]map[int]domain.MonthSeries{}
	for _, b := range buckets {
		local := b.Bucket.In(istanbul)
		if production[b.PlantID] == nil {
			production[b.PlantID] = map[int]domain.MonthSeries{}
		}
		add(production[b.PlantID], local.Year(), int(local.Month()), b.ProductionKwh)
	}

	out := make([]domain.PlantInput, 0, len(list))
	for _, pl := range list {
		in := domain.PlantInput{ID: pl.ID, Name: pl.Name, Kind: pl.PlantKind, Production: production[pl.ID], YearlyTarget: pl.YearlyTargetKwh}
		switch t, err := s.d.Solar.Effective(ctx, sc, pl.ID, p.first()); {
		case err == nil:
			price := t.FeedInTariff
			in.FeedIn = &price
		case !errors.Is(err, store.ErrNotFound):
			return nil, err
		}
		targets, err := s.d.Plants.MonthlyTargets(ctx, sc, pl.ID)
		if err != nil {
			return nil, err
		}
		if len(targets) == 12 {
			in.MonthlyTargets = make([]decimal.Decimal, 12)
			for _, t := range targets {
				in.MonthlyTargets[t.Month-1] = t.TargetKwh
			}
		}
		out = append(out, in)
	}
	return out, nil
}

// gridFactor is R262: the company's own override first, else the platform row.
func (s *Service) gridFactor(ctx context.Context, sc store.Scope) (*domain.GridFactor, error) {
	factors, err := s.d.Carbon.ListFactors(ctx, sc, store.EmissionFactorFilter{Keys: []string{assets.GridFactorKey}, IncludePlatform: true})
	if err != nil {
		return nil, err
	}
	var chosen *model.EmissionFactor
	for i := range factors {
		if chosen == nil || factors[i].CompanyID != nil {
			chosen = &factors[i]
		}
	}
	if chosen == nil {
		return nil, nil
	}
	f := &domain.GridFactor{Value: chosen.BaseFactor, Unit: chosen.BaseUnit}
	if chosen.SourceYear != nil {
		y := int(*chosen.SourceYear)
		f.SourceYear = &y
	}
	return f, nil
}

func billInput(b model.Bill) *domain.Bill {
	return &domain.Bill{
		Currency: string(b.Currency), Total: b.TotalCost, EnergyCost: b.EnergyCost, DistributionCost: b.DistributionCost,
		Taxes: b.VatCost.Add(b.OtherTaxesCost), ReactivePenalty: b.ReactivePenalty, NetConsumption: b.NetConsumption,
		EffectivePrice: b.EffectiveEnergyPrice, GenerationPrice: b.GenerationPricePerKwh,
	}
}

func add(m map[int]domain.MonthSeries, year, month int, v *decimal.Decimal) {
	if v == nil {
		return
	}
	s := m[year]
	s[month-1] = sum(s[month-1], v)
	m[year] = s
}

func sum(a, b *decimal.Decimal) *decimal.Decimal {
	switch {
	case b == nil:
		return a
	case a == nil:
		v := *b
		return &v
	}
	v := a.Add(*b)
	return &v
}

func ptr[T any](v T) *T { return &v }
