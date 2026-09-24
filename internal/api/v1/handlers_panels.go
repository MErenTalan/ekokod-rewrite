package v1

import (
	"net/http"

	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/dto"
	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/mw"
	"github.com/MErenTalan/ekokod-rewrite/internal/auth"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/financial"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/renewable"
)

// panelRoutes is 05 §9's renewable panels (R292, all roles, scope-bound) and
// the financial analysis (R294, company-level).
func panelRoutes() []Route {
	all := auth.AllRoles
	acacr := auth.Roles(roleA, roleCA, roleCR)
	panel := func(name, id, summary string, response any, build func(renewable.Inputs) any) Route {
		return Route{Method: http.MethodGet, Pattern: "/renewable/" + name, OperationID: "renewable." + id, Tag: "renewable",
			Access: RoleGated, Roles: all, Summary: summary, Request: dto.RenewableQuery{}, Response: response, Status: http.StatusOK,
			Handler: renewablePanel(build)}
	}
	return []Route{
		panel("overview", "overview", "Generation totals over the range from the export registers.", dto.RenewableOverview{},
			func(in renewable.Inputs) any { return overviewDTO(renewable.BuildOverview(in)) }),
		panel("realtime", "realtime", "The last complete hour's generation and the last 24 hours.", dto.RenewableRealtime{},
			func(in renewable.Inputs) any { return realtimePanelDTO(renewable.BuildRealtime(in)) }),
		panel("grid-interaction", "grid_interaction", "Today's import and export, power factor and the latest bill's prices.",
			dto.RenewableGridInteraction{}, func(in renewable.Inputs) any { return gridDTO(renewable.BuildGridInteraction(in)) }),
		panel("environmental", "environmental", "CO₂ avoided at the grid factor and seeded equivalences with their sources.",
			dto.RenewableEnvironmental{}, func(in renewable.Inputs) any { return environmentalDTO(renewable.BuildEnvironmental(in)) }),
		panel("efficiency", "efficiency", "Efficiency figures; each states why it is unavailable.", dto.RenewableEfficiency{},
			func(in renewable.Inputs) any { return efficiencyDTO(renewable.BuildEfficiency(in)) }),
		panel("forecast", "forecast", "Consumption forecast sums and accuracy from stored forecast runs.", dto.RenewableForecast{},
			func(in renewable.Inputs) any { return forecastDTO(renewable.BuildForecast(in)) }),
		panel("analytics", "analytics", "Peaks, trend, data availability and the financial gains from bills.", dto.RenewableAnalytics{},
			func(in renewable.Inputs) any { return analyticsDTO(renewable.BuildAnalytics(in)) }),
		panel("system-status", "system_status", "Monitoring and grid connection health from the last reading.", dto.RenewableSystemStatus{},
			func(in renewable.Inputs) any { return systemStatusDTO(renewable.BuildSystemStatus(in)) }),
		{Method: http.MethodGet, Pattern: "/financial/summary", OperationID: "financial.summary", Tag: "financial", Access: RoleGated,
			Roles: acacr, Summary: "Company-wide consumption, cost, production, revenue and net for a year or month, with the tariffs in force.",
			Request: dto.FinancialSummaryRequest{}, Response: dto.FinancialSummary{}, Status: http.StatusOK, Handler: (*Handlers).financialSummary},
		{Method: http.MethodGet, Pattern: "/financial/monthly", OperationID: "financial.monthly", Tag: "financial", Access: RoleGated,
			Roles: acacr, Summary: "Twelve months and the year row; a month without data is null, never zero.",
			Request: dto.FinancialMonthlyRequest{}, Response: dto.FinancialMonthly{}, Status: http.StatusOK, Handler: (*Handlers).financialMonthly},
	}
}

func renewablePanel(build func(renewable.Inputs) any) func(*Handlers, http.ResponseWriter, *http.Request) {
	return func(h *Handlers, w http.ResponseWriter, r *http.Request) {
		serve(w, r, http.StatusOK, func(q dto.RenewableQuery) (any, error) {
			in, err := h.Renewable.Load(r.Context(), mw.ScopeFrom(r), renewable.Query{AnalyzerID: q.AnalyzerID, BuildingID: q.BuildingID,
				From: q.From.Time, To: q.To.AddDate(0, 0, 1)})
			if err != nil {
				return nil, err
			}
			return build(in), nil
		})
	}
}

func points(ps []renewable.Point) []dto.RenewablePoint {
	out := make([]dto.RenewablePoint, len(ps))
	for i, p := range ps {
		out[i] = dto.RenewablePoint{Ts: dto.T(p.Ts), Kwh: dto.DP(p.Kwh)}
	}
	return out
}

