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

// Integration task type names. TypeConsumptionRefresh (R17) is declared
// here so job payloads and the queue names are stable, but F2 registers no
// handler for it: Register never wires a handler for it regardless of
// Handlers' fields, because no Handlers field names a consumer for it yet.
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
}

// SyncPricesPayload asks for an EPİAŞ price sync. A nil Window means "the
// job's own default window" (typically "today and tomorrow" — decided by
// the handler, not this payload).
type SyncPricesPayload struct{ Window *Window }

// ConsumptionRefreshPayload is R17's payload shape, declared now so the
// wire format is stable when a later phase adds its handler. No
// constructor or decoder exists yet: nothing in F2 enqueues this task.
type ConsumptionRefreshPayload struct {
	CompanyID, AnalyzerID uuid.UUID
	From, To              time.Time
}

// TaskOptions carries the one per-enqueue override every integration task
// constructor accepts. A zero TaskOptions leaves the task's retry count at
// asynq's own default.
type TaskOptions struct{ MaxRetry int }

// integMaxRetryOption returns the asynq.MaxRetry option for o, or nil when
// o asks for no override — asynq.MaxRetry(0) would explicitly set zero
// retries, which is not what an unset TaskOptions means.
func integMaxRetryOption(o TaskOptions) []asynq.Option {
	if o.MaxRetry > 0 {
		return []asynq.Option{asynq.MaxRetry(o.MaxRetry)}
	}
	return nil
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
// discarding data the worker never even reads.
func integDecode(taskType string, data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("decode %s payload: %w", taskType, err)
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
	}, integMaxRetryOption(o)...)
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
	}, integMaxRetryOption(o)...)
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
func integFetchReadingsTaskID(p FetchReadingsPayload) string {
	return fmt.Sprintf("backfill:%s:%s:%d:%d", p.AnalyzerID, p.Kind, p.Window.From.Unix(), p.Window.To.Unix())
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
	opts := append([]asynq.Option{asynq.Timeout(10 * time.Minute)}, integMaxRetryOption(o)...)
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
	}, integMaxRetryOption(o)...)
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
	}, integMaxRetryOption(o)...)
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
		return err
	}
	return ClassifyForRetry(h.Ingestion.SyncAnalyzers(ctx, p))
}

// integHandleFetchReadings decodes an integration.fetch_readings payload
// and adapts Handlers.Ingestion.FetchReadings to asynq's handler signature.
func (h *Handlers) integHandleFetchReadings(ctx context.Context, task *asynq.Task) error {
	p, err := DecodeFetchReadings(task)
	if err != nil {
		return err
	}
	return ClassifyForRetry(h.Ingestion.FetchReadings(ctx, p))
}

// integHandleBackfill decodes an integration.backfill payload and adapts
// Handlers.Backfill.Backfill to asynq's handler signature.
func (h *Handlers) integHandleBackfill(ctx context.Context, task *asynq.Task) error {
	p, err := DecodeBackfill(task)
	if err != nil {
		return err
	}
	return ClassifyForRetry(h.Backfill.Backfill(ctx, p))
}

// integHandleSyncPrices decodes an epias.sync_prices payload and adapts
// Handlers.Prices.SyncPrices to asynq's handler signature.
func (h *Handlers) integHandleSyncPrices(ctx context.Context, task *asynq.Task) error {
	p, err := DecodeSyncPrices(task)
	if err != nil {
		return err
	}
	return ClassifyForRetry(h.Prices.SyncPrices(ctx, p))
}
