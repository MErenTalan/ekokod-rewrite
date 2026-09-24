package solar

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	perr "github.com/MErenTalan/ekokod-rewrite/internal/platform/errors"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// connectedWithin is R282's freshness bound for "connected" (three missed 15-min ticks).
const connectedWithin = 45 * time.Minute

// plantFor answers ErrNotFound unless the caller sees the whole company:
// plants are company-level and never building-scoped (F-1).
func (s *Service) plantFor(ctx context.Context, sc store.Scope, id uuid.UUID) (model.PowerPlant, error) {
	if !sc.Valid() || !sc.AllBuildings {
		return model.PowerPlant{}, store.ErrNotFound
	}
	return s.d.Plants.Get(ctx, sc, id)
}

func validation(field string, codes ...string) error {
	return perr.Validation.WithParams(map[string]any{field: codes})
}

// Realtime is R282's summary from the inverters' last snapshots.
type Realtime struct {
	AsOf                   *time.Time
	Stale                  bool
	InverterCount          int
	ActivePowerKw          *decimal.Decimal
	YieldTodayKwh          *decimal.Decimal
	YieldMonthKwh          *decimal.Decimal
	YieldYearKwh           *decimal.Decimal
	YieldTotalKwh          *decimal.Decimal
	CapacityKw             *decimal.Decimal
	CapacityUtilisationPct *decimal.Decimal
	Connection             string // connected | error | never_synced
	ConnectionError        *string
	LastSyncAt             *time.Time
}

// Realtime sums the plant's inverter snapshots (R282).
func (s *Service) Realtime(ctx context.Context, sc store.Scope, plantID uuid.UUID) (Realtime, error) {
	plant, err := s.plantFor(ctx, sc, plantID)
	if err != nil {
		return Realtime{}, err
	}
	devices, err := s.d.Plants.Devices(ctx, sc, plantID)
	if err != nil {
		return Realtime{}, err
	}
	now := s.d.Clock.Now()
	var rt Realtime
	for _, d := range devices {
		if !isInverter(d) || d.SnapshotAt == nil {
			continue
		}
		rt.InverterCount++
		if rt.AsOf == nil || d.SnapshotAt.Before(*rt.AsOf) {
			at := *d.SnapshotAt
			rt.AsOf = &at
		}
		rt.ActivePowerKw = addPtr(rt.ActivePowerKw, d.ActivePowerKw)
		rt.YieldTodayKwh = addPtr(rt.YieldTodayKwh, d.YieldTodayKwh)
		rt.YieldMonthKwh = addPtr(rt.YieldMonthKwh, d.YieldMonthKwh)
		rt.YieldYearKwh = addPtr(rt.YieldYearKwh, d.YieldYearKwh)
		rt.YieldTotalKwh = addPtr(rt.YieldTotalKwh, d.YieldTotalKwh)
	}
	rt.Stale = rt.AsOf != nil && now.Sub(*rt.AsOf) > staleAfter
	rt.CapacityKw = plant.IsolarInstalledKw
	if rt.CapacityKw == nil {
		rt.CapacityKw = plant.TotalCapacityKw
	}
	rt.CapacityUtilisationPct = Utilisation(rt.ActivePowerKw, rt.CapacityKw)
	rt.LastSyncAt, rt.ConnectionError = plant.IsolarLastSyncAt, plant.IsolarLastSyncError
	switch {
	case plant.IsolarLastSyncAt == nil:
		rt.Connection = "never_synced"
	case plant.IsolarLastSyncError != nil || now.Sub(*plant.IsolarLastSyncAt) > connectedWithin:
		rt.Connection = "error"
	default:
		rt.Connection = "connected"
	}
	return rt, nil
}

// Point is one production series value; Basis is nil for monthly points.
type Point struct {
	Ts            time.Time
	ProductionKwh *decimal.Decimal
	Basis         *string
}

// Series is R284's production series.
type Series struct {
	Granularity string
	Points      []Point
	MixedBasis  bool
}

