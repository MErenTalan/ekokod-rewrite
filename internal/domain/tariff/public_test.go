package tariff_test

import (
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/tariff"
)

func pd(s string) *decimal.Decimal { v := decimal.RequireFromString(s); return &v }

// schedule is the shipped legacy table's rows this test uses (seed data, F12a Q-H1).
var schedule = []model.NationalTariffScheduleEntry{
	{UserGroup: "commercial", VoltageLevel: "mv", Term: "binomial", EnergyPrice: *pd("3.307886"), T1Price: pd("3.307886"), T2Price: pd("5.672128"),
		T3Price: pd("1.426752"), DistributionPrice: *pd("1.668345"), PowerPrice: pd("89.147520"), OverusePrice: pd("178.295040"), VatRate: *pd("20")},
	{UserGroup: "residential", VoltageLevel: "lv", Term: "monomial", EnergyPrice: *pd("0.494065"), T1Price: pd("1.950640"), T2Price: pd("3.747551"),
		T3Price: pd("0.513656"), DistributionPrice: *pd("2.424900"), DailyThresholdKwh: pd("8"), VatRate: *pd("20")},
	{UserGroup: "residential_plus", VoltageLevel: "lv", Term: "monomial", EnergyPrice: *pd("1.895808"), T1Price: pd("1.950640"), T2Price: pd("3.747551"),
		T3Price: pd("0.513656"), DistributionPrice: *pd("2.424900"), VatRate: *pd("20")},
	{UserGroup: "commercial", VoltageLevel: "lv", Term: "monomial", EnergyPrice: *pd("2.873087"), T1Price: pd("3.502620"), T2Price: pd("5.973903"),
		T3Price: pd("1.536314"), DistributionPrice: *pd("2.479368"), DailyThresholdKwh: pd("30"), VatRate: *pd("20")},
	{UserGroup: "lighting", VoltageLevel: "lv", Term: "monomial", EnergyPrice: *pd("3.365818"), DistributionPrice: *pd("2.374690"), VatRate: *pd("20")},
}

func pick(group model.DistributionUserGroup, level model.VoltageLevel, term model.TariffTerm) (model.NationalTariffScheduleEntry, bool) {
	for _, r := range schedule {
		if r.UserGroup == group && r.VoltageLevel == level && r.Term == term {
			return r, true
		}
	}
	return model.NationalTariffScheduleEntry{}, false
}

func janDays(days int) (time.Time, time.Time) {
	return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 1, days, 0, 0, 0, 0, time.UTC)
}

func requireMoney(t *testing.T, want string, got decimal.Decimal, name string) {
	t.Helper()
	require.True(t, decimal.RequireFromString(want).Equal(got), "%s: want %s got %s", name, want, got)
}

func requireField(t *testing.T, err error, field, code string) {
	t.Helper()
	var pe *tariff.PublicError
	require.True(t, errors.As(err, &pe), "want %s/%s, got %v", field, code, err)
	require.Equal(t, field, pe.Field)
	require.Equal(t, code, pe.Code)
}

// TestPublicCalculator is 09 §F12's hand-computed example, power charge included.
func TestPublicCalculator(t *testing.T) {
	start, end := janDays(30)
	r, err := tariff.PublicBill(tariff.PublicInput{Group: "commercial", Level: "mv", Term: "binomial", Start: start, End: end,
		Total: pd("10000"), ContractPower: pd("100"), Demand: pd("120")}, pick)
	require.NoError(t, err)
	requireMoney(t, "33078.86", r.Energy, "energy = 10000 × 3.307886")
	requireMoney(t, "16683.45", r.Distribution, "distribution = 10000 × 1.668345")
	requireMoney(t, "8914.75", r.Power, "power = 89.14752 × 100 kW × 30/30")
	requireMoney(t, "3565.90", r.Overuse, "overuse = 20 kW × 178.29504")
	requireMoney(t, "62242.96", r.VatBase, "base")
	requireMoney(t, "12448.59", r.Vat, "VAT 20%")
	requireMoney(t, "74691.56", r.Total, "total")
	require.Equal(t, 30, r.Days)
	require.Equal(t, "total", r.Basis)

	half, err := tariff.PublicBill(tariff.PublicInput{Group: "commercial", Level: "mv", Term: "binomial", Start: start, End: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
		Total: pd("0"), ContractPower: pd("100")}, pick)
	require.NoError(t, err)
	requireMoney(t, "4457.38", half.Power, "15 days of a monthly power price: 89.14752 × 100 × 15/30")
	requireMoney(t, "0", half.Overuse, "no demand over the contract")
}

