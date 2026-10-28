package seed

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/admin"
)

// ensureDemoSeries writes every missing hour of readings and plant production
// in [now-demoHistory, now] and returns the readings it wrote (R550). Only the
// range it wrote is refreshed in the aggregates (R548).
func ensureDemoSeries(ctx context.Context, d DemoDeps, b demoBaseline, now time.Time) (int, error) {
	end := now.UTC().Truncate(time.Hour)
	written, from, err := ensureDemoReadings(ctx, d, b, end, now)
	if err != nil {
		return written, err
	}
	aggregates := admin.NewAggregateRepository(d.Pool)
	if written > 0 {
		for _, view := range []store.AggregateView{store.ViewConsumptionHourly, store.ViewConsumptionDaily} {
			err := refreshDays(from.Add(-48*time.Hour), end.Add(time.Hour), func(w store.TimeRange) error {
				return aggregates.Refresh(ctx, view, w)
			})
			if err != nil {
				return written, fmt.Errorf("seed demo refresh %s: %w", view, err)
			}
		}
	}
	plantFrom, err := ensureDemoProduction(ctx, d, b, end)
	if err != nil || plantFrom == nil {
		return written, err
	}
	// The monthly plant view refuses a window smaller than one bucket, so each
	// window is one whole month, padded by a day (as recompute does).
	last := istanbulDay(end)
	for m := istanbulDay(*plantFrom).AddDate(0, 0, 1-istanbulDay(*plantFrom).Day()); !m.After(last); m = m.AddDate(0, 1, 0) {
		if err := aggregates.RefreshPlantProduction(ctx, store.TimeRange{From: m.AddDate(0, 0, -1), To: m.AddDate(0, 1, 1)}); err != nil {
			return written, fmt.Errorf("seed demo refresh plant views: %w", err)
		}
	}
	return written, nil
}

// refreshDays calls fn over [from, to) in windows of at most 31 days that
// start on Istanbul midnights, so no daily bucket straddles two windows and no
// single refresh outlives the statement timeout. A tail under two days joins
// the window before it.
func refreshDays(from, to time.Time, fn func(store.TimeRange) error) error {
	for start := istanbulDay(from); start.Before(to); {
		stop := start.AddDate(0, 0, 31)
		if to.Sub(stop) < 48*time.Hour {
			stop = to
		}
		if err := fn(store.TimeRange{From: start, To: stop}); err != nil {
			return err
		}
		start = stop
	}
	return nil
}

func ensureDemoReadings(ctx context.Context, d DemoDeps, b demoBaseline, end, now time.Time) (int, time.Time, error) {
	sc := store.SystemScope(b.company)
	analyzers, readings := postgres.NewAnalyzerRepository(d.Pool), postgres.NewReadingRepository(d.Pool)
	total, earliest := 0, end
	netted := map[int]bool{}
	for _, p := range b.plants {
		if p.netting >= 0 {
			netted[b.analyzers[p.netting].building] = true
		}
	}
	for i, a := range b.analyzers {
		row, err := analyzers.Get(ctx, sc, a.id)
		if err != nil {
			return total, earliest, fmt.Errorf("seed demo readings %s: %w", a.number, err)
		}
		start := end.Add(-demoHistory)
		active := decimal.NewFromInt(250000 + int64(i)*37000)
		t1, t2 := active.Mul(decimal.RequireFromString("0.55")), active.Mul(decimal.RequireFromString("0.2"))
		m := demoMeter{active: active, inductive: decimal.NewFromInt(30000), capacitive: decimal.NewFromInt(4000),
			bands: [3]decimal.Decimal{t1, t2, active.Sub(t1).Sub(t2)}}
		if a.solarPlant >= 0 {
			m.export = ptr(decimal.NewFromInt(12000))
		} else if netted[a.building] {
			// The netting tariff covers the whole building; a missing export register flags its bills.
			m.export = ptr(decimal.Zero)
		}
		if row.LastReadingAt != nil {
			last, err := readings.Range(ctx, sc, a.id, store.TimeRange{From: *row.LastReadingAt, To: row.LastReadingAt.Add(time.Second)},
				model.ReadingKindLoadProfile)
			if err != nil {
				return total, earliest, err
			}
			if r := last; len(r) == 1 && r[0].ActiveImport != nil && r[0].ReactiveInductiveImport != nil && r[0].ReactiveCapacitiveImport != nil &&
				r[0].T1Import != nil && r[0].T2Import != nil && r[0].T3Import != nil {
				m = demoMeter{active: *r[0].ActiveImport, inductive: *r[0].ReactiveInductiveImport,
					capacitive: *r[0].ReactiveCapacitiveImport, export: r[0].ActiveExport,
					bands: [3]decimal.Decimal{*r[0].T1Import, *r[0].T2Import, *r[0].T3Import}}
				m.advance(b, i, *row.LastReadingAt)
				start = row.LastReadingAt.Add(time.Hour)
			}
		}
		if start.After(end) {
			continue
		}
		earliest = minTime(earliest, start)
		batch := make([]model.MeterReading, 0, 5000)
		for ts := start; !ts.After(end); ts = ts.Add(time.Hour) {
			batch = append(batch, m.reading(a, ts, now))
			m.advance(b, i, ts)
			if len(batch) == cap(batch) || ts.Equal(end) {
				if _, _, err := readings.BulkInsert(ctx, sc, batch); err != nil {
					return total, earliest, fmt.Errorf("seed demo readings %s: %w", a.number, err)
				}
				total += len(batch)
				batch = batch[:0]
			}
		}
		if err := analyzers.TouchLastReading(ctx, sc, a.id, end); err != nil {
			return total, earliest, err
		}
	}
	return total, earliest, nil
}

