package consumption_test

import (
	"context"
	"fmt"
	"math/rand/v2"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/lock"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/consumption"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

func periodScope() store.Scope {
	return store.Scope{CompanyID: uuid.New(), AllBuildings: true}
}

func win(from, to time.Time) energy.Window { return energy.Window{From: from, To: to} }

// TestPeriodRequestValidation: every invalid request fails ErrInvalidRequest
// before any I/O (noReadings panics on the first repository call).
func TestPeriodRequestValidation(t *testing.T) {
	loc := istanbulLoc(t)
	d := func(m time.Month, day int) time.Time { return time.Date(2026, m, day, 0, 0, 0, 0, loc) }
	a, b2 := uuid.New(), uuid.New()
	good := []energy.Window{win(d(1, 15), d(2, 15)), win(d(2, 15), d(3, 15))}

	days := func(n int) []energy.Window {
		out := make([]energy.Window, n)
		for i := range out {
			out[i] = win(d(1, 1+i), d(1, 2+i))
		}
		return out
	}
	many := make([]uuid.UUID, consumption.MaxAnalyzersPerRequest+1)
	for i := range many {
		many[i] = uuid.New()
	}
	spanStart := d(1, 1)
	half := consumption.MaxRequestSpan / 2

	cases := []struct {
		name    string
		sc      store.Scope
		ids     []uuid.UUID
		windows []energy.Window
		ok      bool
	}{
		{"valid", periodScope(), []uuid.UUID{a, b2}, good, true},
		{"invalid scope", store.Scope{}, []uuid.UUID{a}, good, false},
		{"no analyzers", periodScope(), nil, good, false},
		{"duplicate analyzer", periodScope(), []uuid.UUID{a, a}, good, false},
		{"too many analyzers", periodScope(), many, good, false},
		{"no windows", periodScope(), []uuid.UUID{a}, nil, false},
		{"zero-width window", periodScope(), []uuid.UUID{a}, []energy.Window{win(d(1, 15), d(1, 15))}, false},
		{"reversed window", periodScope(), []uuid.UUID{a}, []energy.Window{win(d(2, 15), d(1, 15))}, false},
		{"window under one hour", periodScope(), []uuid.UUID{a}, []energy.Window{win(d(1, 15), d(1, 15).Add(59*time.Minute))}, false},
		{"window of exactly one hour", periodScope(), []uuid.UUID{a}, []energy.Window{win(d(1, 15), d(1, 15).Add(time.Hour))}, true},
		{"gap between windows", periodScope(), []uuid.UUID{a}, []energy.Window{win(d(1, 15), d(2, 15)), win(d(2, 16), d(3, 15))}, false},
		{"overlapping windows", periodScope(), []uuid.UUID{a}, []energy.Window{win(d(1, 15), d(2, 15)), win(d(2, 14), d(3, 15))}, false},
		{"unsorted windows", periodScope(), []uuid.UUID{a}, []energy.Window{good[1], good[0]}, false},
		{"thirteen windows", periodScope(), []uuid.UUID{a}, days(consumption.MaxPeriodWindows), true},
		{"fourteen windows", periodScope(), []uuid.UUID{a}, days(consumption.MaxPeriodWindows + 1), false},
		{"hull exactly MaxRequestSpan", periodScope(), []uuid.UUID{a}, []energy.Window{win(spanStart, spanStart.Add(half)), win(spanStart.Add(half), spanStart.Add(consumption.MaxRequestSpan))}, true},
		{"hull over MaxRequestSpan", periodScope(), []uuid.UUID{a}, []energy.Window{win(spanStart, spanStart.Add(half)), win(spanStart.Add(half), spanStart.Add(consumption.MaxRequestSpan+time.Microsecond))}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := consumption.PeriodRequest{AnalyzerIDs: tc.ids, Windows: tc.windows}
			if tc.ok {
				b := newBillingAt(t, fakeReadings{}, d(1, 1))
				_, err := b.PeriodConsumption(context.Background(), tc.sc, req)
				require.NoError(t, err)
				return
			}
			b, err := consumption.NewBilling(consumption.BillingDeps{
				Readings: noReadings{}, Anomalies: noAnomalies{}, Ops: noOps{},
				Clock: clock.NewFake(d(1, 1)), Log: testLog(t), Locker: lock.NewMemory(nil),
			})
			require.NoError(t, err)
			_, err = b.PeriodConsumption(context.Background(), tc.sc, req)
			require.ErrorIs(t, err, consumption.ErrInvalidRequest)
			_, err = b.PeriodConsumptionAndRecord(context.Background(), tc.sc, req)
			require.ErrorIs(t, err, consumption.ErrInvalidRequest)
		})
	}
}

