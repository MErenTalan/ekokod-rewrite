// Package renewable builds the renewable dashboard's panels (01 §7.8, R292).
// Every figure is a meter measurement, a documented formula over them, or
// null with a reason in Unavailable — never a constant or a random value
// (10 item 12). The builders are pure; service.go loads their Inputs.
package renewable

import (
	"math"
	"sort"
	"time"

	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
)

var istanbul = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		panic(err)
	}
	return loc
}()

// Equivalence factor keys, seeded as platform emission_factors rows (R293).
const (
	GridFactorKey = "grid_electricity_tr_2022"
	EquivTree     = "equiv_tree_co2_kg_per_year"
	EquivCoal     = "equiv_coal_kg_per_kwh"
	EquivCar      = "equiv_car_co2_kg_per_km"
	EquivHome     = "equiv_home_heating_kwh_per_year"
)

// Unavailable reasons (R292).
const (
	ReasonNoRecentData      = "no_recent_data"
	ReasonNoIrradiance      = "no_irradiance_or_capacity_measurement"
	ReasonNoBattery         = "no_battery_measurement"
	ReasonNoBill            = "no_bill"
	ReasonNoInvestment      = "no_investment_cost"
	ReasonNoSelfConsumption = "no_self_consumption_measurement"
	ReasonNoFactor          = "no_factor"
	ReasonNoTelemetry       = "no_component_telemetry"
	ReasonNoForecasts       = "no_forecasts"
	ReasonNoOverlap         = "no_overlap"
	ReasonNoGenForecast     = "no_generation_forecast"
	ReasonNoReactive        = "no_reactive_data"
	ReasonNoPowerQuality    = "no_power_quality_measurement"
	ReasonNoData            = "no_data"
)

// Factor is one emission or equivalence factor with its provenance.
type Factor struct {
	Value  decimal.Decimal
	Unit   string
	Source string
	Year   *int16
}

// Inputs is everything a panel may read: meter buckets, bills, the last
// reading time, forecasts and factors. Nothing else exists for a panel.
type Inputs struct {
	Now, From, To time.Time
	// Hourly covers [min(From, Now−30 d), Now); Daily covers [From, To).
	Hourly, Daily []model.ConsumptionBucket
	// Bills are the subject's non-superseded bills; LatestBill the newest.
	Bills         []model.Bill
	LatestBill    *model.Bill
	LastReadingAt *time.Time
	Forecasts     []model.Forecast
	GridFactor    *Factor
	Equivalences  map[string]Factor
}

// Point is one series value.
type Point struct {
	Ts  time.Time        `json:"ts"`
	Kwh *decimal.Decimal `json:"kwh"`
}

type reasons map[string]string

func (r reasons) mark(field, why string) { r[field] = why }

// sumBy sums pick over buckets per timestamp (several analyzers add up).
func sumBy(buckets []model.ConsumptionBucket, pick func(model.ConsumptionBucket) *decimal.Decimal, keep func(time.Time) bool) []Point {
	acc := map[int64]*Point{}
	for _, b := range buckets {
		if keep != nil && !keep(b.Bucket) {
			continue
		}
		v := pick(b)
		if v == nil {
			continue
		}
		p, ok := acc[b.Bucket.UnixNano()]
		if !ok {
			p = &Point{Ts: b.Bucket}
			acc[b.Bucket.UnixNano()] = p
		}
		p.Kwh = add(p.Kwh, v)
	}
	out := make([]Point, 0, len(acc))
	for _, p := range acc {
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Ts.Before(out[j].Ts) })
	return out
}

func add(a, b *decimal.Decimal) *decimal.Decimal {
	switch {
	case b == nil:
		return a
	case a == nil:
		v := *b
		return &v
	default:
		v := a.Add(*b)
		return &v
	}
}

func total(points []Point) *decimal.Decimal {
	var s *decimal.Decimal
	for _, p := range points {
		s = add(s, p.Kwh)
	}
	return s
}

func mean(points []Point, places int32) *decimal.Decimal {
	s := total(points)
	if s == nil {
		return nil
	}
	v := s.Div(decimal.NewFromInt(int64(len(points)))).Round(places)
	return &v
}

