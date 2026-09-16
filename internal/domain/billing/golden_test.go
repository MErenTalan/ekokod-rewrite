package billing_test

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/billing"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/tariff"
)

var updateGolden = flag.Bool("update", false, "rewrite testdata/golden from the seed builders")

type goldenLine struct {
	Code      string           `json:"code"`
	Quantity  *decimal.Decimal `json:"quantity"`
	UnitPrice *decimal.Decimal `json:"unit_price"`
	RatePct   *decimal.Decimal `json:"rate_pct"`
	Amount    decimal.Decimal  `json:"amount"`
}

type goldenExpected struct {
	Lines               []goldenLine    `json:"lines"`
	NetConsumption      decimal.Decimal `json:"net_consumption"`
	LowTierKwh          decimal.Decimal `json:"low_tier_kwh"`
	HighTierKwh         decimal.Decimal `json:"high_tier_kwh"`
	EnergyCost          decimal.Decimal `json:"energy_cost"`
	DistributionCost    decimal.Decimal `json:"distribution_cost"`
	PowerCost           decimal.Decimal `json:"power_cost"`
	DemandOverrunCost   decimal.Decimal `json:"demand_overrun_cost"`
	ReactivePenalty     decimal.Decimal `json:"reactive_penalty"`
	ExtraChargesCost    decimal.Decimal `json:"extra_charges_cost"`
	OtherTaxesCost      decimal.Decimal `json:"other_taxes_cost"`
	VatBase             decimal.Decimal `json:"vat_base"`
	VatCost             decimal.Decimal `json:"vat_cost"`
	GenerationCredit    decimal.Decimal `json:"generation_credit"`
	TotalCost           decimal.Decimal `json:"total_cost"`
	Flags               []billing.Flag  `json:"flags"`
	TieredApplied       bool            `json:"tiered_applied"`
	ReactiveApplied     bool            `json:"reactive_applied"`
	ReactiveExempt      bool            `json:"reactive_exempt"`
	DemandDataAvailable bool            `json:"demand_data_available"`
}

type goldenFile struct {
	Description string          `json:"description"`
	Input       *billing.Input  `json:"input,omitempty"`
	Buildings   []billing.Input `json:"buildings,omitempty"`
	Expected    goldenExpected  `json:"expected"`
}

func expectedOf(inv billing.Invoice) goldenExpected {
	e := goldenExpected{
		NetConsumption: inv.NetConsumption, LowTierKwh: inv.LowTierKwh, HighTierKwh: inv.HighTierKwh,
		EnergyCost: inv.EnergyCost, DistributionCost: inv.DistributionCost, PowerCost: inv.PowerCost,
		DemandOverrunCost: inv.DemandOverrunCost, ReactivePenalty: inv.ReactivePenalty, ExtraChargesCost: inv.ExtraChargesCost,
		OtherTaxesCost: inv.OtherTaxesCost, VatBase: inv.VatBase, VatCost: inv.VatCost, GenerationCredit: inv.GenerationCredit,
		TotalCost: inv.TotalCost, Flags: inv.Flags, TieredApplied: inv.TieredApplied, ReactiveApplied: inv.Reactive.Applied,
		ReactiveExempt: inv.Reactive.Exempt, DemandDataAvailable: inv.DemandDataAvailable, Lines: []goldenLine{},
	}
	if e.Flags == nil {
		e.Flags = []billing.Flag{}
	}
	for _, l := range inv.Lines {
		e.Lines = append(e.Lines, goldenLine{Code: l.Code, Quantity: l.Quantity, UnitPrice: l.UnitPrice, RatePct: l.RatePct, Amount: l.Amount})
	}
	return e
}