// TestPeriodConsumptionCutoff15Telescopes: the Jan 15 seam resolves once
// (R96/R107), so the two cut-off periods sum to the single two-period window.
func TestPeriodConsumptionCutoff15Telescopes(t *testing.T) {
	loc := istanbulLoc(t)
	dec15 := time.Date(2025, 12, 15, 0, 0, 0, 0, loc)
	jan15 := time.Date(2026, 1, 15, 0, 0, 0, 0, loc)
	feb15 := time.Date(2026, 2, 15, 0, 0, 0, 0, loc)
	id := uuid.New()
	regs := func(a, t1, ind string) map[string]string {
		return map[string]string{"active_import": a, "t1_import": t1, "reactive_inductive_import": ind}
	}
	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindLoadProfile: {
			readingRow(id, dec15.Add(-2*time.Hour), model.ReadingKindLoadProfile, regs("1000", "400", "10")),
			// The seam's preferred candidate: load_profile beats the fresher daily below.
			readingRow(id, jan15.Add(-4*time.Hour), model.ReadingKindLoadProfile, regs("1830.5", "700.25", "31")),
			readingRow(id, feb15, model.ReadingKindLoadProfile, regs("2600", "1000", "55")),
		},
		model.ReadingKindDaily: {
			readingRow(id, jan15, model.ReadingKindDaily, regs("1900", "760", "40")),
		},
	}}
	b := newBillingAt(t, readings, feb15.Add(consumption.SettleDelayMonthly))
	ctx := context.Background()
	sc := periodScope()

	split, err := b.PeriodConsumption(ctx, sc, consumption.PeriodRequest{AnalyzerIDs: []uuid.UUID{id}, Windows: []energy.Window{win(dec15, jan15), win(jan15, feb15)}})
	require.NoError(t, err)
	require.Len(t, split, 2)
	whole, err := b.PeriodConsumption(ctx, sc, consumption.PeriodRequest{AnalyzerIDs: []uuid.UUID{id}, Windows: []energy.Window{win(dec15, feb15)}})
	require.NoError(t, err)
	require.Len(t, whole, 1)

	for _, reg := range []energy.Register{energy.ActiveImport, energy.T1Import, energy.ReactiveInductiveImport} {
		require.NotNil(t, split[0].Values[reg], reg)
		require.NotNil(t, split[1].Values[reg], reg)
		sum := split[0].Values[reg].Add(*split[1].Values[reg])
		require.True(t, sum.Equal(*whole[0].Values[reg]), "%s: %s + %s != %s", reg, split[0].Values[reg], split[1].Values[reg], whole[0].Values[reg])
	}
	require.Equal(t, "830.5", split[0].Values[energy.ActiveImport].String(), "the seam is the Jan 14 20:00 load_profile reading")
}

// TestPeriodConsumptionIgnoresBillingKindMaxDemandOffCalendar: R111 — a
// billing-kind peak counts only for a window that is exactly a calendar month.
func TestPeriodConsumptionIgnoresBillingKindMaxDemandOffCalendar(t *testing.T) {
	loc := istanbulLoc(t)
	jan1 := time.Date(2026, 1, 1, 0, 0, 0, 0, loc)
	feb1 := time.Date(2026, 2, 1, 0, 0, 0, 0, loc)
	feb15 := time.Date(2026, 2, 15, 0, 0, 0, 0, loc)
	id := uuid.New()
	lp := func(ts time.Time, active, md string) model.MeterReading {
		v := map[string]string{"active_import": active}
		if md != "" {
			v["max_demand_kw"] = md
		}
		return readingRow(id, ts, model.ReadingKindLoadProfile, v)
	}
	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindLoadProfile: {
			lp(jan1, "100", ""),
			lp(jan1.Add(4*24*time.Hour), "150", "50"),
			lp(feb1, "300", ""),
			lp(feb1.Add(4*24*time.Hour), "350", "60"),
			lp(feb15, "500", ""),
		},
		model.ReadingKindBilling: {
			readingRow(id, jan1.Add(19*24*time.Hour), model.ReadingKindBilling, map[string]string{"max_demand_kw": "900"}),
			readingRow(id, feb1.Add(9*24*time.Hour), model.ReadingKindBilling, map[string]string{"max_demand_kw": "800"}),
		},
	}}
	b := newBillingAt(t, readings, feb15.Add(consumption.SettleDelayMonthly))
	rows, err := b.PeriodConsumption(context.Background(), periodScope(), consumption.PeriodRequest{
		AnalyzerIDs: []uuid.UUID{id},
		Windows:     []energy.Window{win(jan1, feb1), win(feb1, feb15)},
	})
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.NotNil(t, rows[0].MaxDemandKw)
	require.Equal(t, "900", rows[0].MaxDemandKw.String(), "January is a calendar month: its billing-kind peak counts")
	require.NotNil(t, rows[1].MaxDemandKw)
	require.Equal(t, "60", rows[1].MaxDemandKw.String(), "Feb 1-15 is not a calendar month: the billing-kind 800 is ignored")
}