// ensureDemoProduction regenerates whole Istanbul days from each plant's last
// stored day, so a partly filled day is replaced, never doubled.
func ensureDemoProduction(ctx context.Context, d DemoDeps, b demoBaseline, end time.Time) (*time.Time, error) {
	sc := store.SystemScope(b.company)
	totals := postgres.NewProductionTotalsRepository(d.Pool)
	var earliest *time.Time
	for _, p := range b.plants {
		var last *time.Time
		if err := d.Pool.QueryRow(ctx, `select max(ts) from plant_production_totals where plant_id = $1`, p.id).Scan(&last); err != nil {
			return nil, fmt.Errorf("seed demo production %s: %w", p.name, err)
		}
		from := end.Add(-demoHistory)
		if last != nil {
			if !last.Before(end) {
				continue
			}
			from = *last
		}
		day := istanbulDay(from)
		if earliest == nil || day.Before(*earliest) {
			earliest = ptr(day)
		}
		for day.Before(end) || day.Equal(end) {
			chunkEnd := day.AddDate(0, 0, 31)
			var days []time.Time
			var rows []model.PlantProductionTotal
			for dd := day; dd.Before(chunkEnd) && !dd.After(end); dd = dd.AddDate(0, 0, 1) {
				days = append(days, dd)
			}
			for ts := day.UTC(); ts.Before(chunkEnd) && !ts.After(end); ts = ts.Add(time.Hour) {
				kwh := decimal.NewFromFloat(solarKwh(p, ts)).Round(3)
				rows = append(rows, model.PlantProductionTotal{PlantID: p.id, Ts: ts, ProductionKwh: &kwh, ActivePowerKw: &kwh,
					Basis: model.BasisPlantMeter})
			}
			if _, err := totals.ReplaceDays(ctx, sc, p.id, days, rows); err != nil {
				return nil, fmt.Errorf("seed demo production %s: %w", p.name, err)
			}
			day = chunkEnd
		}
	}
	return earliest, nil
}

func istanbulDay(t time.Time) time.Time {
	l := t.In(istanbul)
	return time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, istanbul)
}

// demoMeter holds one analyzer's cumulative registers; export is nil for a
// meter without generation behind it. bands are T1 (06–17), T2 (17–22) and
// T3 (22–06) import, which a multi-time tariff bills from.
type demoMeter struct {
	active, inductive, capacitive decimal.Decimal
	bands                         [3]decimal.Decimal
	export                        *decimal.Decimal
}

func (m demoMeter) reading(a demoAnalyzer, ts, now time.Time) model.MeterReading {
	active, inductive, capacitive := m.active, m.inductive, m.capacitive
	t1, t2, t3 := m.bands[0], m.bands[1], m.bands[2]
	r := model.MeterReading{AnalyzerID: a.id, Ts: ts, Kind: model.ReadingKindLoadProfile, ActiveImport: &active,
		ReactiveInductiveImport: &inductive, ReactiveCapacitiveImport: &capacitive, T1Import: &t1, T2Import: &t2, T3Import: &t3,
		MultiplierApplied: decimal.NewFromInt(1),
		SourceProvider:    model.IntegrationProviderOSOS, IngestedAt: now}
	if m.export != nil {
		export := *m.export
		r.ActiveExport = &export
	}
	return r
}

