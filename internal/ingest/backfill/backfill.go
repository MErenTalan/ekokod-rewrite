// Package backfill implements F2 Task 15 (06 §9 Backfill): an explicit,
// operator-specified historical range for one credential, split into
// consecutive per-window integration.fetch_readings tasks and enqueued
// through the same job.NewFetchReadingsTask (Task 1) the cursor-driven
// pipeline (Task 10) uses for a single explicit-window fetch.
//
// Backfill itself never calls Task 10's Service.FetchReadings — it only
// plans and enqueues. Resumability comes entirely from
// job.NewFetchReadingsTask's deterministic TaskID for a windowed payload
// (integFetchReadingsTaskID: a pure function of analyzer, kind and window
// boundaries): re-running the same backfill request produces the exact same
// task IDs, so asynq.ErrTaskIDConflict on the ones already queued or
// retained is the resumability mechanism, not anything this package tracks
// itself.
package backfill

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/ingest"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/normalize"
	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/secret"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// maxBackfillSpan bounds an explicit backfill range (spec silent — ruling:
// 5 years bounds a mistaken request, e.g. a typo'd year in From, rather
// than any real provider or storage limit).
const maxBackfillSpan = 5 * 365 * 24 * time.Hour

// aggregateHorizon is how far back the consumption aggregates reach without
// a consumption.refresh run: 04-data-model.md's `start_offset => interval
// '30 days'` (R17, the F1 start_offset fact).
const aggregateHorizon = 30 * 24 * time.Hour

// Windows splits [from, to) into consecutive half-open windows of at most
// max, aligned to Istanbul-local midnight so a window never splits a local
// day. It is a thin wrapper over normalize.Chunk (S7 — the ONE range-chunking
// implementation in F2, shared with Task 10's pipeline and Task 12's EPİAŞ
// pagination): Windows exists at all only because callers here want
// job.Window, not normalize.Window, and the two are structurally identical
// but distinct types (normalize has zero F2-task dependencies and must stay
// that way, so it cannot import internal/job to return job.Window directly).
func Windows(from, to time.Time, max time.Duration) []job.Window {
	chunks := normalize.Chunk(from, to, max)
	windows := make([]job.Window, len(chunks))
	for i, c := range chunks {
		windows[i] = job.Window{From: c.From, To: c.To}
	}
	return windows
}

// Deps is every dependency Backfiller needs. All fields are required; New
// does no validation of its own (Backfiller is small enough, and every
// missing dep would panic on first use exactly where it is used, which is
// diagnosable — unlike ingest.Service's much larger surface, which is why
// that package's New validates explicitly).
type Deps struct {
	Analyzers   store.AnalyzerRepository
	Ops         store.OpsRepository
	Credentials ingest.CredentialOpener
	Sources     ingest.SourceResolver
	Enqueuer    ingest.Enqueuer
	Clock       clock.Clock
	MaxRetry    int
}

// Backfiller implements job.Backfiller.
type Backfiller struct{ deps Deps }

// New builds a Backfiller.
func New(d Deps) *Backfiller { return &Backfiller{deps: d} }

var _ job.Backfiller = (*Backfiller)(nil)

