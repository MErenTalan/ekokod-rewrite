package ingest

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/secret"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// redacted is the single helper the brief names: every text that reaches
// job_runs.error, ingestion_cursors.last_error or an operational message
// passes through this — never err.Error() directly — so a credential
// fragment embedded in an upstream error (Task 2 already keeps a URL, query
// string or body out of it, but not necessarily a password a provider
// echoed back in a message body) can never reach an operator-facing column.
func redacted(creds integration.Credentials, err error) string {
	if err == nil {
		return ""
	}
	return secret.Redact(err.Error(), creds.Fragments())
}

// redactedCause is the error FetchReadings/SyncAnalyzers return to the job
// layer (I3) once a failure has already gone through redacted() for
// job_runs.error/ingestion_cursors.last_error/an operational message:
// Error() renders that SAME already-redacted text — never cause's own,
// possibly credential-bearing, text — while Unwrap() still reaches cause,
// so job.ClassifyForRetry's errors.Is(cause, integration.ErrRateLimited)
// (etc.) keeps working on the value asynq actually receives and logs.
//
// Modelled on internal/platform/secret's scrubbed type (the same
// "print only the redacted text, but stay traversable via Unwrap" split),
// without secret.Wrap's own "<op>: " prefix: the text here must match
// verbatim what redacted() already computed for this failure's other
// records, not a second, differently-formatted rendering of it.
type redactedCause struct {
	text  string
	cause error
}

func (e *redactedCause) Error() string { return e.text }
func (e *redactedCause) Unwrap() error { return e.cause }

// wrapRedacted returns cause reported to the job layer as text (the value
// already computed via redacted() for this failure's job_runs/cursor/
// message records), while keeping cause reachable via errors.Is/errors.As —
// I3. A nil cause returns nil so callers may use it unconditionally.
func wrapRedacted(text string, cause error) error {
	if cause == nil {
		return nil
	}
	return &redactedCause{text: text, cause: cause}
}

// sortedWarningCodes returns warnings' keys in a fixed, deterministic order
// (M5): warnings is a map, and FetchReadings iterates it to emit one
// operational message per distinct adapter warning code — without a sort,
// two runs with the same warnings would emit their messages in a different,
// unpredictable order every time.
func sortedWarningCodes(warnings map[string]int32) []string {
	codes := make([]string, 0, len(warnings))
	for code := range warnings {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	return codes
}

// countsStatus is job_runs.status for a run whose only failure unit is
// "one item of many failed", used by Dispatch and SyncAnalyzers: success
// when nothing failed, partial when something succeeded (processed or
// skipped) alongside a failure, failed when nothing did.
func countsStatus(processed, skipped, failed int32) string {
	switch {
	case failed == 0:
		return "success"
	case processed > 0 || skipped > 0:
		return "partial"
	default:
		return "failed"
	}
}

// fetchRunStatus is job_runs.status for FetchReadings, whose only possible
// failure is the page loop aborting on an adapter error: failedPages is 0 or
// 1 (the loop returns immediately on the first one), and anyPersisted is
// whether any row was inserted or updated before that happened.
func fetchRunStatus(failedPages int32, anyPersisted bool) string {
	switch {
	case failedPages == 0:
		return "success"
	case anyPersisted:
		return "partial"
	default:
		return "failed"
	}
}

// fetchRunScope is job_runs.scope for integration.fetch_readings, per the
// brief: {"analyzer_id", "kind", "window"}.
type fetchRunScope struct {
	AnalyzerID uuid.UUID         `json:"analyzer_id"`
	Kind       model.ReadingKind `json:"kind"`
	Window     *windowJSON       `json:"window,omitempty"`
}

type windowJSON struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

func newFetchRunScope(analyzerID uuid.UUID, kind model.ReadingKind, w *job.Window) json.RawMessage {
	sc := fetchRunScope{AnalyzerID: analyzerID, Kind: kind}
	if w != nil {
		sc.Window = &windowJSON{From: w.From, To: w.To}
	}
	return mustJSON(sc)
}

// syncRunScope is job_runs.scope for integration.sync_analyzers.
type syncRunScope struct {
	CredentialID uuid.UUID `json:"credential_id"`
}

func newSyncRunScope(credentialID uuid.UUID) json.RawMessage {
	return mustJSON(syncRunScope{CredentialID: credentialID})
}

// fetchRunDetail is job_runs.detail for FetchReadings (the brief's
// "rejections_by_reason, warnings_by_code, affected_from, affected_to,
// anomalies_created").
type fetchRunDetail struct {
	RejectionsByReason map[RejectReason]int32 `json:"rejections_by_reason,omitempty"`
	WarningsByCode     map[string]int32       `json:"warnings_by_code,omitempty"`
	AffectedFrom       *time.Time             `json:"affected_from,omitempty"`
	AffectedTo         *time.Time             `json:"affected_to,omitempty"`
	AnomaliesCreated   int32                  `json:"anomalies_created"`
}

// syncFailedPoint is one entry of syncRunDetail.FailedPoints: which
// metering point SyncAnalyzers could not upsert, and why (already redacted
// where the failure could have come from an adapter).
type syncFailedPoint struct {
	InstallationNumber string `json:"installation_number"`
	Reason             string `json:"reason"`
}

type syncRunDetail struct {
	FailedPoints []syncFailedPoint `json:"failed_points,omitempty"`
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		// Every value passed here is a small struct of this package's own
		// types (uuid.UUID, time.Time, string, small maps/slices of them) —
		// none of which json.Marshal can fail on. A failure here would be a
		// bug in this file, not a runtime condition a caller could act on.
		return json.RawMessage("{}")
	}
	return b
}