// advance adds the hour starting at ts: the analyzer's load curve, netted
// against its rooftop plant when it has one.
func (m *demoMeter) advance(b demoBaseline, i int, ts time.Time) {
	a := b.analyzers[i]
	seed := uint32(ts.Unix()/3600)*2654435761 + uint32(i+1)*40503
	load := demoLoad(a, ts, seed)
	if a.solarPlant >= 0 && m.export != nil {
		if pv := solarKwh(b.plants[a.solarPlant], ts); pv > load {
			*m.export = m.export.Add(decimal.NewFromFloat(pv - load).Round(3))
			load = 0
		} else {
			load -= pv
		}
	}
	step := decimal.NewFromFloat(load).Round(3)
	shares := a.inductive
	if month := ts.In(istanbul).Month(); a.summerInductive[1] > 0 && month >= time.June && month <= time.August {
		shares = a.summerInductive
	}
	inductive := float64(shares[0]) + float64(shares[1]-shares[0])*unit(mulberry32(seed+1))
	m.active = m.active.Add(step)
	band := 2
	if h := ts.In(istanbul).Hour(); h >= 6 && h < 17 {
		band = 0
	} else if h >= 17 && h < 22 {
		band = 1
	}
	m.bands[band] = m.bands[band].Add(step)
	m.inductive = m.inductive.Add(step.Mul(decimal.NewFromFloat(inductive / 1000)).Round(3))
	m.capacitive = m.capacitive.Add(step.Mul(decimal.NewFromFloat(0.01 + 0.03*unit(mulberry32(seed+2)))).Round(3))
}

// demoLoad is kWh for the hour at ts: working or idle level, weekend and
// holiday factor, a seasonal swing and ±10 % noise.
func demoLoad(a demoAnalyzer, ts time.Time, seed uint32) float64 {
	local := ts.In(istanbul)
	base := float64(a.night)
	if h := local.Hour(); h >= a.from && h < a.to {
		base = float64(a.day)
	}
	if wd := local.Weekday(); wd == time.Saturday || wd == time.Sunday || demoHoliday(local) {
		base *= a.weekend
	}
	base *= 1 + a.season*math.Cos(2*math.Pi*float64(local.YearDay()-196)/365)
	return base * (0.9 + 0.2*unit(mulberry32(seed)))
}

// demoClearness is the mean share of clear-sky yield per month.
var demoClearness = [12]float64{0.5, 0.56, 0.66, 0.72, 0.8, 0.9, 0.94, 0.93, 0.86, 0.74, 0.58, 0.5}

// solarKwh is the plant's yield for the hour at ts: a clear-sky curve from the
// sun's elevation at mid-hour, times a daily cloud factor and hourly noise.
func solarKwh(p demoPlant, ts time.Time) float64 {
	mid := ts.Add(30 * time.Minute).UTC()
	decl := 23.44 * math.Pi / 180 * math.Sin(2*math.Pi*float64(284+mid.YearDay())/365)
	solarHour := float64(mid.Hour()) + float64(mid.Minute())/60 + p.lon/15
	ha := (solarHour - 12) * 15 * math.Pi / 180
	lat := p.lat * math.Pi / 180
	sinAlt := math.Sin(lat)*math.Sin(decl) + math.Cos(lat)*math.Cos(decl)*math.Cos(ha)
	if sinAlt <= 0.02 {
		return 0
	}
	local := ts.In(istanbul)
	daySeed := uint32(local.Year()*1000+local.YearDay()) * 2246822519
	cloud := math.Min(1, demoClearness[local.Month()-1]*(0.55+0.9*unit(mulberry32(daySeed+uint32(p.kwp)))))
	noise := 0.93 + 0.07*unit(mulberry32(uint32(ts.Unix()/3600)+uint32(p.kwp)))
	return float64(p.kwp) * 0.86 * math.Pow(sinAlt, 1.15) * cloud * noise
}

func unit(x uint32) float64 { return float64(x) / 4294967296 }
