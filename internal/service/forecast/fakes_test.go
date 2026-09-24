package forecast_test

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/ml"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

var (
	company  = uuid.MustParse("00000000-0000-0000-0000-00000000c001")
	analyzer = uuid.MustParse("00000000-0000-0000-0000-00000000a001")
	foreign  = uuid.MustParse("00000000-0000-0000-0000-00000000a999")
)

type fakeAnalyzers struct {
	store.AnalyzerRepository
	list []model.Analyzer
}

func (f *fakeAnalyzers) Get(_ context.Context, _ store.Scope, id uuid.UUID) (model.Analyzer, error) {
	if id != analyzer {
		return model.Analyzer{}, store.ErrNotFound
	}
	return model.Analyzer{ID: analyzer, CompanyID: company, IsActive: true}, nil
}

func (f *fakeAnalyzers) List(_ context.Context, _ store.Scope, _ store.AnalyzerFilter) ([]model.Analyzer, error) {
	return f.list, nil
}

type fakeAnalytics struct {
	store.AnalyticsRepository
	hourly map[time.Time]decimal.Decimal
	daily  map[time.Time]decimal.Decimal
	asked  []store.TimeRange
}

func buckets(m map[time.Time]decimal.Decimal, r store.TimeRange) []model.ConsumptionBucket {
	var out []model.ConsumptionBucket
	for ts, v := range m {
		if !ts.Before(r.From) && ts.Before(r.To) {
			v := v
			out = append(out, model.ConsumptionBucket{AnalyzerID: analyzer, Bucket: ts, ActiveConsumption: &v})
		}
	}
	return out
}

func (f *fakeAnalytics) ConsumptionHourly(_ context.Context, _ store.Scope, _ []uuid.UUID, r store.TimeRange) ([]model.ConsumptionBucket, error) {
	f.asked = append(f.asked, r)
	return buckets(f.hourly, r), nil
}

func (f *fakeAnalytics) ConsumptionDaily(_ context.Context, _ store.Scope, _ []uuid.UUID, r store.TimeRange) ([]model.ConsumptionBucket, error) {
	f.asked = append(f.asked, r)
	return buckets(f.daily, r), nil
}

type fakeCalendar struct {
	store.CalendarRepository
	weekend   []int16
	vacations []model.CompanyVacation
}

func (f *fakeCalendar) WeekendDays(_ context.Context, _ store.Scope) ([]model.CompanyWeekendDay, error) {
	out := make([]model.CompanyWeekendDay, len(f.weekend))
	for i, d := range f.weekend {
		out[i] = model.CompanyWeekendDay{CompanyID: company, DayOfWeek: d}
	}
	return out, nil
}

func (f *fakeCalendar) Vacations(_ context.Context, _ store.Scope, _ *store.TimeRange) ([]model.CompanyVacation, error) {
	return f.vacations, nil
}

type fakeForecasts struct {
	store.ForecastRepository
	rows []model.Forecast
	gaps []model.ForecastGap
}

func (f *fakeForecasts) BulkInsert(_ context.Context, _ store.Scope, rows []model.Forecast) (int, int, error) {
	f.rows = append(f.rows, rows...)
	return len(rows), 0, nil
}

func (f *fakeForecasts) RecordGaps(_ context.Context, _ store.Scope, gaps []model.ForecastGap) error {
	f.gaps = append(f.gaps, gaps...)
	return nil
}

func (f *fakeForecasts) LatestRun(_ context.Context, _ store.Scope, id uuid.UUID, r store.TimeRange) ([]model.Forecast, error) {
	if id != analyzer {
		return nil, store.ErrNotFound
	}
	var out []model.Forecast
	for _, row := range f.rows {
		if !row.Ts.Before(r.From) && row.Ts.Before(r.To) {
			out = append(out, row)
		}
	}
	return out, nil
}

func (f *fakeForecasts) Gaps(_ context.Context, _ store.Scope, _ uuid.UUID, at time.Time) ([]model.ForecastGap, error) {
	var out []model.ForecastGap
	for _, g := range f.gaps {
		if g.GeneratedAt.Equal(at) {
			out = append(out, g)
		}
	}
	return out, nil
}

// fakeML answers every forecast with `status`; values = 10 + step index.
type fakeML struct {
	status   string
	err      error
	requests []ml.ForecastRequest
	anomaly  []ml.AnomalyRequest
	gaps     []ml.Gap
}

func (f *fakeML) Forecast(_ context.Context, req ml.ForecastRequest) (ml.ForecastResponse, error) {
	f.requests = append(f.requests, req)
	if f.err != nil {
		return ml.ForecastResponse{}, f.err
	}
	res := ml.ForecastResponse{Status: f.status, ModelID: "random_forest", ModelVersion: "1.0.0", Gaps: f.gaps, UsedCovariates: []string{"day_type"}}
	if f.status != "ok" {
		return res, nil
	}
	step := time.Hour
	if req.Granularity == "day" {
		step = 24 * time.Hour
	}
	last := req.History[len(req.History)-1].Ts
	for k := range req.Horizon {
		res.Timestamps = append(res.Timestamps, last.Add(time.Duration(k+1)*step))
		res.Median = append(res.Median, 10+float64(k))
		res.P10 = append(res.P10, 9+float64(k))
		res.P90 = append(res.P90, 11+float64(k))
	}
	return res, nil
}

func (f *fakeML) Anomaly(_ context.Context, req ml.AnomalyRequest) (ml.AnomalyResponse, error) {
	f.anomaly = append(f.anomaly, req)
	if f.err != nil {
		return ml.AnomalyResponse{}, f.err
	}
	score := 4.2
	return ml.AnomalyResponse{IsAnomaly: true, Score: &score, Method: "robust_zscore_same_hour_of_week", ModelID: "m", ModelVersion: "1"}, nil
}
