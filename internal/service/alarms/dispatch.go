package alarms

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// DispatchDeps are what the hourly platform tick needs. It acts for no single
// tenant, so it goes through the Admin* interfaces and never a borrowed Scope.
type DispatchDeps struct {
	Companies store.AdminAlarmRepository
	Journal   store.AdminJournalRepository
	// Enqueue schedules one company's evaluation.
	Enqueue func(ctx context.Context, p job.AlarmEvaluatePayload) error
}

// WithDispatch returns a Service that can run the platform tick.
func (s *Service) WithDispatch(d DispatchDeps) *Service {
	clone := *s
	clone.dd = &d
	return &clone
}

// Dispatch is 01 §8's hourly alarm check: find every tenant with an enabled
// rule and enqueue one evaluation each, all stamped with the same hour so
// R225's TaskID de-duplicates a manual trigger against the tick.
func (s *Service) Dispatch(ctx context.Context) error {
	if s.dd == nil {
		return errors.New("alarms: Dispatch needs DispatchDeps")
	}
	now := s.d.Clock.Now()
	hour := now.UTC().Truncate(time.Hour)

	run, err := s.dd.Journal.StartPlatformRun(ctx, model.JobRun{
		ID: uuid.New(), JobType: job.TypeAlarmDispatch, StartedAt: now,
	})
	if err != nil {
		return err
	}

	companies, err := s.dd.Companies.CompaniesWithEnabledAlarms(ctx)
	if err != nil {
		errText := err.Error()
		s.finishPlatformRun(ctx, run.ID, "failed", 0, 0, 0, &errText, now)
		return err
	}

	var processed, failed int32
	for _, companyID := range companies {
		// One tenant's enqueue failing must not stop the others: 01 §8's
		// isolated-failure rule applies to the fan-out too.
		if err := s.dd.Enqueue(ctx, job.AlarmEvaluatePayload{CompanyID: companyID, Hour: hour}); err != nil {
			failed++
			continue
		}
		processed++
	}

	status := "success"
	switch {
	case failed > 0 && processed == 0:
		status = "failed"
	case failed > 0:
		status = "partial"
	}
	s.finishPlatformRun(ctx, run.ID, status, processed, 0, failed, nil, s.d.Clock.Now())
	return nil
}

func (s *Service) finishPlatformRun(ctx context.Context, id uuid.UUID, status string,
	processed, skipped, failed int32, errText *string, at time.Time,
) {
	detail, err := json.Marshal(map[string]any{"companies": processed})
	if err != nil {
		detail = nil
	}
	_, _ = s.dd.Journal.FinishPlatformRun(ctx, id, status, processed, skipped, failed, errText, detail, at)
}
