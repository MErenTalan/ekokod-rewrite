package tariff_test

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/tariff"
)

// skttMarket gives every hour of each day from first the day's PTF with a
// matched quantity of 1, so the day's weighted PTF is that value.
func skttMarket(loc *time.Location, first time.Time, daily []string) tariff.Market {
	m := tariff.Market{PTF: map[time.Time]decimal.Decimal{}, Quantity: map[time.Time]decimal.Decimal{},
		Yekdem: map[tariff.YearMonth]decimal.Decimal{}, ManualYekdem: map[tariff.YearMonth]decimal.Decimal{},
		YekdemForecast: map[tariff.YearMonth]decimal.Decimal{}}
	for i, v := range daily {
		day := first.AddDate(0, 0, i)
		for h := day; h.Before(day.AddDate(0, 0, 1)); h = h.Add(time.Hour) {
			m.PTF[h.UTC()], m.Quantity[h.UTC()] = d(v), d("1")
		}
	}
	return m
}

func skttTariff() model.Tariff {
	t := ptfTariff()
	t.SupplyCompany = model.SupplyCompanyIncumbent
	t.KbkEnergy = dp("1.0938")
	return t
}

// TestPriceSKTTMatchesMepasOctober2026 replays MEPAŞ bill MEP2026001236645
// (reads 16.09 → 17.10.2026): the Tebliğ formula with EPİAŞ's daily
// weighted PTF and the EPDK forecast YEKDEM gives 3.616905 TL/kWh against
// the bill's 3.616873 (6.096273 all-in − 2.4794 distribution).
func TestPriceSKTTMatchesMepasOctober2026(t *testing.T) {
	loc := istanbul(t)
	first := time.Date(2026, 9, 16, 0, 0, 0, 0, loc)
	m := skttMarket(loc, first, []string{"2908.95", "3232.26", "3161.27", "2735.86", "1850.45", "3275.72", "3080.33", "2819.17",
		"3248.76", "2992.93", "2625.47", "1472.39", "2264.67", "2819.57", "2468.15", "2540.28", "2246.79", "1617.37", "1749.78",
		"2661.53", "2910.61", "3177.25", "3076.58", "2988.27", "3179.39", "2392.56", "3538.33", "3426.35", "3094.28", "2931.67",
		"3055.41", "2617.71"})
	m.YekdemForecast[tariff.YearMonth{Year: 2026, Month: time.September}] = d("581.14")
	m.YekdemForecast[tariff.YearMonth{Year: 2026, Month: time.October}] = d("526.39")
	m.Yekdem[tariff.YearMonth{Year: 2026, Month: time.September}] = d("444.994") // published actual: not used

	w := energy.Window{From: first, To: time.Date(2026, 10, 17, 0, 0, 0, 0, loc)}
	p, err := tariff.Price(skttTariff(), w, nil, true, m, seedParams(), loc)
	require.NoError(t, err)
	require.Equal(t, tariff.MethodSKTT, p.Method)
	require.True(t, p.Complete, p.Reason)
	require.Equal(t, "3.616905", p.EnergyUnitPrice.Round(6).StringFixed(6))
	require.Equal(t, "2754.73", p.PTFAverage.Round(2).StringFixed(2))
	require.Equal(t, "552.00", p.YekdemUsed.Round(2).StringFixed(2))
}

// TestPriceSKTTHalfWeightsTheReadDays: the two read dates count half.
func TestPriceSKTTHalfWeightsTheReadDays(t *testing.T) {
	loc := istanbul(t)
	first := time.Date(2026, 1, 1, 0, 0, 0, 0, loc)
	m := skttMarket(loc, first, []string{"1000", "2000", "4000"})
	m.YekdemForecast[tariff.YearMonth{Year: 2026, Month: time.January}] = d("0")
	tr := skttTariff()
	tr.KbkEnergy = dp("1")
	// reads on 01.01 15:00 and 03.01 15:00 → (0.5·1000 + 2000 + 0.5·4000) / 2
	w := energy.Window{From: first.Add(15 * time.Hour), To: first.AddDate(0, 0, 2).Add(15 * time.Hour)}
	p, err := tariff.Price(tr, w, nil, false, m, seedParams(), loc)
	require.NoError(t, err)
	require.Equal(t, "2250", p.PTFAverage.String())
	require.Equal(t, "2.25", p.EnergyUnitPrice.String())
}

// TestPriceSKTTDailyAverageIsVolumeWeightedAndRounded: a day's PTF is
// Σ quantity·price / Σ quantity, rounded to kuruş like EPİAŞ publishes it.
func TestPriceSKTTDailyAverageIsVolumeWeightedAndRounded(t *testing.T) {
	loc := istanbul(t)
	first := time.Date(2026, 1, 1, 0, 0, 0, 0, loc)
	m := skttMarket(loc, first, []string{"1000", "1000"})
	m.PTF[first.UTC()], m.Quantity[first.UTC()] = d("1001"), d("2") // (1001·2 + 1000·23)/25 = 1000.08
	m.YekdemForecast[tariff.YearMonth{Year: 2026, Month: time.January}] = d("0")
	tr := skttTariff()
	tr.KbkEnergy = dp("1")
	w := energy.Window{From: first, To: first.AddDate(0, 0, 1)}
	p, err := tariff.Price(tr, w, nil, false, m, seedParams(), loc)
	require.NoError(t, err)
	require.Equal(t, "1000.04", p.PTFAverage.String()) // (1000.08 + 1000) / 2
}

// TestPriceSKTTForecastYekdemMissingIsYekdemMissing: no forecast and no
// published value → yekdem_missing, never PTF missing.
func TestPriceSKTTYekdemFallsBackToPublishedThenFlags(t *testing.T) {
	loc := istanbul(t)
	first := time.Date(2026, 1, 1, 0, 0, 0, 0, loc)
	m := skttMarket(loc, first, []string{"1000", "1000"})
	w := energy.Window{From: first, To: first.AddDate(0, 0, 1)}
	p, err := tariff.Price(skttTariff(), w, nil, false, m, seedParams(), loc)
	require.NoError(t, err)
	require.Equal(t, tariff.ReasonYekdemMissing, p.Reason)
	require.Nil(t, p.EnergyUnitPrice)

	jan(m, "500")
	p, err = tariff.Price(skttTariff(), w, nil, false, m, seedParams(), loc)
	require.NoError(t, err)
	require.True(t, p.Complete)
	require.Equal(t, "500", p.YekdemUsed.String())
}

// TestPriceSKTTDayWithoutQuantityIsMissing: a day with an hour lacking its
// matched quantity has no weighted PTF; over the tolerance → incomplete.
func TestPriceSKTTDayWithoutQuantityIsMissing(t *testing.T) {
	loc := istanbul(t)
	first := time.Date(2026, 1, 1, 0, 0, 0, 0, loc)
	m := skttMarket(loc, first, []string{"1000", "1000", "1000"})
	m.YekdemForecast[tariff.YearMonth{Year: 2026, Month: time.January}] = d("0")
	delete(m.Quantity, first.AddDate(0, 0, 1).Add(5*time.Hour).UTC())
	w := energy.Window{From: first, To: first.AddDate(0, 0, 2)}
	p, err := tariff.Price(skttTariff(), w, nil, false, m, seedParams(), loc)
	require.NoError(t, err)
	require.Equal(t, tariff.ReasonOverTolerance, p.Reason)
}