func overviewDTO(v renewable.Overview) dto.RenewableOverview {
	return dto.RenewableOverview{ActiveGenerationKwh: dto.DP(v.ActiveGenerationKwh), InductiveGenerationKvarh: dto.DP(v.InductiveGenerationKvarh),
		CapacitiveGenerationKvarh: dto.DP(v.CapacitiveGenerationKvarh), AverageGenerationKwh: dto.DP(v.AverageGenerationKwh),
		Unavailable: dto.Unavailable(v.Unavailable)}
}

func realtimePanelDTO(v renewable.Realtime) dto.RenewableRealtime {
	return dto.RenewableRealtime{CurrentPowerKw: dto.DP(v.CurrentPowerKw), TodayKwh: dto.DP(v.TodayKwh), MaxPowerKw: dto.DP(v.MaxPowerKw),
		AvgPowerKw: dto.DP(v.AvgPowerKw), Status: v.Status, Series24h: points(v.Series24h), SystemEfficiencyPct: dto.DP(v.SystemEfficiencyPct),
		Unavailable: dto.Unavailable(v.Unavailable)}
}

func gridDTO(v renewable.GridInteraction) dto.RenewableGridInteraction {
	return dto.RenewableGridInteraction{Direction: v.Direction, TodayImportKwh: dto.DP(v.TodayImportKwh), TodayExportKwh: dto.DP(v.TodayExportKwh),
		PowerFactor: dto.DP(v.PowerFactor), VoltageV: dto.DP(v.VoltageV), FrequencyHz: dto.DP(v.FrequencyHz), ImportPrice: dto.DP(v.ImportPrice),
		ExportPrice: dto.DP(v.ExportPrice), Currency: v.Currency, NetToday: dto.DP(v.NetToday), Unavailable: dto.Unavailable(v.Unavailable)}
}

func environmentalDTO(v renewable.Environmental) dto.RenewableEnvironmental {
	factors := make([]dto.EquivalenceFactor, len(v.Factors))
	for i, f := range v.Factors {
		factors[i] = dto.EquivalenceFactor{Key: f.Key, Factor: dto.D(f.Factor), Unit: f.Unit, Source: f.Source, Year: f.Year}
	}
	return dto.RenewableEnvironmental{GenerationKwh: dto.DP(v.GenerationKwh), Co2AvoidedKg: dto.DP(v.Co2AvoidedKg), Trees: dto.DP(v.Trees),
		CoalKg: dto.DP(v.CoalKg), CarKm: dto.DP(v.CarKm), Homes: dto.DP(v.Homes), GridFactor: dto.DP(v.GridFactor),
		GridFactorUnit: v.GridFactorUnit, GridFactorSource: v.GridFactorSource, Factors: factors, Unavailable: dto.Unavailable(v.Unavailable)}
}

func efficiencyDTO(v renewable.Efficiency) dto.RenewableEfficiency {
	recs := v.Recommendations
	if recs == nil {
		recs = []string{}
	}
	return dto.RenewableEfficiency{OverallPct: dto.DP(v.OverallPct), PanelPct: dto.DP(v.PanelPct), InverterPct: dto.DP(v.InverterPct),
		BatteryPct: dto.DP(v.BatteryPct), GridPct: dto.DP(v.GridPct), Trend: points(v.Trend), Recommendations: recs,
		Unavailable: dto.Unavailable(v.Unavailable)}
}

func forecastDTO(v renewable.Forecast) dto.RenewableForecast {
	return dto.RenewableForecast{ConsumptionNext24hKwh: dto.DP(v.ConsumptionNext24hKwh), ConsumptionNext7dKwh: dto.DP(v.ConsumptionNext7dKwh),
		ConsumptionNext28dKwh: dto.DP(v.ConsumptionNext28dKwh), AccuracyDailyPct: dto.DP(v.AccuracyDailyPct),
		AccuracyWeeklyPct: dto.DP(v.AccuracyWeeklyPct), AccuracyOverallPct: dto.DP(v.AccuracyOverallPct),
		EstimatedGenerationKwh: dto.DP(v.EstimatedGenerationKwh), NetExcessKwh: dto.DP(v.NetExcessKwh), WeatherImpact: v.WeatherImpact,
		Unavailable: dto.Unavailable(v.Unavailable)}
}

