package financial

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

var istanbul = func() *time.Location {
	l, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		panic(err)
	}
	return l
}()

const pageSize = 500

// Deps is what the service reads.
type Deps struct {
	Bills     store.BillRepository
	Plants    store.PlantRepository
	Analytics store.AnalyticsRepository
	Solar     store.SolarTariffRepository
	Analyzers store.AnalyzerRepository
	Buildings store.BuildingRepository
	Tariffs   store.TariffRepository
	Clock     clock.Clock
}

// Service answers R294's two routes.
type Service struct{ d Deps }

// New builds the service.
func New(d Deps) *Service { return &Service{d: d} }

// Period is the summary's year and optional month (0 = the whole year).
type Period struct{ Year, Month int }

// BuildingTariff is one building's purchase tariff in force today.
type BuildingTariff struct {
	BuildingID         uuid.UUID
	BuildingName       string
	PriceType          model.PriceType
	Single, T1, T2, T3 *decimal.Decimal
	Currency           model.CurrencyCode
}

// PlantFeedIn is one plant's feed-in price in force today.
type PlantFeedIn struct {
	PlantID   uuid.UUID
	PlantName string
	Price     Price
}

// TariffPanel lists the tariffs per building and plant (R294).
type TariffPanel struct {
	Purchase                     []BuildingTariff
	Sale                         []PlantFeedIn
	PurchaseMissing, SaleMissing bool
}

// Summary is the headline: counts, the period's figures and the tariffs.
type Summary struct {
	Period                    Period
	AnalyzerCount, PlantCount int
	Figures                   Month
	WithData, Of              int
	Tariffs                   TariffPanel
}

// Year loads and builds R294's months for a company-level scope.
func (s *Service) Year(ctx context.Context, sc store.Scope, year int) (Year, error) {
	if !sc.AllBuildings {
		return Year{}, store.ErrNotFound
	}
	in := YearInputs{Year: year, Production: map[uuid.UUID]map[int]decimal.Decimal{}, FeedIn: map[uuid.UUID]map[int]Price{}}
	scope := model.BillScopeBuilding
	for m := 1; m <= 12; m++ {
		key := fmt.Sprintf("%04d-%02d", year, m)
		for offset := int32(0); ; offset += pageSize {
			page, err := s.d.Bills.List(ctx, sc, store.BillFilter{BillScope: &scope, PeriodKey: &key, Page: store.Page{Limit: pageSize, Offset: offset}})
			if err != nil {
				return Year{}, err
			}
			in.Bills = append(in.Bills, page...)
			if len(page) < pageSize {
				break
			}
		}
	}
	plants, err := s.plants(ctx, sc)
	if err != nil {
		return Year{}, err
	}
	if len(plants) == 0 {
		return BuildYear(in), nil
	}
	ids := make([]uuid.UUID, len(plants))
	for i, p := range plants {
		ids[i] = p.ID
	}
	from := time.Date(year, 1, 1, 0, 0, 0, 0, istanbul)
	buckets, err := s.d.Analytics.ProductionDaily(ctx, sc, ids, store.TimeRange{From: from, To: from.AddDate(1, 0, 0)})
	if err != nil {
		return Year{}, err
	}
	for _, b := range buckets {
		if b.ProductionKwh == nil {
			continue
		}
		m := int(b.Bucket.In(istanbul).Month())
		if in.Production[b.PlantID] == nil {
			in.Production[b.PlantID] = map[int]decimal.Decimal{}
		}
		in.Production[b.PlantID][m] = in.Production[b.PlantID][m].Add(*b.ProductionKwh)
	}
	for plant, months := range in.Production {
		in.FeedIn[plant] = map[int]Price{}
		for m := range months {
			t, err := s.d.Solar.Effective(ctx, sc, plant, time.Date(year, time.Month(m), 1, 0, 0, 0, 0, istanbul))
			if errors.Is(err, store.ErrNotFound) {
				continue
			}
			if err != nil {
				return Year{}, err
			}
			in.FeedIn[plant][m] = Price{Value: t.FeedInTariff, Currency: t.Currency}
		}
	}
	return BuildYear(in), nil
}

