package renewable_test

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/renewable"
)

var ist = func() *time.Location { l, _ := time.LoadLocation("Europe/Istanbul"); return l }()

// now is 12:30 Istanbul on 10 March 2026; the last complete hour is 11:00–12:00.
var now = time.Date(2026, time.March, 10, 12, 30, 0, 0, ist)

func dp(s string) *decimal.Decimal { v := decimal.RequireFromString(s); return &v }

var analyzer = uuid.MustParse("11111111-1111-1111-1111-111111111111")

func bucket(ts time.Time, imp, gen, ind string) model.ConsumptionBucket {
	return model.ConsumptionBucket{AnalyzerID: analyzer, Bucket: ts, ActiveConsumption: dp(imp), ActiveGeneration: dp(gen),
		InductiveConsumption: dp(ind), InductiveGeneration: dp("0.5"), CapacitiveGeneration: dp("0.25"), ReadingCount: 4}
}

// fixture builds inputs whose every register is scaled by k and shifted by s
// over `hours` hourly buckets, so two different fixtures differ in every figure.
func fixture(k, s string, hours int) renewable.Inputs {
	K, S := decimal.RequireFromString(k), decimal.RequireFromString(s)
	v := func(base string) string { return decimal.RequireFromString(base).Mul(K).Add(S).String() }
	var hourly []model.ConsumptionBucket
	for h := 0; h < hours; h++ {
		ts := now.Truncate(time.Hour).Add(-time.Duration(h+1) * time.Hour)
		gen := "0"
		if l := ts.In(ist).Hour(); l >= 8 && l <= 17 {
			gen = decimal.NewFromInt(int64(l % 7)).Add(decimal.NewFromInt(1)).String()
		}
		b := bucket(ts, v("3"), v(gen), v("1.2"))
		b.InductiveGeneration, b.CapacitiveGeneration = dp(v("0.5")), dp(v("0.25"))
		hourly = append(hourly, b)
	}
	var daily []model.ConsumptionBucket
	for d := 0; d < 9; d++ {
		b := bucket(time.Date(2026, 3, 1+d, 0, 0, 0, 0, ist), v(decimal.NewFromInt(int64(40+d)).String()), v(decimal.NewFromInt(int64(20+2*d)).String()), v("9"))
		b.InductiveGeneration, b.CapacitiveGeneration = dp(v("0.5")), dp(v("0.25"))
		daily = append(daily, b)
	}
	price, gprice := dp(v("2.5")), dp(v("1.8"))
	bill := model.Bill{ID: uuid.New(), AnalyzerID: &analyzer, Scope: model.BillScopeAnalyzer, PeriodKey: "2026-02", Currency: model.CurrencyTRY,
		EffectiveEnergyPrice: price, GenerationPricePerKwh: gprice, GenerationCredit: decimal.RequireFromString(v("310")),
		ActiveExport: decimal.RequireFromString(v("172"))}
	march := bill
	march.ID, march.PeriodKey, march.GenerationCredit = uuid.New(), "2026-03", decimal.RequireFromString(v("120"))
	var forecasts []model.Forecast
	for h := -48; h < 24*7; h++ {
		ts := now.Truncate(time.Hour).Add(time.Duration(h) * time.Hour)
		forecasts = append(forecasts, model.Forecast{AnalyzerID: analyzer, Ts: ts, GeneratedAt: ts.Add(-24 * time.Hour), Median: decimal.RequireFromString(v("2.9"))})
	}
	last := now.Add(-40 * time.Minute)
	return renewable.Inputs{
		Now: now, From: time.Date(2026, 3, 1, 0, 0, 0, 0, ist), To: time.Date(2026, 3, 11, 0, 0, 0, 0, ist),
		Hourly: hourly, Daily: daily, Bills: []model.Bill{bill, march}, LatestBill: &march, LastReadingAt: &last, Forecasts: forecasts,
		GridFactor: &renewable.Factor{Value: decimal.RequireFromString(v("0.44")), Unit: "kg CO2e/kWh", Source: "TR grid"},
		Equivalences: map[string]renewable.Factor{
			renewable.EquivTree: {Value: decimal.RequireFromString(v("21.77")), Unit: "kg CO2/tree-year"},
			renewable.EquivCoal: {Value: decimal.RequireFromString(v("0.404")), Unit: "kg/kWh"},
			renewable.EquivCar:  {Value: decimal.RequireFromString(v("0.12")), Unit: "kg CO2/km"},
			renewable.EquivHome: {Value: decimal.RequireFromString(v("12000")), Unit: "kWh/home-year"},
		},
	}
}
