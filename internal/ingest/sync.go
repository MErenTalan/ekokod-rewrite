package ingest

import (
	"context"
	"errors"
	"time"

	"github.com/hibiken/asynq"
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/google/uuid"
)

// SyncAnalyzers implements job.Ingestion.SyncAnalyzers (06 §9): verify the
// credential, discover the provider's metering points, reconcile them
// against stored analyzers, then fan out one integration.fetch_readings task
// per (active analyzer of this credential's provider/subtype) x (reading
// kind the adapter wants for these credentials).
func (s *Service) SyncAnalyzers(ctx context.Context, p job.SyncAnalyzersPayload) error {
	now := s.deps.Clock.Now()
	sc := store.SystemScope(p.CompanyID)
	companyID := p.CompanyID

	run, err := s.deps.Ops.StartRun(ctx, sc, model.JobRun{
		ID: uuid.New(), CompanyID: &companyID, JobType: job.TypeIntegrationSyncAnalyzers,
		Scope: newSyncRunScope(p.CredentialID), StartedAt: now,
	})
	if err != nil {
		return err
	}

	creds, err := s.deps.Credentials.Open(ctx, sc, p.CredentialID)
	if err != nil {
		errText := err.Error()
		s.finishRun(ctx, sc, run.ID, "failed", 0, 0, 0, &errText, nil, now)
		return err
	}

	src, err := s.deps.Sources.Source(creds.Provider)
	if err != nil {
		errText := redacted(creds, err)
		s.finishRun(ctx, sc, run.ID, "failed", 0, 0, 0, &errText, nil, now)
		return wrapRedacted(errText, err)
	}

	if verr := src.Verify(ctx, creds); verr != nil {
		errText := redacted(creds, verr)
		s.finishRun(ctx, sc, run.ID, "failed", 0, 0, 0, &errText, nil, now)
		s.appendMessage(ctx, sc, p.CompanyID, "job", "analyzer-refresh", "error", errText, nil)
		return wrapRedacted(errText, verr)
	}

	modelProvider, ok := creds.Provider.ModelProvider()
	if !ok {
		err := &integration.Error{Kind: integration.ErrMalformedPayload, Provider: creds.Provider, Op: "sync_analyzers.provider"}
		errText := redacted(creds, err)
		s.finishRun(ctx, sc, run.ID, "failed", 0, 0, 0, &errText, nil, now)
		return wrapRedacted(errText, err)
	}

	points, derr := src.DiscoverMeteringPoints(ctx, creds)
	if derr != nil {
		errText := redacted(creds, derr)
		s.finishRun(ctx, sc, run.ID, "failed", 0, 0, 0, &errText, nil, now)
		s.appendMessage(ctx, sc, p.CompanyID, "job", "analyzer-refresh", "error", errText, nil)
		return wrapRedacted(errText, derr)
	}

	var failed int32
	var failedPoints []syncFailedPoint
	for _, pt := range points {
		if _, uerr := s.upsertMeteringPoint(ctx, sc, modelProvider, creds.Subtype, pt, now); uerr != nil {
			failed++
			failedPoints = append(failedPoints, syncFailedPoint{
				InstallationNumber: pt.InstallationNumber,
				Reason:             redacted(creds, uerr),
			})
		}
	}

	analyzers, lerr := s.deps.Analyzers.List(ctx, sc, store.AnalyzerFilter{
		Providers: []model.IntegrationProvider{modelProvider}, IsActive: ptrBool(true),
	})
	if lerr != nil {
		errText := lerr.Error()
		s.finishRun(ctx, sc, run.ID, "failed", 0, 0, failed, &errText, mustJSON(syncRunDetail{FailedPoints: failedPoints}), now)
		return lerr
	}

	kinds := src.Kinds(creds)
	var processed, skipped int32
	for _, a := range analyzers {
		if a.ProviderSubtype != creds.Subtype {
			continue
		}
		for _, kind := range kinds {
			task, terr := job.NewFetchReadingsTask(
				job.FetchReadingsPayload{CompanyID: p.CompanyID, CredentialID: p.CredentialID, AnalyzerID: a.ID, Kind: kind},
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
	}

	status := countsStatus(processed, skipped, failed)
	s.finishRun(ctx, sc, run.ID, status, processed, skipped, failed, nil, mustJSON(syncRunDetail{FailedPoints: failedPoints}), now)
	return nil
}

// upsertMeteringPoint resolves one discovered MeteringPoint against the
// stored analyzer with the same natural key (provider, subtype,
// installation_number): GetByInstallation -> Update on a hit, Create (R27:
// created inactive) on ErrNotFound.
func (s *Service) upsertMeteringPoint(ctx context.Context, sc store.Scope, provider model.IntegrationProvider, subtype string, pt integration.MeteringPoint, now time.Time) (model.Analyzer, error) {
	existing, err := s.deps.Analyzers.GetByInstallation(ctx, sc, provider, subtype, pt.InstallationNumber)
	switch {
	case err == nil:
		updated := applyMeteringPointFields(existing, pt)
		updated.UpdatedAt = now
		result, uerr := s.deps.Analyzers.Update(ctx, sc, updated)
		if uerr != nil {
			return model.Analyzer{}, uerr
		}
		// I5/R27: a multiplier change found by sync is applied AND reported
		// — it changes bills — the same shape the fetch path (fetch.go)
		// uses for a multiplier resolved mid-fetch.
		if pt.MeterMultiplier != nil && !existing.MeterMultiplier.Equal(*pt.MeterMultiplier) {
			s.appendMessage(ctx, sc, sc.CompanyID, "job", "analyzer-refresh", "warning", "meter multiplier changed",
				mustJSON(map[string]any{"analyzer_id": result.ID, "multiplier": result.MeterMultiplier.String()}))
		}
		return result, nil
	case errors.Is(err, store.ErrNotFound):
		a := newAnalyzerFromMeteringPoint(sc.CompanyID, provider, subtype, pt, now)
		return s.deps.Analyzers.Create(ctx, sc, a)
	default:
		return model.Analyzer{}, err
	}
}

// newAnalyzerFromMeteringPoint builds a brand-new analyzer from a discovered
// point. R27: newly discovered analyzers are created INACTIVE — an operator
// reviews and activates them, rather than the pipeline starting to fetch
// readings for a metering point nobody has confirmed belongs to this
// company yet.
func newAnalyzerFromMeteringPoint(companyID uuid.UUID, provider model.IntegrationProvider, subtype string, pt integration.MeteringPoint, now time.Time) model.Analyzer {
	multiplier := decimal.NewFromInt(1)
	if pt.MeterMultiplier != nil {
		multiplier = *pt.MeterMultiplier
	}
	return model.Analyzer{
		CompanyID:          companyID,
		Provider:           provider,
		ProviderSubtype:    subtype,
		InstallationNumber: pt.InstallationNumber,
		CustomerName:       pt.CustomerName,
		Address:            pt.Address,
		Province:           pt.Province,
		District:           pt.District,
		Neighbourhood:      pt.Neighbourhood,
		Street:             pt.Street,
		TariffType:         pt.TariffType,
		TariffKind:         pt.TariffKind,
		InstallationKind:   pt.InstallationKind,
		InstalledPowerKw:   pt.InstalledPowerKw,
		MeterNumber:        pt.MeterNumber,
		MeterModel:         pt.MeterModel,
		MeterMultiplier:    multiplier,
		CounterpartyNo:     pt.CounterpartyNo,
		MeteringPointName:  pt.MeteringPointName,
		Latitude:           pt.Latitude,
		Longitude:          pt.Longitude,
		EtsoCode:           pt.EtsoCode,
		DefinitionType:     pt.DefinitionType,
		IsActive:           false,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
}

// applyMeteringPointFields updates every descriptive field of existing from
// pt EXCEPT ContractedPowerKw — model.Analyzer has no matching column (R30):
// the value stays visible only in the adapter's own Raw/fixture data, never
// silently dropped from a field this struct actually has. IsActive is left
// untouched: R27 sets it only at creation, and a sync must never silently
// flip an operator's own activation decision. MeterMultiplier is updated
// only when the point reports one; a nil MeterMultiplier means "the
// provider did not report one this time", not "reset to 1".
func applyMeteringPointFields(existing model.Analyzer, pt integration.MeteringPoint) model.Analyzer {
	existing.CustomerName = pt.CustomerName
	existing.Address = pt.Address
	existing.Province = pt.Province
	existing.District = pt.District
	existing.Neighbourhood = pt.Neighbourhood
	existing.Street = pt.Street
	existing.TariffType = pt.TariffType
	existing.TariffKind = pt.TariffKind
	existing.InstallationKind = pt.InstallationKind
	existing.InstalledPowerKw = pt.InstalledPowerKw
	existing.MeterNumber = pt.MeterNumber
	existing.MeterModel = pt.MeterModel
	if pt.MeterMultiplier != nil {
		existing.MeterMultiplier = *pt.MeterMultiplier
	}
	existing.CounterpartyNo = pt.CounterpartyNo
	existing.MeteringPointName = pt.MeteringPointName
	existing.Latitude = pt.Latitude
	existing.Longitude = pt.Longitude
	existing.EtsoCode = pt.EtsoCode
	existing.DefinitionType = pt.DefinitionType
	return existing
}
