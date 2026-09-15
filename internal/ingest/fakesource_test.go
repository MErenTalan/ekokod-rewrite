package ingest_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/ingest"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// testAnalyzerID is the fixed AnalyzerID every reading()/resetAt() helper
// below stamps, so unit tests that build readings by hand never have to
// thread an id through every call.
var testAnalyzerID = uuid.MustParse("11111111-1111-4111-8111-111111111111")

// ts parses an RFC3339 timestamp, panicking on a typo — every literal here
// is a constant in a test file, so a parse failure is a bug in the test, not
// a runtime condition worth threading an error for.
func ts(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic("ingest_test: bad timestamp literal " + s + ": " + err.Error())
	}
	return t
}

func ptrString(s string) *string { return &s }

// registerFields maps the canonical register names reading() accepts to the
// model.MeterReading field each one sets.
var registerFields = map[string]func(*model.MeterReading, *decimal.Decimal){
	"active_import":              func(r *model.MeterReading, v *decimal.Decimal) { r.ActiveImport = v },
	"reactive_inductive_import":  func(r *model.MeterReading, v *decimal.Decimal) { r.ReactiveInductiveImport = v },
	"reactive_capacitive_import": func(r *model.MeterReading, v *decimal.Decimal) { r.ReactiveCapacitiveImport = v },
	"t1_import":                  func(r *model.MeterReading, v *decimal.Decimal) { r.T1Import = v },
	"t2_import":                  func(r *model.MeterReading, v *decimal.Decimal) { r.T2Import = v },
	"t3_import":                  func(r *model.MeterReading, v *decimal.Decimal) { r.T3Import = v },
	"active_export":              func(r *model.MeterReading, v *decimal.Decimal) { r.ActiveExport = v },
	"reactive_inductive_export":  func(r *model.MeterReading, v *decimal.Decimal) { r.ReactiveInductiveExport = v },
	"reactive_capacitive_export": func(r *model.MeterReading, v *decimal.Decimal) { r.ReactiveCapacitiveExport = v },
	"t1_export":                  func(r *model.MeterReading, v *decimal.Decimal) { r.T1Export = v },
	"t2_export":                  func(r *model.MeterReading, v *decimal.Decimal) { r.T2Export = v },
	"t3_export":                  func(r *model.MeterReading, v *decimal.Decimal) { r.T3Export = v },
	"max_demand_kw":              func(r *model.MeterReading, v *decimal.Decimal) { r.MaxDemandKw = v },
	"interval_generation_kwh":    func(r *model.MeterReading, v *decimal.Decimal) { r.IntervalGenerationKwh = v },
}

// reading builds a load_profile MeterReading for testAnalyzerID at tsStr,
// with registers set from a canonical-name -> decimal-literal map.
func reading(tsStr string, registers map[string]string) model.MeterReading {
	r := model.MeterReading{
		AnalyzerID: testAnalyzerID, Ts: ts(tsStr), Kind: model.ReadingKindLoadProfile,
		MultiplierApplied: decimal.NewFromInt(1), SourceProvider: model.IntegrationProviderOSOS,
	}
	for name, lit := range registers {
		set, ok := registerFields[name]
		if !ok {
			panic("ingest_test: unknown register " + name)
		}
		v := decimal.RequireFromString(lit)
		set(&r, &v)
	}
	return r
}

// readingKind is reading with an explicit Kind, for AnalyzerID/Kind grouping
// tests (Dedupe, the ReadingKindReset fixtures DetectNegativeDeltas takes).
func readingKind(tsStr string, kind model.ReadingKind, registers map[string]string) model.MeterReading {
	r := reading(tsStr, registers)
	r.Kind = kind
	return r
}

// readingFor is reading for an explicit analyzer and kind — what the
// integration tests use, since they exercise real, tenant-fixture analyzer
// ids rather than the fixed testAnalyzerID the pure unit tests share.
func readingFor(analyzerID uuid.UUID, tsStr string, kind model.ReadingKind, registers map[string]string) model.MeterReading {
	r := reading(tsStr, registers)
	r.AnalyzerID = analyzerID
	r.Kind = kind
	return r
}

// resetAt builds a bare reset-kind reading at tsStr: DetectNegativeDeltas
// only ever looks at resets' Ts, never their registers.
func resetAt(tsStr string) model.MeterReading {
	return model.MeterReading{AnalyzerID: testAnalyzerID, Ts: ts(tsStr), Kind: model.ReadingKindReset}
}

// fetchStep is one scripted response fakeAdapter.FetchReadings returns, in
// order, to successive calls.
type fetchStep struct {
	result integration.FetchResult
	err    error
}