func TestThresholdSwitchesToThePlusRow(t *testing.T) {
	start, end := janDays(30)
	high, err := tariff.PublicBill(tariff.PublicInput{Group: "residential", Level: "lv", Term: "monomial", Start: start, End: end, Total: pd("300")}, pick)
	require.NoError(t, err)
	require.Equal(t, model.DistributionUserGroup("residential_plus"), high.GroupUsed, "10 kWh/day > 8")
	requireMoney(t, "568.74", high.Energy, "300 × 1.895808")
	requireMoney(t, "727.47", high.Distribution, "300 × 2.4249")
	requireMoney(t, "1555.45", high.Total, "(568.7424 + 727.47) × 1.2")

	low, err := tariff.PublicBill(tariff.PublicInput{Group: "residential", Level: "lv", Term: "monomial", Start: start, End: end, Total: pd("240")}, pick)
	require.NoError(t, err)
	require.Equal(t, model.DistributionUserGroup("residential"), low.GroupUsed, "8 kWh/day is not above the threshold")
	requireMoney(t, "118.58", low.Energy, "240 × 0.494065")
}

func TestTotalAndBandsRule(t *testing.T) {
	start, end := janDays(30)
	multi, err := tariff.PublicBill(tariff.PublicInput{Group: "commercial", Level: "lv", Term: "monomial", MultiTime: true, Start: start, End: end,
		T1: pd("100"), T2: pd("50"), T3: pd("50")}, pick)
	require.NoError(t, err)
	requireMoney(t, "725.77", multi.Energy, "100×3.50262 + 50×5.973903 + 50×1.536314")
	requireMoney(t, "495.87", multi.Distribution, "200 × 2.479368")
	require.Equal(t, "bands", multi.Basis)

	_, err = tariff.PublicBill(tariff.PublicInput{Group: "commercial", Level: "lv", Term: "monomial", MultiTime: true, Start: start, End: end,
		Total: pd("250"), T1: pd("100"), T2: pd("50"), T3: pd("50")}, pick)
	requireField(t, err, "total_consumption", "mismatch")
	agree, err := tariff.PublicBill(tariff.PublicInput{Group: "commercial", Level: "lv", Term: "monomial", MultiTime: true, Start: start, End: end,
		Total: pd("200"), T1: pd("100"), T2: pd("50"), T3: pd("50")}, pick)
	require.NoError(t, err)
	require.True(t, agree.Energy.Equal(multi.Energy), "an agreeing total changes nothing")

	_, err = tariff.PublicBill(tariff.PublicInput{Group: "commercial", Level: "lv", Term: "monomial", MultiTime: true, Start: start, End: end, T1: pd("100")}, pick)
	requireField(t, err, "t2", "required")

	single, err := tariff.PublicBill(tariff.PublicInput{Group: "commercial", Level: "lv", Term: "monomial", Start: start, End: end, T1: pd("100"), T2: pd("50"), T3: pd("50")}, pick)
	require.NoError(t, err)
	requireMoney(t, "574.62", single.Energy, "single-time with only bands: their sum × 2.873087")
	_, err = tariff.PublicBill(tariff.PublicInput{Group: "commercial", Level: "lv", Term: "monomial", Start: start, End: end, Total: pd("300"), T1: pd("100")}, pick)
	requireField(t, err, "total_consumption", "mismatch")
}

func TestPublicRefusals(t *testing.T) {
	start, end := janDays(30)
	for name, c := range map[string]struct {
		in          tariff.PublicInput
		field, code string
	}{
		"unknown group":        {tariff.PublicInput{Group: "martyrs_families", Level: "lv", Term: "monomial", Start: start, End: end}, "group", "invalid"},
		"binomial on lv":       {tariff.PublicInput{Group: "commercial", Level: "lv", Term: "binomial", Start: start, End: end}, "term", "invalid"},
		"end before start":     {tariff.PublicInput{Group: "commercial", Level: "lv", Term: "monomial", Start: end, End: start}, "end", "invalid_range"},
		"over 366 days":        {tariff.PublicInput{Group: "commercial", Level: "lv", Term: "monomial", Start: start, End: start.AddDate(1, 0, 1)}, "end", "range_too_long"},
		"negative":             {tariff.PublicInput{Group: "commercial", Level: "lv", Term: "monomial", Start: start, End: end, Total: pd("-1")}, "total_consumption", "out_of_range"},
		"no row":               {tariff.PublicInput{Group: "industrial", Level: "lv", Term: "monomial", Start: start, End: end}, "start", "no_tariff"},
		"lighting multi-time":  {tariff.PublicInput{Group: "lighting", Level: "lv", Term: "monomial", MultiTime: true, Start: start, End: end, T1: pd("1"), T2: pd("1"), T3: pd("1")}, "multi_time", "not_available"},
		"binomial no contract": {tariff.PublicInput{Group: "commercial", Level: "mv", Term: "binomial", Start: start, End: end, Total: pd("1")}, "contract_power", "required"},
	} {
		_, err := tariff.PublicBill(c.in, pick)
		requireField(t, err, c.field, c.code)
		_ = name
	}
}