// Backfill implements job.Backfiller.Backfill (06 §9 Backfill).
//
// Order of work: validate the range (no job run — a malformed request never
// attempted anything); StartRun; open credentials and resolve the adapter
// (either failing finishes the run 'failed' and returns the error, mirroring
// ingest.Service.FetchReadings' pre-loop failure handling); resolve
// analyzers and kinds; enqueue one deterministic-ID fetch_readings task per
// analyzer x kind x window, counting processed/skipped/failed; append the
// R17 aggregate-horizon info message if From reaches before it; finish the
// run. Per-item failures (an invisible analyzer id, a task-ID conflict, an
// enqueue error) are counted, never returned — like SyncAnalyzers, the
// overall call returns nil once the run itself started, so one bad item
// never aborts the rest of the plan.
func (b *Backfiller) Backfill(ctx context.Context, p job.BackfillPayload) error {
	now := b.deps.Clock.Now()
	if err := validateBackfillRange(p.From, p.To, now); err != nil {
		return err
	}

	sc := store.SystemScope(p.CompanyID)
	companyID := p.CompanyID

	run, err := b.deps.Ops.StartRun(ctx, sc, model.JobRun{
		ID: uuid.New(), CompanyID: &companyID, JobType: job.TypeIntegrationBackfill,
		Scope: newBackfillRunScope(p), StartedAt: now,
	})
	if err != nil {
		return err
	}

	creds, err := b.deps.Credentials.Open(ctx, sc, p.CredentialID)
	if err != nil {
		errText := err.Error()
		b.finishRun(ctx, sc, run.ID, "failed", 0, 0, 1, &errText, nil, now)
		return err
	}

	// X-M3, final review B: see internal/ingest/fetch.go's identical gate —
	// an operator-deactivated credential must stop the backfill here,
	// before any window is even planned, non-retryably (ErrConfig ->
	// job.ClassifyForRetry -> asynq.SkipRetry, applied by the job handler
	// around this call) and with a clear operational message.
	if !creds.IsActive {
		cerr := &integration.Error{Kind: integration.ErrConfig, Provider: creds.Provider, Op: "backfill.credential_inactive"}
		errText := redacted(creds, cerr)
		b.finishRun(ctx, sc, run.ID, "failed", 0, 0, 1, &errText, nil, now)
		b.appendMessage(ctx, sc, p.CompanyID, "job", "backfill", "error", errText, nil)
		return cerr
	}

	src, err := b.deps.Sources.Source(creds.Provider)
	if err != nil {
		errText := redacted(creds, err)
		b.finishRun(ctx, sc, run.ID, "failed", 0, 0, 1, &errText, nil, now)
		return err
	}

	analyzers, missing, err := b.resolveAnalyzers(ctx, sc, creds, p.AnalyzerIDs)
	if err != nil {
		errText := redacted(creds, err)
		b.finishRun(ctx, sc, run.ID, "failed", 0, 0, 1, &errText, nil, now)
		return err
	}

	kinds := resolveKinds(p.Kinds, src.Kinds(creds))

	// R53: p.Force re-mints every window's task id with THIS run's own id
	// (run.ID, already generated above by StartRun), so a re-run after
	// fixing whatever failed earlier re-enqueues every window instead of
	// every one colliding with an earlier run's still-retained id. nil
	// (the default) keeps the plain, resumable-across-retries id shape.
	var forceRunID *uuid.UUID
	if p.Force {
		forceRunID = &run.ID
	}

	var processed, skipped, failed, alreadyEnqueued int32
	var failures []backfillFailure

	for _, id := range missing {
		mid := id
		failed++
		failures = append(failures, backfillFailure{AnalyzerID: &mid, Reason: "analyzer not visible"})
	}

	for _, a := range analyzers {
		for _, kind := range kinds {
			for _, w := range Windows(p.From, p.To, src.MaxWindow(kind)) {
				win := w
				aid := a.ID
				task, terr := job.NewFetchReadingsTask(
					job.FetchReadingsPayload{
						CompanyID: p.CompanyID, CredentialID: p.CredentialID,
						AnalyzerID: aid, Kind: kind, Window: &win, ForceRunID: forceRunID,
					},
					job.TaskOptions{MaxRetry: b.deps.MaxRetry},
				)
				if terr != nil {
					failed++
					failures = append(failures, backfillFailure{AnalyzerID: &aid, Kind: kind, Reason: redacted(creds, terr)})
					continue
				}
				if _, eerr := b.deps.Enqueuer.Enqueue(ctx, task); eerr != nil {
					if errors.Is(eerr, asynq.ErrTaskIDConflict) {
						// R53: without Force, a window already queued or
						// still retained (job.NewFetchReadingsTask's 30-day
						// Retention) from an earlier run of this same
						// backfill collides on its deterministic TaskID.
						// This is expected, not a failure — but it is also
						// NOT a silent success: it is counted separately as
						// already_enqueued (surfaced in job_runs.detail
						// below) and forces the run's status to at most
						// "partial" (see countsStatus), never "success",
						// so a caller can tell "nothing new happened" apart
						// from "everything really ran".
						skipped++
						alreadyEnqueued++
						continue
					}
					failed++
					failures = append(failures, backfillFailure{AnalyzerID: &aid, Kind: kind, Reason: redacted(creds, eerr)})
					continue
				}
				processed++
			}
		}
	}

	if p.From.Before(now.Add(-aggregateHorizon)) {
		horizon := now.Add(-aggregateHorizon)
		b.appendMessage(ctx, sc, p.CompanyID, "job", "backfill", "info",
			fmt.Sprintf("Backfilled range before %s is not in the consumption aggregates until consumption.refresh runs for it (F3)",
				horizon.Format(time.RFC3339)),
			nil)
	}

	status := countsStatus(processed, skipped, failed)
	b.finishRun(ctx, sc, run.ID, status, processed, skipped, failed, nil,
		mustJSON(backfillRunDetail{Failures: failures, AlreadyEnqueued: alreadyEnqueued}), now)
	return nil
}