// ingestTestSpyHook is an ingest.PostPersistHook that records every call's
// arguments instead of doing anything — M2's proof that hooks see the
// range actually persisted THAT PAGE, not the outer chunk bound.
type ingestTestSpyHook struct {
	mu    sync.Mutex
	calls []ingestTestHookCall
}

type ingestTestHookCall struct {
	Kind     model.ReadingKind
	From, To time.Time
}

func (h *ingestTestSpyHook) AfterPersist(_ context.Context, _ store.Scope, _ model.Analyzer, kind model.ReadingKind, from, to time.Time) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, ingestTestHookCall{Kind: kind, From: from, To: to})
	return nil
}

func (h *ingestTestSpyHook) callsSoFar() []ingestTestHookCall {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]ingestTestHookCall, len(h.calls))
	copy(out, h.calls)
	return out
}

// ingestTestSecretLeakingAnalyzerRepo wraps a real store.AnalyzerRepository
// and forces Create to fail for one installation number with an error whose
// text embeds a credential fragment — I4's fixture for proving SyncAnalyzers'
// failed_points detail is redacted exactly like every other record
// TestIngestionFailureMessageContainsNoSecret already pins for FetchReadings.
type ingestTestSecretLeakingAnalyzerRepo struct {
	store.AnalyzerRepository
	failInstallation string
	secret           string
}

func (r *ingestTestSecretLeakingAnalyzerRepo) Create(ctx context.Context, s store.Scope, a model.Analyzer) (model.Analyzer, error) {
	if a.InstallationNumber == r.failInstallation {
		return model.Analyzer{}, fmt.Errorf("ingest_test: create failed, upstream said password %s was wrong", r.secret)
	}
	return r.AnalyzerRepository.Create(ctx, s, a)
}

// ingestTestFailingUpdateAnalyzerRepo wraps a real store.AnalyzerRepository
// and forces every Update to fail — M1's fixture for proving a failed
// multiplier-update neither reports "meter multiplier changed" nor mutates
// the in-memory analyzer FetchReadings keeps using for the rest of its run.
type ingestTestFailingUpdateAnalyzerRepo struct {
	store.AnalyzerRepository
}

func (r *ingestTestFailingUpdateAnalyzerRepo) Update(context.Context, store.Scope, model.Analyzer) (model.Analyzer, error) {
	return model.Analyzer{}, errors.New("ingest_test: forced update failure")
}

// fakeAdapter is the in-memory integration.Adapter every ingest test drives
// instead of a real provider (Global Constraints: F2's pipeline tests are
// never allowed to reach a real endpoint, and Tasks 6-9/12/13's real
// adapters are not merged into this worktree at all).
//
// It is scripted with setSteps/appendSteps: each call to FetchReadings pops
// the next fetchStep. Once the script is exhausted, FetchReadings returns a
// zero FetchResult (no readings, no NextCursor) — a well-behaved provider
// reporting "nothing more in this window" — so a test does not have to
// script every last page.
type fakeAdapter struct {
	provider  integration.Provider
	maxWindow time.Duration
	kinds     []model.ReadingKind

	verifyErr   error
	points      []integration.MeteringPoint
	discoverErr error

	mu          sync.Mutex
	steps       []fetchStep
	perAnalyzer map[uuid.UUID][]fetchStep
	requests    []integration.FetchRequest
}

func newFakeAdapter(provider integration.Provider, maxWindow time.Duration, kinds ...model.ReadingKind) *fakeAdapter {
	return &fakeAdapter{provider: provider, maxWindow: maxWindow, kinds: kinds}
}

func (a *fakeAdapter) Provider() integration.Provider { return a.provider }

func (a *fakeAdapter) Verify(context.Context, integration.Credentials) error { return a.verifyErr }

func (a *fakeAdapter) DiscoverMeteringPoints(context.Context, integration.Credentials) ([]integration.MeteringPoint, error) {
	return a.points, a.discoverErr
}

func (a *fakeAdapter) Kinds(integration.Credentials) []model.ReadingKind { return a.kinds }

func (a *fakeAdapter) MaxWindow(model.ReadingKind) time.Duration { return a.maxWindow }

func (a *fakeAdapter) FetchReadings(_ context.Context, _ integration.Credentials, req integration.FetchRequest) (integration.FetchResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.requests = append(a.requests, req)

	// A per-analyzer script, when one was set for req.AnalyzerID, takes
	// priority over the shared default queue — this is what lets a test
	// drive several analyzers through ONE shared adapter (the way a real
	// provider serves every analyzer of that provider) with independently
	// scripted behaviour per analyzer (e.g. "B's adapter script fails every
	// page" while A and C succeed).
	if steps, ok := a.perAnalyzer[req.AnalyzerID]; ok {
		if len(steps) == 0 {
			return integration.FetchResult{}, nil
		}
		step := steps[0]
		a.perAnalyzer[req.AnalyzerID] = steps[1:]
		return step.result, step.err
	}

	if len(a.steps) == 0 {
		return integration.FetchResult{}, nil
	}
	step := a.steps[0]
	a.steps = a.steps[1:]
	return step.result, step.err
}