// Summary is the headline for a year or one of its months.
func (s *Service) Summary(ctx context.Context, sc store.Scope, year int, month *int) (Summary, error) {
	y, err := s.Year(ctx, sc, year)
	if err != nil {
		return Summary{}, err
	}
	out := Summary{Period: Period{Year: year}, Figures: y.Total, WithData: y.ConsumptionMonths, Of: 12}
	if month != nil {
		out.Period.Month = *month
		out.Figures, out.Of, out.WithData = y.Months[*month-1], 1, 0
		if out.Figures.ConsumptionKwh != nil {
			out.WithData = 1
		}
	}
	analyzers, err := s.analyzerCount(ctx, sc)
	if err != nil {
		return Summary{}, err
	}
	plants, err := s.plants(ctx, sc)
	if err != nil {
		return Summary{}, err
	}
	out.AnalyzerCount, out.PlantCount = analyzers, len(plants)
	out.Tariffs, err = s.tariffPanel(ctx, sc, plants)
	return out, err
}

func (s *Service) tariffPanel(ctx context.Context, sc store.Scope, plants []model.PowerPlant) (TariffPanel, error) {
	today := s.d.Clock.Now().In(istanbul)
	panel := TariffPanel{Purchase: []BuildingTariff{}, Sale: []PlantFeedIn{}}
	for offset := int32(0); ; offset += pageSize {
		page, err := s.d.Buildings.List(ctx, sc, store.BuildingFilter{Page: store.Page{Limit: pageSize, Offset: offset}})
		if err != nil {
			return TariffPanel{}, err
		}
		for _, b := range page {
			t, err := s.d.Tariffs.Effective(ctx, sc, b.ID, today)
			if errors.Is(err, store.ErrNotFound) {
				continue
			}
			if err != nil {
				return TariffPanel{}, err
			}
			panel.Purchase = append(panel.Purchase, BuildingTariff{BuildingID: b.ID, BuildingName: b.Name, PriceType: t.PriceType,
				Single: t.SingleTimePrice, T1: t.T1Price, T2: t.T2Price, T3: t.T3Price, Currency: t.Currency})
		}
		if len(page) < pageSize {
			break
		}
	}
	for _, p := range plants {
		t, err := s.d.Solar.Effective(ctx, sc, p.ID, today)
		if errors.Is(err, store.ErrNotFound) {
			continue
		}
		if err != nil {
			return TariffPanel{}, err
		}
		panel.Sale = append(panel.Sale, PlantFeedIn{PlantID: p.ID, PlantName: p.Name, Price: Price{Value: t.FeedInTariff, Currency: t.Currency}})
	}
	panel.PurchaseMissing, panel.SaleMissing = len(panel.Purchase) == 0, len(panel.Sale) == 0
	return panel, nil
}

func (s *Service) plants(ctx context.Context, sc store.Scope) ([]model.PowerPlant, error) {
	var out []model.PowerPlant
	for offset := int32(0); ; offset += pageSize {
		page, err := s.d.Plants.List(ctx, sc, store.PlantFilter{Page: store.Page{Limit: pageSize, Offset: offset}})
		if err != nil {
			return nil, err
		}
		out = append(out, page...)
		if len(page) < pageSize {
			return out, nil
		}
	}
}

func (s *Service) analyzerCount(ctx context.Context, sc store.Scope) (int, error) {
	n := 0
	for offset := int32(0); ; offset += pageSize {
		page, err := s.d.Analyzers.List(ctx, sc, store.AnalyzerFilter{Page: store.Page{Limit: pageSize, Offset: offset}})
		if err != nil {
			return 0, err
		}
		n += len(page)
		if len(page) < pageSize {
			return n, nil
		}
	}
}