// validateBackfillRange enforces the brief's three range rules. A malformed
// range never starts a job run: nothing was attempted, so there is nothing
// for a job_runs row to record.
func validateBackfillRange(from, to, now time.Time) error {
	switch {
	case !from.Before(to):
		return fmt.Errorf("backfill: from (%s) must be before to (%s): %w",
			from.Format(time.RFC3339), to.Format(time.RFC3339), integration.ErrMalformedPayload)
	case to.After(now):
		return fmt.Errorf("backfill: to (%s) must not be after now (%s): %w",
			to.Format(time.RFC3339), now.Format(time.RFC3339), integration.ErrMalformedPayload)
	case to.Sub(from) > maxBackfillSpan:
		return fmt.Errorf("backfill: range %s..%s exceeds the %s maximum span: %w",
			from.Format(time.RFC3339), to.Format(time.RFC3339), maxBackfillSpan, integration.ErrMalformedPayload)
	default:
		return nil
	}
}

// resolveAnalyzers implements the brief's analyzer-resolution rule: a
// non-empty p.AnalyzerIDs is resolved by id, with any id not visible to sc
// reported back in missing (never silently dropped); an empty one lists the
// credential's own ACTIVE analyzers explicitly (Providers + IsActive,
// post-filtered to creds.Subtype) — never AnalyzerFilter{IDs: nil}, which
// the store treats as "no id narrowing" (every analyzer in scope, active or
// not, of any provider), not "the credential's active analyzers".
func (b *Backfiller) resolveAnalyzers(ctx context.Context, sc store.Scope, creds integration.Credentials, ids []uuid.UUID) (analyzers []model.Analyzer, missing []uuid.UUID, err error) {
	if len(ids) > 0 {
		found, lerr := b.deps.Analyzers.List(ctx, sc, store.AnalyzerFilter{IDs: ids})
		if lerr != nil {
			return nil, nil, lerr
		}
		foundSet := make(map[uuid.UUID]bool, len(found))
		for _, a := range found {
			foundSet[a.ID] = true
		}
		for _, id := range ids {
			if !foundSet[id] {
				missing = append(missing, id)
			}
		}
		return found, missing, nil
	}

	modelProvider, ok := creds.Provider.ModelProvider()
	if !ok {
		return nil, nil, &integration.Error{Kind: integration.ErrMalformedPayload, Provider: creds.Provider, Op: "backfill.provider"}
	}
	all, lerr := b.deps.Analyzers.List(ctx, sc, store.AnalyzerFilter{
		Providers: []model.IntegrationProvider{modelProvider}, IsActive: ptrBool(true),
	})
	if lerr != nil {
		return nil, nil, lerr
	}
	for _, a := range all {
		if a.ProviderSubtype == creds.Subtype {
			analyzers = append(analyzers, a)
		}
	}
	return analyzers, nil, nil
}

// resolveKinds implements "p.Kinds ∩ src.Kinds(creds), else all of
// src.Kinds(creds)".
func resolveKinds(requested, supported []model.ReadingKind) []model.ReadingKind {
	if len(requested) == 0 {
		return supported
	}
	supportedSet := make(map[model.ReadingKind]bool, len(supported))
	for _, k := range supported {
		supportedSet[k] = true
	}
	var out []model.ReadingKind
	for _, k := range requested {
		if supportedSet[k] {
			out = append(out, k)
		}
	}
	return out
}

