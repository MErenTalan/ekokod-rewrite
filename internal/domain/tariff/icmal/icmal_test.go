package icmal_test

import (
	"encoding/csv"
	"encoding/json"
	"os"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/tariff/icmal"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func dp(s string) *decimal.Decimal {
	v := decimal.RequireFromString(s)
	return &v
}

func fixture(t *testing.T) icmal.Table {
	t.Helper()
	f, err := os.Open("testdata/ck_icmal_2025_11_12_anonymised.csv")
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	records, err := r.ReadAll()
	require.NoError(t, err)
	return icmal.Table{Header: records[0], Rows: records[1:]}
}

func parsed(t *testing.T) icmal.Parsed {
	t.Helper()
	p, err := icmal.Parse(fixture(t))
	require.NoError(t, err)
	return p
}

var synthBase = map[string]decimal.Decimal{"202511": d("2.5"), "202512": d("2.6")}

func byEtso(as []icmal.Analysis, etso string) icmal.Analysis {
	for _, a := range as {
		if a.EtsoCode == etso {
			return a
		}
	}
	return icmal.Analysis{}
}

func codes(ws []icmal.Warning) map[string][]int {
	out := map[string][]int{}
	for _, w := range ws {
		out[w.Code] = append(out[w.Code], w.Row)
	}
	return out
}

func TestDetectUSFormatOnRealFixture(t *testing.T) {
	f, err := icmal.DetectNumberFormat(fixture(t))
	require.NoError(t, err)
	require.Equal(t, icmal.FormatUS, f)
}

func TestDetectTurkishFormat(t *testing.T) {
	tbl := icmal.Table{Header: []string{"Muhasebe Dönemi", "Toplam Kwh", "Enerji Bedeli"}, Rows: [][]string{{"202511", "1.234,56", "12"}, {"202511", "1,234", "-5,5"}}}
	f, err := icmal.DetectNumberFormat(tbl)
	require.NoError(t, err)
	require.Equal(t, icmal.FormatTurkish, f)

	ints := icmal.Table{Header: tbl.Header, Rows: [][]string{{"202511", "1,234", "7"}}}
	f, err = icmal.DetectNumberFormat(ints)
	require.NoError(t, err)
	require.Equal(t, icmal.FormatTurkish, f, "integer-only cells default to Turkish")
}

func TestDetectMixedFormatsIsError(t *testing.T) {
	tbl := icmal.Table{Header: []string{"Muhasebe Dönemi", "Toplam Kwh", "Enerji Bedeli"}, Rows: [][]string{{"202511", "1.234,56", "-65,800.13"}}}
	_, err := icmal.DetectNumberFormat(tbl)
	require.ErrorIs(t, err, icmal.ErrMixedNumberFormats)
	_, err = icmal.Parse(tbl)
	require.ErrorIs(t, err, icmal.ErrMixedNumberFormats)
}

func TestParseNumberTurkish(t *testing.T) {
	for in, want := range map[string]string{"1.234,56": "1234.56", "0,125": "0.125", "1.234": "1234", "-1.234,56": "-1234.56", `"12,5"`: "12.5"} {
		got, err := icmal.ParseNumber(in, icmal.FormatTurkish)
		require.NoError(t, err, in)
		require.True(t, got.Equal(d(want)), "%s → %s", in, got)
	}
	for _, empty := range []string{"", " - ", "BOŞ", "boş"} {
		got, err := icmal.ParseNumber(empty, icmal.FormatTurkish)
		require.NoError(t, err)
		require.Nil(t, got, empty)
	}
	_, err := icmal.ParseNumber("12a", icmal.FormatTurkish)
	require.Error(t, err)
}

func TestParseNumberUS(t *testing.T) {
	for in, want := range map[string]string{"-65,800.13": "-65800.13", `"53,337.25"`: "53337.25", "0.00": "0", "1,590,855.00": "1590855"} {
		got, err := icmal.ParseNumber(in, icmal.FormatUS)
		require.NoError(t, err, in)
		require.True(t, got.Equal(d(want)), "%s → %s", in, got)
	}
}

func TestNormaliseHeaderTurkishDotlessI(t *testing.T) {
	require.Equal(t, icmal.NormaliseHeader("Fatura İptal Mi?"), icmal.NormaliseHeader("FATURA IPTAL MI"))
	require.Equal(t, "faturaiptalmi", icmal.NormaliseHeader("Fatura İptal Mi?"))
	require.Equal(t, "toplamdagitimbedeli", icmal.NormaliseHeader("Toplam Dağıtım Bedeli"))
	require.Equal(t, "trtbedeli", icmal.NormaliseHeader("Trt Bedeli "))
}

