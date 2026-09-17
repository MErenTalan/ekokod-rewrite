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

type hourFn func(i int, h time.Time) (ptf, kwh *decimal.Decimal)

// build fills a market and consumption over n hours from from.
func build(from time.Time, n int, f hourFn) (energy.Window, tariff.Market, []tariff.HourConsumption) {
	m := tariff.Market{PTF: map[time.Time]decimal.Decimal{}, Yekdem: map[tariff.YearMonth]decimal.Decimal{}, ManualYekdem: map[tariff.YearMonth]decimal.Decimal{}}
	var hours []tariff.HourConsumption
	for i := 0; i < n; i++ {
		h := from.Add(time.Duration(i) * time.Hour).UTC()
		ptf, kwh := f(i, h)
		if ptf != nil {
			m.PTF[h] = *ptf
		}
		if kwh != nil {
			hours = append(hours, tariff.HourConsumption{Hour: h, Kwh: *kwh})
		}
	}
	return energy.Window{From: from, To: from.Add(time.Duration(n) * time.Hour)}, m, hours
}

func flat(ptf, kwh string) hourFn {
	return func(int, time.Time) (*decimal.Decimal, *decimal.Decimal) { return dp(ptf), dp(kwh) }
}

func jan(m tariff.Market, v string) {
	m.Yekdem[tariff.YearMonth{Year: 2026, Month: time.January}] = d(v)
}

func TestPriceRejectsNonPTFTariff(t *testing.T) {
	loc := istanbul(t)
	w, m, h := build(time.Date(2026, 1, 1, 0, 0, 0, 0, loc), 1, flat("1", "1"))
	_, err := tariff.Price(fixedTariff(), w, h, true, m, seedParams(), loc)
	require.ErrorIs(t, err, tariff.ErrNotPTF)
}

func TestPriceHourlyWeightsByConsumption(t *testing.T) {
	loc := istanbul(t)
	vals := [][2]string{{"1000", "10"}, {"2000", "20"}, {"3000", "30"}}
	w, m, h := build(time.Date(2026, 1, 1, 0, 0, 0, 0, loc), 3, func(i int, _ time.Time) (*decimal.Decimal, *decimal.Decimal) {
		return dp(vals[i][0]), dp(vals[i][1])
	})
	jan(m, "500")
	p, err := tariff.Price(ptfTariff(), w, h, true, m, seedParams(), loc)
	require.NoError(t, err)
	require.Equal(t, tariff.MethodHourly, p.Method)
	require.True(t, p.Complete)
	require.Len(t, p.Hourly, 3)
	sum := decimal.Zero
	for i, want := range []string{"1.65", "2.75", "3.85"} {
		require.Equal(t, want, p.Hourly[i].UnitPrice.String())
		sum = sum.Add(p.Hourly[i].Cost)
	}
	require.Equal(t, "187", sum.String())
	require.Equal(t, "3.11666666666666666667", p.EnergyUnitPrice.String())
	require.Equal(t, 3, p.HoursMatched)
}

func TestPriceHourlyUsesEachHoursOwnMonthYekdem(t *testing.T) {
	loc := istanbul(t)
	w, m, h := build(time.Date(2026, 1, 31, 23, 0, 0, 0, loc), 2, flat("1000", "10"))
	jan(m, "500")
	m.Yekdem[tariff.YearMonth{Year: 2026, Month: time.February}] = d("1000")
	tr := ptfTariff()
	tr.KbkEnergy = dp("1")
	p, err := tariff.Price(tr, w, h, true, m, seedParams(), loc)
	require.NoError(t, err)
	require.Equal(t, "1.5", p.Hourly[0].UnitPrice.String())
	require.Equal(t, "2", p.Hourly[1].UnitPrice.String())
	require.Equal(t, "1.75", p.EnergyUnitPrice.String())
	require.Equal(t, "750", p.YekdemUsed.String())
}

func TestPriceHourlyManualYekdemOverridesPublished(t *testing.T) {
	loc := istanbul(t)
	w, m, h := build(time.Date(2026, 1, 31, 23, 0, 0, 0, loc), 2, flat("1000", "10"))
	jan(m, "500")
	m.Yekdem[tariff.YearMonth{Year: 2026, Month: time.February}] = d("1000")
	m.ManualYekdem[tariff.YearMonth{Year: 2026, Month: time.January}] = d("700")
	tr := ptfTariff()
	tr.KbkEnergy = dp("1")
	tr.UseManualYekdem = true
	p, err := tariff.Price(tr, w, h, true, m, seedParams(), loc)
	require.NoError(t, err)
	require.Equal(t, "1.7", p.Hourly[0].UnitPrice.String(), "manual entry wins")
	require.Equal(t, "2", p.Hourly[1].UnitPrice.String(), "published when no manual entry for the month")

	tr.UseManualYekdem = false
	p, err = tariff.Price(tr, w, h, true, m, seedParams(), loc)
	require.NoError(t, err)
	require.Equal(t, "1.5", p.Hourly[0].UnitPrice.String(), "manual entries ignored unless enabled")
}

