package job

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

// Integration task type names. TypeConsumptionRefresh (R17) was declared
// here in F2 so job payloads and the queue names were stable ahead of its
// handler; F3 Task 11a adds that handler (see consumption.go) and
// Register now wires it whenever Handlers.ConsumptionRefresh is non-nil.
const (
	TypeIntegrationSyncDispatch  = "integration.sync_dispatch" // R16, platform
	TypeIntegrationSyncAnalyzers = "integration.sync_analyzers"
	TypeIntegrationFetchReadings = "integration.fetch_readings"
	TypeIntegrationBackfill      = "integration.backfill" // R16
	TypeEPIASSyncPrices          = "epias.sync_prices"
	TypeConsumptionRefresh       = "consumption.refresh" // R17: declared, not handled, in F2
)

// Window is a half-open [From, To) UTC range carried by a job payload.
type Window struct{ From, To time.Time }

// SyncAnalyzersPayload asks the ingestion job to (re)discover the metering
// points of one credential and reconcile them against stored analyzers.
type SyncAnalyzersPayload struct{ CompanyID, CredentialID uuid.UUID }

// FetchReadingsPayload asks the ingestion job to fetch one reading kind for
// one analyzer. A nil Window means "resume from the stored cursor"; a
// non-nil Window targets one explicit range (used for a single backfill
// chunk) and gets a deterministic task ID instead of the cursor-driven
// task's uniqueness window — see NewFetchReadingsTask.
type FetchReadingsPayload struct {
	CompanyID, CredentialID, AnalyzerID uuid.UUID
	Kind                                model.ReadingKind
	Window                              *Window // nil: cursor-driven

	// ForceRunID, when non-nil, is folded into the deterministic windowed
	// task ID (R53): it lets a Force backfill re-mint a fresh, never
	// -before-seen id for a window that would otherwise collide with one
	// already queued or still retained from an earlier run of the same
	// backfill, guaranteeing the window is enqueued again instead of
	// silently skipped as already_enqueued. The zero value (nil) is the
	// default, resumable id shape every non-Force caller keeps using.
	ForceRunID *uuid.UUID
}

// BackfillPayload asks the ingestion job to backfill a range for a
// credential. An empty AnalyzerIDs means every ACTIVE analyzer of the
// credential, resolved at run time by the handler — never "no filter" in a
// query (Global Constraints: "a required positional id list that is empty
// means NO rows"; this list is the one deliberate, documented exception,
// and the handler is what turns "empty" into the real list before any
// query runs).
type BackfillPayload struct {
	CompanyID, CredentialID uuid.UUID
	AnalyzerIDs             []uuid.UUID
	Kinds                   []model.ReadingKind
	From, To                time.Time
	// Force, when true, re-mints every window's fetch_readings task id with
	// THIS backfill run's own id (R53): a re-run after fixing whatever
	// caused an earlier attempt to fail (a bad credential, a config error)
	// re-fetches every window instead of every window silently colliding
	// with the earlier run's still-retained (30 day) task ids and being
	// skipped as already_enqueued. False (the default) keeps the
	// deterministic, resumable id shape — a retry of the SAME request
	// stays cheap and idempotent.
	Force bool
}

// SyncPricesPayload asks for an EPİAŞ price sync. A nil Window means "the
// job's own default window" (typically "today and tomorrow" — decided by
// the handler, not this payload).
type SyncPricesPayload struct{ Window *Window }

// ConsumptionRefreshPayload is R17's payload shape, declared in F2 so the
// wire format was stable ahead of its handler. F3 Task 11a adds its
// constructor (NewConsumptionRefreshTask), decoder (DecodeConsumptionRefresh)
// and handler seam (Refresher) in consumption.go. Nothing enqueues this task
// yet — internal/ingest wiring the enqueue call is Task 11b.
type ConsumptionRefreshPayload struct {
	CompanyID, AnalyzerID uuid.UUID
	From, To              time.Time
}

// TaskOptions carries the one per-enqueue override every integration task
// constructor accepts. Unlike a typical "zero means unset" option struct,
// TaskOptions.MaxRetry is always passed through to asynq explicitly — see
// integMaxRetryOptions. A caller that wants the platform's configured
// default (config.Worker.MaxRetries, itself defaulted to 5 where config is
// loaded — platform/config/load.go) passes that resolved value here; F2
// never lets a zero TaskOptions silently fall through to asynq's own
// built-in default of 25, which would make "0 retries" and "did not think
// about it" indistinguishable.
type TaskOptions struct{ MaxRetry int }

// integMaxRetryOptions always returns an explicit asynq.MaxRetry(o.MaxRetry)
// option, including for o.MaxRetry == 0. Config documents 0 as a legitimate
// "no retries" (platform/config/load.go), so treating a zero TaskOptions as
// "leave retry count unset" would silently substitute asynq's built-in
// default of 25 for an operator's deliberate "do not retry this" — the
// opposite of what they configured. The platform's own default (5) is
// applied once, where config is loaded, never here.
func integMaxRetryOptions(o TaskOptions) []asynq.Option {
	return []asynq.Option{asynq.MaxRetry(o.MaxRetry)}
}