func evaluate(t *testing.T, g goldenFile) goldenExpected {
	t.Helper()
	if g.Input != nil {
		return expectedOf(compute(t, *g.Input))
	}
	invoices := make([]billing.Invoice, len(g.Buildings))
	for i, b := range g.Buildings {
		invoices[i] = compute(t, b)
	}
	c, err := billing.CombineCompany(g.Buildings[0].PeriodKey, invoices)
	require.NoError(t, err)
	return expectedOf(c)
}

// TestGolden locks every §F4 acceptance shape. Inputs live in the JSON files;
// -update rebuilds them from goldenSeeds and must be hand-checked (Task 4 rule).
func TestGolden(t *testing.T) {
	seeds := goldenSeeds(t)
	dir := filepath.Join("testdata", "golden")
	if *updateGolden {
		require.NoError(t, os.MkdirAll(dir, 0o755))
		for name, seed := range seeds {
			g := seed()
			g.Expected = evaluate(t, g)
			raw, err := json.MarshalIndent(g, "", "  ")
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(dir, name+".json"), append(raw, '\n'), 0o644))
		}
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	require.NoError(t, err)
	names := make([]string, 0, len(files))
	for _, f := range files {
		names = append(names, filepath.Base(f[:len(f)-len(".json")]))
	}
	want := make([]string, 0, len(seeds))
	for name := range seeds {
		want = append(want, name)
	}
	sort.Strings(want)
	require.Equal(t, want, names, "every seed has exactly one golden file")

	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			raw, err := os.ReadFile(f)
			require.NoError(t, err)
			var g goldenFile
			require.NoError(t, json.Unmarshal(raw, &g))
			wantJSON, err := json.Marshal(g.Expected)
			require.NoError(t, err)
			gotJSON, err := json.Marshal(evaluate(t, g))
			require.NoError(t, err)
			require.JSONEq(t, string(wantJSON), string(gotJSON))
		})
	}
}