// finishRun is OpsRepository.FinishRun with the error/logging boilerplate
// factored out: a failure to finish a run is logged, never panics or
// silently discarded, but also never overrides the error the caller is
// already returning (finishing the bookkeeping row is best-effort once the
// substantive outcome is already decided).
func (s *Service) finishRun(ctx context.Context, sc store.Scope, runID uuid.UUID, status string, processed, skipped, failed int32, errText *string, detail json.RawMessage, at time.Time) {
	if _, err := s.deps.Ops.FinishRun(ctx, sc, runID, status, processed, skipped, failed, errText, detail, at); err != nil {
		s.deps.Log.ErrorContext(ctx, "ingest: finish job run failed", "run_id", runID, "error", err)
	}
}

// finishPlatformRun is finishRun's AdminJournalRepository sibling, for
// Dispatch's platform-wide run.
func (s *Service) finishPlatformRun(ctx context.Context, runID uuid.UUID, status string, processed, skipped, failed int32, errText *string, detail json.RawMessage, at time.Time) {
	if _, err := s.deps.AdminJournal.FinishPlatformRun(ctx, runID, status, processed, skipped, failed, errText, detail, at); err != nil {
		s.deps.Log.ErrorContext(ctx, "ingest: finish platform job run failed", "run_id", runID, "error", err)
	}
}

// appendMessage is OpsRepository.AppendMessage with the same
// best-effort-and-logged failure handling as finishRun.
func (s *Service) appendMessage(ctx context.Context, sc store.Scope, companyID uuid.UUID, kind, category, status, message string, metadata json.RawMessage) {
	cid := companyID
	if _, err := s.deps.Ops.AppendMessage(ctx, sc, model.OperationalMessage{
		CompanyID: &cid, Kind: kind, Category: category, Status: status, Message: message, Metadata: metadata,
	}); err != nil {
		s.deps.Log.ErrorContext(ctx, "ingest: append operational message failed", "error", err)
	}
}

// partitionSanityFlags splits Validate's rejected slice into the readings
// genuinely excluded (real) and the RejectSanityJump/"sustained" entries
// (flags) that mark a reading Validate actually KEPT — see Validate's doc.
// Only real counts toward skipped, rejections_by_reason and the aggregate
// rejection warning; flags become their own `error` messages.
func partitionSanityFlags(rejected []Rejection) (real, flags []Rejection) {
	for _, r := range rejected {
		if r.Reason == RejectSanityJump && r.Register == "sustained" {
			flags = append(flags, r)
			continue
		}
		real = append(real, r)
	}
	return real, flags
}

func ptrTime(t time.Time) *time.Time { return &t }

func ptrBool(b bool) *bool { return &b }