// setSteps replaces the whole default script (used when no per-analyzer
// script is set for the requesting analyzer).
func (a *fakeAdapter) setSteps(steps ...fetchStep) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.steps = steps
}

// setStepsFor scripts one analyzer independently of the default queue and
// every other analyzer's own script.
func (a *fakeAdapter) setStepsFor(analyzerID uuid.UUID, steps ...fetchStep) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.perAnalyzer == nil {
		a.perAnalyzer = make(map[uuid.UUID][]fetchStep)
	}
	a.perAnalyzer[analyzerID] = steps
}

// requestLog returns every FetchRequest passed to FetchReadings so far, in
// call order.
func (a *fakeAdapter) requestLog() []integration.FetchRequest {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]integration.FetchRequest, len(a.requests))
	copy(out, a.requests)
	return out
}

var _ integration.Adapter = (*fakeAdapter)(nil)

// credentialOpenerFunc adapts a plain function to ingest.CredentialOpener.
type credentialOpenerFunc func(ctx context.Context, s store.Scope, credentialID uuid.UUID) (integration.Credentials, error)

func (f credentialOpenerFunc) Open(ctx context.Context, s store.Scope, credentialID uuid.UUID) (integration.Credentials, error) {
	return f(ctx, s, credentialID)
}

// fixedCredentialOpener always returns creds, regardless of the requested
// credentialID, for tests that only ever exercise one credential. IsActive
// is forced true (X-M3, final review B introduced the field; every existing
// caller here builds creds without ever meaning to exercise the new
// inactive-credential gate, so this keeps them all passing) — a test that
// specifically wants an INACTIVE credential builds its own CredentialOpener
// inline instead of using this helper.
func fixedCredentialOpener(creds integration.Credentials) credentialOpenerFunc {
	creds.IsActive = true
	return func(context.Context, store.Scope, uuid.UUID) (integration.Credentials, error) { return creds, nil }
}

// sourceMap is the simplest possible ingest.SourceResolver: a fixed
// provider -> adapter table, with no registry construction rules to satisfy
// (unlike integration.NewRegistry, which refuses duplicate providers — not
// a concern any test here has).
type sourceMap map[integration.Provider]integration.Adapter

func (m sourceMap) Source(p integration.Provider) (integration.Adapter, error) {
	a, ok := m[p]
	if !ok {
		return nil, &integration.Error{Kind: integration.ErrNotFound, Provider: p, Op: "source_map.source"}
	}
	return a, nil
}

// recordingEnqueuer is an ingest.Enqueuer that records every task it is
// asked to enqueue and, for a task ID already seen, returns
// asynq.ErrDuplicateTask — the same signal a real asynq.Client gives back
// for NewFetchReadingsTask's Unique(time.Hour) / TaskID dedup, so Dispatch
// and SyncAnalyzers' "skipped (duplicates)" counting is exercised without a
// broker.
type recordingEnqueuer struct {
	mu    sync.Mutex
	tasks []*asynq.Task
	seen  map[string]bool
	// failType, when non-empty, makes every Enqueue of that task type return
	// a plain (non-duplicate) error, for a test proving the "not a
	// duplicate" branch of skipped/failed counting.
	failType string
}

func newRecordingEnqueuer() *recordingEnqueuer { return &recordingEnqueuer{seen: map[string]bool{}} }

func (e *recordingEnqueuer) Enqueue(_ context.Context, task *asynq.Task, _ ...asynq.Option) (*asynq.TaskInfo, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.failType != "" && task.Type() == e.failType {
		return nil, errors.New("recordingEnqueuer: forced failure")
	}
	id := task.Type() + ":" + string(task.Payload())
	if e.seen[id] {
		return nil, asynq.ErrDuplicateTask
	}
	e.seen[id] = true
	e.tasks = append(e.tasks, task)
	return &asynq.TaskInfo{ID: uuid.NewString(), Type: task.Type(), Queue: "default"}, nil
}

func (e *recordingEnqueuer) enqueued() []*asynq.Task {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]*asynq.Task, len(e.tasks))
	copy(out, e.tasks)
	return out
}

var _ ingest.Enqueuer = (*recordingEnqueuer)(nil)
