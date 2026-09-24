package forecast_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/ml"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/forecast"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

type fakeTenants struct {
	store.AdminTenantRepository
	companies []model.Company
}

func (f *fakeTenants) ListCompanies(_ context.Context, fl store.CompanyFilter) ([]model.Company, error) {
	if fl.Page.Offset > 0 {
		return nil, nil
	}
	return f.companies, nil
}

type finished struct {
	company                    uuid.UUID
	status                     string
	processed, skipped, failed int32
	errText                    *string
}

type fakeOps struct {
	store.OpsRepository
	started  []model.JobRun
	finished []finished
}

func (f *fakeOps) StartRun(_ context.Context, sc store.Scope, run model.JobRun) (model.JobRun, error) {
	run.ID = uuid.New()
	f.started = append(f.started, run)
	return run, nil
}

func (f *fakeOps) FinishRun(_ context.Context, sc store.Scope, _ uuid.UUID, status string, p, s, fl int32, errText *string, _ []byte, _ time.Time) (model.JobRun, error) {
	f.finished = append(f.finished, finished{sc.CompanyID, status, p, s, fl, errText})
	return model.JobRun{}, nil
}

func jobRig(status string) (*forecast.Service, *rig, *fakeOps) {
	r := newRig(status)
	ops := &fakeOps{}
	svc := forecast.New(forecast.Deps{
		Analyzers: &fakeAnalyzers{list: []model.Analyzer{{ID: analyzer, CompanyID: company, IsActive: true}, {ID: foreign, CompanyID: company, IsActive: true}}},
		Analytics: r.analytics, Calendar: r.calendar, Forecasts: r.forecasts, ML: r.ml, Clock: clock.NewFake(now),
		Ops: ops, Tenants: &fakeTenants{companies: []model.Company{{ID: company}}},
	})
	return svc, r, ops
}

func TestJobForecastsEveryActiveAnalyzerAndIsolatesFailures(t *testing.T) {
	svc, r, ops := jobRig("ok")
	require.NoError(t, svc.RunAll(context.Background()))
	require.Len(t, ops.started, 1)
	require.Equal(t, "forecast.run", ops.started[0].JobType)
	require.Equal(t, []finished{{company, "partial", 1, 0, 1, nil}}, ops.finished, "the foreign analyzer fails alone")
	require.Len(t, r.forecasts.rows, 168, "R376/Q-I14: one week ahead")
}

func TestJobCountsInsufficientDataAsSkipped(t *testing.T) {
	svc, _, ops := jobRig("insufficient_data")
	require.NoError(t, svc.RunAll(context.Background()))
	require.Equal(t, int32(1), ops.finished[0].skipped)
	require.Equal(t, int32(0), ops.finished[0].processed)
}

func TestJobFailsWholeWhenTheServiceIsDown(t *testing.T) {
	svc, r, ops := jobRig("ok")
	r.ml.err = ml.ErrUnavailable
	err := svc.RunAll(context.Background())
	require.ErrorIs(t, err, forecast.ErrUnavailable, "the task retries later")
	require.Equal(t, "failed", ops.finished[0].status)
	require.NotNil(t, ops.finished[0].errText)
}
