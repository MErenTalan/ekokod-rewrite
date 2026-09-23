package renewable

// SourceKind says where a panel field comes from (R292).
type SourceKind string

// Source kinds: a meter/bill value, a formula over them, a fixed reason for
// absence, or a descriptive label carried with a figure (unit, currency).
const (
	Measured    SourceKind = "measured"
	Derived     SourceKind = "derived"
	Unavailable SourceKind = "unavailable"
	Label       SourceKind = "label"
)

// Source is one field's provenance.
type Source struct {
	Kind SourceKind
	From string
}

func m(from string) Source { return Source{Kind: Measured, From: from} }
func d(from string) Source { return Source{Kind: Derived, From: from} }
func l(from string) Source { return Source{Kind: Label, From: from} }

var never = Source{Kind: Unavailable}

// Sources is R292's table in code: every JSON field of every panel.
// TestNoSyntheticData fails when a builder adds a field this table does not
// declare, or when a declared figure behaves like a constant or a random value.
var Sources = map[string]map[string]Source{
	"overview": {
		"active_generation_kwh":       m("Σ consumption_daily.active_generation over the range"),
		"inductive_generation_kvarh":  m("Σ consumption_daily.inductive_generation over the range"),
		"capacitive_generation_kvarh": m("Σ consumption_daily.capacitive_generation over the range"),
		"average_generation_kwh":      d("active_generation_kwh ÷ days with generation"),
	},
	"realtime": {
		"current_power_kw":      d("last complete hour's consumption_hourly.active_generation ÷ 1 h"),
		"today_kwh":             m("Σ today's consumption_hourly.active_generation"),
		"max_power_kw":          d("max hourly generation over the last 24 h"),
		"avg_power_kw":          d("mean hourly generation over the last 24 h"),
		"status":                d("producing / idle / no_data from the last complete hour"),
		"series_24h":            m("consumption_hourly.active_generation, last 24 h"),
		"system_efficiency_pct": never,
	},
	"grid_interaction": {
		"direction":        d("last complete hour: generation vs consumption"),
		"today_import_kwh": m("Σ today's consumption_hourly.active_consumption"),
		"today_export_kwh": m("Σ today's consumption_hourly.active_generation"),
		"power_factor":     d("P ÷ √(P² + Q²) over today's active and inductive consumption"),
		"voltage_v":        never,
		"frequency_hz":     never,
		"import_price":     m("latest bill effective_energy_price"),
		"export_price":     m("latest bill generation_price_per_kwh"),
		"currency":         l("latest bill currency"),
		"net_today":        d("today_export × export_price − today_import × import_price"),
	},
	"environmental": {
		"generation_kwh":     m("Σ consumption_daily.active_generation over the range"),
		"co2_avoided_kg":     d("generation_kwh × grid emission factor"),
		"trees":              d("co2_avoided_kg ÷ equiv_tree_co2_kg_per_year"),
		"coal_kg":            d("generation_kwh × equiv_coal_kg_per_kwh"),
		"car_km":             d("co2_avoided_kg ÷ equiv_car_co2_kg_per_km"),
		"homes":              d("generation_kwh ÷ equiv_home_heating_kwh_per_year"),
		"grid_factor":        m("emission_factors grid_electricity_tr_2022"),
		"grid_factor_unit":   l("emission_factors base_unit"),
		"grid_factor_source": l("emission_factors source"),
	},
	"efficiency": {
		"overall_pct": never, "panel_pct": never, "inverter_pct": never, "battery_pct": never, "grid_pct": never,
		"trend": never, "recommendations": never,
	},
	"forecast": {
		"consumption_next_24h_kwh": d("Σ latest forecast run medians, next 24 h"),
		"consumption_next_7d_kwh":  d("Σ latest forecast run medians, next 7 days"),
		"consumption_next_28d_kwh": d("Σ latest forecast run medians, next 28 days"),
		"accuracy_daily_pct":       d("100 − MAPE of forecasts vs hourly consumption, last day"),
		"accuracy_weekly_pct":      d("100 − MAPE, last 7 days"),
		"accuracy_overall_pct":     d("100 − MAPE, last 30 days"),
		"estimated_generation_kwh": never,
		"net_excess_kwh":           never,
		"weather_impact":           never,
	},
	"analytics": {
		"peak_generation_kwh":            m("max consumption_hourly.active_generation in the range"),
		"peak_generation_at":             m("that hour"),
		"average_generation_kwh":         d("mean hourly generation in the range"),
		"trend":                          m("consumption_daily.active_generation in the range"),
		"peak_hour":                      d("hour of day with the highest mean generation"),
		"data_availability_pct":          d("hours with readings ÷ hours in the range × 100"),
		"system_efficiency_pct":          never,
		"efficiency_change_30d_pct":      never,
		"consumption_optimisation":       never,
		"maintenance_required":           never,
		"financial.import_price":         m("latest bill effective_energy_price"),
		"financial.export_price":         m("latest bill generation_price_per_kwh"),
		"financial.currency":             l("latest bill currency"),
		"financial.today_import_cost":    d("today's consumption × import_price"),
		"financial.today_export_revenue": d("today's generation × export_price"),
		"financial.net_today":            d("today_export_revenue − today_import_cost"),
		"financial.month_earnings":       m("Σ generation_credit of this month's bills"),
		"financial.year_earnings":        m("Σ generation_credit of this year's bills"),
		"financial.total_savings":        m("Σ generation_credit of the range's bills"),
		"financial.roi_pct":              never,
		"financial.payback_years":        never,
		"financial.bill_savings":         never,
	},
	"system_status": {
		"overall":                d("worst of the available components"),
		"monitoring":             d("age of the last reading: 26 h / 72 h"),
		"grid_connection":        d("consumption or generation in the last 26 h"),
		"last_reading_at":        m("analyzers.last_reading_at"),
		"solar_panels":           never,
		"inverter":               never,
		"battery":                never,
		"security":               never,
		"total_generation_kwh":   m("Σ consumption_daily.active_generation over the range"),
		"average_efficiency_pct": never,
	},
}
