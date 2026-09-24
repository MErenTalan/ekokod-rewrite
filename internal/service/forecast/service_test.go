package forecast_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/ml"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	perr "github.com/MErenTalan/ekokod-rewrite/internal/platform/errors"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/forecast"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// Wednesday 2026-09-23 10:37 Istanbul (07:37 UTC).
var now = time.Date(2026, 9, 23, 7, 37, 0, 0, time.UTC)

type rig struct {
	svc       *forecast.Service
	analytics *fakeAnalytics
	calendar  *fakeCalendar
	forecasts *fakeForecasts
	ml        *fakeML
}

func newRig(status string) *rig {
	r := &rig{
		analytics: &fakeAnalytics{hourly: map[time.Time]decimal.Decimal{}, daily: map[time.Time]decimal.Decimal{}},
		calendar:  &fakeCalendar{weekend: []int16{0, 6}},
		forecasts: &fakeForecasts{},
		ml:        &fakeML{status: status},
	}
	// Four weeks of hourly data ending at the last complete hour, one hour missing.
	for h := 1; h <= 4*168; h++ {
		ts := now.Truncate(time.Hour).Add(-time.Duration(h) * time.Hour)
		if h != 5 {
			r.analytics.hourly[ts] = decimal.NewFromInt(int64(h % 24))
		}
	}
	r.svc = forecast.New(forecast.Deps{Analyzers: &fakeAnalyzers{}, Analytics: r.analytics, Calendar: r.calendar,
		Forecasts: r.forecasts, ML: r.ml, Clock: clock.NewFake(now)})
	return r
}

var sc = store.Scope{CompanyID: company, AllBuildings: true}

func requireValidation(t *testing.T, err error, field, code string) {
	t.Helper()
	var pe *perr.Error
	require.True(t, errors.As(err, &pe), "want validation, got %v", err)
	require.Equal(t, "validation_failed", pe.Code)
	require.Equal(t, []string{code}, pe.Params[field], "params %v", pe.Params)
}

func TestRunBuildsTheContractRequest(t *testing.T) {
	r := newRig("ok")
	r.calendar.vacations = []model.CompanyVacation{{StartDate: time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC), EndDate: time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC)}}
	_, err := r.svc.Run(context.Background(), sc, analyzer, 48)
	require.NoError(t, err)
	req := r.ml.requests[0]
	require.Equal(t, analyzer.String(), req.SeriesID)
	require.Equal(t, "hour", req.Granularity)
	require.Equal(t, 48, req.Horizon)
	last := now.Truncate(time.Hour).Add(-time.Hour)
	require.Equal(t, last, req.History[len(req.History)-1].Ts, "history ends at the last complete hour")
	require.Equal(t, last.Add(-(4*168-1)*time.Hour), req.History[0].Ts, "leading hours before the first reading are trimmed (Q-I10)")
	require.Nil(t, req.History[len(req.History)-5].Value, "a missing bucket travels as null (R379)")
	types := map[string]string{}
	for _, d := range req.Covariates.DayType {
		types[d.Date] = d.Type
	}
	require.Equal(t, "weekend", types["2026-09-19"])
	require.Equal(t, "workday", types["2026-09-24"])
	require.Contains(t, types, "2026-09-25", "day types cover the horizon too")
	vac := map[string]bool{}
	for _, v := range req.Covariates.Vacation {
		vac[v.Date] = v.Vacation
	}
	require.True(t, vac["2026-09-21"])
	require.True(t, vac["2026-09-22"])
	require.False(t, vac["2026-09-23"])
	require.False(t, vac["2026-09-20"])
}

func TestRunPersistsAnOkRunAndItsGaps(t *testing.T) {
	r := newRig("ok")
	r.ml.gaps = []ml.Gap{{Start: now.Add(-5 * time.Hour), End: now.Add(-5 * time.Hour), MissingHours: 1}}
	res, err := r.svc.Run(context.Background(), sc, analyzer, 24)
	require.NoError(t, err)
	require.Equal(t, "ok", res.Status)
	require.Len(t, res.Points, 24)
	require.Len(t, r.forecasts.rows, 24)
	row := r.forecasts.rows[0]
	require.Equal(t, now, row.GeneratedAt)
	require.EqualValues(t, 24, row.HorizonHours)
	require.True(t, row.Median.Equal(decimal.NewFromInt(10)))
	require.Equal(t, "random_forest", row.ModelID)
	require.Equal(t, "1.0.0", *row.ModelVersion)
	require.Len(t, r.forecasts.gaps, 1)
	require.EqualValues(t, 1, r.forecasts.gaps[0].MissingHours)
	require.Equal(t, now, r.forecasts.gaps[0].GeneratedAt)
}

func TestRunWithoutAForecastStoresOnlyGaps(t *testing.T) {
	r := newRig("insufficient_data")
	r.ml.gaps = []ml.Gap{{Start: now.Add(-9 * time.Hour), End: now.Add(-8 * time.Hour), MissingHours: 2}}
	res, err := r.svc.Run(context.Background(), sc, analyzer, 24)
	require.NoError(t, err)
	require.Equal(t, "insufficient_data", res.Status)
	require.Empty(t, res.Points)
	require.Empty(t, r.forecasts.rows)
	require.Len(t, r.forecasts.gaps, 1)
}

func TestUnavailableServiceIs503AndWritesNothing(t *testing.T) {
	for _, cause := range []error{ml.ErrUnavailable, ml.ErrRejected} {
		r := newRig("ok")
		r.ml.err = cause
		_, err := r.svc.Run(context.Background(), sc, analyzer, 24)
		require.ErrorIs(t, err, forecast.ErrUnavailable)
		var pe *perr.Error
		require.True(t, errors.As(err, &pe))
		require.Equal(t, 503, pe.HTTPStatus)
		require.Empty(t, r.forecasts.rows)
		require.Empty(t, r.forecasts.gaps)
	}
}