func maxOf(points []Point) *Point {
	var best *Point
	for i := range points {
		if points[i].Kwh != nil && (best == nil || points[i].Kwh.GreaterThan(*best.Kwh)) {
			best = &points[i]
		}
	}
	return best
}

func gen(b model.ConsumptionBucket) *decimal.Decimal { return b.ActiveGeneration }
func imp(b model.ConsumptionBucket) *decimal.Decimal { return b.ActiveConsumption }

func dayStart(t time.Time) time.Time {
	l := t.In(istanbul)
	return time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, istanbul)
}

func lastHour(now time.Time) time.Time { return now.Truncate(time.Hour).Add(-time.Hour) }

func within(from, to time.Time) func(time.Time) bool {
	return func(t time.Time) bool { return !t.Before(from) && t.Before(to) }
}

func mul(a *decimal.Decimal, b decimal.Decimal, places int32) *decimal.Decimal {
	if a == nil {
		return nil
	}
	v := a.Mul(b).Round(places)
	return &v
}

func div(a *decimal.Decimal, b decimal.Decimal, places int32) *decimal.Decimal {
	if a == nil || b.IsZero() {
		return nil
	}
	v := a.Div(b).Round(places)
	return &v
}

func str(s string) *string { return &s }

// Overview is the summary cards over the range (§7.8).
type Overview struct {
	ActiveGenerationKwh       *decimal.Decimal `json:"active_generation_kwh"`
	InductiveGenerationKvarh  *decimal.Decimal `json:"inductive_generation_kvarh"`
	CapacitiveGenerationKvarh *decimal.Decimal `json:"capacitive_generation_kvarh"`
	AverageGenerationKwh      *decimal.Decimal `json:"average_generation_kwh"`
	Unavailable               reasons          `json:"unavailable"`
}

// BuildOverview sums the export registers of the range's daily buckets.
func BuildOverview(in Inputs) Overview {
	daily := sumBy(in.Daily, gen, nil)
	return Overview{
		ActiveGenerationKwh:       total(daily),
		InductiveGenerationKvarh:  total(sumBy(in.Daily, func(b model.ConsumptionBucket) *decimal.Decimal { return b.InductiveGeneration }, nil)),
		CapacitiveGenerationKvarh: total(sumBy(in.Daily, func(b model.ConsumptionBucket) *decimal.Decimal { return b.CapacitiveGeneration }, nil)),
		AverageGenerationKwh:      mean(daily, 4),
		Unavailable:               reasons{},
	}
}

// Realtime is the real-time generation panel.
type Realtime struct {
	CurrentPowerKw      *decimal.Decimal `json:"current_power_kw"`
	TodayKwh            *decimal.Decimal `json:"today_kwh"`
	MaxPowerKw          *decimal.Decimal `json:"max_power_kw"`
	AvgPowerKw          *decimal.Decimal `json:"avg_power_kw"`
	Status              *string          `json:"status"`
	Series24h           []Point          `json:"series_24h"`
	SystemEfficiencyPct *decimal.Decimal `json:"system_efficiency_pct"`
	Unavailable         reasons          `json:"unavailable"`
}

// BuildRealtime: an hour's kWh is that hour's mean power in kW (R292).
func BuildRealtime(in Inputs) Realtime {
	r := Realtime{Unavailable: reasons{}}
	last := lastHour(in.Now)
	series := sumBy(in.Hourly, gen, within(last.Add(-23*time.Hour), last.Add(time.Hour)))
	r.Series24h = series
	if series == nil {
		r.Series24h = []Point{}
	}
	for _, p := range series {
		if p.Ts.Equal(last) {
			r.CurrentPowerKw = p.Kwh
		}
	}
	switch {
	case len(series) == 0:
		r.Status = str("no_data")
		r.Unavailable.mark("current_power_kw", ReasonNoRecentData)
	case r.CurrentPowerKw == nil:
		r.Status = str("no_data")
		r.Unavailable.mark("current_power_kw", ReasonNoRecentData)
	case r.CurrentPowerKw.IsPositive():
		r.Status = str("producing")
	default:
		r.Status = str("idle")
	}
	r.TodayKwh = total(sumBy(in.Hourly, gen, within(dayStart(in.Now), in.Now)))
	if m := maxOf(series); m != nil {
		r.MaxPowerKw = m.Kwh
	}
	r.AvgPowerKw = mean(series, 4)
	r.Unavailable.mark("system_efficiency_pct", ReasonNoIrradiance)
	return r
}