func TestParseRealFixtureRowCountAndTotals(t *testing.T) {
	p := parsed(t)
	require.Len(t, p.Rows, 45)
	require.Empty(t, p.Warnings)
	kwh, payable := decimal.Zero, decimal.Zero
	for _, r := range p.Rows {
		kwh = kwh.Add(*r.TotalKwh)
		var raw map[string]string
		require.NoError(t, json.Unmarshal(r.Raw, &raw))
		v, err := icmal.ParseNumber(raw["Fatura Tutarı"], p.Format)
		require.NoError(t, err)
		payable = payable.Add(*v)
	}
	require.Equal(t, "1175379.55", kwh.StringFixed(2))
	require.Equal(t, "16670670.00", payable.StringFixed(2))

	line31 := p.Rows[29]
	require.Equal(t, "202511", line31.Period)
	require.Equal(t, "40ZTEST000000030", *line31.EtsoCode)
	require.Equal(t, "13011.01", line31.PowerCharge.String())
	require.Equal(t, "441.6", line31.DemandKw.String())
	require.Equal(t, "Çift Terimli", *line31.Term)
	require.Equal(t, "OG", *line31.VoltageLevel)
	require.Equal(t, "164005.69", line31.Vat.String())
	require.Equal(t, "169833.84", line31.T0Kwh.String(), "the first of the duplicate T0 Kwh headers wins")
}

func TestParseTimeTypeFromColumnNotBands(t *testing.T) {
	p := parsed(t)
	for _, r := range p.Rows {
		require.NotNil(t, r.IsMultiTime)
		require.False(t, *r.IsMultiTime, "every real row says Tek Zamanlı even with T1–T3 kWh (R133)")
	}
	require.True(t, p.Rows[29].T1Kwh.IsPositive())

	tbl := icmal.Table{Header: []string{"Muhasebe Dönemi", "Toplam Kwh", "TekZaman/Üç Zaman"}, Rows: [][]string{{"202511", "1.0", "Üç Zamanlı"}}}
	multi, err := icmal.Parse(tbl)
	require.NoError(t, err)
	require.True(t, *multi.Rows[0].IsMultiTime)
	absent, err := icmal.Parse(icmal.Table{Header: []string{"Muhasebe Dönemi", "Toplam Kwh", "T1 Kwh"}, Rows: [][]string{{"202511", "1.0", "5.0"}}})
	require.NoError(t, err)
	require.Nil(t, absent.Rows[0].IsMultiTime)
}

func TestParseRequiresPeriodAndTotalKwh(t *testing.T) {
	_, err := icmal.Parse(icmal.Table{Header: []string{"Muhasebe Dönemi"}})
	require.ErrorIs(t, err, icmal.ErrMissingColumn)
}

func TestParseCancelledRowsExcluded(t *testing.T) {
	tbl := fixture(t)
	cancel := -1
	for i, h := range tbl.Header {
		if h == "Fatura İptal Mi?" {
			cancel = i
		}
	}
	tbl.Rows[1][cancel] = "EVET" // line 3
	p, err := icmal.Parse(tbl)
	require.NoError(t, err)
	require.True(t, p.Rows[1].IsCancelled)
	a := byEtso(icmal.Analyse(p.Rows, synthBase, nil, d("2")), "40ZTEST000000002")
	require.Equal(t, map[string][]int{icmal.WarnCancelledExcluded: {2}}, codes(a.Warnings))
	require.Zero(t, a.DistributionTlPerKwh.Samples)
}