var seriesLimit = map[string]func(from time.Time) time.Time{
	"hour":  func(f time.Time) time.Time { return f.AddDate(0, 0, 31) },
	"day":   func(f time.Time) time.Time { return f.AddDate(0, 0, 366) },
	"month": func(f time.Time) time.Time { return f.AddDate(10, 0, 0) },
}

// ProductionSeries is R284: hour and day from the totals (with their basis),
// month from the real-time daily aggregate, so the open month is always whole.
func (s *Service) ProductionSeries(ctx context.Context, sc store.Scope, plantID uuid.UUID, gran string, from, to time.Time) (Series, error) {
	limit, ok := seriesLimit[gran]
	if !ok {
		return Series{}, validation("granularity", "oneof")
	}
	if !from.Before(to) {
		return Series{}, validation("to", "after_from")
	}
	if to.After(limit(from)) {
		return Series{}, validation("to", "range_too_long")
	}
	if _, err := s.plantFor(ctx, sc, plantID); err != nil {
		return Series{}, err
	}
	out := Series{Granularity: gran}
	if gran == "month" {
		buckets, err := s.d.Analytics.ProductionDaily(ctx, sc, []uuid.UUID{plantID}, store.TimeRange{From: from, To: to})
		if err != nil {
			return Series{}, err
		}
		byMonth := map[time.Time]*decimal.Decimal{}
		for _, b := range buckets {
			l := b.Bucket.In(istanbul)
			m := time.Date(l.Year(), l.Month(), 1, 0, 0, 0, 0, istanbul)
			byMonth[m] = addPtr(byMonth[m], b.ProductionKwh)
		}
		for m, v := range byMonth {
			out.Points = append(out.Points, Point{Ts: m, ProductionKwh: v})
		}
		sort.Slice(out.Points, func(i, j int) bool { return out.Points[i].Ts.Before(out.Points[j].Ts) })
		return out, nil
	}
	rows, err := s.d.Totals.Range(ctx, sc, plantID, store.TimeRange{From: from, To: to})
	if err != nil {
		return Series{}, err
	}
	type acc struct {
		kwh   *decimal.Decimal
		basis string
	}
	buckets := map[time.Time]*acc{}
	bases := map[string]bool{}
	for _, r := range rows {
		l := r.Ts.In(istanbul)
		key := time.Date(l.Year(), l.Month(), l.Day(), l.Hour(), 0, 0, 0, istanbul)
		if gran == "day" {
			key = dayOf(r.Ts)
		}
		a := buckets[key]
		if a == nil {
			a = &acc{basis: r.Basis}
			buckets[key] = a
		}
		a.kwh = addPtr(a.kwh, r.ProductionKwh)
		bases[r.Basis] = true
	}
	for ts, a := range buckets {
		basis := a.basis
		out.Points = append(out.Points, Point{Ts: ts, ProductionKwh: a.kwh, Basis: &basis})
	}
	sort.Slice(out.Points, func(i, j int) bool { return out.Points[i].Ts.Before(out.Points[j].Ts) })
	out.MixedBasis = len(bases) > 1
	return out, nil
}

// DeviceView is one device row of the devices tab (R285).
type DeviceView struct {
	ID            uuid.UUID
	DeviceSN      string
	DeviceName    *string
	DeviceType    *int32
	Status        *string
	ActivePowerKw *decimal.Decimal
	YieldTodayKwh *decimal.Decimal
	YieldTotalKwh *decimal.Decimal
	LastUpdate    *time.Time
}

// Devices lists the plant's devices, filtered by name or serial (R285).
func (s *Service) Devices(ctx context.Context, sc store.Scope, plantID uuid.UUID, q string) ([]DeviceView, error) {
	if _, err := s.plantFor(ctx, sc, plantID); err != nil {
		return nil, err
	}
	devices, err := s.d.Plants.Devices(ctx, sc, plantID)
	if err != nil {
		return nil, err
	}
	needle := strings.ToLower(strings.TrimSpace(q))
	now := s.d.Clock.Now()
	out := []DeviceView{}
	for _, d := range devices {
		name := ""
		if d.DeviceName != nil {
			name = *d.DeviceName
		}
		if needle != "" && !strings.Contains(strings.ToLower(d.DeviceSN), needle) && !strings.Contains(strings.ToLower(name), needle) {
			continue
		}
		out = append(out, DeviceView{ID: d.ID, DeviceSN: d.DeviceSN, DeviceName: d.DeviceName, DeviceType: d.DeviceType,
			Status: StatusOf(d.FaultStatus, d.SnapshotAt, now), ActivePowerKw: d.ActivePowerKw, YieldTodayKwh: d.YieldTodayKwh,
			YieldTotalKwh: d.YieldTotalKwh, LastUpdate: d.SnapshotAt})
	}
	return out, nil
}