// GridInteraction is the grid panel.
type GridInteraction struct {
	Direction      *string          `json:"direction"`
	TodayImportKwh *decimal.Decimal `json:"today_import_kwh"`
	TodayExportKwh *decimal.Decimal `json:"today_export_kwh"`
	PowerFactor    *decimal.Decimal `json:"power_factor"`
	VoltageV       *decimal.Decimal `json:"voltage_v"`
	FrequencyHz    *decimal.Decimal `json:"frequency_hz"`
	ImportPrice    *decimal.Decimal `json:"import_price"`
	ExportPrice    *decimal.Decimal `json:"export_price"`
	Currency       *string          `json:"currency"`
	NetToday       *decimal.Decimal `json:"net_today"`
	Unavailable    reasons          `json:"unavailable"`
}

// BuildGridInteraction: direction from the last complete hour, PF from
// today's active and inductive energy (R292).
func BuildGridInteraction(in Inputs) GridInteraction {
	g := GridInteraction{Unavailable: reasons{}}
	last := lastHour(in.Now)
	lastImp := total(sumBy(in.Hourly, imp, within(last, last.Add(time.Hour))))
	lastGen := total(sumBy(in.Hourly, gen, within(last, last.Add(time.Hour))))
	switch {
	case lastImp == nil && lastGen == nil:
		g.Unavailable.mark("direction", ReasonNoRecentData)
	case lastGen != nil && (lastImp == nil || lastGen.GreaterThan(*lastImp)):
		g.Direction = str("export")
	case lastImp != nil && (lastGen == nil || lastImp.GreaterThan(*lastGen)):
		g.Direction = str("import")
	default:
		g.Direction = str("balanced")
	}
	today := within(dayStart(in.Now), in.Now)
	g.TodayImportKwh = total(sumBy(in.Hourly, imp, today))
	g.TodayExportKwh = total(sumBy(in.Hourly, gen, today))
	ind := total(sumBy(in.Hourly, func(b model.ConsumptionBucket) *decimal.Decimal { return b.InductiveConsumption }, today))
	if g.TodayImportKwh != nil && ind != nil && !(g.TodayImportKwh.IsZero() && ind.IsZero()) {
		p, q := g.TodayImportKwh.InexactFloat64(), ind.InexactFloat64()
		// PF = P / √(P² + Q²); the square root needs floats, the result is rounded to 3 dp.
		pf := decimal.NewFromFloat(p / math.Sqrt(p*p+q*q)).Round(3)
		g.PowerFactor = &pf
	} else {
		g.Unavailable.mark("power_factor", ReasonNoReactive)
	}
	g.Unavailable.mark("voltage_v", ReasonNoPowerQuality)
	g.Unavailable.mark("frequency_hz", ReasonNoPowerQuality)
	if b := in.LatestBill; b != nil && b.EffectiveEnergyPrice != nil && b.GenerationPricePerKwh != nil {
		g.ImportPrice, g.ExportPrice, g.Currency = b.EffectiveEnergyPrice, b.GenerationPricePerKwh, str(string(b.Currency))
		cost, revenue := mul(g.TodayImportKwh, *b.EffectiveEnergyPrice, 2), mul(g.TodayExportKwh, *b.GenerationPricePerKwh, 2)
		if cost != nil && revenue != nil {
			net := revenue.Sub(*cost)
			g.NetToday = &net
		}
	} else {
		for _, f := range []string{"import_price", "export_price", "currency", "net_today"} {
			g.Unavailable.mark(f, ReasonNoBill)
		}
	}
	return g
}

// Environmental is the environmental impact panel (R293).
type Environmental struct {
	GenerationKwh    *decimal.Decimal `json:"generation_kwh"`
	Co2AvoidedKg     *decimal.Decimal `json:"co2_avoided_kg"`
	Trees            *decimal.Decimal `json:"trees"`
	CoalKg           *decimal.Decimal `json:"coal_kg"`
	CarKm            *decimal.Decimal `json:"car_km"`
	Homes            *decimal.Decimal `json:"homes"`
	GridFactor       *decimal.Decimal `json:"grid_factor"`
	GridFactorUnit   *string          `json:"grid_factor_unit"`
	GridFactorSource *string          `json:"grid_factor_source"`
	Unavailable      reasons          `json:"unavailable"`
}