func TestAnalyseExcludesSupplementaryAndZeroKwhRows(t *testing.T) {
	as := icmal.Analyse(parsed(t).Rows, synthBase, nil, d("2"))
	for etso, want := range map[string]string{"40ZTEST000000001": "0.8106", "40ZTEST000000036": "0.8106", "40ZTEST000000037": "0.8954"} {
		a := byEtso(as, etso)
		require.Equal(t, want, a.DistributionTlPerKwh.Value.StringFixed(4), etso)
		require.Equal(t, 1, a.DistributionTlPerKwh.Samples, etso)
		w := codes(a.Warnings)
		require.Len(t, w[icmal.WarnSupplementaryExcluded], 1, etso)
		require.Len(t, w[icmal.WarnZeroKwhExcluded], 1, etso)
	}
	require.Equal(t, []int{35}, codes(byEtso(as, "40ZTEST000000001").Warnings)[icmal.WarnSupplementaryExcluded])

	// An (Ek) group row is excluded on its label alone, even with positive kWh.
	ek := synthRow("EK", "202511", "100", "250", "80")
	ek.Raw = json.RawMessage(`{"Fatura Abone Grubu":"Sanayi Tarifesi (Ek)","Ek Tüketim T0 Kwh":"0.00"}`)
	sup := byEtso(icmal.Analyse([]model.IcmalRow{ek}, synthBase, nil, d("2")), "EK")
	require.Equal(t, map[string][]int{icmal.WarnSupplementaryExcluded: {1}}, codes(sup.Warnings))
	ek.Raw = json.RawMessage(`{"Fatura Abone Grubu":"Sanayi Tarifesi","Ek Tüketim T0 Kwh":"12.50"}`)
	sup = byEtso(icmal.Analyse([]model.IcmalRow{ek}, synthBase, nil, d("2")), "EK")
	require.Equal(t, map[string][]int{icmal.WarnSupplementaryExcluded: {1}}, codes(sup.Warnings), "supplementary kWh alone")
}

func synthRow(etso, period string, kwh, energy, distribution string) model.IcmalRow {
	return model.IcmalRow{Period: period, EtsoCode: &etso, TotalKwh: dp(kwh), EnergyCharge: dp(energy), DistributionCharge: dp(distribution), Raw: json.RawMessage(`{}`)}
}

func TestAnalyseMedianAndPopulationStdDev(t *testing.T) {
	rows := []model.IcmalRow{synthRow("X", "202501", "1", "2.5", "1"), synthRow("X", "202502", "1", "2.5", "2"), synthRow("X", "202503", "1", "2.5", "4")}
	a := icmal.Analyse(rows, map[string]decimal.Decimal{"202501": d("1"), "202502": d("1"), "202503": d("1")}, nil, d("2"))[0]
	c := a.DistributionTlPerKwh
	require.Equal(t, "2", c.Value.String())
	require.Equal(t, 3, c.Samples)
	require.Equal(t, "1.2472191289", c.StdDev.String(), "population: sqrt(14/9)")
	require.False(t, *c.Stable)
	require.Equal(t, []string{"202501", "202502", "202503"}, a.Periods)
	require.Equal(t, "2.5", a.EnergyKbk.Value.String())
	require.True(t, *a.EnergyKbk.Stable)

	even := icmal.Analyse(rows[:2], map[string]decimal.Decimal{"202501": d("1"), "202502": d("1")}, nil, d("2"))[0]
	require.Equal(t, "1.5", even.DistributionTlPerKwh.Value.String(), "even count: mean of the middle two")
	single := icmal.Analyse(rows[:1], nil, nil, d("2"))[0]
	require.Nil(t, single.DistributionTlPerKwh.Stable)
	require.Nil(t, single.EnergyKbk.Value, "no base price")
	require.Equal(t, map[string][]int{icmal.WarnBasePriceMissing: {1}}, codes(single.Warnings))
}

func TestAnalysePowerUnitPriceFromOverrunRow(t *testing.T) {
	a := byEtso(icmal.Analyse(parsed(t).Rows, synthBase, nil, d("2")), "40ZTEST000000030")
	require.Equal(t, "43.3700", a.PowerUnitPrice.Value.StringFixed(4))
	require.Equal(t, "300", a.ImpliedContractedPowerKw.String())
	require.True(t, a.OveruseRequiresManualEntry)

	noContract := byEtso(icmal.Analyse(parsed(t).Rows, synthBase, nil, d("2")), "40ZTEST000000009")
	require.Nil(t, noContract.PowerUnitPrice.Value)
	require.Equal(t, []int{9}, codes(noContract.Warnings)[icmal.WarnPowerManualEntry])
}

func TestAnalysePowerUnitPriceFromContractedPower(t *testing.T) {
	a := byEtso(icmal.Analyse(parsed(t).Rows, synthBase, map[string]decimal.Decimal{"40ZTEST000000009": d("400")}, d("2")), "40ZTEST000000009")
	require.Equal(t, "43.3700", a.PowerUnitPrice.Value.StringFixed(4))
	require.Equal(t, "400", a.ImpliedContractedPowerKw.Round(0).String())
}

