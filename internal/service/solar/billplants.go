package solar

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	domain "github.com/MErenTalan/ekokod-rewrite/internal/domain/billing"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// BillPlants is the bills dashboard's plant section (R290): company-level
// only, one row per plant for the Istanbul calendar month.
func (s *Service) BillPlants(ctx context.Context, sc store.Scope, year, month int) (domain.DashboardPlants, error) {
	if !sc.AllBuildings {
		return domain.DashboardPlants{Reason: "plants_not_in_scope"}, nil
	}
	var plants []model.PowerPlant
	for offset := int32(0); ; offset += 500 {
		page, err := s.d.Plants.List(ctx, sc, store.PlantFilter{Page: store.Page{Limit: 500, Offset: offset}})
		if err != nil {
			return domain.DashboardPlants{}, err
		}
		plants = append(plants, page...)
		if len(page) < 500 {
			break
		}
	}
	out := domain.DashboardPlants{Available: true, Rows: []domain.DashboardPlantRow{}}
	if len(plants) == 0 {
		return out, nil
	}
	from := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, istanbul)
	ids := make([]uuid.UUID, len(plants))
	for i, p := range plants {
		ids[i] = p.ID
	}
	// The real-time daily aggregate: the open month is whole, never lagging.
	buckets, err := s.d.Analytics.ProductionDaily(ctx, sc, ids, store.TimeRange{From: from, To: from.AddDate(0, 1, 0)})
	if err != nil {
		return domain.DashboardPlants{}, err
	}
	production := map[uuid.UUID]*decimal.Decimal{}
	for _, b := range buckets {
		production[b.PlantID] = addPtr(production[b.PlantID], b.ProductionKwh)
	}
	period := fmt.Sprintf("%04d-%02d", year, month)
	invoices := map[model.CurrencyCode]decimal.Decimal{}
	for _, p := range plants {
		row := domain.DashboardPlantRow{PlantID: p.ID, PlantName: p.Name, ProductionKwh: production[p.ID]}
		switch t, err := s.d.Solar.Effective(ctx, sc, p.ID, from); {
		case err == nil:
			price, currency := t.FeedInTariff, t.Currency
			row.ProductionPrice, row.ProductionCurrency = &price, &currency
		case !errors.Is(err, store.ErrNotFound):
			return domain.DashboardPlants{}, err
		}
		if p.NettingAnalyzerID != nil {
			if err := s.nettingColumns(ctx, sc, *p.NettingAnalyzerID, period, &row); err != nil {
				return domain.DashboardPlants{}, err
			}
		}
		if row.ProductionKwh != nil && row.ProductionPrice != nil {
			amount := row.ProductionKwh.Mul(*row.ProductionPrice).Round(2)
			row.InvoiceAmount = &amount
			invoices[*row.ProductionCurrency] = invoices[*row.ProductionCurrency].Add(amount)
		}
		out.TotalProductionKwh = addPtr(out.TotalProductionKwh, row.ProductionKwh)
		out.Rows = append(out.Rows, row)
	}
	for _, c := range model.CurrencyCodes() {
		if v, ok := invoices[c]; ok {
			out.TotalInvoice = append(out.TotalInvoice, domain.DashboardMoney{Currency: c, Amount: v})
		}
	}
	return out, nil
}

// nettingColumns fills the analyzer's name, installation number and its
// current analyzer bill for the period (R289, R290).
func (s *Service) nettingColumns(ctx context.Context, sc store.Scope, analyzerID uuid.UUID, period string, row *domain.DashboardPlantRow) error {
	a, err := s.d.Analyzers.Get(ctx, sc, analyzerID)
	if errors.Is(err, store.ErrNotFound) {
		return nil // deleted since it was chosen: the row simply has no analyzer
	}
	if err != nil {
		return err
	}
	name, number := a.InstallationNumber, a.InstallationNumber
	if a.MeteringPointName != nil && *a.MeteringPointName != "" {
		name = *a.MeteringPointName
	} else if a.CustomerName != nil && *a.CustomerName != "" {
		name = *a.CustomerName
	}
	row.AnalyzerName, row.InstallationNumber = &name, &number
	bill, err := s.d.Bills.Current(ctx, sc, model.BillScopeAnalyzer, analyzerID, period)
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	row.ConsumptionPrice, row.BillID = bill.EffectiveEnergyPrice, &bill.ID
	return nil
}