// BuildEnvironmental: CO₂ = kWh × grid factor; each equivalence divides or
// multiplies by its own seeded factor, or is unavailable without it.
func BuildEnvironmental(in Inputs) Environmental {
	e := Environmental{Unavailable: reasons{}, GenerationKwh: total(sumBy(in.Daily, gen, nil))}
	if in.GridFactor == nil {
		for _, f := range []string{"co2_avoided_kg", "trees", "car_km", "grid_factor", "grid_factor_unit", "grid_factor_source"} {
			e.Unavailable.mark(f, ReasonNoFactor)
		}
	} else {
		e.GridFactor, e.GridFactorUnit, e.GridFactorSource = &in.GridFactor.Value, str(in.GridFactor.Unit), str(in.GridFactor.Source)
		e.Co2AvoidedKg = mul(e.GenerationKwh, in.GridFactor.Value, 2)
	}
	eq := func(key, field string, f func(Factor) *decimal.Decimal) *decimal.Decimal {
		factor, ok := in.Equivalences[key]
		if !ok {
			e.Unavailable.mark(field, ReasonNoFactor)
			return nil
		}
		return f(factor)
	}
	if e.Co2AvoidedKg != nil {
		e.Trees = eq(EquivTree, "trees", func(f Factor) *decimal.Decimal { return div(e.Co2AvoidedKg, f.Value, 2) })
		e.CarKm = eq(EquivCar, "car_km", func(f Factor) *decimal.Decimal { return div(e.Co2AvoidedKg, f.Value, 1) })
	}
	e.CoalKg = eq(EquivCoal, "coal_kg", func(f Factor) *decimal.Decimal { return mul(e.GenerationKwh, f.Value, 2) })
	e.Homes = eq(EquivHome, "homes", func(f Factor) *decimal.Decimal { return div(e.GenerationKwh, f.Value, 4) })
	return e
}

// Efficiency is the efficiency panel: nothing here is measurable from a meter.
type Efficiency struct {
	OverallPct      *decimal.Decimal `json:"overall_pct"`
	PanelPct        *decimal.Decimal `json:"panel_pct"`
	InverterPct     *decimal.Decimal `json:"inverter_pct"`
	BatteryPct      *decimal.Decimal `json:"battery_pct"`
	GridPct         *decimal.Decimal `json:"grid_pct"`
	Trend           []Point          `json:"trend"`
	Recommendations []string         `json:"recommendations"`
	Unavailable     reasons          `json:"unavailable"`
}

// BuildEfficiency says why every efficiency figure is unavailable (R292).
func BuildEfficiency(Inputs) Efficiency {
	e := Efficiency{Unavailable: reasons{}}
	for _, f := range []string{"overall_pct", "panel_pct", "inverter_pct", "grid_pct", "trend", "recommendations"} {
		e.Unavailable.mark(f, ReasonNoIrradiance)
	}
	e.Unavailable.mark("battery_pct", ReasonNoBattery)
	return e
}

// Forecast is the forecast panel: consumption only; there is no generation model.
type Forecast struct {
	ConsumptionNext24hKwh  *decimal.Decimal `json:"consumption_next_24h_kwh"`
	ConsumptionNext7dKwh   *decimal.Decimal `json:"consumption_next_7d_kwh"`
	ConsumptionNext28dKwh  *decimal.Decimal `json:"consumption_next_28d_kwh"`
	AccuracyDailyPct       *decimal.Decimal `json:"accuracy_daily_pct"`
	AccuracyWeeklyPct      *decimal.Decimal `json:"accuracy_weekly_pct"`
	AccuracyOverallPct     *decimal.Decimal `json:"accuracy_overall_pct"`
	EstimatedGenerationKwh *decimal.Decimal `json:"estimated_generation_kwh"`
	NetExcessKwh           *decimal.Decimal `json:"net_excess_kwh"`
	WeatherImpact          *string          `json:"weather_impact"`
	Unavailable            reasons          `json:"unavailable"`
}

