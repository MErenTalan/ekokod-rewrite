package legacy_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/migrate/legacy"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/admin"
)

var (
	rcCompany, rcBuilding, rcAnalyzer = uuid.New(), uuid.New(), uuid.New()
)

func dp(s string) *decimal.Decimal { d := decimal.RequireFromString(s); return &d }
func dv(s string) decimal.Decimal  { return decimal.RequireFromString(s) }
func sp(s string) *string          { return &s }

// A legacy bill and its recomputation that agree: 1000 kWh × 2 + 1000 × 0.5, 20 % VAT.
func agreeing() (admin.LegacyBillRow, admin.LiveBillRow) {
	payload, _ := json.Marshal(map[string]any{"isTwoPartCalculation": false, "normalConsumption": 1000})
	applied := false
	lb := admin.LegacyBillRow{CompanyID: rcCompany, BuildingID: rcBuilding, Scope: "building", Period: "2026-07", TotalActiveKwh: dp("1000"),
		EnergyCost: dp("2000"), DistributionCost: dp("500"), CapacityCost: dp("0"), PowerCost: dp("0"), GreenEnergyCost: dp("0"),
		ReactivePenalty: dp("0"), VatCost: dp("500"), OtherTaxesCost: dp("0"), TotalCost: dp("3000"), ReactivePenaltyApplied: &applied, Payload: payload}
	nb := admin.LiveBillRow{CompanyID: rcCompany, BuildingID: rcBuilding, Scope: "building", Period: "2026-07", Status: "issued",
		ActiveImport: dv("1000"), NetConsumption: dv("1000"), EnergyCost: dv("2000"), DistributionCost: dv("500"), VatBase: dv("2500"),
		VatCost: dv("500"), TotalCost: dv("3000"), PriceType: sp("single_time"), Term: sp("binomial"), UserGroup: sp("commercial"),
		TariffVatRate: dp("20"), TariffDistributionPrice: dp("0.5"), InstalledPower: dp("50")}
	return lb, nb
}

func run(lb admin.LegacyBillRow, nb *admin.LiveBillRow, extra func(*legacy.Inputs)) legacy.Report {
	in := legacy.Inputs{Now: time.Now(), LegacyBills: []admin.LegacyBillRow{lb},
		Names: admin.Names{Companies: map[uuid.UUID]string{rcCompany: "Acme"}, Buildings: map[uuid.UUID]string{rcBuilding: "Merkez"},
			Analyzers: map[uuid.UUID]string{rcAnalyzer: "A1"}, AnalyzerBuilding: map[uuid.UUID]uuid.UUID{rcAnalyzer: rcBuilding},
			BuildingCompany: map[uuid.UUID]uuid.UUID{rcBuilding: rcCompany}}}
	if nb != nil {
		in.LiveBills = []admin.LiveBillRow{*nb}
	}
	if extra != nil {
		extra(&in)
	}
	return legacy.Reconcile(in)
}

func reasonsOf(rep legacy.Report, check string) []legacy.Reason {
	for _, r := range rep.Companies[0].Rows {
		if r.Check == check {
			return r.Reasons
		}
	}
	return nil
}

func TestAgreeingBillsPass(t *testing.T) {
	lb, nb := agreeing()
	rep := run(lb, &nb, nil)
	require.True(t, rep.Pass)
	require.Empty(t, rep.Companies[0].Rows)
}