func TestRunRefusesForeignAnalyzersAndBadHorizons(t *testing.T) {
	r := newRig("ok")
	_, err := r.svc.Run(context.Background(), sc, foreign, 24)
	require.ErrorIs(t, err, store.ErrNotFound)
	for _, h := range []int{0, 745} {
		_, err := r.svc.Run(context.Background(), sc, analyzer, h)
		requireValidation(t, err, "horizon_hours", "out_of_range")
	}
	require.Empty(t, r.ml.requests)
}

func TestReadReturnsTheLatestRunWithItsGaps(t *testing.T) {
	r := newRig("ok")
	r.ml.gaps = []ml.Gap{{Start: now.Add(-5 * time.Hour), End: now.Add(-5 * time.Hour), MissingHours: 1}}
	_, err := r.svc.Run(context.Background(), sc, analyzer, 48)
	require.NoError(t, err)
	res, err := r.svc.Read(context.Background(), sc, analyzer, now, now.Add(24*time.Hour))
	require.NoError(t, err)
	require.Equal(t, "ok", res.Status)
	require.Len(t, res.Points, 24)
	require.Equal(t, now, *res.GeneratedAt)
	require.Len(t, res.Gaps, 1)

	empty, err := r.svc.Read(context.Background(), sc, analyzer, now.Add(30*24*time.Hour), now.Add(31*24*time.Hour))
	require.NoError(t, err)
	require.Equal(t, "none", empty.Status)

	_, err = r.svc.Read(context.Background(), sc, analyzer, now, now.Add(63*24*time.Hour))
	requireValidation(t, err, "to", "range_too_long")
	_, err = r.svc.Read(context.Background(), sc, foreign, now, now.Add(time.Hour))
	require.ErrorIs(t, err, store.ErrNotFound)
}

func TestWeeklyReturnsTheWeekWithoutStoringIt(t *testing.T) {
	r := newRig("ok")
	res, err := r.svc.Weekly(context.Background(), sc, analyzer, time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.Len(t, res.Points, 168)
	istanbul, _ := time.LoadLocation("Europe/Istanbul")
	require.Equal(t, time.Date(2026, 9, 28, 0, 0, 0, 0, istanbul), res.Points[0].Ts.In(istanbul))
	require.Empty(t, r.forecasts.rows, "weekly is on demand (Q-I11)")

	_, err = r.svc.Weekly(context.Background(), sc, analyzer, time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC))
	requireValidation(t, err, "week_start", "not_monday")
	_, err = r.svc.Weekly(context.Background(), sc, analyzer, time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC))
	requireValidation(t, err, "week_start", "out_of_range")
	_, err = r.svc.Weekly(context.Background(), sc, analyzer, time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC))
	requireValidation(t, err, "week_start", "out_of_range")
}

func TestMonthlyForecastsDaysOfTheMonth(t *testing.T) {
	r := newRig("ok")
	istanbul, _ := time.LoadLocation("Europe/Istanbul")
	for d := 1; d <= 60; d++ {
		day := time.Date(2026, 9, 23, 0, 0, 0, 0, istanbul).AddDate(0, 0, -d)
		r.analytics.daily[day.UTC()] = decimal.NewFromInt(100)
	}
	res, err := r.svc.Monthly(context.Background(), sc, analyzer, "2026-10")
	require.NoError(t, err)
	require.Len(t, res.Points, 31)
	require.Equal(t, time.Date(2026, 10, 1, 0, 0, 0, 0, istanbul), res.Points[0].Ts.In(istanbul))
	req := r.ml.requests[0]
	require.Equal(t, "day", req.Granularity)
	require.Equal(t, 39, req.Horizon, "Sep 23 .. Oct 31")
	require.Empty(t, r.forecasts.rows)

	_, err = r.svc.Monthly(context.Background(), sc, analyzer, "2027-01")
	requireValidation(t, err, "month", "out_of_range")
	_, err = r.svc.Monthly(context.Background(), sc, analyzer, "2026-08")
	requireValidation(t, err, "month", "out_of_range")
	_, err = r.svc.Monthly(context.Background(), sc, analyzer, "2026-13")
	requireValidation(t, err, "month", "invalid")
}

func TestAnomalyUsesTheStoredActualWhenNoneIsGiven(t *testing.T) {
	r := newRig("ok")
	ts := now.Truncate(time.Hour).Add(-2 * time.Hour)
	res, err := r.svc.Anomaly(context.Background(), sc, analyzer, ts, nil)
	require.NoError(t, err)
	require.True(t, res.IsAnomaly)
	require.True(t, res.Actual.Equal(r.analytics.hourly[ts]))
	req := r.ml.anomaly[0]
	require.InDelta(t, r.analytics.hourly[ts].InexactFloat64(), req.Actual, 1e-9)
	require.Equal(t, ts, req.Ts)
	require.True(t, req.History[len(req.History)-1].Ts.Before(ts), "history ends before the checked hour")

	given := decimal.NewFromInt(999)
	res, err = r.svc.Anomaly(context.Background(), sc, analyzer, ts, &given)
	require.NoError(t, err)
	require.True(t, res.Actual.Equal(given))

	_, err = r.svc.Anomaly(context.Background(), sc, analyzer, now.Add(-5*time.Hour).Truncate(time.Hour), nil)
	requireValidation(t, err, "actual", "required")
	_, err = r.svc.Anomaly(context.Background(), sc, analyzer, now.Add(3*time.Hour), &given)
	requireValidation(t, err, "ts", "future")
}