// latestBefore is, per hour, the median of the newest run generated at or before cutoff(ts).
func latestBefore(forecasts []model.Forecast, cutoff func(ts time.Time) time.Time) map[int64]decimal.Decimal {
	type pick struct {
		at time.Time
		v  decimal.Decimal
	}
	best := map[int64]map[string]pick{} // ts → analyzer → newest run
	for _, f := range forecasts {
		if f.GeneratedAt.After(cutoff(f.Ts)) {
			continue
		}
		key := f.Ts.UnixNano()
		if best[key] == nil {
			best[key] = map[string]pick{}
		}
		if p, ok := best[key][f.AnalyzerID.String()]; !ok || f.GeneratedAt.After(p.at) {
			best[key][f.AnalyzerID.String()] = pick{at: f.GeneratedAt, v: f.Median}
		}
	}
	out := map[int64]decimal.Decimal{}
	for ts, byAnalyzer := range best {
		s := decimal.Zero
		for _, p := range byAnalyzer {
			s = s.Add(p.v)
		}
		out[ts] = s
	}
	return out
}

// BuildForecast: sums of the latest run's medians ahead; accuracy is
// 100 − MAPE against actual hourly consumption behind (R292).
func BuildForecast(in Inputs) Forecast {
	f := Forecast{Unavailable: reasons{}}
	for _, field := range []string{"estimated_generation_kwh", "net_excess_kwh", "weather_impact"} {
		f.Unavailable.mark(field, ReasonNoGenForecast)
	}
	if len(in.Forecasts) == 0 {
		for _, field := range []string{"consumption_next_24h_kwh", "consumption_next_7d_kwh", "consumption_next_28d_kwh",
			"accuracy_daily_pct", "accuracy_weekly_pct", "accuracy_overall_pct"} {
			f.Unavailable.mark(field, ReasonNoForecasts)
		}
		return f
	}
	ahead := latestBefore(in.Forecasts, func(time.Time) time.Time { return in.Now })
	start := in.Now.Truncate(time.Hour)
	sumAhead := func(d time.Duration) *decimal.Decimal {
		var s *decimal.Decimal
		for ts, v := range ahead {
			t := time.Unix(0, ts)
			if !t.Before(start) && t.Before(start.Add(d)) {
				v := v
				s = add(s, &v)
			}
		}
		return s
	}
	f.ConsumptionNext24hKwh, f.ConsumptionNext7dKwh, f.ConsumptionNext28dKwh = sumAhead(24*time.Hour), sumAhead(7*24*time.Hour), sumAhead(28*24*time.Hour)
	behind := latestBefore(in.Forecasts, func(ts time.Time) time.Time { return ts })
	actual := sumBy(in.Hourly, imp, nil)
	accuracy := func(field string, d time.Duration) *decimal.Decimal {
		var errs []decimal.Decimal
		for _, a := range actual {
			if a.Ts.Before(in.Now.Add(-d)) || !a.Ts.Before(in.Now) || a.Kwh == nil || !a.Kwh.IsPositive() {
				continue
			}
			if fc, ok := behind[a.Ts.UnixNano()]; ok {
				errs = append(errs, fc.Sub(*a.Kwh).Abs().Div(*a.Kwh).Mul(decimal.NewFromInt(100)))
			}
		}
		if len(errs) == 0 {
			f.Unavailable.mark(field, ReasonNoOverlap)
			return nil
		}
		s := decimal.Zero
		for _, e := range errs {
			s = s.Add(e)
		}
		v := decimal.NewFromInt(100).Sub(s.Div(decimal.NewFromInt(int64(len(errs))))).Round(2)
		return &v
	}
	f.AccuracyDailyPct = accuracy("accuracy_daily_pct", 24*time.Hour)
	f.AccuracyWeeklyPct = accuracy("accuracy_weekly_pct", 7*24*time.Hour)
	f.AccuracyOverallPct = accuracy("accuracy_overall_pct", 30*24*time.Hour)
	return f
}