func goldenSeeds(t *testing.T) map[string]func() goldenFile {
	loc := istanbul(t)
	single := func(desc string, build func() billing.Input) func() goldenFile {
		return func() goldenFile {
			in := build()
			in.Tariff.ID = uuid.Nil
			return goldenFile{Description: desc, Input: &in}
		}
	}
	with := func(build func() billing.Input, mutate func(in *billing.Input)) func() billing.Input {
		return func() billing.Input { in := build(); mutate(&in); return in }
	}
	base := func() billing.Input { return baseInput(t) }
	react := func() billing.Input { return reactiveInput(t) }
	ptf := func(n int, mutate func(tr *model.Tariff), hourly bool, missing int) func() billing.Input {
		return func() billing.Input {
			in := baseInput(t)
			from := time.Date(2026, 1, 1, 0, 0, 0, 0, loc)
			in.Period, in.Days = energy.Window{From: from, To: from.Add(time.Duration(n) * time.Hour)}, 1
			tr := in.Tariff
			tr.UsePtfYekdem, tr.SingleTimePrice, tr.SupplyCompany = true, nil, model.SupplyCompanyPrivate
			tr.KbkEnergy, tr.KbkReactivePower, tr.KbkDistributionCostTlPerKwh = dp("1.1"), dp("0.5"), dp("0.8")
			if mutate != nil {
				mutate(&tr)
			}
			in.Tariff = tr
			m := tariff.Market{PTF: map[time.Time]decimal.Decimal{}, Yekdem: map[tariff.YearMonth]decimal.Decimal{{Year: 2026, Month: time.January}: d("500")},
				ManualYekdem: map[tariff.YearMonth]decimal.Decimal{{Year: 2026, Month: time.January}: d("700")}}
			var hours []tariff.HourConsumption
			total := decimal.Zero
			for i := range n {
				h := from.Add(time.Duration(i) * time.Hour).UTC()
				m.PTF[h] = decimal.NewFromInt(int64(1000 + 50*i))
				kwh := decimal.NewFromInt(int64(10 + i))
				total = total.Add(kwh)
				if i >= missing {
					hours = append(hours, tariff.HourConsumption{Hour: h, Kwh: kwh})
				}
			}
			in.Quantities.ActiveImport = &total
			p, err := tariff.Price(tr, in.Period, hours, hourly, m, in.Params, loc)
			require.NoError(t, err)
			in.Pricing = &p
			return in
		}
	}
	return map[string]func() goldenFile{
		"single_rate_untiered":           single("commercial 600 kWh / 30 days, under the 30 kWh/day threshold", base),
		"single_rate_tiered_residential": single("residential 400 kWh / 30 days: 240 low at 2, 160 high at 3 (02 §6.2)", func() billing.Input { return residential(t, "400") }),
		"single_rate_tiered_commercial":  single("commercial 1200 kWh / 30 days: 900 low at 2.5, 300 high at 3.5", with(base, func(in *billing.Input) { in.Quantities.ActiveImport = dp("1200") })),
		"multi_time_no_tiering":          single("residential multi-time far above threshold: bands at their own prices, never tiered or averaged", func() billing.Input { return multiTime(t) }),
		"generation_none":                single("export 100 ignored under generation_usage none", with(base, func(in *billing.Input) { in.Quantities.ActiveExport = dp("100") })),
		"generation_subtract_from_consumption": single("net 400 = 500 − 100; low tier 30×8 − 100 = 140 (R122)", func() billing.Input {
			in := residential(t, "500")
			in.Tariff.GenerationUsage = model.GenerationUsageSubtractFromConsumption
			in.Quantities.ActiveExport = dp("100")
			return in
		}),
		"generation_subtract_from_total": single("credit 100 × 1.5 after VAT", with(base, func(in *billing.Input) {
			in.Tariff.GenerationUsage, in.Tariff.GenerationPricePerKwh = model.GenerationUsageSubtractFromTotal, dp("1.5")
			in.Quantities.ActiveExport = dp("100")
		})),
		"reactive_applied":            single("binomial 50 kW, inductive 0.5 > 0.20: whole 500 kVArh × 2", react),
		"reactive_exempt_monomial":    single("monomial exempt (R118 default)", with(react, func(in *billing.Input) { in.Tariff.Term = model.TariffTermMonomial })),
		"reactive_exempt_residential": single("residential exempt", with(react, func(in *billing.Input) { in.Tariff.UserGroup = model.UserGroupResidential })),
		"reactive_exempt_lighting":    single("lighting exempt", with(react, func(in *billing.Input) { in.Tariff.UserGroup = model.UserGroupLighting })),
		"reactive_exempt_generation": single("export 5 kWh > 1 exempts", with(react, func(in *billing.Input) {
			in.Quantities.ActiveExport, in.Quantities.ActiveExportKnownSum = dp("5"), d("5")
		})),
		"reactive_exempt_below_9kw": single("installed 8 kW < 9 exempts", with(react, func(in *billing.Input) { in.InstalledPowerKw = dp("8") })),
		"demand_overrun":            single("contracted 100 kW at 40, max 130: overrun 30 × 40 × 2", func() billing.Input { return binomial(t) }),
		"named_taxes": single("BTV 5% and TRT 2% of the rounded energy cost", with(base, func(in *billing.Input) {
			in.Taxes = []model.TariffTax{{Name: "BTV", Rate: d("5"), SortOrder: 1}, {Name: "TRT", Rate: d("2"), SortOrder: 2}}
		})),
		"ptf_hourly":                 single("24 hours, consumption-weighted PTF+YEKDEM × 1.1 (R108)", ptf(24, nil, true, 0)),
		"ptf_period_average":         single("no hourly data: base × kbk (02 §7.2)", ptf(24, nil, false, 0)),
		"ptf_manual_yekdem_override": single("manual YEKDEM 700 replaces published 500", ptf(24, func(tr *model.Tariff) { tr.UseManualYekdem = true }, true, 0)),
		"ptf_missing_hours_flagged":  single("3 of 24 hours without consumption (12.5% > 2%) → flagged (R109)", ptf(24, nil, true, 3)),
		"kbk_null_coefficient_zero_line": single("binomial PTF with nil kbk power and reactive, inductive 400/516 exceeded: no power or reactive line", with(ptf(24, func(tr *model.Tariff) {
			tr.Term, tr.ContractedPowerKw, tr.KbkReactivePower = model.TariffTermBinomial, dp("100"), nil
		}, false, 0), func(in *billing.Input) { in.Quantities.ReactiveInductive = dp("400") })),
		"extra_charges_custom_power": single("every R126 basis incl. a negative pct_of_energy discount", func() billing.Input {
			in := binomial(t)
			in.ExtraCharges = []model.TariffExtraCharge{
				{Name: "kwh", Basis: model.ExtraChargeBasisPerKwh, Amount: d("0.01"), SortOrder: 1},
				{Name: "cap", Basis: model.ExtraChargeBasisPerContractedKw, Amount: d("2"), SortOrder: 2},
				{Name: "peak", Basis: model.ExtraChargeBasisPerMaxDemandKw, Amount: d("1.5"), SortOrder: 3},
				{Name: "discount", Basis: model.ExtraChargeBasisPctOfEnergy, Amount: d("-10"), SortOrder: 4},
				{Name: "fixed", Basis: model.ExtraChargeBasisFixedPerPeriod, Amount: d("25"), SortOrder: 5},
			}
			return in
		}),
		"icmal_row_assembly_real": single("anonymised real icmal line 31 (OG binomial, power + overrun): matrah 820028.45, KDV 164005.69", func() billing.Input {
			in := icmalRow(t, "169833.84", "552557.29", "214549.90")
			in.Tariff.Term = model.TariffTermBinomial
			in.Tariff.ContractedPowerKw = dp("300")
			unit := d("13011.01").DivRound(d("300"), tariff.DivisionScale)
			in.Tariff.PowerUnitPrice = &unit
			over := d("12282.39").DivRound(unit.Mul(d("2")), tariff.DivisionScale).Add(d("300"))
			in.Quantities.MaxDemandKw = &over
			in.Quantities.ReactiveInductive, in.Quantities.ReactiveCapacitive = dp("2870.40"), dp("4504.32")
			return in
		}),
		"company_two_buildings": func() goldenFile {
			a := residential(t, "400")
			b := baseInput(t)
			w, err := billing.Period("2026-01", 15, loc)
			require.NoError(t, err)
			b.Period, b.Days = w, billing.DaysInPeriod(w, loc)
			b.Tariff.SingleTimePrice = dp("3")
			a.Tariff.ID, b.Tariff.ID = uuid.Nil, uuid.Nil
			return goldenFile{Description: "cut-off 1 residential + cut-off 15 commercial, each with its own tariff and window (R115)", Buildings: []billing.Input{a, b}}
		},
		"tariff_resolution_mid_month_change": single("versions 2026-01-01 at 2.5 and 2026-01-15 at 3: the period start date picks 2.5, no proration (R110)", func() billing.Input {
			in := baseInput(t)
			v1, v2 := in.Tariff, in.Tariff
			v1.EffectiveFrom, v2.EffectiveFrom = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
			v2.SingleTimePrice = dp("3")
			resolved, ok := tariff.Resolve([]model.Tariff{v2, v1}, uuid.New(), in.Period.From, loc)
			require.True(t, ok)
			in.Tariff = resolved
			return in
		}),
		"period_cutoff_31_february": single("2026-02 at cut-off 31 = 28-02 → 31-03, 31 days (02 §5)", func() billing.Input {
			in := baseInput(t)
			w, err := billing.Period("2026-02", 31, loc)
			require.NoError(t, err)
			in.PeriodKey, in.Period, in.Days = "2026-02", w, billing.DaysInPeriod(w, loc)
			in.Quantities.ActiveImport = dp("900")
			return in
		}),
	}
}