func TestPriceHourlyMissingHoursOverToleranceIsIncomplete(t *testing.T) {
	loc := istanbul(t)
	w, m, h := build(time.Date(2026, 1, 1, 0, 0, 0, 0, loc), 744, func(i int, _ time.Time) (*decimal.Decimal, *decimal.Decimal) {
		if i < 48 {
			return dp("1000"), nil
		}
		return dp("1000"), dp("1")
	})
	jan(m, "500")
	p, err := tariff.Price(ptfTariff(), w, h, true, m, seedParams(), loc)
	require.NoError(t, err)
	require.False(t, p.Complete)
	require.Equal(t, tariff.ReasonOverTolerance, p.Reason)
	require.Equal(t, 744, p.HoursExpected)
	require.Equal(t, 48, p.HoursMissingConsumption)
}

func TestPriceHourlyAtToleranceIsComplete(t *testing.T) {
	loc := istanbul(t)
	run := func(missing int) tariff.Pricing {
		w, m, h := build(time.Date(2026, 1, 1, 0, 0, 0, 0, loc), 100, func(i int, _ time.Time) (*decimal.Decimal, *decimal.Decimal) {
			if i < missing {
				return dp("1000"), nil
			}
			return dp("1000"), dp("1")
		})
		jan(m, "500")
		p, err := tariff.Price(ptfTariff(), w, h, true, m, seedParams(), loc)
		require.NoError(t, err)
		return p
	}
	require.True(t, run(2).Complete, "2 of 100 is exactly the 2% tolerance")
	require.False(t, run(3).Complete)
}

func TestPricePeriodAverageAtToleranceIsComplete(t *testing.T) {
	loc := istanbul(t)
	run := func(missing int) tariff.Pricing {
		w, m, _ := build(time.Date(2026, 1, 1, 0, 0, 0, 0, loc), 100, func(i int, _ time.Time) (*decimal.Decimal, *decimal.Decimal) {
			if i < missing {
				return nil, nil
			}
			return dp("1000"), nil
		})
		jan(m, "500")
		p, err := tariff.Price(ptfTariff(), w, nil, false, m, seedParams(), loc)
		require.NoError(t, err)
		return p
	}
	require.True(t, run(2).Complete)
	require.False(t, run(3).Complete)
}

func TestPriceCountsPriceMissingAndConsumptionMissingSeparately(t *testing.T) {
	loc := istanbul(t)
	w, m, h := build(time.Date(2026, 1, 1, 0, 0, 0, 0, loc), 10, func(i int, _ time.Time) (*decimal.Decimal, *decimal.Decimal) {
		switch {
		case i < 2:
			return nil, dp("1")
		case i < 5:
			return dp("1000"), nil
		}
		return dp("1000"), dp("1")
	})
	jan(m, "500")
	p, err := tariff.Price(ptfTariff(), w, h, true, m, seedParams(), loc)
	require.NoError(t, err)
	require.Equal(t, 2, p.HoursMissingPrice)
	require.Equal(t, 3, p.HoursMissingConsumption)
	require.Equal(t, 5, p.HoursMatched)
	require.Equal(t, "1000", p.PTFAverage.String(), "mean over priced hours only")
}

func TestPricePeriodAverageDerivesEachCoefficientIndependently(t *testing.T) {
	loc := istanbul(t)
	w, m, _ := build(time.Date(2026, 1, 1, 0, 0, 0, 0, loc), 24, flat("1000", "1"))
	jan(m, "500")
	tr := ptfTariff()
	tr.KbkPowerPrice = dp("2")
	tr.KbkReactivePower, tr.KbkDistributionCostTlPerKwh = nil, nil
	p, err := tariff.Price(tr, w, nil, false, m, seedParams(), loc)
	require.NoError(t, err)
	require.Equal(t, tariff.MethodPeriodAverage, p.Method)
	require.Equal(t, "1.5", p.BasePrice.String())
	require.Equal(t, "1.65", p.EnergyUnitPrice.String())
	require.Equal(t, "3", p.PowerUnitPrice.String())
	require.Nil(t, p.ReactiveUnitPrice)
	require.Nil(t, p.DistributionUnitPrice)
	require.Nil(t, p.T1)
	require.Nil(t, p.T2)
	require.Nil(t, p.T3)
}

