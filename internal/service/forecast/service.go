// Package forecast builds 03 §6.3 requests from stored consumption and the
// company calendar, calls the ML service and persists runs (F13b R370–R379).
package forecast

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/ml"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	perr "github.com/MErenTalan/ekokod-rewrite/internal/platform/errors"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// ErrUnavailable is R371: forecasting is down; nothing else is.
var ErrUnavailable = perr.New("forecast_unavailable", 503, "errors.forecast.unavailable")

const (
	historyWeeks     = 8
	anomalyWeeks     = 12
	dailyHistoryDays = 180
	maxHorizonHours  = 744
	maxHorizonDays   = 62
	maxReadDays      = 62
	weekAheadDays    = 30
)

var istanbul = mustZone("Europe/Istanbul")

func mustZone(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}

// Predictor is the ML client's surface the service uses.
type Predictor interface {
	Forecast(ctx context.Context, req ml.ForecastRequest) (ml.ForecastResponse, error)
	Anomaly(ctx context.Context, req ml.AnomalyRequest) (ml.AnomalyResponse, error)
}

// Deps is what the service reads and writes.
type Deps struct {
	Analyzers store.AnalyzerRepository
	Analytics store.AnalyticsRepository
	Calendar  store.CalendarRepository
	Forecasts store.ForecastRepository
	Ops       store.OpsRepository
	Tenants   store.AdminTenantRepository
	ML        Predictor
	Clock     clock.Clock
}

// Service is the forecasting module.
type Service struct{ d Deps }

// New builds the service.
func New(d Deps) *Service {
	if d.Clock == nil {
		d.Clock = clock.System()
	}
	return &Service{d: d}
}

// Point is one forecast step.
type Point struct {
	Ts     time.Time
	Median decimal.Decimal
	P10    *decimal.Decimal
	P90    *decimal.Decimal
}

// Result is one forecast answer; Status is the ML status, or "none" for a read with no run.
type Result struct {
	Status         string
	ModelID        string
	ModelVersion   string
	GeneratedAt    *time.Time
	Points         []Point
	Gaps           []model.ForecastGap
	UsedCovariates []string
	FallbackFrom   *string
}

// AnomalyResult is the ML verdict plus the actual value that was checked; the
// ML service's floats are decimals from here on (no floats past the adapter).
type AnomalyResult struct {
	IsAnomaly                bool
	Score                    *decimal.Decimal
	Expected, Lower, Upper   *decimal.Decimal
	Method, ModelID, Version string
	Actual                   decimal.Decimal
}

func decOrNil(f *float64) *decimal.Decimal {
	if f == nil {
		return nil
	}
	d := dec(*f)
	return &d
}

func validation(field, code string) error {
	return perr.Validation.WithParams(map[string]any{field: []string{code}})
}

func unavailable(err error) error {
	return perr.Wrap(err, ErrUnavailable.Code, ErrUnavailable.HTTPStatus, ErrUnavailable.MessageKey)
}

// Run is R372: forecast now and persist the run.
func (s *Service) Run(ctx context.Context, sc store.Scope, analyzerID uuid.UUID, horizon int) (Result, error) {
	if horizon < 1 || horizon > maxHorizonHours {
		return Result{}, validation("horizon_hours", "out_of_range")
	}
	a, err := s.d.Analyzers.Get(ctx, sc, analyzerID)
	if err != nil {
		return Result{}, err
	}
	res, err := s.hourly(ctx, sc, a, horizon)
	if err != nil {
		return Result{}, err
	}
	generated := s.d.Clock.Now().UTC()
	if err := s.persist(ctx, sc, a.ID, generated, horizon, &res); err != nil {
		return Result{}, err
	}
	return res, nil
}

func (s *Service) persist(ctx context.Context, sc store.Scope, analyzerID uuid.UUID, generated time.Time, horizon int, res *Result) error {
	res.GeneratedAt = &generated
	for i := range res.Gaps {
		res.Gaps[i].AnalyzerID, res.Gaps[i].GeneratedAt = analyzerID, generated
	}
	if len(res.Gaps) > 0 {
		if err := s.d.Forecasts.RecordGaps(ctx, sc, res.Gaps); err != nil {
			return err
		}
	}
	if res.Status != "ok" || len(res.Points) == 0 {
		return nil
	}
	version := res.ModelVersion
	rows := make([]model.Forecast, len(res.Points))
	for i, p := range res.Points {
		rows[i] = model.Forecast{AnalyzerID: analyzerID, Ts: p.Ts, GeneratedAt: generated, HorizonHours: int32(horizon), //nolint:gosec // horizon ≤ 744
			Median: p.Median, P10: p.P10, P90: p.P90, ModelID: res.ModelID, ModelVersion: &version}
	}
	_, _, err := s.d.Forecasts.BulkInsert(ctx, sc, rows)
	return err
}

// lastHour is the start of the current (incomplete) hour: history ends before it.
func (s *Service) lastHour() time.Time { return s.d.Clock.Now().UTC().Truncate(time.Hour) }

