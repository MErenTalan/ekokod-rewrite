package solar_test

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/isolar"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/solar"
)

var ist = func() *time.Location { l, _ := time.LoadLocation("Europe/Istanbul"); return l }()

func d(v string) *decimal.Decimal { x := decimal.RequireFromString(v); return &x }

func at(day, hour int) time.Time { return time.Date(2026, time.March, day, hour, 0, 0, 0, ist) }

func requireDec(t *testing.T, want string, got *decimal.Decimal, msg string) {
	t.Helper()
	if want == "" {
		require.Nil(t, got, msg)
		return
	}
	require.NotNil(t, got, msg)
	require.True(t, decimal.RequireFromString(want).Equal(*got), "%s: want %s got %s", msg, want, got)
}

// R277: yield-today is cumulative; a reset is never negative production.
func TestIntervalsResetAndGap(t *testing.T) {
	samples := []isolar.YieldSample{
		{Ts: at(10, 3), YieldTodayKwh: d("5.0")},
		{Ts: at(10, 0), YieldTodayKwh: d("1.0")},
		{Ts: at(10, 1), YieldTodayKwh: d("3.5"), ActivePowerKw: d("2.4")},
		{Ts: at(10, 2), YieldTodayKwh: nil},
		{Ts: at(10, 4), YieldTodayKwh: d("0.2")},
	}
	out, resets := solar.Intervals(samples)
	require.Equal(t, 1, resets)
	require.Len(t, out, 5)
	for i, want := range []string{"1.0", "2.5", "", "1.5", ""} {
		requireDec(t, want, out[i].Kwh, out[i].Ts.String())
	}
	requireDec(t, "2.4", out[1].PowerKw, "power passes through")
}

func TestIntervalsRestartEachIstanbulDay(t *testing.T) {
	out, resets := solar.Intervals([]isolar.YieldSample{
		{Ts: at(10, 23), YieldTodayKwh: d("40")},
		{Ts: at(11, 0), YieldTodayKwh: d("0.5")},
		{Ts: at(11, 1), YieldTodayKwh: d("1.5")},
	})
	require.Zero(t, resets, "a new day starts from zero, not a reset")
	requireDec(t, "40", out[0].Kwh, "first of day 10")
	requireDec(t, "0.5", out[1].Kwh, "first of day 11")
	requireDec(t, "1.0", out[2].Kwh, "delta")
}

func TestSumByTimestamp(t *testing.T) {
	a := []solar.Interval{{Ts: at(10, 1), Kwh: d("1")}, {Ts: at(10, 2), Kwh: nil}}
	b := []solar.Interval{{Ts: at(10, 1), Kwh: d("2"), PowerKw: d("3")}, {Ts: at(10, 2), Kwh: nil}, {Ts: at(10, 3), Kwh: d("4")}}
	out := solar.SumByTimestamp(a, b)
	require.Len(t, out, 3)
	requireDec(t, "3", out[0].Kwh, "sum")
	requireDec(t, "3", out[0].PowerKw, "power sum of known")
	requireDec(t, "", out[1].Kwh, "all missing stays missing")
	requireDec(t, "4", out[2].Kwh, "one series")
}

func daysOf(from time.Time, n int, kwh string) map[time.Time]decimal.Decimal {
	out := map[time.Time]decimal.Decimal{}
	for i := range n {
		out[from.AddDate(0, 0, i)] = decimal.RequireFromString(kwh)
	}
	return out
}

// R283: the tariff effective each day prices that day.
func TestRevenueTariffChangeMidMonth(t *testing.T) {
	days := daysOf(at(1, 0), 20, "10")
	out, unpriced := solar.RevenueOver(days, []solar.DayTariff{
		{From: at(11, 0), Price: decimal.RequireFromString("2.50"), Currency: model.CurrencyTRY},
		{From: at(1, 0), Price: decimal.RequireFromString("2.00"), Currency: model.CurrencyTRY},
	})
	require.Zero(t, unpriced)
	require.Len(t, out, 1)
	require.Equal(t, model.CurrencyTRY, out[0].Currency)
	require.True(t, decimal.RequireFromString("450.00").Equal(out[0].Amount), "got %s", out[0].Amount)
}

func TestRevenueTwoCurrencies(t *testing.T) {
	days := daysOf(at(1, 0), 4, "1")
	out, _ := solar.RevenueOver(days, []solar.DayTariff{
		{From: at(1, 0), Price: decimal.RequireFromString("2"), Currency: model.CurrencyTRY},
		{From: at(3, 0), Price: decimal.RequireFromString("0.1"), Currency: model.CurrencyUSD},
	})
	require.Len(t, out, 2)
	require.Equal(t, model.CurrencyTRY, out[0].Currency)
	require.True(t, decimal.RequireFromString("4").Equal(out[0].Amount))
	require.Equal(t, model.CurrencyUSD, out[1].Currency)
	require.True(t, decimal.RequireFromString("0.2").Equal(out[1].Amount))
}

func TestRevenueUnpricedDay(t *testing.T) {
	days := daysOf(at(1, 0), 3, "1")
	out, unpriced := solar.RevenueOver(days, []solar.DayTariff{{From: at(2, 0), Price: decimal.RequireFromString("1"), Currency: model.CurrencyTRY}})
	require.Equal(t, 1, unpriced)
	require.True(t, decimal.RequireFromString("2").Equal(out[0].Amount))
}

func TestRevenueRoundsOnceAtTheEnd(t *testing.T) {
	days := daysOf(at(1, 0), 3, "1.001")
	out, _ := solar.RevenueOver(days, []solar.DayTariff{{From: at(1, 0), Price: decimal.RequireFromString("1.005"), Currency: model.CurrencyTRY}})
	require.Equal(t, "3.02", out[0].Amount.StringFixed(2), "3 × 1.001 × 1.005 = 3.018015 → 3.02")
}

func TestTranslateFaultExactAndFallback(t *testing.T) {
	got, ok := solar.TranslateFault("电网掉电")
	require.True(t, ok)
	require.Equal(t, "şebeke kesintisi", got)
	got, ok = solar.TranslateFault("xx电网yy")
	require.True(t, ok)
	require.Equal(t, "şebeke problemi", got)
	got, ok = solar.TranslateFault("Grid Overvoltage")
	require.False(t, ok)
	require.Equal(t, "Grid Overvoltage", got, "unknown text is kept, marked untranslated")
}

func TestStatusOf(t *testing.T) {
	now := at(10, 12)
	s := func(v int32) *int32 { return &v }
	recent := now.Add(-time.Hour)
	old := now.Add(-3 * time.Hour)
	require.Nil(t, solar.StatusOf(s(4), nil, now))
	for status, want := range map[int32]string{4: "normal", 2: "alarm", 1: "fault", 9: "offline"} {
		require.Equal(t, want, *solar.StatusOf(s(status), &recent, now))
	}
	require.Equal(t, "offline", *solar.StatusOf(s(4), &old, now), "stale snapshot")
}

func TestUtilisationNilAndZero(t *testing.T) {
	requireDec(t, "50.0", solar.Utilisation(d("5.0"), d("10")), "5 of 10 kW")
	requireDec(t, "33.3", solar.Utilisation(d("1"), d("3")), "1 dp")
	requireDec(t, "", solar.Utilisation(nil, d("10")), "no power")
	requireDec(t, "", solar.Utilisation(d("1"), d("0")), "zero capacity")
	requireDec(t, "", solar.Utilisation(d("1"), nil), "no capacity")
}
