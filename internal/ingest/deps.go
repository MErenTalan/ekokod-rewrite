package ingest

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// CredentialOpener decrypts one company's integration credentials. It is
// satisfied structurally by *credentials.Service (F2 Task 14); ingest
// declares its own narrow interface rather than importing internal/credentials
// so that Task 10 and Task 14 stay independently buildable (Task 14 depends
// on ingest.Enqueuer, not the reverse).
type CredentialOpener interface {
	Open(ctx context.Context, s store.Scope, credentialID uuid.UUID) (integration.Credentials, error)
}

// SourceResolver looks up the Adapter for a provider. Satisfied by
// *integration.Registry.
type SourceResolver interface {
	Source(p integration.Provider) (integration.Adapter, error)
}

// Enqueuer schedules a task. Satisfied by *job.Client.
type Enqueuer interface {
	Enqueue(ctx context.Context, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

// PostPersistHook runs after a page of readings is persisted and BEFORE the
// cursor advances: a hook that returns an error leaves the cursor untouched
// and FetchReadings returns that error, exactly as a page fetch failure
// would. F2 Task 11's *generation.Accumulator implements this for pm5340,
// reconciling MeterReading.IntervalGenerationKwh against generation_anchors.
type PostPersistHook interface {
	AfterPersist(ctx context.Context, s store.Scope, a model.Analyzer, kind model.ReadingKind, from, to time.Time) error
}

// ConsumptionRefreshEnqueuer enqueues consumption.refresh for an affected
// range. F2 declared it for wire-format stability without ever calling it
// (R17: the task was declared, not enqueued). F3 Task 11b is the enqueue
// call site (FetchReadings' finishRun and failFetchRun paths, R73): when
// Deps.ConsumptionRefresh is non-nil, Options.ConsumptionRefreshEnabled is
// true, and the run's affected range reaches back further than
// consumptionRefreshThreshold, Service calls it with the affected range —
// never the requested window. worker/wiring.go's adapter wraps *job.Client
// with job.NewConsumptionRefreshTask. A nil Deps.ConsumptionRefresh (as in
// every F2-era caller that has not been updated) simply disables the seam,
// exactly as before.
type ConsumptionRefreshEnqueuer interface {
	EnqueueConsumptionRefresh(ctx context.Context, p job.ConsumptionRefreshPayload) error
}

// Deps is every dependency Service needs. Every field but Hooks and
// ConsumptionRefresh is required; New returns an error naming the first nil
// one it finds.
type Deps struct {
	Analyzers      store.AnalyzerRepository
	Readings       store.ReadingRepository
	Cursors        store.CursorRepository
	Anomalies      store.AnomalyRepository
	Ops            store.OpsRepository
	ProviderSeries store.ProviderSeriesRepository
	AdminIngestion store.AdminIngestionRepository
	AdminJournal   store.AdminJournalRepository
	Credentials    CredentialOpener
	Sources        SourceResolver
	Enqueuer       Enqueuer
	// Hooks is keyed by the provider whose pages should run it. A provider
	// with no entry (including a nil map) simply runs no hook.
	Hooks map[model.IntegrationProvider][]PostPersistHook
	// ConsumptionRefresh enqueues consumption.refresh (Task 11b) — see
	// ConsumptionRefreshEnqueuer. nil disables the enqueue seam entirely,
	// exactly as F2 left it (R17).
	ConsumptionRefresh ConsumptionRefreshEnqueuer
	Clock              clock.Clock
	Log                *slog.Logger
}

// Options are Service's tunables. New fills in the documented default for
// any zero-valued field below (MaxRetry is deliberately excluded: job.
// TaskOptions.MaxRetry's own doc says a zero value is a legitimate,
// deliberate "no retries" and must never be silently replaced — the same
// rule applies here).
type Options struct {
	// FutureTolerance bounds how far into the future a reading's Ts may sit
	// before Validate rejects it (R12). Default 15 minutes.
	FutureTolerance time.Duration
	// SanityMultiple is R13's configurable sanity multiple: a register jump
	// beyond this many times the point's typical interval consumption is
	// rejected. Default 10.
	SanityMultiple decimal.Decimal
	// InitialLookback is how far back FetchReadings starts a first-ever
	// fetch (no stored cursor) for an analyzer and kind (R18). Default 720h
	// (30 days).
	InitialLookback time.Duration
	// MaxRetry is passed through to job.TaskOptions for every task Service
	// enqueues (sync_analyzers from Dispatch, fetch_readings from
	// SyncAnalyzers). Zero is a legitimate "no retries", never defaulted.
	MaxRetry int
	// MaxPagesPerRun bounds FetchReadings' pagination loop across the whole
	// run (every chunk, every page), so a misbehaving adapter that never
	// stops returning NextCursor cannot hang a worker forever. Default 1000.
	MaxPagesPerRun int
	// SanityStreak carries R13's per-run "at most three consecutive
	// sanity-jump rejections" state across every Validate call FetchReadings
	// makes for one run — see SanityStreak's doc. Service sets this to a
	// fresh *SanityStreak at the start of every run; a caller of Validate
	// directly (every unit test) leaves it nil and gets Validate's
	// single-call-scoped fallback instead.
	SanityStreak *SanityStreak
	// ConsumptionRefreshEnabled gates the consumption.refresh enqueue call
	// site (R73, Task 11b): EKOKOD_CONSUMPTION_REFRESH_ENABLED, default
	// true, resolved by internal/platform/config and wired here by
	// worker/wiring.go. Deliberately EXCLUDED from New's zero-value
	// defaulting — like MaxRetry above, false is a legitimate, deliberate
	// "do not enqueue" that New must never silently replace with true.
	ConsumptionRefreshEnabled bool
}

// Default values for the Options fields New fills in when left zero.
const (
	DefaultFutureTolerance = 15 * time.Minute
	DefaultInitialLookback = 720 * time.Hour // R18: 30 days
	DefaultMaxPagesPerRun  = 1000
)

// DefaultSanityMultiple is R13's default sanity multiple, 10.
func DefaultSanityMultiple() decimal.Decimal { return decimal.NewFromInt(10) }