// backfillRunScope is job_runs.scope for integration.backfill.
type backfillRunScope struct {
	CredentialID uuid.UUID           `json:"credential_id"`
	AnalyzerIDs  []uuid.UUID         `json:"analyzer_ids,omitempty"`
	Kinds        []model.ReadingKind `json:"kinds,omitempty"`
	From         time.Time           `json:"from"`
	To           time.Time           `json:"to"`
}

func newBackfillRunScope(p job.BackfillPayload) json.RawMessage {
	return mustJSON(backfillRunScope{
		CredentialID: p.CredentialID, AnalyzerIDs: p.AnalyzerIDs, Kinds: p.Kinds,
		From: p.From, To: p.To,
	})
}

// backfillFailure is one entry of job_runs.detail's failures: an
// invisible/missing analyzer id, or one analyzer x kind whose task could not
// be built or enqueued (already redacted where the failure could have come
// from the enqueuer).
type backfillFailure struct {
	AnalyzerID *uuid.UUID        `json:"analyzer_id,omitempty"`
	Kind       model.ReadingKind `json:"kind,omitempty"`
	Reason     string            `json:"reason"`
}

type backfillRunDetail struct {
	Failures []backfillFailure `json:"failures,omitempty"`
	// AlreadyEnqueued is R53's count of windows skipped because their
	// deterministic task id already existed (queued or still retained from
	// an earlier run) — distinct from Failures, which are real errors.
	AlreadyEnqueued int32 `json:"already_enqueued,omitempty"`
}

// redacted is the same discipline ingest.Service.report.go's redacted
// helper follows: every text that reaches job_runs.error or an operational
// message passes through secret.Redact, never err.Error() directly.
func redacted(creds integration.Credentials, err error) string {
	if err == nil {
		return ""
	}
	return secret.Redact(err.Error(), creds.Fragments())
}

// countsStatus mirrors ingest.Service's own countsStatus, with one R53
// addition: success requires nothing failed AND nothing was skipped
// (already_enqueued) — a run where every window collided with an earlier
// run's still-retained id did not itself fetch anything new, so it must
// never report "success" and read as "this backfill request did nothing,
// and that's fine" (final-review-A I4). failed is reserved for a run where
// EVERYTHING failed and NOTHING even reached already-enqueued; every other
// mix (some skipped, some failed, or a mix of processed/skipped/failed) is
// partial.
func countsStatus(processed, skipped, failed int32) string {
	switch {
	case failed == 0 && skipped == 0:
		return "success"
	case failed > 0 && processed == 0 && skipped == 0:
		return "failed"
	default:
		return "partial"
	}
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		// Every value passed here is a small struct of this package's own
		// types (uuid.UUID, time.Time, string, small slices of them) — none
		// of which json.Marshal can fail on. A failure here would be a bug
		// in this file, not a runtime condition a caller could act on.
		return json.RawMessage("{}")
	}
	return b
}

func ptrBool(v bool) *bool { return &v }

// finishRun and appendMessage are best-effort, like ingest.Service's own:
// Deps carries no logger (this package's whole surface is one method), so a
// bookkeeping failure here is swallowed rather than overriding the
// substantive outcome Backfill is already returning.
func (b *Backfiller) finishRun(ctx context.Context, sc store.Scope, runID uuid.UUID, status string, processed, skipped, failed int32, errText *string, detail json.RawMessage, at time.Time) {
	_, _ = b.deps.Ops.FinishRun(ctx, sc, runID, status, processed, skipped, failed, errText, detail, at)
}

func (b *Backfiller) appendMessage(ctx context.Context, sc store.Scope, companyID uuid.UUID, kind, category, status, message string, metadata json.RawMessage) {
	cid := companyID
	_, _ = b.deps.Ops.AppendMessage(ctx, sc, model.OperationalMessage{
		CompanyID: &cid, Kind: kind, Category: category, Status: status, Message: message, Metadata: metadata,
	})
}