// TestPeriodConsumptionUsesLoadProfileNotBillingAtANonMonthStartCutoff: A1 —
// a billing snapshot on the 1st is within 72 h of the 3rd but must not stand
// in for a cut-off-day-3 boundary.
func TestPeriodConsumptionUsesLoadProfileNotBillingAtANonMonthStartCutoff(t *testing.T) {
	loc := istanbulLoc(t)
	d := func(m time.Month, day int) time.Time { return time.Date(2026, m, day, 0, 0, 0, 0, loc) }
	id := uuid.New()
	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindBilling: {
			readingRow(id, d(1, 1), model.ReadingKindBilling, map[string]string{"active_import": "1000"}),
			readingRow(id, d(2, 1), model.ReadingKindBilling, map[string]string{"active_import": "2000"}),
			readingRow(id, d(3, 1), model.ReadingKindBilling, map[string]string{"active_import": "3000"}),
		},
		model.ReadingKindLoadProfile: {
			readingRow(id, d(1, 3), model.ReadingKindLoadProfile, map[string]string{"active_import": "1100"}),
			readingRow(id, d(2, 3), model.ReadingKindLoadProfile, map[string]string{"active_import": "2150"}),
			readingRow(id, d(3, 3), model.ReadingKindLoadProfile, map[string]string{"active_import": "3300"}),
		},
	}}
	b := newBillingAt(t, readings, d(3, 3).Add(consumption.SettleDelayMonthly))
	rows, err := b.PeriodConsumption(context.Background(), periodScope(), consumption.PeriodRequest{
		AnalyzerIDs: []uuid.UUID{id},
		Windows:     []energy.Window{win(d(1, 3), d(2, 3)), win(d(2, 3), d(3, 3))},
	})
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, "1050", rows[0].Values[energy.ActiveImport].String())
	require.Equal(t, "1150", rows[1].Values[energy.ActiveImport].String())
	require.Equal(t, energy.KindLoadProfile, rows[0].Source)
	require.Equal(t, energy.KindLoadProfile, rows[1].Source)
}

// TestPeriodConsumptionToleranceCapsAtHalfTheSmallerWindow: R107 caps a kind
// tolerance at half the smaller explicit window meeting at the instant — here
// 24 h at the seam of a 2-day and a 10-day window.
func TestPeriodConsumptionToleranceCapsAtHalfTheSmallerWindow(t *testing.T) {
	loc := istanbulLoc(t)
	d0 := time.Date(2026, 3, 10, 0, 0, 0, 0, loc)
	seam := d0.Add(48 * time.Hour)
	end := seam.Add(240 * time.Hour)
	windows := []energy.Window{win(d0, seam), win(seam, end)}

	run := func(seamOffset time.Duration) []consumption.Row {
		id := uuid.New()
		readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
			model.ReadingKindLoadProfile: {
				readingRow(id, d0, model.ReadingKindLoadProfile, map[string]string{"active_import": "100"}),
				readingRow(id, seam.Add(-seamOffset), model.ReadingKindLoadProfile, map[string]string{"active_import": "180"}),
				readingRow(id, end, model.ReadingKindLoadProfile, map[string]string{"active_import": "400"}),
			},
		}}
		b := newBillingAt(t, readings, end.Add(consumption.SettleDelayMonthly))
		rows, err := b.PeriodConsumption(context.Background(), periodScope(), consumption.PeriodRequest{AnalyzerIDs: []uuid.UUID{id}, Windows: windows})
		require.NoError(t, err)
		return rows
	}

	within := run(20 * time.Hour)
	require.Len(t, within, 2, "20 h is inside the 24 h cap")
	require.Equal(t, "80", within[0].Values[energy.ActiveImport].String())
	require.Equal(t, "220", within[1].Values[energy.ActiveImport].String())

	require.Empty(t, run(30*time.Hour), "30 h is inside load_profile's own 36 h but outside the 24 h cap")
}