func analyticsDTO(v renewable.Analytics) dto.RenewableAnalytics {
	f := v.Financial
	return dto.RenewableAnalytics{PeakGenerationKwh: dto.DP(v.PeakGenerationKwh), PeakGenerationAt: dto.TP(v.PeakGenerationAt),
		AverageGenerationKwh: dto.DP(v.AverageGenerationKwh), Trend: points(v.Trend), PeakHour: v.PeakHour,
		DataAvailabilityPct: dto.DP(v.DataAvailabilityPct), SystemEfficiencyPct: dto.DP(v.SystemEfficiencyPct),
		EfficiencyChange30dPct: dto.DP(v.EfficiencyChange30dPct), ConsumptionOptimisation: v.ConsumptionOptimisation,
		MaintenanceRequired: v.MaintenanceRequired, Unavailable: dto.Unavailable(v.Unavailable),
		Financial: dto.RenewableFinancial{ImportPrice: dto.DP(f.ImportPrice), ExportPrice: dto.DP(f.ExportPrice), Currency: f.Currency,
			TodayImportCost: dto.DP(f.TodayImportCost), TodayExportRevenue: dto.DP(f.TodayExportRevenue), NetToday: dto.DP(f.NetToday),
			MonthEarnings: dto.DP(f.MonthEarnings), YearEarnings: dto.DP(f.YearEarnings), TotalSavings: dto.DP(f.TotalSavings),
			RoiPct: dto.DP(f.RoiPct), PaybackYears: dto.DP(f.PaybackYears), BillSavings: dto.DP(f.BillSavings),
			Unavailable: dto.Unavailable(f.Unavailable)}}
}

func systemStatusDTO(v renewable.SystemStatus) dto.RenewableSystemStatus {
	return dto.RenewableSystemStatus{Overall: v.Overall, Monitoring: v.Monitoring, GridConnection: v.GridConnection,
		LastReadingAt: dto.TP(v.LastReadingAt), SolarPanels: v.SolarPanels, Inverter: v.Inverter, Battery: v.Battery, Security: v.Security,
		TotalGenerationKwh: dto.DP(v.TotalGenerationKwh), AverageEfficiencyPct: dto.DP(v.AverageEfficiencyPct),
		Unavailable: dto.Unavailable(v.Unavailable)}
}

func moneyList(list []financial.Money) []dto.MoneyAmount {
	out := make([]dto.MoneyAmount, len(list))
	for i, m := range list {
		out[i] = dto.MoneyAmount{Currency: string(m.Currency), Amount: dto.D(m.Amount)}
	}
	return out
}

func financialMonthDTO(m financial.Month) dto.FinancialMonth {
	return dto.FinancialMonth{Month: m.Month, ConsumptionKwh: dto.DP(m.ConsumptionKwh), Cost: moneyList(m.Cost),
		ProductionKwh: dto.DP(m.ProductionKwh), Revenue: moneyList(m.Revenue), RevenuePartial: m.RevenuePartial,
		OffsetKwh: dto.DP(m.OffsetKwh), GridPurchaseKwh: dto.DP(m.GridPurchaseKwh), GridSaleKwh: dto.DP(m.GridSaleKwh), Net: moneyList(m.Net)}
}

func (h *Handlers) financialMonthly(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(q dto.FinancialMonthlyRequest) (any, error) {
		y, err := h.Financial.Year(r.Context(), mw.ScopeFrom(r), q.Year)
		if err != nil {
			return nil, err
		}
		out := dto.FinancialMonthly{Items: make([]dto.FinancialMonth, 12), Total: financialMonthDTO(y.Total),
			Coverage: dto.FinancialCoverage{ConsumptionMonths: y.ConsumptionMonths, ProductionMonths: y.ProductionMonths, Of: 12}}
		for i, m := range y.Months {
			out.Items[i] = financialMonthDTO(m)
		}
		return out, nil
	})
}

func (h *Handlers) financialSummary(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(q dto.FinancialSummaryRequest) (any, error) {
		s, err := h.Financial.Summary(r.Context(), mw.ScopeFrom(r), q.Year, q.Month)
		if err != nil {
			return nil, err
		}
		out := dto.FinancialSummary{Year: s.Period.Year, Month: q.Month, AnalyzerCount: s.AnalyzerCount, PlantCount: s.PlantCount,
			Figures: financialMonthDTO(s.Figures), WithData: s.WithData, Of: s.Of,
			Tariffs: dto.FinancialTariffs{Purchase: make([]dto.FinancialBuildingTariff, len(s.Tariffs.Purchase)),
				Sale: make([]dto.FinancialPlantFeedIn, len(s.Tariffs.Sale)), PurchaseMissing: s.Tariffs.PurchaseMissing,
				SaleMissing: s.Tariffs.SaleMissing}}
		for i, t := range s.Tariffs.Purchase {
			out.Tariffs.Purchase[i] = dto.FinancialBuildingTariff{BuildingID: t.BuildingID, BuildingName: t.BuildingName,
				PriceType: string(t.PriceType), Single: dto.DP(t.Single), T1: dto.DP(t.T1), T2: dto.DP(t.T2), T3: dto.DP(t.T3),
				Currency: string(t.Currency)}
		}
		for i, p := range s.Tariffs.Sale {
			out.Tariffs.Sale[i] = dto.FinancialPlantFeedIn{PlantID: p.PlantID, PlantName: p.PlantName, Price: dto.D(p.Price.Value),
				Currency: string(p.Price.Currency)}
		}
		return out, nil
	})
}
