package billing

import (
	"time"

	"github.com/shopspring/decimal"

	domain "github.com/MErenTalan/ekokod-rewrite/internal/domain/billing"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/tariff"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/consumption"
)

// maxChunk is the analyzer batch the consumption service accepts (I-7).
const maxChunk = consumption.MaxAnalyzersPerRequest

// quantitiesFromRow maps one member's Billing row onto invoice quantities.
func quantitiesFromRow(r consumption.Row) domain.Quantities {
	q := domain.Quantities{
		ActiveImport: r.Values[energy.ActiveImport],
		T1:           r.Values[energy.T1Import], T2: r.Values[energy.T2Import], T3: r.Values[energy.T3Import],
		ReactiveInductive:  r.Values[energy.ReactiveInductiveImport],
		ReactiveCapacitive: r.Values[energy.ReactiveCapacitiveImport],
		ActiveExport:       r.Values[energy.ActiveExport],
		MaxDemandKw:        r.MaxDemandKw,
		IndexStart:         indexMap(r.StartIndexes),
		IndexEnd:           indexMap(r.Indexes),
	}
	if q.ActiveExport != nil {
		q.ActiveExportKnownSum = *q.ActiveExport
	}
	return q
}

func indexMap(m map[energy.Register]*decimal.Decimal) map[string]*decimal.Decimal {
	out := make(map[string]*decimal.Decimal, len(m))
	for k, v := range m {
		out[string(k)] = v
	}
	return out
}

// soundHours keeps rows that are a real single hour with sound active import
// (R108, I-3): a row that absorbed a gap spans more than an hour and prices
// nothing.
func soundHours(rows []consumption.Row) []tariff.HourConsumption {
	var out []tariff.HourConsumption
	for _, r := range rows {
		v := r.Values[energy.ActiveImport]
		if v == nil || r.SpanFrom.IsZero() || r.SpanTo.Sub(r.SpanFrom) > time.Hour || len(r.Suspect) > 0 {
			continue
		}
		out = append(out, tariff.HourConsumption{Hour: r.Window.From.UTC(), Kwh: *v})
	}
	return out
}

// overlaps reports whether an anomaly period overlaps w (I-5).
func overlaps(a model.ConsumptionAnomaly, w energy.Window) bool {
	return a.PeriodEnd.After(w.From) && a.PeriodStart.Before(w.To)
}

// monthsTouched lists the Istanbul months the window spans.
func monthsTouched(w energy.Window, loc *time.Location) []tariff.YearMonth {
	var out []tariff.YearMonth
	from := w.From.In(loc)
	m := time.Date(from.Year(), from.Month(), 1, 0, 0, 0, 0, loc)
	for m.Before(w.To) {
		out = append(out, tariff.YearMonth{Year: m.Year(), Month: m.Month()})
		m = m.AddDate(0, 1, 0)
	}
	return out
}

func chunks[T any](items []T, size int) [][]T {
	var out [][]T
	for size > 0 && len(items) > 0 {
		n := min(size, len(items))
		out = append(out, items[:n])
		items = items[n:]
	}
	return out
}