// TestPeriodConsumptionNegativeDeltaIsNilAndSuspect: a suspect cut-off period
// is nil plus a Suspicion, never a number (removed-behaviour 1).
func TestPeriodConsumptionNegativeDeltaIsNilAndSuspect(t *testing.T) {
	loc := istanbulLoc(t)
	jan15 := time.Date(2026, 1, 15, 0, 0, 0, 0, loc)
	feb15 := time.Date(2026, 2, 15, 0, 0, 0, 0, loc)
	id := uuid.New()
	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindLoadProfile: {
			readingRow(id, jan15, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000", "t1_import": "10"}),
			readingRow(id, feb15, model.ReadingKindLoadProfile, map[string]string{"active_import": "900", "t1_import": "20"}),
		},
	}}
	b := newBillingAt(t, readings, feb15.Add(consumption.SettleDelayMonthly))
	rows, err := b.PeriodConsumption(context.Background(), periodScope(), consumption.PeriodRequest{AnalyzerIDs: []uuid.UUID{id}, Windows: []energy.Window{win(jan15, feb15)}})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Nil(t, rows[0].Values[energy.ActiveImport])
	require.Equal(t, energy.ReasonNegativeDelta, rows[0].Suspect[energy.ActiveImport].Reason)
	require.Equal(t, "10", rows[0].Values[energy.T1Import].String(), "a sound register keeps its number")
}

// TestPeriodConsumptionDropsUnsettledWindow: R104(1) — a cut-off window
// settles SettleDelayMonthly after its To.
func TestPeriodConsumptionDropsUnsettledWindow(t *testing.T) {
	loc := istanbulLoc(t)
	jan15 := time.Date(2026, 1, 15, 0, 0, 0, 0, loc)
	feb15 := time.Date(2026, 2, 15, 0, 0, 0, 0, loc)
	id := uuid.New()
	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindLoadProfile: {
			readingRow(id, jan15, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
			readingRow(id, feb15, model.ReadingKindLoadProfile, map[string]string{"active_import": "1200"}),
		},
	}}
	req := consumption.PeriodRequest{AnalyzerIDs: []uuid.UUID{id}, Windows: []energy.Window{win(jan15, feb15)}}

	early, err := newBillingAt(t, readings, feb15.Add(71*time.Hour+59*time.Minute)).PeriodConsumption(context.Background(), periodScope(), req)
	require.NoError(t, err)
	require.Empty(t, early)

	settled, err := newBillingAt(t, readings, feb15.Add(72*time.Hour)).PeriodConsumption(context.Background(), periodScope(), req)
	require.NoError(t, err)
	require.Len(t, settled, 1)
}