// integEncode marshals a payload for a task of the given type, wrapping any
// (implausible, since every payload field type is JSON-safe) marshal error
// with the task type so it is identifiable without a stack trace.
func integEncode(taskType string, payload any) ([]byte, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode %s payload: %w", taskType, err)
	}
	return data, nil
}

// integDecode is Decode<Name>'s shared body: unknown JSON fields are
// refused, so a payload produced by a newer version of this code (a field
// a running worker does not know about) fails loudly instead of silently
// discarding data the worker never even reads. Trailing bytes after the one
// JSON object are refused too (dec.More reports whether the stream has
// another token left once the object is consumed) — asynq's Task.Payload is
// a byte slice, not a framed single value, so nothing else guarantees a
// stray second value (or trailing garbage) appended after the object gets
// noticed rather than silently ignored.
func integDecode(taskType string, data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("decode %s payload: %w", taskType, err)
	}
	if dec.More() {
		return fmt.Errorf("decode %s payload: trailing data after JSON value", taskType)
	}
	return nil
}

// NewSyncDispatchTask builds the platform-wide integration.sync_dispatch
// task (R16): it has no payload, is unique for an hour so a scheduler tick
// cannot pile up duplicates, and is bounded to 10 minutes.
func NewSyncDispatchTask(o TaskOptions) (*asynq.Task, error) {
	opts := append([]asynq.Option{
		asynq.Unique(time.Hour),
		asynq.Timeout(10 * time.Minute),
		asynq.Queue(QueueDefault),
	}, integMaxRetryOptions(o)...)
	return asynq.NewTask(TypeIntegrationSyncDispatch, []byte("{}"), opts...), nil
}

// NewSyncAnalyzersTask builds an integration.sync_analyzers task.
func NewSyncAnalyzersTask(p SyncAnalyzersPayload, o TaskOptions) (*asynq.Task, error) {
	payload, err := integEncode(TypeIntegrationSyncAnalyzers, p)
	if err != nil {
		return nil, err
	}
	opts := append([]asynq.Option{
		asynq.Unique(time.Hour),
		asynq.Timeout(10 * time.Minute),
	}, integMaxRetryOptions(o)...)
	return asynq.NewTask(TypeIntegrationSyncAnalyzers, payload, opts...), nil
}

// DecodeSyncAnalyzers reads an integration.sync_analyzers payload.
func DecodeSyncAnalyzers(t *asynq.Task) (SyncAnalyzersPayload, error) {
	var p SyncAnalyzersPayload
	if err := integDecode(TypeIntegrationSyncAnalyzers, t.Payload(), &p); err != nil {
		return SyncAnalyzersPayload{}, err
	}
	return p, nil
}

// integFetchReadingsTaskID computes the deterministic task ID a windowed
// (non-cursor) FetchReadingsPayload gets, so that re-enqueuing the same
// range for the same analyzer and kind — the shape a backfill chunk or a
// retried API request takes — collides with the original instead of
// silently duplicating it. p.Window must be non-nil.
//
// R53: when p.ForceRunID is set, it is folded into the id, which changes
// the id on every Force backfill run (a fresh run id every time) while
// still being fully deterministic WITHIN one run — two Force windows
// enqueued twice inside the SAME run (a retry of the backfill task itself,
// not a brand new operator request) still collide with each other, only a
// NEW run (a new ForceRunID) mints fresh ids.
func integFetchReadingsTaskID(p FetchReadingsPayload) string {
	base := fmt.Sprintf("backfill:%s:%s:%d:%d", p.AnalyzerID, p.Kind, p.Window.From.Unix(), p.Window.To.Unix())
	if p.ForceRunID != nil {
		return base + ":force:" + p.ForceRunID.String()
	}
	return base
}

// NewFetchReadingsTask builds an integration.fetch_readings task. A nil
// Window is the cursor-driven shape and is deduplicated with a one-hour
// uniqueness window, like the other dispatch-triggered tasks; a non-nil
// Window (a single explicit range) instead gets the deterministic task ID
// from integFetchReadingsTaskID and a 30-day retention so its completion
// record — and the dedup it provides — survives long enough to cover a
// slow backfill.
func NewFetchReadingsTask(p FetchReadingsPayload, o TaskOptions) (*asynq.Task, error) {
	payload, err := integEncode(TypeIntegrationFetchReadings, p)
	if err != nil {
		return nil, err
	}
	opts := append([]asynq.Option{asynq.Timeout(10 * time.Minute)}, integMaxRetryOptions(o)...)
	if p.Window == nil {
		opts = append(opts, asynq.Unique(time.Hour))
	} else {
		opts = append(opts,
			asynq.TaskID(integFetchReadingsTaskID(p)),
			asynq.Retention(30*24*time.Hour),
		)
	}
	return asynq.NewTask(TypeIntegrationFetchReadings, payload, opts...), nil
}