// Financial is §7.8's financial gains panel, served inside analytics.
type Financial struct {
	ImportPrice        *decimal.Decimal `json:"import_price"`
	ExportPrice        *decimal.Decimal `json:"export_price"`
	Currency           *string          `json:"currency"`
	TodayImportCost    *decimal.Decimal `json:"today_import_cost"`
	TodayExportRevenue *decimal.Decimal `json:"today_export_revenue"`
	NetToday           *decimal.Decimal `json:"net_today"`
	MonthEarnings      *decimal.Decimal `json:"month_earnings"`
	YearEarnings       *decimal.Decimal `json:"year_earnings"`
	TotalSavings       *decimal.Decimal `json:"total_savings"`
	RoiPct             *decimal.Decimal `json:"roi_pct"`
	PaybackYears       *decimal.Decimal `json:"payback_years"`
	BillSavings        *decimal.Decimal `json:"bill_savings"`
	Unavailable        reasons          `json:"unavailable"`
}

// Analytics is the analytics panel.
type Analytics struct {
	PeakGenerationKwh       *decimal.Decimal `json:"peak_generation_kwh"`
	PeakGenerationAt        *time.Time       `json:"peak_generation_at"`
	AverageGenerationKwh    *decimal.Decimal `json:"average_generation_kwh"`
	Trend                   []Point          `json:"trend"`
	PeakHour                *int             `json:"peak_hour"`
	DataAvailabilityPct     *decimal.Decimal `json:"data_availability_pct"`
	SystemEfficiencyPct     *decimal.Decimal `json:"system_efficiency_pct"`
	EfficiencyChange30dPct  *decimal.Decimal `json:"efficiency_change_30d_pct"`
	ConsumptionOptimisation *string          `json:"consumption_optimisation"`
	MaintenanceRequired     *string          `json:"maintenance_required"`
	Financial               Financial        `json:"financial"`
	Unavailable             reasons          `json:"unavailable"`
}

// BuildAnalytics reads the range's hourly and daily buckets and the bills.
func BuildAnalytics(in Inputs) Analytics {
	a := Analytics{Unavailable: reasons{}, Financial: buildFinancial(in)}
	to := in.To
	if in.Now.Before(to) {
		to = in.Now
	}
	hours := sumBy(in.Hourly, gen, within(in.From, to))
	if peak := maxOf(hours); peak != nil {
		a.PeakGenerationKwh, a.PeakGenerationAt = peak.Kwh, &peak.Ts
	}
	a.AverageGenerationKwh = mean(hours, 4)
	a.Trend = sumBy(in.Daily, gen, nil)
	byHour := map[int][]Point{}
	for _, p := range hours {
		h := p.Ts.In(istanbul).Hour()
		byHour[h] = append(byHour[h], p)
	}
	var bestHour *int
	var bestMean *decimal.Decimal
	for h := 0; h < 24; h++ {
		if m := mean(byHour[h], 6); m != nil && m.IsPositive() && (bestMean == nil || m.GreaterThan(*bestMean)) {
			hh := h
			bestHour, bestMean = &hh, m
		}
	}
	a.PeakHour = bestHour
	if bestHour == nil {
		a.Unavailable.mark("peak_hour", ReasonNoData)
	}
	span := to.Sub(in.From).Hours()
	withData := sumBy(in.Hourly, func(b model.ConsumptionBucket) *decimal.Decimal {
		if b.ReadingCount <= 0 {
			return nil
		}
		one := decimal.NewFromInt(1)
		return &one
	}, within(in.From, to))
	if len(withData) > 0 && span > 0 {
		v := decimal.NewFromInt(int64(len(withData))).Div(decimal.NewFromFloat(math.Ceil(span))).Mul(decimal.NewFromInt(100)).Round(2)
		a.DataAvailabilityPct = &v
	}
	for _, f := range []string{"system_efficiency_pct", "efficiency_change_30d_pct", "consumption_optimisation", "maintenance_required"} {
		a.Unavailable.mark(f, ReasonNoIrradiance)
	}
	return a
}