func TestAnalyseBTVIsPercentOfEnergy(t *testing.T) {
	as := icmal.Analyse(parsed(t).Rows, synthBase, nil, d("2"))
	require.Equal(t, "5.00", byEtso(as, "40ZTEST000000002").Taxes[icmal.TaxBTV].Value.StringFixed(2))
	require.Equal(t, "1.00", byEtso(as, "40ZTEST000000001").Taxes[icmal.TaxBTV].Value.StringFixed(2), "industrial ETSO")
	require.NotContains(t, byEtso(as, "40ZTEST000000002").Taxes, icmal.TaxTRT, "zero columns derive nothing")
	require.Equal(t, "20.00", byEtso(as, "40ZTEST000000002").VatRate.Value.StringFixed(2))
}

func TestAnalyseReactiveUnitPriceFromRealRows(t *testing.T) {
	rows := parsed(t).Rows
	as := icmal.Analyse(rows, synthBase, nil, d("2"))
	for _, etso := range []string{"40ZTEST000000014", "40ZTEST000000017", "40ZTEST000000020", "40ZTEST000000024", "40ZTEST000000028", "40ZTEST000000032"} {
		a := byEtso(as, etso)
		require.NotNil(t, a.ReactiveUnitPrice.Value, etso)
		require.Equal(t, "2.6455", a.ReactiveUnitPrice.Value.StringFixed(4), etso)
	}
	require.Equal(t, []int{19}, codes(byEtso(as, "40ZTEST000000019").Warnings)[icmal.WarnReactiveQuantity])

	odd := synthRow("ODD", "202511", "100", "250", "80")
	odd.ReactiveCharge, odd.InductiveKvarh = dp("100"), dp("10")
	withOdd := icmal.Analyse(append(rows, odd), synthBase, nil, d("2"))
	require.Equal(t, []int{46}, codes(byEtso(withOdd, "ODD").Warnings)[icmal.WarnReactiveAmbiguous])
	require.Equal(t, "2.6455", byEtso(withOdd, "40ZTEST000000014").ReactiveUnitPrice.Value.StringFixed(4))
}

func TestAnalyseBackCalculatesWithMedianNotPerRow(t *testing.T) {
	base := map[string]decimal.Decimal{"202501": d("2"), "202502": d("2"), "202503": d("2")}
	rows := []model.IcmalRow{
		synthRow("X", "202501", "100", "200", "1"), // kbk 1.0
		synthRow("X", "202502", "100", "200", "1"), // kbk 1.0
		synthRow("X", "202503", "100", "300", "1"), // kbk 1.5 outlier
	}
	a := icmal.Analyse(rows, base, nil, d("2"))[0]
	require.Equal(t, "1", a.EnergyKbk.Value.String())
	require.Equal(t, "33.33", a.EnergyKbk.BackCalcErrorPct.StringFixed(2), "|200 − 300| / 300 with the median KBK")
	require.False(t, a.WithinTolerance)
}

func TestAnalyseRealFixtureWithinTwoPercent(t *testing.T) {
	rows := parsed(t).Rows
	second := rows[1] // line 3, 40ZTEST000000002, as a synthetic 202512 period with a 0.5 % price drift
	second.Period = "202512"
	energy := second.EnergyCharge.Mul(d("2.6")).DivRound(d("2.5"), 20).Mul(d("1.005")).Round(2)
	second.EnergyCharge = &energy
	as := icmal.Analyse(append(rows, second), synthBase, nil, d("2"))

	multi := byEtso(as, "40ZTEST000000002")
	require.Equal(t, []string{"202511", "202512"}, multi.Periods)
	require.Equal(t, 2, multi.EnergyKbk.Samples)
	require.True(t, multi.EnergyKbk.BackCalcErrorPct.LessThan(d("2")))
	require.Equal(t, "1.8774", multi.DistributionTlPerKwh.Value.StringFixed(4))
	for _, a := range as {
		require.True(t, a.WithinTolerance, a.EtsoCode)
	}
	require.Equal(t, "1.5758", byEtso(as, "40ZTEST000000003").DistributionTlPerKwh.Value.StringFixed(4))
	require.Equal(t, "1.2633", byEtso(as, "40ZTEST000000005").DistributionTlPerKwh.Value.StringFixed(4))
}