func TestEachDocumentedReasonIsAttributed(t *testing.T) {
	for name, tc := range map[string]struct {
		mutate func(*admin.LegacyBillRow, *admin.LiveBillRow)
		want   []legacy.Reason
	}{
		"distribution on low tier only (02 §6.3)": {func(lb *admin.LegacyBillRow, nb *admin.LiveBillRow) {
			lb.TotalActiveKwh, nb.ActiveImport = dp("1500"), dv("1500")
			lb.EnergyCost, nb.EnergyCost = dp("3000"), dv("3000")
			lb.DistributionCost, nb.DistributionCost = dp("500"), dv("750") // legacy: 1000 low-tier kWh × 0.5
			lb.VatCost, lb.TotalCost = dp("700"), dp("4200")
			nb.VatCost, nb.TotalCost = dv("750"), dv("4500")
		}, []legacy.Reason{legacy.ReasonDistributionOnTotal}},
		"tiering on a multi-time tariff (02 §6.2)": {func(lb *admin.LegacyBillRow, nb *admin.LiveBillRow) {
			lb.Payload = json.RawMessage(`{"isTwoPartCalculation":true,"normalConsumption":600}`)
			nb.PriceType = sp("multi_time")
			nb.EnergyCost, nb.VatCost, nb.TotalCost = dv("2400"), dv("580"), dv("3480")
		}, []legacy.Reason{legacy.ReasonNoTieringMultiTime}},
		"reactive below 9 kW (02 §6.6)": {func(lb *admin.LegacyBillRow, nb *admin.LiveBillRow) {
			applied := true
			lb.ReactivePenalty, lb.ReactivePenaltyApplied, lb.VatCost, lb.TotalCost = dp("100"), &applied, dp("520"), dp("3120")
			nb.InstalledPower = dp("5")
		}, []legacy.Reason{legacy.ReasonReactiveBelow9kW}},
		"reactive on a monomial tariff": {func(lb *admin.LegacyBillRow, nb *admin.LiveBillRow) {
			lb.ReactivePenalty, lb.VatCost, lb.TotalCost = dp("100"), dp("520"), dp("3120")
			nb.Term = sp("monomial")
		}, []legacy.Reason{legacy.ReasonReactiveNotApplies}},
		"VAT 18 % → 20 % (02 §6.8)": {func(lb *admin.LegacyBillRow, _ *admin.LiveBillRow) {
			lb.VatCost, lb.TotalCost = dp("450"), dp("2950")
		}, []legacy.Reason{legacy.ReasonVATRateCorrected}},
		"capacity cost (02 §6.5)": {func(lb *admin.LegacyBillRow, _ *admin.LiveBillRow) {
			lb.CapacityCost, lb.VatCost, lb.TotalCost = dp("200"), dp("540"), dp("3240")
		}, []legacy.Reason{legacy.ReasonCapacityRemoved}},
		"monomial power charge (R125)": {func(lb *admin.LegacyBillRow, nb *admin.LiveBillRow) {
			lb.PowerCost, lb.VatCost, lb.TotalCost = dp("1000"), dp("700"), dp("4200")
			nb.Term = sp("monomial")
		}, []legacy.Reason{legacy.ReasonMonomialPower}},
		"PTF gap flagged (02 §7.1)": {func(_ *admin.LegacyBillRow, nb *admin.LiveBillRow) {
			nb.Status, nb.FlagReason, nb.EnergyCost, nb.TotalCost = "flagged", sp("ptf_data_missing"), dv("1500"), dv("2400")
		}, []legacy.Reason{legacy.ReasonPTFGapFlagged}},
		"a total difference no component explains": {func(_ *admin.LegacyBillRow, nb *admin.LiveBillRow) {
			nb.OtherTaxesCost, nb.TotalCost = dv("100"), dv("3100")
		}, []legacy.Reason{legacy.Unexplained}},
		"an energy difference with no rule": {func(_ *admin.LegacyBillRow, nb *admin.LiveBillRow) {
			nb.EnergyCost, nb.VatCost, nb.TotalCost = dv("2500"), dv("600"), dv("3600")
		}, []legacy.Reason{legacy.Unexplained}},
	} {
		t.Run(name, func(t *testing.T) {
			lb, nb := agreeing()
			tc.mutate(&lb, &nb)
			rep := run(lb, &nb, nil)
			require.Equal(t, tc.want, reasonsOf(rep, legacy.CheckBuildingInvoice))
			require.Equal(t, tc.want[0] != legacy.Unexplained, rep.Pass, "only unexplained blocks cutover")
		})
	}
}

func TestAWithheldPeriodIsAMeterReset(t *testing.T) {
	lb, _ := agreeing()
	withAnomaly := func(in *legacy.Inputs) {
		in.Anomalies = []admin.Anomaly{{AnalyzerID: rcAnalyzer, From: time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC)}}
	}
	require.Equal(t, []legacy.Reason{legacy.ReasonMeterResetWithheld}, reasonsOf(run(lb, nil, withAnomaly), legacy.CheckBuildingInvoice))
	require.Equal(t, []legacy.Reason{legacy.Unexplained}, reasonsOf(run(lb, nil, nil), legacy.CheckBuildingInvoice), "missing with no anomaly")
}

func TestReadingsAndCarbon(t *testing.T) {
	lb, nb := agreeing()
	rep := run(lb, &nb, func(in *legacy.Inputs) {
		in.Expected = map[uuid.UUID]map[string]int{rcAnalyzer: {"2026-07": 744}}
		in.Readings = map[uuid.UUID]map[string]int{}
		in.Withheld = map[uuid.UUID]bool{rcAnalyzer: true}
		in.CarbonLegacy = map[uuid.UUID]map[int]decimal.Decimal{rcBuilding: {2025: dv("1000")}}
		in.CarbonNew = map[uuid.UUID]map[int]decimal.Decimal{rcBuilding: {2025: dv("470")}}
		in.CarbonAuto = map[uuid.UUID]map[int]bool{rcBuilding: {2025: true}}
	})
	require.Equal(t, []legacy.Reason{legacy.ReasonMultiplierPending}, reasonsOf(rep, legacy.CheckReadings))
	require.Equal(t, []legacy.Reason{legacy.ReasonAutomatedCarbon}, reasonsOf(rep, legacy.CheckCarbon))
	require.True(t, rep.Pass)
}