// FaultView is one alarm row (R286).
type FaultView struct {
	model.PlantFault
	MessageTr  string
	Translated bool
}

// Alarms pages the plant's stored faults, newest first.
func (s *Service) Alarms(ctx context.Context, sc store.Scope, plantID uuid.UUID, p store.Page) ([]FaultView, int, error) {
	if _, err := s.plantFor(ctx, sc, plantID); err != nil {
		return nil, 0, err
	}
	faults, total, err := s.d.Faults.List(ctx, sc, plantID, p)
	if err != nil {
		return nil, 0, err
	}
	out := make([]FaultView, len(faults))
	for i, f := range faults {
		_, translated := TranslateFault(f.Name)
		out[i] = FaultView{PlantFault: f, MessageTr: FaultSentence(f), Translated: translated}
	}
	return out, total, nil
}

// RevenuePeriod is one revenue card (R283).
type RevenuePeriod struct {
	Amounts      []Revenue
	UnpricedDays int
	Partial      bool
	Since        *time.Time
}

// RevenueView is GET /plants/{id}/revenue.
type RevenueView struct {
	Available                     bool
	Reason                        string
	Daily, Monthly, Yearly, Total RevenuePeriod
}

// Revenue prices stored daily production at the feed-in tariff effective each day (R283).
func (s *Service) Revenue(ctx context.Context, sc store.Scope, plantID uuid.UUID) (RevenueView, error) {
	if _, err := s.plantFor(ctx, sc, plantID); err != nil {
		return RevenueView{}, err
	}
	history, err := s.d.Solar.List(ctx, sc, store.SolarTariffFilter{PlantID: &plantID, Page: store.Page{Limit: 500}})
	if err != nil {
		return RevenueView{}, err
	}
	if len(history) == 0 {
		return RevenueView{Reason: "no_solar_tariff"}, nil
	}
	tariffs := make([]DayTariff, len(history))
	for i, t := range history {
		tariffs[i] = DayTariff{From: dayOf(t.EffectiveFrom), Price: t.FeedInTariff, Currency: t.Currency}
	}
	view := RevenueView{Available: true}
	first, err := s.d.Totals.FirstDay(ctx, sc, plantID)
	if err != nil || first == nil {
		return view, err
	}
	today := dayOf(s.d.Clock.Now())
	buckets, err := s.d.Analytics.ProductionDaily(ctx, sc, []uuid.UUID{plantID}, store.TimeRange{From: *first, To: today.AddDate(0, 0, 1)})
	if err != nil {
		return RevenueView{}, err
	}
	all := map[time.Time]decimal.Decimal{}
	for _, b := range buckets {
		if b.ProductionKwh != nil {
			all[dayOf(b.Bucket)] = *b.ProductionKwh
		}
	}
	period := func(from time.Time) RevenuePeriod {
		days := map[time.Time]decimal.Decimal{}
		for d, v := range all {
			if !d.Before(from) {
				days[d] = v
			}
		}
		amounts, unpriced := RevenueOver(days, tariffs)
		return RevenuePeriod{Amounts: amounts, UnpricedDays: unpriced, Partial: unpriced > 0}
	}
	view.Daily = period(today)
	view.Monthly = period(time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, istanbul))
	view.Yearly = period(time.Date(today.Year(), 1, 1, 0, 0, 0, 0, istanbul))
	view.Total = period(*first)
	view.Total.Since = first
	return view, nil
}
