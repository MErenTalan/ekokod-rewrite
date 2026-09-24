package reportpdf

import (
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/report"
)

func dp(s string) *decimal.Decimal {
	v := decimal.RequireFromString(s)
	return &v
}

func every(v string) report.MonthSeries {
	var s report.MonthSeries
	for i := range s {
		s[i] = dp(v)
	}
	return s
}

var (
	buildingID = uuid.MustParse("11111111-2222-3333-4444-555555555555")
	plantID    = uuid.MustParse("66666666-7777-8888-9999-aaaaaaaaaaaa")
)

func monthlyPayload() report.Payload {
	var bills [12]*report.Bill
	for i := range bills {
		bills[i] = &report.Bill{Currency: "TRY", Total: *dp("48250.75"), EnergyCost: *dp("31800.25"), NetConsumption: *dp("18450.5"), ReactivePenalty: *dp("120")}
	}
	m := report.BuildMonthly(report.MonthlyInput{Year: 2026, Month: 3, Selection: report.SelectionAll,
		Buildings: []report.BuildingInput{{ID: buildingID, Name: "ĞÜŞİÖÇ Merkez",
			Consumption: map[int]report.MonthSeries{2026: every("18450.5"), 2025: every("17000")},
			Export:      map[int]report.MonthSeries{2026: every("320")},
			Bills:       map[int][12]*report.Bill{2026: bills}}},
		Plants: []report.PlantInput{{ID: plantID, Name: "Arazi GES", Kind: report.KindGrid, FeedIn: dp("2.1"),
			Production: map[int]report.MonthSeries{2026: every("9000")}}}})
	return report.Payload{Version: 1, Type: report.TypeMonthly, Period: "2026-03", Monthly: &m}
}

func yearlyPayload() report.Payload {
	year := 2022
	y := report.BuildYearly(report.YearlyInput{Year: 2025, Selection: report.SelectionAll,
		Buildings: []report.BuildingInput{{ID: buildingID, Name: "ĞÜŞİÖÇ Merkez",
			Consumption: map[int]report.MonthSeries{2025: every("18000"), 2024: every("17000")},
			Export:      map[int]report.MonthSeries{2025: every("300")}}},
		Plants: []report.PlantInput{{ID: plantID, Name: "Arazi GES", Kind: report.KindGrid, YearlyTarget: dp("120000"),
			Production: map[int]report.MonthSeries{2025: every("9000")}}},
		Factor: &report.GridFactor{Value: *dp("0.45"), Unit: "kg CO2e/kWh", SourceYear: &year}})
	return report.Payload{Version: 1, Type: report.TypeYearly, Period: "2025", Yearly: &y}
}
