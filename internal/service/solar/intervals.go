// Package solar owns iSolar sync, fault forwarding and the solar read models (F9).
package solar

import (
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/isolar"
)

var istanbul = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		panic(err)
	}
	return loc
}()

// dayOf is the Istanbul midnight of t.
func dayOf(t time.Time) time.Time {
	l := t.In(istanbul)
	return time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, istanbul)
}

// Interval is one derived production interval (R277).
type Interval struct {
	Ts      time.Time
	Kwh     *decimal.Decimal
	PowerKw *decimal.Decimal
}

// Intervals turns one series of cumulative yield-today samples into interval
// kWh: the first sample of an Istanbul day is its own value, later ones the
// delta to the last known value; a negative delta is null and counted.
func Intervals(samples []isolar.YieldSample) (out []Interval, resets int) {
	sorted := append([]isolar.YieldSample(nil), samples...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Ts.Before(sorted[j].Ts) })
	var prev *decimal.Decimal
	var prevDay time.Time
	for _, s := range sorted {
		if day := dayOf(s.Ts); !day.Equal(prevDay) {
			prevDay, prev = day, nil
		}
		iv := Interval{Ts: s.Ts, PowerKw: s.ActivePowerKw}
		if s.YieldTodayKwh != nil {
			v := *s.YieldTodayKwh
			switch {
			case prev == nil:
				iv.Kwh = &v
			case v.LessThan(*prev):
				resets++
			default:
				delta := v.Sub(*prev)
				iv.Kwh = &delta
			}
			prev = &v
		}
		out = append(out, iv)
	}
	return out, resets
}

// SumByTimestamp adds several series per timestamp (the inverter_sum basis);
// a timestamp where every series is missing stays missing.
func SumByTimestamp(series ...[]Interval) []Interval {
	byTs := map[int64]*Interval{}
	for _, s := range series {
		for _, iv := range s {
			key := iv.Ts.UnixNano()
			acc, ok := byTs[key]
			if !ok {
				acc = &Interval{Ts: iv.Ts}
				byTs[key] = acc
			}
			acc.Kwh = addPtr(acc.Kwh, iv.Kwh)
			acc.PowerKw = addPtr(acc.PowerKw, iv.PowerKw)
		}
	}
	out := make([]Interval, 0, len(byTs))
	for _, iv := range byTs {
		out = append(out, *iv)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Ts.Before(out[j].Ts) })
	return out
}

func addPtr(a, b *decimal.Decimal) *decimal.Decimal {
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

// Utilisation is power ÷ capacity × 100 at 1 dp (R282); nil when unknown.
func Utilisation(power, capacity *decimal.Decimal) *decimal.Decimal {
	if power == nil || capacity == nil || capacity.IsZero() {
		return nil
	}
	v := power.Div(*capacity).Mul(decimal.NewFromInt(100)).Round(1)
	return &v
}

// Device statuses (R285).
const (
	StatusNormal  = "normal"
	StatusAlarm   = "alarm"
	StatusFault   = "fault"
	StatusOffline = "offline"
)

// staleAfter is R282/R285's snapshot age limit.
const staleAfter = 2 * time.Hour

// StatusOf maps dev_fault_status (4 normal, 2 alarm, 1 fault) and snapshot age.
func StatusOf(fault *int32, at *time.Time, now time.Time) *string {
	if at == nil {
		return nil
	}
	status := StatusOffline
	if now.Sub(*at) <= staleAfter && fault != nil {
		switch *fault {
		case 4:
			status = StatusNormal
		case 2:
			status = StatusAlarm
		case 1:
			status = StatusFault
		}
	}
	return &status
}

// DayTariff is a feed-in price effective from a day (R283).
type DayTariff struct {
	From     time.Time
	Price    decimal.Decimal
	Currency model.CurrencyCode
}

// Revenue is one currency's amount.
type Revenue struct {
	Currency model.CurrencyCode
	Amount   decimal.Decimal
}

// RevenueOver prices each day at the tariff effective that day, per currency,
// rounding half-up to 2 dp once at the end; days before any tariff are unpriced.
func RevenueOver(days map[time.Time]decimal.Decimal, tariffs []DayTariff) (out []Revenue, unpriced int) {
	sorted := append([]DayTariff(nil), tariffs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].From.Before(sorted[j].From) })
	sums := map[model.CurrencyCode]decimal.Decimal{}
	for day, kwh := range days {
		var t *DayTariff
		for i := range sorted {
			if !sorted[i].From.After(day) {
				t = &sorted[i]
			}
		}
		if t == nil {
			unpriced++
			continue
		}
		sums[t.Currency] = sums[t.Currency].Add(kwh.Mul(t.Price))
	}
	for _, c := range model.CurrencyCodes() {
		if v, ok := sums[c]; ok {
			out = append(out, Revenue{Currency: c, Amount: v.Round(2)})
		}
	}
	return out, unpriced
}

// Legacy alarmTranslator.ts: exact names first, then substring rules in order.
var (
	exactFaults = map[string]string{
		"电网掉电":     "şebeke kesintisi",
		"系统绝缘阻抗低":  "sistem izolasyon direnci düşük",
		"设备不发电":    "üretim yapmama",
		"系统故障":     "sistem arızası",
		"FRAM读取告警": "FRAM okuma alarmı",
	}
	faultRules = [][2]string{
		{"电网", "şebeke problemi"}, {"掉电", "enerji kesintisi"}, {"绝缘", "izolasyon hatası"},
		{"不发电", "üretim yapmama"}, {"系统故障", "sistem arızası"}, {"故障", "cihaz arızası"},
		{"告警", "alarm durumu"}, {"读取", "okuma hatası"}, {"FRAM", "FRAM ile ilgili hata"},
	}
)

// TranslateFault gives the Turkish text of an iSolar fault name (R286); an
// unknown name comes back unchanged with translated = false.
func TranslateFault(name string) (text string, translated bool) {
	name = strings.TrimSpace(name)
	if v, ok := exactFaults[name]; ok {
		return v, true
	}
	for _, r := range faultRules {
		if strings.Contains(name, r[0]) {
			return r[1], true
		}
	}
	return name, false
}