func TestPricePeriodAverageYekdemIsHourWeightedAcrossMonths(t *testing.T) {
	loc := istanbul(t)
	from := time.Date(2026, 1, 31, 0, 0, 0, 0, loc)
	hours := int(time.Date(2026, 2, 28, 0, 0, 0, 0, loc).Sub(from) / time.Hour)
	w, m, _ := build(from, hours, flat("1000", "1"))
	jan(m, "100")
	m.Yekdem[tariff.YearMonth{Year: 2026, Month: time.February}] = d("800")
	p, err := tariff.Price(ptfTariff(), w, nil, false, m, seedParams(), loc)
	require.NoError(t, err)
	require.Equal(t, 672, p.HoursExpected)
	require.Equal(t, "775", p.YekdemUsed.String(), "(24×100 + 648×800) / 672")
	require.Equal(t, "1.775", p.BasePrice.String())
}

func TestPriceFixedSourcesUseTariffColumns(t *testing.T) {
	loc := istanbul(t)
	w, m, _ := build(time.Date(2026, 1, 1, 0, 0, 0, 0, loc), 24, flat("1000", "1"))
	jan(m, "500")
	tr := ptfTariff()
	tr.PowerUnitPrice, tr.KbkPowerPrice = dp("43.37"), dp("2")
	tr.PowerPriceSource, tr.ReactivePriceSource, tr.DistributionPriceSource = model.PriceSourceFixed, model.PriceSourceFixed, model.PriceSourceFixed
	p, err := tariff.Price(tr, w, nil, false, m, seedParams(), loc)
	require.NoError(t, err)
	require.Equal(t, "43.37", p.PowerUnitPrice.String())
	require.Equal(t, "1", p.ReactiveUnitPrice.String())
	require.Equal(t, "0.5", p.DistributionUnitPrice.String())

	power, reactive, distribution := tariff.FixedUnitPrices(fixedTariff())
	require.Nil(t, power)
	require.Equal(t, "1", reactive.String())
	require.Equal(t, "0.5", distribution.String())
}

func TestPriceMissingYekdemIsIncompleteNeverZero(t *testing.T) {
	loc := istanbul(t)
	w, m, _ := build(time.Date(2026, 1, 1, 0, 0, 0, 0, loc), 24, flat("1000", "1"))
	p, err := tariff.Price(ptfTariff(), w, nil, false, m, seedParams(), loc)
	require.NoError(t, err)
	require.False(t, p.Complete)
	require.Equal(t, tariff.ReasonYekdemMissing, p.Reason)
	require.Nil(t, p.BasePrice)
	require.Nil(t, p.EnergyUnitPrice)
	require.Nil(t, p.YekdemUsed)
}

func TestPriceNoPTFIsIncomplete(t *testing.T) {
	loc := istanbul(t)
	w, m, _ := build(time.Date(2026, 1, 1, 0, 0, 0, 0, loc), 24, func(int, time.Time) (*decimal.Decimal, *decimal.Decimal) { return nil, nil })
	jan(m, "500")
	p, err := tariff.Price(ptfTariff(), w, nil, false, m, seedParams(), loc)
	require.NoError(t, err)
	require.Equal(t, tariff.ReasonNoPTF, p.Reason)
	require.Nil(t, p.BasePrice)
}

func TestPriceMultiTimeNeverUsesHourlyMethod(t *testing.T) {
	loc := istanbul(t)
	w, m, h := build(time.Date(2026, 1, 1, 0, 0, 0, 0, loc), 24, flat("1000", "1"))
	jan(m, "500")
	tr := ptfTariff()
	tr.PriceType = model.PriceTypeMultiTime
	tr.KbkT1, tr.KbkT2, tr.KbkT3 = dp("1.2"), dp("1"), dp("0.8")
	p, err := tariff.Price(tr, w, h, true, m, seedParams(), loc)
	require.NoError(t, err)
	require.Equal(t, tariff.MethodPeriodAverage, p.Method)
	require.Empty(t, p.Hourly)
	require.Equal(t, "1.8", p.T1.String())
	require.Equal(t, "1.5", p.T2.String())
	require.Equal(t, "1.2", p.T3.String())
}

func TestPriceExpectedHoursOnDSTDay(t *testing.T) {
	loc := istanbul(t)
	from := time.Date(2015, 3, 29, 0, 0, 0, 0, loc)
	to := time.Date(2015, 3, 30, 0, 0, 0, 0, loc)
	m := tariff.Market{PTF: map[time.Time]decimal.Decimal{}, Yekdem: map[tariff.YearMonth]decimal.Decimal{{Year: 2015, Month: time.March}: d("1")}}
	p, err := tariff.Price(ptfTariff(), energy.Window{From: from, To: to}, nil, false, m, seedParams(), loc)
	require.NoError(t, err)
	require.Equal(t, 23, p.HoursExpected)
}