// TestBillingRowCarriesStartIndexesAndSpan: A3/A4 — both Billing entry points
// expose the start boundary's indexes and the resolved readings' own instants.
func TestBillingRowCarriesStartIndexesAndSpan(t *testing.T) {
	loc := istanbulLoc(t)
	jan1 := time.Date(2026, 1, 1, 0, 0, 0, 0, loc)
	feb1 := time.Date(2026, 2, 1, 0, 0, 0, 0, loc)
	id := uuid.New()
	startTS := jan1.Add(-3 * time.Hour)
	endTS := feb1.Add(-5 * time.Hour)
	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindDaily: {
			readingRow(id, startTS, model.ReadingKindDaily, map[string]string{"active_import": "1000", "t2_import": "7"}),
			readingRow(id, endTS, model.ReadingKindDaily, map[string]string{"active_import": "1500", "t2_import": "9"}),
		},
	}}
	b := newBillingAt(t, readings, feb1.Add(consumption.SettleDelayMonthly))
	ctx := context.Background()

	monthly, err := b.Consumption(ctx, periodScope(), consumption.SeriesRequest{AnalyzerIDs: []uuid.UUID{id}, Level: energy.Monthly, Range: store.TimeRange{From: jan1, To: feb1}})
	require.NoError(t, err)
	// UTC-located windows still come back as the Istanbul Monthly window.
	period, err := b.PeriodConsumption(ctx, periodScope(), consumption.PeriodRequest{AnalyzerIDs: []uuid.UUID{id}, Windows: []energy.Window{win(jan1.UTC(), feb1.UTC())}})
	require.NoError(t, err)
	require.Len(t, period, 1)
	require.Equal(t, monthly[0].Window, period[0].Window)

	for name, rows := range map[string][]consumption.Row{"Consumption": monthly, "PeriodConsumption": period} {
		require.Len(t, rows, 1, name)
		r := rows[0]
		require.Equal(t, "1000", r.StartIndexes[energy.ActiveImport].String(), name)
		require.Equal(t, "7", r.StartIndexes[energy.T2Import].String(), name)
		require.Nil(t, r.StartIndexes[energy.T3Import], name)
		require.Equal(t, "1500", r.Indexes[energy.ActiveImport].String(), name)
		require.True(t, r.SpanFrom.Equal(startTS), "%s SpanFrom %s", name, r.SpanFrom)
		require.True(t, r.SpanTo.Equal(endTS), "%s SpanTo %s", name, r.SpanTo)
	}
}

// TestBillingHourlyRowSpanCoversAnAbsorbedGap: A4 — §3.1 absorbs a 3-hour
// reading gap into the next emitted hour, whose span is then 4 h.
func TestBillingHourlyRowSpanCoversAnAbsorbedGap(t *testing.T) {
	h0 := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	id := uuid.New()
	lp := func(hours int, v string) model.MeterReading {
		return readingRow(id, h0.Add(time.Duration(hours)*time.Hour), model.ReadingKindLoadProfile, map[string]string{"active_import": v})
	}
	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindLoadProfile: {lp(0, "10"), lp(1, "20"), lp(5, "60")},
	}}
	b := newBillingAt(t, readings, h0.Add(5*time.Hour+consumption.SettleDelayHourly))
	rows, err := b.Consumption(context.Background(), periodScope(), consumption.SeriesRequest{AnalyzerIDs: []uuid.UUID{id}, Level: energy.Hourly, Range: store.TimeRange{From: h0, To: h0.Add(5 * time.Hour)}})
	require.NoError(t, err)
	require.Len(t, rows, 2)

	require.Equal(t, time.Hour, rows[0].SpanTo.Sub(rows[0].SpanFrom))
	require.Equal(t, time.Hour, rows[0].Window.To.Sub(rows[0].Window.From))

	absorbed := rows[1]
	require.True(t, absorbed.Window.From.Equal(h0.Add(4*time.Hour)))
	require.Equal(t, time.Hour, absorbed.Window.To.Sub(absorbed.Window.From))
	require.Equal(t, 4*time.Hour, absorbed.SpanTo.Sub(absorbed.SpanFrom))
	require.True(t, decimal.RequireFromString("40").Equal(*absorbed.Values[energy.ActiveImport]))
}

// bruteForceScenario is one random analyzer history around 1-4 consecutive
// calendar months, plus a random clock.
type bruteForceScenario struct {
	bounds   []time.Time
	readings fakeReadings
	now      time.Time
}