// DecodeFetchReadings reads an integration.fetch_readings payload.
func DecodeFetchReadings(t *asynq.Task) (FetchReadingsPayload, error) {
	var p FetchReadingsPayload
	if err := integDecode(TypeIntegrationFetchReadings, t.Payload(), &p); err != nil {
		return FetchReadingsPayload{}, err
	}
	return p, nil
}

// NewBackfillTask builds an integration.backfill task (R16): bounded to 30
// minutes and run on the low-priority queue, since a backfill is bulk,
// deferrable work that must never starve the near-real-time sync tasks.
func NewBackfillTask(p BackfillPayload, o TaskOptions) (*asynq.Task, error) {
	payload, err := integEncode(TypeIntegrationBackfill, p)
	if err != nil {
		return nil, err
	}
	opts := append([]asynq.Option{
		asynq.Timeout(30 * time.Minute),
		asynq.Queue(QueueLow),
	}, integMaxRetryOptions(o)...)
	return asynq.NewTask(TypeIntegrationBackfill, payload, opts...), nil
}

// DecodeBackfill reads an integration.backfill payload.
func DecodeBackfill(t *asynq.Task) (BackfillPayload, error) {
	var p BackfillPayload
	if err := integDecode(TypeIntegrationBackfill, t.Payload(), &p); err != nil {
		return BackfillPayload{}, err
	}
	return p, nil
}

// NewSyncPricesTask builds an epias.sync_prices task.
func NewSyncPricesTask(p SyncPricesPayload, o TaskOptions) (*asynq.Task, error) {
	payload, err := integEncode(TypeEPIASSyncPrices, p)
	if err != nil {
		return nil, err
	}
	opts := append([]asynq.Option{
		asynq.Unique(time.Hour),
		asynq.Timeout(10 * time.Minute),
	}, integMaxRetryOptions(o)...)
	return asynq.NewTask(TypeEPIASSyncPrices, payload, opts...), nil
}

// DecodeSyncPrices reads an epias.sync_prices payload.
func DecodeSyncPrices(t *asynq.Task) (SyncPricesPayload, error) {
	var p SyncPricesPayload
	if err := integDecode(TypeEPIASSyncPrices, t.Payload(), &p); err != nil {
		return SyncPricesPayload{}, err
	}
	return p, nil
}

// Ingestion is what handles the day-to-day integration tasks: the platform
// dispatch tick, per-credential analyzer discovery and per-analyzer
// reading fetches. Task 10 implements it.
type Ingestion interface {
	Dispatch(ctx context.Context) error
	SyncAnalyzers(ctx context.Context, p SyncAnalyzersPayload) error
	FetchReadings(ctx context.Context, p FetchReadingsPayload) error
}

// Backfiller handles integration.backfill. Task 10/15 implements it.
type Backfiller interface {
	Backfill(ctx context.Context, p BackfillPayload) error
}

// PriceSyncer handles epias.sync_prices. Task 12 implements it.
type PriceSyncer interface {
	SyncPrices(ctx context.Context, p SyncPricesPayload) error
}

// integHandleSyncDispatch adapts Handlers.Ingestion.Dispatch to asynq's
// handler signature.
func (h *Handlers) integHandleSyncDispatch(ctx context.Context, _ *asynq.Task) error {
	return ClassifyForRetry(h.Ingestion.Dispatch(ctx))
}

// integHandleSyncAnalyzers decodes an integration.sync_analyzers payload
// and adapts Handlers.Ingestion.SyncAnalyzers to asynq's handler signature.
func (h *Handlers) integHandleSyncAnalyzers(ctx context.Context, task *asynq.Task) error {
	p, err := DecodeSyncAnalyzers(task)
	if err != nil {
		return SkipRetry(err)
	}
	return ClassifyForRetry(h.Ingestion.SyncAnalyzers(ctx, p))
}

// integHandleFetchReadings decodes an integration.fetch_readings payload
// and adapts Handlers.Ingestion.FetchReadings to asynq's handler signature.
func (h *Handlers) integHandleFetchReadings(ctx context.Context, task *asynq.Task) error {
	p, err := DecodeFetchReadings(task)
	if err != nil {
		return SkipRetry(err)
	}
	return ClassifyForRetry(h.Ingestion.FetchReadings(ctx, p))
}

// integHandleBackfill decodes an integration.backfill payload and adapts
// Handlers.Backfill.Backfill to asynq's handler signature.
func (h *Handlers) integHandleBackfill(ctx context.Context, task *asynq.Task) error {
	p, err := DecodeBackfill(task)
	if err != nil {
		return SkipRetry(err)
	}
	return ClassifyForRetry(h.Backfill.Backfill(ctx, p))
}

// integHandleSyncPrices decodes an epias.sync_prices payload and adapts
// Handlers.Prices.SyncPrices to asynq's handler signature.
func (h *Handlers) integHandleSyncPrices(ctx context.Context, task *asynq.Task) error {
	p, err := DecodeSyncPrices(task)
	if err != nil {
		return SkipRetry(err)
	}
	return ClassifyForRetry(h.Prices.SyncPrices(ctx, p))
}
