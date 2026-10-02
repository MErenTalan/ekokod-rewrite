package ingest

import (
	"context"
	"errors"
	"fmt"

	"github.com/hibiken/asynq"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// RefreshAnalyzer enqueues cursor-driven fetches for one analyzer: the
// load profile for "hourly", every other kind the adapter serves for
// "energy" (R164).
func (s *Service) RefreshAnalyzer(ctx context.Context, p job.RefreshAnalyzerPayload) error {
	sc := store.SystemScope(p.CompanyID)
	analyzer, err := s.deps.Analyzers.Get(ctx, sc, p.AnalyzerID)
	if err != nil {
		return classifyStoreErr(err)
	}
	creds, err := s.deps.Credentials.Open(ctx, sc, p.CredentialID)
	if err != nil {
		return classifyStoreErr(err)
	}
	if modelProvider, ok := creds.Provider.ModelProvider(); !ok || modelProvider != analyzer.Provider || creds.Subtype != analyzer.ProviderSubtype {
		s.appendMessage(ctx, sc, p.CompanyID, "job", "analyzer-refresh", "error",
			fmt.Sprintf("Analizör %s için entegrasyon eşleşmedi.", analyzer.InstallationNumber), nil)
		return fmt.Errorf("%w: credential does not serve analyzer", asynq.SkipRetry)
	}
	src, err := s.deps.Sources.Source(creds.Provider)
	if err != nil {
		return wrapRedacted(redacted(creds, err), err)
	}
	var enqueued int
	for _, kind := range src.Kinds(creds) {
		hourly := kind == model.ReadingKindLoadProfile
		if hourly != (p.Mode == job.RefreshModeHourly) {
			continue
		}
		payload := job.FetchReadingsPayload{
			CompanyID: p.CompanyID, CredentialID: p.CredentialID, AnalyzerID: p.AnalyzerID, Kind: kind,
		}
		if parent := ownTaskID(ctx); parent != nil {
			payload.ParentTaskID = *parent
		}
		task, err := job.NewFetchReadingsTask(payload, job.TaskOptions{MaxRetry: s.opts.MaxRetry})
		if err != nil {
			return err
		}
		if _, err := s.deps.Enqueuer.Enqueue(ctx, task); err != nil && !errors.Is(err, asynq.ErrDuplicateTask) {
			return err
		}
		enqueued++
	}
	if enqueued == 0 {
		s.appendMessage(ctx, sc, p.CompanyID, "job", "analyzer-refresh", "warning",
			fmt.Sprintf("Analizör %s: sağlayıcısı %s sunmuyor, yenilenecek veri yok.", analyzer.InstallationNumber, refreshModeText(p.Mode)), nil)
	}
	return nil
}

// refreshModeText names a refresh mode for an operator.
func refreshModeText(mode string) string {
	if mode == job.RefreshModeHourly {
		return "saatlik değer"
	}
	return "enerji (endeks) değeri"
}