func genBruteForceScenario(rng *rand.Rand, loc *time.Location, id uuid.UUID) bruteForceScenario {
	start := time.Date(2025, time.Month(1+rng.IntN(20)), 1, 0, 0, 0, 0, loc)
	months := 1 + rng.IntN(4)
	bounds := make([]time.Time, months+1)
	for i := range bounds {
		bounds[i] = time.Date(start.Year(), start.Month()+time.Month(i), 1, 0, 0, 0, 0, loc)
	}

	type event struct {
		ts   time.Time
		kind model.ReadingKind
	}
	var events []event
	jitter := func(maxHours int) time.Duration {
		return time.Duration(rng.IntN(maxHours*60+1)) * time.Minute
	}
	for _, b := range bounds {
		if rng.IntN(10) < 6 {
			events = append(events, event{b.Add(-jitter(100)), model.ReadingKindBilling})
		}
		if rng.IntN(10) < 6 {
			events = append(events, event{b.Add(-jitter(45)), model.ReadingKindLoadProfile})
		}
		if rng.IntN(10) < 3 {
			events = append(events, event{b.Add(jitter(6)), model.ReadingKindLoadProfile})
		}
		if rng.IntN(10) < 5 {
			events = append(events, event{b.Add(-jitter(45)), model.ReadingKindDaily})
		}
	}
	hull := bounds[len(bounds)-1].Sub(bounds[0])
	for i, n := 0, rng.IntN(6); i < n; i++ {
		events = append(events, event{bounds[0].Add(time.Duration(rng.Int64N(int64(hull)))), model.ReadingKindLoadProfile})
	}
	for i, n := 0, rng.IntN(3); i < n; i++ {
		events = append(events, event{bounds[0].Add(time.Duration(rng.Int64N(int64(hull)))), model.ReadingKindBilling})
	}
	for i := 0; i < months; i++ {
		if rng.IntN(10) < 3 {
			width := bounds[i+1].Sub(bounds[i])
			events = append(events, event{bounds[i].Add(time.Duration(rng.Int64N(int64(width)))), model.ReadingKindReset})
		}
	}
	sort.SliceStable(events, func(i, j int) bool { return events[i].ts.Before(events[j].ts) })

	active, t1 := int64(1000+rng.IntN(5000)), int64(rng.IntN(3000))
	seen := map[string]bool{}
	byKind := map[model.ReadingKind][]model.MeterReading{}
	for _, e := range events {
		key := fmt.Sprintf("%d/%s", e.ts.UnixNano(), e.kind)
		if seen[key] {
			continue
		}
		seen[key] = true
		if e.kind == model.ReadingKindReset {
			active, t1 = int64(rng.IntN(50)), int64(rng.IntN(20))
		} else {
			active += int64(rng.IntN(800))
			t1 += int64(rng.IntN(300))
			if rng.IntN(20) == 0 {
				active -= int64(rng.IntN(400))
			}
		}
		v := map[string]string{"active_import": fmt.Sprint(active)}
		if rng.IntN(10) > 0 {
			v["t1_import"] = fmt.Sprint(t1)
		}
		if e.kind != model.ReadingKindReset && rng.IntN(3) == 0 {
			v["max_demand_kw"] = fmt.Sprint(rng.IntN(1000))
		}
		byKind[e.kind] = append(byKind[e.kind], readingRow(id, e.ts, e.kind, v))
	}

	now := bounds[len(bounds)-1].Add(time.Duration(rng.IntN(130)-24) * time.Hour)
	return bruteForceScenario{bounds: bounds, readings: fakeReadings{byKind: byKind}, now: now}
}

func recordedAnomalies(a *fakeAnomalies) []string {
	out := make([]string, 0, len(a.rows))
	for _, r := range a.rows {
		out = append(out, fmt.Sprintf("%s|%s|%s|%s", r.PeriodStart.UTC().Format(time.RFC3339), r.PeriodEnd.UTC().Format(time.RFC3339), r.Reason, r.Detail))
	}
	sort.Strings(out)
	return out
}