func buildFinancial(in Inputs) Financial {
	f := Financial{Unavailable: reasons{}}
	f.Unavailable.mark("roi_pct", ReasonNoInvestment)
	f.Unavailable.mark("payback_years", ReasonNoInvestment)
	f.Unavailable.mark("bill_savings", ReasonNoSelfConsumption)
	b := in.LatestBill
	if b == nil || b.EffectiveEnergyPrice == nil || b.GenerationPricePerKwh == nil {
		for _, field := range []string{"import_price", "export_price", "currency", "today_import_cost", "today_export_revenue",
			"net_today", "month_earnings", "year_earnings", "total_savings"} {
			f.Unavailable.mark(field, ReasonNoBill)
		}
		return f
	}
	f.ImportPrice, f.ExportPrice, f.Currency = b.EffectiveEnergyPrice, b.GenerationPricePerKwh, str(string(b.Currency))
	today := within(dayStart(in.Now), in.Now)
	f.TodayImportCost = mul(total(sumBy(in.Hourly, imp, today)), *b.EffectiveEnergyPrice, 2)
	f.TodayExportRevenue = mul(total(sumBy(in.Hourly, gen, today)), *b.GenerationPricePerKwh, 2)
	if f.TodayImportCost != nil && f.TodayExportRevenue != nil {
		net := f.TodayExportRevenue.Sub(*f.TodayImportCost)
		f.NetToday = &net
	}
	local := in.Now.In(istanbul)
	month, year := local.Format("2006-01"), local.Format("2006")
	for _, bill := range in.Bills {
		if bill.Currency != b.Currency {
			continue // currencies never add (R253)
		}
		credit := bill.GenerationCredit
		if bill.PeriodKey == month {
			f.MonthEarnings = add(f.MonthEarnings, &credit)
		}
		if len(bill.PeriodKey) >= 4 && bill.PeriodKey[:4] == year {
			f.YearEarnings = add(f.YearEarnings, &credit)
		}
		f.TotalSavings = add(f.TotalSavings, &credit)
	}
	return f
}

// SystemStatus is the system status panel.
type SystemStatus struct {
	Overall              *string          `json:"overall"`
	Monitoring           *string          `json:"monitoring"`
	GridConnection       *string          `json:"grid_connection"`
	LastReadingAt        *time.Time       `json:"last_reading_at"`
	SolarPanels          *string          `json:"solar_panels"`
	Inverter             *string          `json:"inverter"`
	Battery              *string          `json:"battery"`
	Security             *string          `json:"security"`
	TotalGenerationKwh   *decimal.Decimal `json:"total_generation_kwh"`
	AverageEfficiencyPct *decimal.Decimal `json:"average_efficiency_pct"`
	Unavailable          reasons          `json:"unavailable"`
}

var severity = map[string]int{"healthy": 0, "attention": 1, "critical": 2}

// BuildSystemStatus: monitoring from reading age (26 h / 72 h, readings
// arrive daily), grid connection from recent data; the worst available wins.
func BuildSystemStatus(in Inputs) SystemStatus {
	s := SystemStatus{Unavailable: reasons{}, LastReadingAt: in.LastReadingAt, TotalGenerationKwh: total(sumBy(in.Daily, gen, nil))}
	if in.LastReadingAt != nil {
		age := in.Now.Sub(*in.LastReadingAt)
		switch {
		case age <= 26*time.Hour:
			s.Monitoring = str("healthy")
		case age <= 72*time.Hour:
			s.Monitoring = str("attention")
		default:
			s.Monitoring = str("critical")
		}
	} else {
		s.Unavailable.mark("monitoring", ReasonNoData)
		s.Unavailable.mark("last_reading_at", ReasonNoData)
	}
	recent := within(in.Now.Add(-26*time.Hour), in.Now)
	if len(sumBy(in.Hourly, imp, recent))+len(sumBy(in.Hourly, gen, recent)) > 0 {
		s.GridConnection = str("healthy")
	} else if in.LastReadingAt != nil {
		s.GridConnection = str("critical")
	} else {
		s.Unavailable.mark("grid_connection", ReasonNoData)
	}
	for _, f := range []string{"solar_panels", "inverter", "battery", "security"} {
		s.Unavailable.mark(f, ReasonNoTelemetry)
	}
	s.Unavailable.mark("average_efficiency_pct", ReasonNoIrradiance)
	for _, c := range []*string{s.Monitoring, s.GridConnection} {
		if c != nil && (s.Overall == nil || severity[*c] > severity[*s.Overall]) {
			v := *c
			s.Overall = &v
		}
	}
	if s.Overall == nil {
		s.Unavailable.mark("overall", ReasonNoData)
	}
	return s
}
