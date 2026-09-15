package ingest

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/job"
)

// Dispatch implements job.Ingestion.Dispatch: the scheduled tick (06 §9)
// that fans one integration.sync_analyzers task out per active meter
// credential. It is platform-wide work — it acts for no single tenant — so
// it goes through AdminIngestionRepository and AdminJournalRepository, never
// a borrowed Scope (Global Constraints: "platform-wide work goes only
// through the Admin* interfaces").
func (s *Service) Dispatch(ctx context.Context) error {
	now := s.deps.Clock.Now()

	run, err := s.deps.AdminJournal.StartPlatformRun(ctx, model.JobRun{
		ID: uuid.New(), JobType: job.TypeIntegrationSyncDispatch, StartedAt: now,
	})
	if err != nil {
		return err
	}

	refs, err := s.deps.AdminIngestion.ActiveCredentials(ctx)
	if err != nil {
		errText := err.Error()
		s.finishPlatformRun(ctx, run.ID, "failed", 0, 0, 0, &errText, nil, now)
		return err
	}

	var processed, skipped, failed int32
	for _, ref := range refs {
		// iSolar is not a meter provider — its plants are synced through
		// F2 Task 13's own path, never integration.sync_analyzers.
		if ref.Provider == model.IntegrationProviderISolar {
			continue
		}

		task, terr := job.NewSyncAnalyzersTask(
			job.SyncAnalyzersPayload{CompanyID: ref.CompanyID, CredentialID: ref.CredentialID},
			job.TaskOptions{MaxRetry: s.opts.MaxRetry},
		)
		if terr != nil {
			failed++
			continue
		}
		if _, eerr := s.deps.Enqueuer.Enqueue(ctx, task); eerr != nil {
			if errors.Is(eerr, asynq.ErrDuplicateTask) {
				skipped++
			} else {
				failed++
			}
			continue
		}
		processed++
	}

	status := countsStatus(processed, skipped, failed)
	s.finishPlatformRun(ctx, run.ID, status, processed, skipped, failed, nil, nil, now)
	return nil
}