// TestPeriodConsumptionCutoffOneMatchesMonthlyBruteForce: R107 — over random
// histories, cut-off-day-1 windows give exactly Monthly's rows and exactly
// the anomalies ConsumptionAndRecord writes.
func TestPeriodConsumptionCutoffOneMatchesMonthlyBruteForce(t *testing.T) {
	const scenarios = 1000
	loc := istanbulLoc(t)
	rng := rand.New(rand.NewPCG(20260917, 107))
	ctx := context.Background()
	var withRows, withSuspect, withBillingSource, withAnomalies int

	for i := range scenarios {
		id := uuid.New()
		sc := genBruteForceScenario(rng, loc, id)
		scope := periodScope()

		monthlyAnomalies, periodAnomalies := &fakeAnomalies{}, &fakeAnomalies{}
		bm := resolveBilling(t, sc.readings, monthlyAnomalies, &fakeOps{}, fakeAnalyzers{}, lock.NewMemory(nil), sc.now)
		bp := resolveBilling(t, sc.readings, periodAnomalies, &fakeOps{}, fakeAnalyzers{}, lock.NewMemory(nil), sc.now)

		monthly, err := bm.ConsumptionAndRecord(ctx, scope, consumption.SeriesRequest{
			AnalyzerIDs: []uuid.UUID{id}, Level: energy.Monthly,
			Range: store.TimeRange{From: sc.bounds[0], To: sc.bounds[len(sc.bounds)-1]},
		})
		require.NoError(t, err, "scenario %d", i)
		windows := make([]energy.Window, len(sc.bounds)-1)
		for j := range windows {
			windows[j] = win(sc.bounds[j], sc.bounds[j+1])
		}
		period, err := bp.PeriodConsumptionAndRecord(ctx, scope, consumption.PeriodRequest{AnalyzerIDs: []uuid.UUID{id}, Windows: windows})
		require.NoError(t, err, "scenario %d", i)

		require.Equal(t, monthly, period, "scenario %d: rows differ", i)
		require.Equal(t, recordedAnomalies(monthlyAnomalies), recordedAnomalies(periodAnomalies), "scenario %d: anomalies differ", i)

		if len(monthly) > 0 {
			withRows++
		}
		for _, r := range monthly {
			if len(r.Suspect) > 0 {
				withSuspect++
				break
			}
		}
		for _, r := range monthly {
			if r.Source == energy.KindBilling {
				withBillingSource++
				break
			}
		}
		if len(monthlyAnomalies.rows) > 0 {
			withAnomalies++
		}
	}
	t.Logf("scenarios=%d withRows=%d withSuspect=%d withBillingSource=%d withAnomalies=%d", scenarios, withRows, withSuspect, withBillingSource, withAnomalies)
	for name, n := range map[string]int{"rows": withRows, "suspect": withSuspect, "billing source": withBillingSource, "anomalies": withAnomalies} {
		require.Greater(t, n, scenarios/10, "the generator must exercise %s", name)
	}
}

// TestPeriodConsumptionAppliesAGapOverrideOnlyForThatExactWindow: a resolved
// missing_readings override fills exactly its own cut-off window (R97 rule 7).
func TestPeriodConsumptionAppliesAGapOverrideOnlyForThatExactWindow(t *testing.T) {
	loc := istanbulLoc(t)
	jan15 := time.Date(2026, 1, 15, 0, 0, 0, 0, loc)
	feb15 := time.Date(2026, 2, 15, 0, 0, 0, 0, loc)
	feb16 := feb15.Add(24 * time.Hour)
	id := uuid.New()
	readings := fakeReadings{byKind: map[model.ReadingKind][]model.MeterReading{
		model.ReadingKindLoadProfile: {readingRow(id, jan15, model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"})},
	}}
	resolvedAt := jan15
	resolution := string(consumption.ResolveByOverride)
	anomalies := &fakeAnomalies{}
	anomalies.seed(model.ConsumptionAnomaly{
		AnalyzerID: id, PeriodStart: jan15, PeriodEnd: feb15, Reason: string(energy.ReasonMissingReadings),
		Detail:     []byte(`{"code":"consumption.suspect_period","boundaries":["end"]}`),
		ResolvedAt: &resolvedAt, Resolution: &resolution, OverrideValues: []byte(`{"active_import":"42"}`),
	})
	b := resolveBilling(t, readings, anomalies, &fakeOps{}, fakeAnalyzers{}, lock.NewMemory(nil), feb16.Add(consumption.SettleDelayMonthly))
	ctx := context.Background()

	exact, err := b.PeriodConsumption(ctx, periodScope(), consumption.PeriodRequest{AnalyzerIDs: []uuid.UUID{id}, Windows: []energy.Window{win(jan15, feb15)}})
	require.NoError(t, err)
	require.Len(t, exact, 1)
	require.Equal(t, "42", exact[0].Values[energy.ActiveImport].String())
	require.Equal(t, "1000", exact[0].StartIndexes[energy.ActiveImport].String())
	require.True(t, exact[0].SpanFrom.IsZero() && exact[0].SpanTo.IsZero(), "an override row has no derived span")

	other, err := b.PeriodConsumption(ctx, periodScope(), consumption.PeriodRequest{AnalyzerIDs: []uuid.UUID{id}, Windows: []energy.Window{win(jan15, feb16)}})
	require.NoError(t, err)
	require.Empty(t, other)
}
