package forecast

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/ml"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// Read is R373: the latest stored run covering the window; never calls the ML service.
func (s *Service) Read(ctx context.Context, sc store.Scope, analyzerID uuid.UUID, from, to time.Time) (Result, error) {
	r := store.TimeRange{From: from, To: to}
	if !r.Valid() {
		return Result{}, validation("to", "invalid_range")
	}
	if to.Sub(from) > maxReadDays*24*time.Hour {
		return Result{}, validation("to", "range_too_long")
	}
	a, err := s.d.Analyzers.Get(ctx, sc, analyzerID)
	if err != nil {
		return Result{}, err
	}
	rows, err := s.d.Forecasts.LatestRun(ctx, sc, a.ID, r)
	if err != nil {
		return Result{}, err
	}
	if len(rows) == 0 {
		return Result{Status: "none"}, nil
	}
	generated := rows[0].GeneratedAt
	res := Result{Status: "ok", ModelID: rows[0].ModelID, GeneratedAt: &generated}
	if rows[0].ModelVersion != nil {
		res.ModelVersion = *rows[0].ModelVersion
	}
	for _, row := range rows {
		res.Points = append(res.Points, Point{Ts: row.Ts, Median: row.Median, P10: row.P10, P90: row.P90})
	}
	if res.Gaps, err = s.d.Forecasts.Gaps(ctx, sc, a.ID, generated); err != nil {
		return Result{}, err
	}
	return res, nil
}

// Weekly is R374: an on-demand hourly forecast for the week starting weekStart (an Istanbul Monday).
func (s *Service) Weekly(ctx context.Context, sc store.Scope, analyzerID uuid.UUID, weekStart time.Time) (Result, error) {
	ws := civil(weekStart)
	if ws.Weekday() != time.Monday {
		return Result{}, validation("week_start", "not_monday")
	}
	today := civil(s.d.Clock.Now().In(istanbul))
	thisMonday := today.AddDate(0, 0, -((int(today.Weekday()) + 6) % 7))
	if ws.Before(thisMonday) || ws.After(today.AddDate(0, 0, weekAheadDays)) {
		return Result{}, validation("week_start", "out_of_range")
	}
	a, err := s.d.Analyzers.Get(ctx, sc, analyzerID)
	if err != nil {
		return Result{}, err
	}
	start, end := midnight(ws), midnight(ws.AddDate(0, 0, 7))
	horizon := int(end.Sub(s.lastHour()) / time.Hour)
	res, err := s.hourly(ctx, sc, a, horizon)
	if err != nil {
		return Result{}, err
	}
	return window(res, start, end), nil
}

// Monthly is R374: an on-demand daily forecast for a calendar month.
func (s *Service) Monthly(ctx context.Context, sc store.Scope, analyzerID uuid.UUID, month string) (Result, error) {
	first, err := time.Parse("2006-01", month)
	if err != nil {
		return Result{}, validation("month", "invalid")
	}
	today := civil(s.d.Clock.Now().In(istanbul))
	after := first.AddDate(0, 1, 0) // first day after the month
	horizon := int(after.Sub(today).Hours() / 24)
	if !after.After(today) || horizon > maxHorizonDays {
		return Result{}, validation("month", "out_of_range")
	}
	a, err := s.d.Analyzers.Get(ctx, sc, analyzerID)
	if err != nil {
		return Result{}, err
	}
	history, err := s.series(ctx, sc, a.ID, store.TimeRange{From: midnight(today.AddDate(0, 0, -dailyHistoryDays)), To: midnight(today)}, 24*time.Hour)
	if err != nil {
		return Result{}, err
	}
	cov, err := s.covariates(ctx, sc, midnight(today.AddDate(0, 0, -dailyHistoryDays)), midnight(after))
	if err != nil {
		return Result{}, err
	}
	res, err := s.call(ctx, ml.ForecastRequest{SeriesID: a.ID.String(), Granularity: "day", Horizon: horizon, History: history, Covariates: cov})
	if err != nil {
		return Result{}, err
	}
	return window(res, midnight(first), midnight(after)), nil
}

func window(res Result, from, to time.Time) Result {
	kept := res.Points[:0:0]
	for _, p := range res.Points {
		if !p.Ts.Before(from) && p.Ts.Before(to) {
			kept = append(kept, p)
		}
	}
	res.Points = kept
	return res
}

// Anomaly is R375/Q-I13: check one hour against the 12 weeks before it.
func (s *Service) Anomaly(ctx context.Context, sc store.Scope, analyzerID uuid.UUID, ts time.Time, actual *decimal.Decimal) (AnomalyResult, error) {
	ts = ts.UTC().Truncate(time.Hour)
	if ts.After(s.d.Clock.Now()) {
		return AnomalyResult{}, validation("ts", "future")
	}
	a, err := s.d.Analyzers.Get(ctx, sc, analyzerID)
	if err != nil {
		return AnomalyResult{}, err
	}
	if actual == nil {
		at, err := s.series(ctx, sc, a.ID, store.TimeRange{From: ts, To: ts.Add(time.Hour)}, time.Hour)
		if err != nil {
			return AnomalyResult{}, err
		}
		if len(at) == 0 || at[0].Value == nil {
			return AnomalyResult{}, validation("actual", "required")
		}
		v := decimal.NewFromFloat(*at[0].Value)
		actual = &v
	}
	history, err := s.series(ctx, sc, a.ID, store.TimeRange{From: ts.Add(-anomalyWeeks * 7 * 24 * time.Hour), To: ts}, time.Hour)
	if err != nil {
		return AnomalyResult{}, err
	}
	out, err := s.d.ML.Anomaly(ctx, ml.AnomalyRequest{SeriesID: a.ID.String(), Ts: ts, Actual: actual.InexactFloat64(), History: history})
	if err != nil {
		return AnomalyResult{}, unavailable(err)
	}
	return AnomalyResult{IsAnomaly: out.IsAnomaly, Score: decOrNil(out.Score), Expected: decOrNil(out.Expected), Lower: decOrNil(out.Lower),
		Upper: decOrNil(out.Upper), Method: out.Method, ModelID: out.ModelID, Version: out.ModelVersion, Actual: *actual}, nil
}