func (s *Service) hourly(ctx context.Context, sc store.Scope, a model.Analyzer, horizon int) (Result, error) {
	to := s.lastHour()
	from := to.Add(-historyWeeks * 7 * 24 * time.Hour)
	history, err := s.series(ctx, sc, a.ID, store.TimeRange{From: from, To: to}, time.Hour)
	if err != nil {
		return Result{}, err
	}
	cov, err := s.covariates(ctx, sc, from, to.Add(time.Duration(horizon)*time.Hour))
	if err != nil {
		return Result{}, err
	}
	return s.call(ctx, ml.ForecastRequest{SeriesID: a.ID.String(), Granularity: "hour", Horizon: horizon, History: history, Covariates: cov})
}

func (s *Service) call(ctx context.Context, req ml.ForecastRequest) (Result, error) {
	out, err := s.d.ML.Forecast(ctx, req)
	if err != nil {
		if errors.Is(err, ml.ErrUnavailable) || errors.Is(err, ml.ErrRejected) {
			return Result{}, unavailable(err)
		}
		return Result{}, err
	}
	if len(out.Median) != len(out.Timestamps) || len(out.P10) != len(out.Timestamps) || len(out.P90) != len(out.Timestamps) {
		return Result{}, unavailable(fmt.Errorf("ml: %d timestamps, %d/%d/%d values", len(out.Timestamps), len(out.Median), len(out.P10), len(out.P90)))
	}
	res := Result{Status: out.Status, ModelID: out.ModelID, ModelVersion: out.ModelVersion, UsedCovariates: out.UsedCovariates, FallbackFrom: out.FallbackFrom}
	for i, ts := range out.Timestamps {
		p10, p90 := dec(out.P10[i]), dec(out.P90[i])
		res.Points = append(res.Points, Point{Ts: ts.UTC(), Median: dec(out.Median[i]), P10: &p10, P90: &p90})
	}
	for _, g := range out.Gaps {
		res.Gaps = append(res.Gaps, model.ForecastGap{GapStart: g.Start.UTC(), GapEnd: g.End.UTC(), MissingHours: int32(g.MissingHours)}) //nolint:gosec // bounded by the history window
	}
	return res, nil
}

func dec(f float64) decimal.Decimal { return decimal.NewFromFloat(f).Round(4) }

// series is R379: every step in r (half-open), null where no bucket exists;
// steps before the first reading are trimmed (Q-I10) so a new meter is not one big gap.
func (s *Service) series(ctx context.Context, sc store.Scope, id uuid.UUID, r store.TimeRange, step time.Duration) ([]ml.Point, error) {
	read := s.d.Analytics.ConsumptionHourly
	if step != time.Hour {
		read = s.d.Analytics.ConsumptionDaily
	}
	buckets, err := read(ctx, sc, []uuid.UUID{id}, r)
	if err != nil {
		return nil, err
	}
	values := make(map[int64]float64, len(buckets))
	for _, b := range buckets {
		if b.ActiveConsumption != nil {
			values[b.Bucket.UTC().Unix()] = b.ActiveConsumption.InexactFloat64()
		}
	}
	out := []ml.Point{}
	for t := r.From.UTC(); t.Before(r.To); t = t.Add(step) {
		p := ml.Point{Ts: t}
		if v, ok := values[t.Unix()]; ok {
			p.Value = &v
		}
		if len(out) == 0 && p.Value == nil {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}

// covariates is Q-I9: workday/weekend from the company's weekend days
// (Saturday and Sunday when none are set) and its vacations, on Istanbul dates.
func (s *Service) covariates(ctx context.Context, sc store.Scope, from, to time.Time) (ml.Covariates, error) {
	days, err := s.d.Calendar.WeekendDays(ctx, sc)
	if err != nil {
		return ml.Covariates{}, err
	}
	weekend := map[time.Weekday]bool{}
	for _, d := range days {
		weekend[time.Weekday(d.DayOfWeek)] = true
	}
	if len(weekend) == 0 {
		weekend[time.Saturday], weekend[time.Sunday] = true, true
	}
	first, last := civil(from.In(istanbul)), civil(to.In(istanbul))
	vacs, err := s.d.Calendar.Vacations(ctx, sc, &store.TimeRange{From: first, To: last.AddDate(0, 0, 1)})
	if err != nil {
		return ml.Covariates{}, err
	}
	var cov ml.Covariates
	for d := first; !d.After(last); d = d.AddDate(0, 0, 1) {
		kind := "workday"
		if weekend[d.Weekday()] {
			kind = "weekend"
		}
		date := d.Format(time.DateOnly)
		cov.DayType = append(cov.DayType, ml.DayType{Date: date, Type: kind})
		for _, v := range vacs {
			if !d.Before(civil(v.StartDate)) && !d.After(civil(v.EndDate)) {
				cov.Vacation = append(cov.Vacation, ml.Vacation{Date: date, Vacation: true})
				break
			}
		}
	}
	return cov, nil
}

func civil(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// midnight is the instant an Istanbul civil date starts.
func midnight(d time.Time) time.Time {
	y, m, dd := d.Date()
	return time.Date(y, m, dd, 0, 0, 0, 0, istanbul).UTC()
}
