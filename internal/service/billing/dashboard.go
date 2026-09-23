package billing

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	domain "github.com/MErenTalan/ekokod-rewrite/internal/domain/billing"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/render/dashboardpdf"
	"github.com/MErenTalan/ekokod-rewrite/internal/render/dashboardxlsx"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// DashboardInput selects the month, as 05 §7's own query parameters do.
type DashboardInput struct {
	Year  int
	Month int
}

// Dashboard is R233: one read of the period's bills in scope, then arithmetic.
// No consumption or generation series joins in — a bill's period follows the
// building's cutoff day, so a calendar-month series would silently disagree
// with the invoice sitting next to it.
func (s *Service) Dashboard(ctx context.Context, sc store.Scope, in DashboardInput) (domain.DashboardResult, error) {
	period := fmt.Sprintf("%04d-%02d", in.Year, in.Month)

	analyzerBills, err := s.billsFor(ctx, sc, period, model.BillScopeAnalyzer)
	if err != nil {
		return domain.DashboardResult{}, err
	}
	buildingBills, err := s.billsFor(ctx, sc, period, model.BillScopeBuilding)
	if err != nil {
		return domain.DashboardResult{}, err
	}
	companyBills, err := s.billsFor(ctx, sc, period, model.BillScopeCompany)
	if err != nil {
		return domain.DashboardResult{}, err
	}

	names, err := s.dashboardNames(ctx, sc)
	if err != nil {
		return domain.DashboardResult{}, err
	}
	rows := func(bills []model.Bill) []domain.DashboardRow {
		out := make([]domain.DashboardRow, 0, len(bills))
		for _, b := range bills {
			out = append(out, names.row(b))
		}
		return out
	}
	return domain.BuildDashboard(rows(analyzerBills), rows(buildingBills), rows(companyBills)), nil
}

func (s *Service) billsFor(ctx context.Context, sc store.Scope, period string, scope model.BillScope) ([]model.Bill, error) {
	var out []model.Bill
	for offset := int32(0); ; offset += dashboardPage {
		page, err := s.deps.Bills.List(ctx, sc, store.BillFilter{PeriodKey: &period, BillScope: &scope,
			Page: store.Page{Limit: dashboardPage, Offset: offset}})
		if err != nil {
			return nil, err
		}
		out = append(out, page...)
		if len(page) < dashboardPage {
			return out, nil
		}
	}
}

const dashboardPage = 500

// dashboardLabels resolves the names the table prints. A bill whose building
// or analyzer was soft-deleted after it was issued keeps its row with an empty
// name rather than disappearing: the invoice was still issued.
type dashboardLabels struct {
	buildings map[uuid.UUID]string
	analyzers map[uuid.UUID]model.Analyzer
}

func (s *Service) dashboardNames(ctx context.Context, sc store.Scope) (dashboardLabels, error) {
	out := dashboardLabels{buildings: map[uuid.UUID]string{}, analyzers: map[uuid.UUID]model.Analyzer{}}
	for offset := int32(0); ; offset += dashboardPage {
		page, err := s.deps.Buildings.List(ctx, sc, store.BuildingFilter{Page: store.Page{Limit: dashboardPage, Offset: offset}})
		if err != nil {
			return dashboardLabels{}, err
		}
		for _, b := range page {
			out.buildings[b.ID] = b.Name
		}
		if len(page) < dashboardPage {
			break
		}
	}
	for offset := int32(0); ; offset += dashboardPage {
		page, err := s.deps.Analyzers.List(ctx, sc, store.AnalyzerFilter{Page: store.Page{Limit: dashboardPage, Offset: offset}})
		if err != nil {
			return dashboardLabels{}, err
		}
		for _, a := range page {
			out.analyzers[a.ID] = a
		}
		if len(page) < dashboardPage {
			break
		}
	}
	return out, nil
}

func (l dashboardLabels) row(b model.Bill) domain.DashboardRow {
	row := domain.DashboardRow{
		BillID: b.ID, BuildingID: b.BuildingID, AnalyzerID: b.AnalyzerID, PeriodKey: b.PeriodKey,
		Consumption: b.ActiveImport, Production: b.ActiveExport, ConsumptionPrice: b.EffectiveEnergyPrice,
		ProductionPrice: b.GenerationPricePerKwh, Invoice: b.TotalCost, Currency: b.Currency,
		Superseded: b.Status == model.BillStatusSuperseded,
	}
	if b.BuildingID != nil {
		row.BuildingName = l.buildings[*b.BuildingID]
	}
	if b.AnalyzerID != nil {
		if a, ok := l.analyzers[*b.AnalyzerID]; ok {
			row.InstallationNumber = a.InstallationNumber
			row.AnalyzerName = a.InstallationNumber
			if a.MeteringPointName != nil && *a.MeteringPointName != "" {
				row.AnalyzerName = *a.MeteringPointName
			} else if a.CustomerName != nil && *a.CustomerName != "" {
				row.AnalyzerName = *a.CustomerName
			}
			if a.EtsoCode != nil {
				row.EtsoCode = *a.EtsoCode
			}
		}
	}
	return row
}

// Dashboard is Service.Dashboard plus the plant section, the one aggregate the
// screen and both exports read (R239, R290).
func (q Requests) Dashboard(ctx context.Context, sc store.Scope, in DashboardInput) (domain.DashboardResult, error) {
	res, err := q.Service.Dashboard(ctx, sc, in)
	if err != nil || q.Plants == nil {
		return res, err
	}
	res.Plants, err = q.Plants.BillPlants(ctx, sc, in.Year, in.Month)
	return res, err
}

// DashboardExport renders the same aggregate the screen shows (R239), so the
// file and the page cannot disagree. It returns the body and the file name.
func (q Requests) DashboardExport(ctx context.Context, sc store.Scope, in DashboardInput, format, locale string) ([]byte, string, error) {
	res, err := q.Dashboard(ctx, sc, in)
	if err != nil {
		return nil, "", err
	}
	period := fmt.Sprintf("%04d-%02d", in.Year, in.Month)
	if format == "pdf" {
		company, err := q.Renderer.Companies.Get(ctx, sc, sc.CompanyID)
		if err != nil {
			return nil, "", err
		}
		body, err := dashboardpdf.Render(dashboardpdf.Document{Result: res, Period: period, CompanyName: company.Name,
			Locale: locale, GeneratedAt: q.Service.deps.Clock.Now().UTC()})
		return body, "fatura-panosu-" + period + ".pdf", err
	}
	body, err := dashboardxlsx.Render(res, period, locale)
	return body, "fatura-panosu-" + period + ".xlsx", err
}
