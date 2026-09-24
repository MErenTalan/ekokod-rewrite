package forecast

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// JobHorizonHours is Q-I14: the scheduled run forecasts one week ahead.
const JobHorizonHours = 168

const pageSize = 1000

// RunTask is forecast.run's handler entry point.
func (s *Service) RunTask(ctx context.Context) error { return s.RunAll(ctx) }

// RunAll is R376: every company's active analyzers, one job_runs row per company.
// One analyzer's failure never stops another's; an unreachable ML service stops
// the run (every later call would fail too) so the task retries.
func (s *Service) RunAll(ctx context.Context) error {
	for offset := int32(0); ; offset += pageSize {
		companies, err := s.d.Tenants.ListCompanies(ctx, store.CompanyFilter{Page: store.Page{Limit: pageSize, Offset: offset}})
		if err != nil {
			return err
		}
		for _, c := range companies {
			if err := s.runCompany(ctx, c.ID); err != nil {
				return err
			}
		}
		if len(companies) < pageSize {
			return nil
		}
	}
}

func (s *Service) runCompany(ctx context.Context, companyID uuid.UUID) error {
	sc := store.SystemScope(companyID)
	scope, _ := json.Marshal(map[string]any{"horizon_hours": JobHorizonHours})
	run, runErr := s.d.Ops.StartRun(ctx, sc, model.JobRun{CompanyID: &companyID, JobType: job.TypeForecastRun, Scope: scope,
		StartedAt: s.d.Clock.Now(), Status: "running"})
	finish := func(status string, processed, skipped, failed int32, errText *string) {
		if runErr == nil {
			_, _ = s.d.Ops.FinishRun(ctx, sc, run.ID, status, processed, skipped, failed, errText, nil, s.d.Clock.Now())
		}
	}
	var processed, skipped, failed int32
	active := true
	for offset := int32(0); ; offset += pageSize {
		analyzers, err := s.d.Analyzers.List(ctx, sc, store.AnalyzerFilter{IsActive: &active, Page: store.Page{Limit: pageSize, Offset: offset}})
		if err != nil {
			text := err.Error()
			finish("failed", processed, skipped, failed+1, &text)
			return err
		}
		for _, a := range analyzers {
			res, err := s.Run(ctx, sc, a.ID, JobHorizonHours)
			switch {
			case errors.Is(err, ErrUnavailable):
				text := err.Error()
				finish("failed", processed, skipped, failed, &text)
				return err
			case err != nil:
				failed++
			case res.Status == "ok":
				processed++
			default:
				skipped++
			}
		}
		if len(analyzers) < pageSize {
			break
		}
	}
	status := "success"
	if failed > 0 {
		status = "partial"
	}
	finish(status, processed, skipped, failed, nil)
	return nil
}
