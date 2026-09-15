//go:build integration

package backfill_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/ingest"
	"github.com/MErenTalan/ekokod-rewrite/internal/ingest/backfill"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

// backfillTestNow is comfortably after every fixed range these tests use.
var backfillTestNow = time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)

// --- fakes -------------------------------------------------------------

// fakePlanner is the minimal integration.Adapter Backfill needs: Kinds and
// MaxWindow (integration.Planner) to plan windows, and Provider to be found
// by sourceMap. Verify/DiscoverMeteringPoints/FetchReadings are never
// called by Backfill (it only enqueues fetch_readings tasks, never runs
// them), so FetchReadings panics if reached — that would mean Backfill
// regressed into calling the pipeline directly instead of enqueuing.
type fakePlanner struct {
	provider  integration.Provider
	kinds     []model.ReadingKind
	maxWindow time.Duration
}

func (a *fakePlanner) Provider() integration.Provider { return a.provider }

func (a *fakePlanner) Verify(context.Context, integration.Credentials) error { return nil }

func (a *fakePlanner) DiscoverMeteringPoints(context.Context, integration.Credentials) ([]integration.MeteringPoint, error) {
	return nil, nil
}

func (a *fakePlanner) FetchReadings(context.Context, integration.Credentials, integration.FetchRequest) (integration.FetchResult, error) {
	panic("backfill_test: Backfiller must never call FetchReadings — it only enqueues fetch_readings tasks")
}

func (a *fakePlanner) Kinds(integration.Credentials) []model.ReadingKind { return a.kinds }

func (a *fakePlanner) MaxWindow(model.ReadingKind) time.Duration { return a.maxWindow }

var _ integration.Adapter = (*fakePlanner)(nil)

// credentialOpenerFunc adapts a plain function to ingest.CredentialOpener.
type credentialOpenerFunc func(ctx context.Context, s store.Scope, credentialID uuid.UUID) (integration.Credentials, error)

func (f credentialOpenerFunc) Open(ctx context.Context, s store.Scope, credentialID uuid.UUID) (integration.Credentials, error) {
	return f(ctx, s, credentialID)
}

// fixedCredentialOpener always returns creds, regardless of the requested
// credentialID. IsActive is forced true (X-M3, final review B introduced
// the field; every existing caller here builds creds without ever meaning
// to exercise the new inactive-credential gate) — the one test that
// specifically wants an INACTIVE credential builds its own CredentialOpener
// inline instead of using this helper.
func fixedCredentialOpener(creds integration.Credentials) credentialOpenerFunc {
	creds.IsActive = true
	return func(context.Context, store.Scope, uuid.UUID) (integration.Credentials, error) { return creds, nil }
}

// panicCredentialOpener fails the test the instant it is called — used to
// prove a malformed payload is rejected before any dependency beyond the
// clock is touched.
func panicCredentialOpener(t *testing.T) credentialOpenerFunc {
	return func(context.Context, store.Scope, uuid.UUID) (integration.Credentials, error) {
		t.Fatal("backfill_test: Credentials.Open must not be called for a malformed payload")
		return integration.Credentials{}, nil
	}
}

// sourceMap is the simplest possible ingest.SourceResolver: a fixed
// provider -> adapter table.
type sourceMap map[integration.Provider]integration.Adapter

func (m sourceMap) Source(p integration.Provider) (integration.Adapter, error) {
	a, ok := m[p]
	if !ok {
		return nil, &integration.Error{Kind: integration.ErrNotFound, Provider: p, Op: "source_map.source"}
	}
	return a, nil
}

var _ ingest.SourceResolver = (sourceMap)(nil)

// backfillRealEnqueuer wraps a real *job.Client over a real (isolated)
// Redis (M5, final review B): every Enqueue call this file's tests make
// goes through asynq's own deterministic-TaskID uniqueness in Redis, not a
// payload-keyed proxy. The payload-keyed fake this replaced could not
// catch a regression that drops ForceRunID from the real TaskID derivation
// (job's own integFetchReadingsTaskID) while leaving the JSON payload
// itself unchanged — the payload is ALL a payload-keyed fake can see, so
// such a mutation is invisible to it; asynq's own Redis-backed uniqueness
// depends on the real TaskID (which DOES fold in ForceRunID), so it is not.
type backfillRealEnqueuer struct {
	client *job.Client

	mu    sync.Mutex
	tasks []*asynq.Task // one per call that actually succeeded, in order
}

// newBackfillRealEnqueuer starts an isolated Redis (testfixtures.StartRedis,
// via RedisConfig) and opens a real job.Client against it.
func newBackfillRealEnqueuer(t *testing.T) *backfillRealEnqueuer {
	t.Helper()
	cfg := testfixtures.RedisConfig(t)
	client, err := job.NewClient(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })
	return &backfillRealEnqueuer{client: client}
}

func (e *backfillRealEnqueuer) Enqueue(ctx context.Context, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	info, err := e.client.Enqueue(ctx, task, opts...)
	if err != nil {
		return info, err
	}
	e.mu.Lock()
	e.tasks = append(e.tasks, task)
	e.mu.Unlock()
	return info, nil
}

// enqueued returns every task that has ACTUALLY been accepted (not
// conflicted) so far, oldest first.
func (e *backfillRealEnqueuer) enqueued() []*asynq.Task {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]*asynq.Task, len(e.tasks))
	copy(out, e.tasks)
	return out
}

var _ ingest.Enqueuer = (*backfillRealEnqueuer)(nil)

// panicEnqueuer fails the test the instant Enqueue is called — used to
// prove a malformed backfill range is rejected before any window is ever
// enqueued (mirrors panicCredentialOpener's own "must not be called" shape
// above).
type panicEnqueuer struct{ t *testing.T }

func (e panicEnqueuer) Enqueue(context.Context, *asynq.Task, ...asynq.Option) (*asynq.TaskInfo, error) {
	e.t.Fatal("backfill_test: Enqueue must not be called for a malformed range")
	return nil, nil
}

var _ ingest.Enqueuer = panicEnqueuer{}

// backfillTaskKey identifies an actual task by its payload shape, for
// same/different comparisons in this file's assertions — it plays no part
// in dedup itself, which now happens for real, in Redis, via job.Client.
func backfillTaskKey(task *asynq.Task) string { return task.Type() + ":" + string(task.Payload()) }

// backfillWantTaskKey builds the key backfillTaskKey would compute for one
// analyzer/kind/window fetch_readings task, via the real
// job.NewFetchReadingsTask — never a hand-rolled formula that could drift
// from production — for comparison against an actually-enqueued task.
func backfillWantTaskKey(t *testing.T, companyID, credentialID, analyzerID uuid.UUID, kind model.ReadingKind, w job.Window) string {
	t.Helper()
	win := w
	task, err := job.NewFetchReadingsTask(
		job.FetchReadingsPayload{CompanyID: companyID, CredentialID: credentialID, AnalyzerID: analyzerID, Kind: kind, Window: &win},
		job.TaskOptions{MaxRetry: 0},
	)
	require.NoError(t, err)
	return backfillTaskKey(task)
}

// backfillPreEnqueue enqueues one fetch_readings task for REAL, through the
// SAME enqueuer Backfill itself will use, simulating a previous, partial
// backfill run that already queued this exact analyzer/kind/window — so a
// re-run's own Enqueue call collides against a REAL asynq.ErrTaskIDConflict
// from Redis, never a hand-rolled key match.
func backfillPreEnqueue(t *testing.T, ctx context.Context, enq *backfillRealEnqueuer, companyID, credentialID, analyzerID uuid.UUID, kind model.ReadingKind, w job.Window) {
	t.Helper()
	win := w
	task, err := job.NewFetchReadingsTask(
		job.FetchReadingsPayload{CompanyID: companyID, CredentialID: credentialID, AnalyzerID: analyzerID, Kind: kind, Window: &win},
		job.TaskOptions{MaxRetry: 0},
	)
	require.NoError(t, err)
	_, err = enq.Enqueue(ctx, task)
	require.NoError(t, err)
}

// --- test scaffolding ----------------------------------------------------

// backfillTestDeps wires a Backfiller against real postgres repositories, a
// fixed credential/adapter pair and the given clock/enqueuer.
func backfillTestDeps(pool *pgxpool.Pool, creds integration.Credentials, src integration.Adapter, enq ingest.Enqueuer, clk clock.Clock) backfill.Deps {
	return backfill.Deps{
		Analyzers:   postgres.NewAnalyzerRepository(pool),
		Ops:         postgres.NewOpsRepository(pool),
		Credentials: fixedCredentialOpener(creds),
		Sources:     sourceMap{creds.Provider: src},
		Enqueuer:    enq,
		Clock:       clk,
		MaxRetry:    3,
	}
}

// backfillTestRuns reads every integration.backfill job_runs row for sc.
func backfillTestRuns(t *testing.T, ctx context.Context, ops *postgres.OpsRepository, sc store.Scope) []model.JobRun {
	t.Helper()
	jt := job.TypeIntegrationBackfill
	runs, err := ops.ListRuns(ctx, sc, store.JobRunFilter{JobType: &jt, Page: store.Page{Limit: 200}})
	require.NoError(t, err)
	return runs
}

func backfillTestSetInactive(t *testing.T, ctx context.Context, analyzers *postgres.AnalyzerRepository, sc store.Scope, id uuid.UUID) {
	t.Helper()
	a, err := analyzers.Get(ctx, sc, id)
	require.NoError(t, err)
	a.IsActive = false
	_, err = analyzers.Update(ctx, sc, a)
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------

// TestBackfillEnqueuesEveryWindowOnce is the brief's acceptance fixture: 45
// days x 2 analyzers x 1 kind at a 30-day max gives 4 tasks with distinct
// deterministic TaskIDs (here: 4 distinct recordingEnqueuer keys — see its
// doc for why that is the same equivalence relation as the real TaskID).
func TestBackfillEnqueuesEveryWindowOnce(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tn := testfixtures.NewTenant(t, ctx, pool, 15001)

	analyzerX := tn.Analyzers[0] // OSOS, building 0
	analyzerY := tn.Analyzers[2] // OSOS, building 1 — same provider/subtype
	require.Equal(t, model.IntegrationProviderOSOS, analyzerX.Provider)
	require.Equal(t, model.IntegrationProviderOSOS, analyzerY.Provider)
	require.Equal(t, analyzerX.ProviderSubtype, analyzerY.ProviderSubtype)

	creds := integration.Credentials{CredentialID: uuid.New(), CompanyID: tn.Company.ID, Provider: integration.ProviderOSOS, Subtype: analyzerX.ProviderSubtype}
	src := &fakePlanner{provider: integration.ProviderOSOS, kinds: []model.ReadingKind{model.ReadingKindLoadProfile}, maxWindow: 30 * 24 * time.Hour}
	enq := newBackfillRealEnqueuer(t)
	clk := clock.NewFake(backfillTestNow)
	b := backfill.New(backfillTestDeps(pool, creds, src, enq, clk))

	from := backfillTestNow.Add(-60 * 24 * time.Hour)
	to := from.Add(45 * 24 * time.Hour)
	payload := job.BackfillPayload{
		CompanyID: tn.Company.ID, CredentialID: creds.CredentialID,
		AnalyzerIDs: []uuid.UUID{analyzerX.ID, analyzerY.ID},
		From:        from, To: to,
	}

	require.NoError(t, b.Backfill(ctx, payload))

	tasks := enq.enqueued()
	require.Len(t, tasks, 4, "45 days x 2 analyzers x 1 kind at a 30-day max = 4 windows")
	seen := make(map[string]bool, len(tasks))
	for _, task := range tasks {
		key := backfillTaskKey(task)
		require.False(t, seen[key], "every enqueued task must have a distinct key (proxy for a distinct deterministic TaskID)")
		seen[key] = true
	}

	runs := backfillTestRuns(t, ctx, postgres.NewOpsRepository(pool), tn.Scope)
	require.Len(t, runs, 1)
	require.Equal(t, "success", runs[0].Status)
	require.EqualValues(t, 4, runs[0].Processed)
	require.EqualValues(t, 0, runs[0].Skipped)
	require.EqualValues(t, 0, runs[0].Failed)
}

// TestBackfillRerunIsResumable: 2 of the 4 windows from the fixture above
// are already enqueued (simulating a previous, partial run); re-running the
// same backfill request enqueues only the 2 that are new and counts the 2
// pre-existing ones as skipped, never failed. R53: a run with any skipped
// (already_enqueued) window is "partial", never "success" — resumability
// working as intended must still be visible to an operator as "this run did
// not do everything", not silently read as a clean success.
func TestBackfillRerunIsResumable(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tn := testfixtures.NewTenant(t, ctx, pool, 15002)

	analyzerX := tn.Analyzers[0] // OSOS
	analyzerY := tn.Analyzers[2] // OSOS

	creds := integration.Credentials{CredentialID: uuid.New(), CompanyID: tn.Company.ID, Provider: integration.ProviderOSOS, Subtype: analyzerX.ProviderSubtype}
	maxWindow := 30 * 24 * time.Hour
	src := &fakePlanner{provider: integration.ProviderOSOS, kinds: []model.ReadingKind{model.ReadingKindLoadProfile}, maxWindow: maxWindow}
	clk := clock.NewFake(backfillTestNow)

	from := backfillTestNow.Add(-60 * 24 * time.Hour)
	to := from.Add(45 * 24 * time.Hour)

	// analyzerX's two windows are already REALLY enqueued (M5: through the
	// same job.Client/Redis Backfill itself will use), simulating an
	// earlier, partial run of this exact backfill request.
	windows := backfill.Windows(from, to, maxWindow)
	require.Len(t, windows, 2, "45 days at a 30-day max is 2 windows")
	enq := newBackfillRealEnqueuer(t)
	for _, w := range windows {
		backfillPreEnqueue(t, ctx, enq, tn.Company.ID, creds.CredentialID, analyzerX.ID, model.ReadingKindLoadProfile, w)
	}
	preEnqueued := len(enq.enqueued())

	b := backfill.New(backfillTestDeps(pool, creds, src, enq, clk))
	payload := job.BackfillPayload{
		CompanyID: tn.Company.ID, CredentialID: creds.CredentialID,
		AnalyzerIDs: []uuid.UUID{analyzerX.ID, analyzerY.ID},
		From:        from, To: to,
	}

	require.NoError(t, b.Backfill(ctx, payload))

	require.Len(t, enq.enqueued(), preEnqueued+2, "only analyzerY's 2 new windows are actually enqueued")

	runs := backfillTestRuns(t, ctx, postgres.NewOpsRepository(pool), tn.Scope)
	require.Len(t, runs, 1)
	require.EqualValues(t, 2, runs[0].Processed, "analyzerY's 2 new windows")
	require.EqualValues(t, 2, runs[0].Skipped, "analyzerX's 2 already-queued windows, counted skipped not failed")
	require.EqualValues(t, 0, runs[0].Failed, "a task-ID conflict is resumability working as intended, never a failure")
	require.Equal(t, "partial", runs[0].Status, "R53: any skipped window forces partial, never success")

	var detail struct {
		AlreadyEnqueued int32 `json:"already_enqueued"`
	}
	require.NoError(t, json.Unmarshal(runs[0].Detail, &detail))
	require.EqualValues(t, 2, detail.AlreadyEnqueued, "the 2 skipped windows must be counted as already_enqueued in job_runs.detail (R53)")
}

// TestBackfillForceReEnqueuesAlreadySeenWindows is R53's Force half: the
// exact same "2 of 4 windows already seen" fixture
// TestBackfillRerunIsResumable uses, but with Force: true — every window,
// including analyzerX's two that would otherwise collide on the plain
// deterministic id, must be enqueued again (a re-run after fixing a bad
// credential must never silently no-op), processed=4, skipped=0, and the
// run reports a clean "success" (nothing was skipped this time).
func TestBackfillForceReEnqueuesAlreadySeenWindows(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tn := testfixtures.NewTenant(t, ctx, pool, 15009)

	analyzerX := tn.Analyzers[0] // OSOS
	analyzerY := tn.Analyzers[2] // OSOS

	creds := integration.Credentials{CredentialID: uuid.New(), CompanyID: tn.Company.ID, Provider: integration.ProviderOSOS, Subtype: analyzerX.ProviderSubtype}
	maxWindow := 30 * 24 * time.Hour
	src := &fakePlanner{provider: integration.ProviderOSOS, kinds: []model.ReadingKind{model.ReadingKindLoadProfile}, maxWindow: maxWindow}
	clk := clock.NewFake(backfillTestNow)

	from := backfillTestNow.Add(-60 * 24 * time.Hour)
	to := from.Add(45 * 24 * time.Hour)

	// analyzerX's two windows are already REALLY enqueued from an earlier
	// run — this time under a PLAIN (non-Force) id, exactly the id a Force
	// run must NOT collide with (M5: through the same job.Client/Redis
	// Backfill itself will use).
	windows := backfill.Windows(from, to, maxWindow)
	require.Len(t, windows, 2, "45 days at a 30-day max is 2 windows")
	enq := newBackfillRealEnqueuer(t)
	for _, w := range windows {
		backfillPreEnqueue(t, ctx, enq, tn.Company.ID, creds.CredentialID, analyzerX.ID, model.ReadingKindLoadProfile, w)
	}
	preEnqueued := len(enq.enqueued())

	b := backfill.New(backfillTestDeps(pool, creds, src, enq, clk))
	payload := job.BackfillPayload{
		CompanyID: tn.Company.ID, CredentialID: creds.CredentialID,
		AnalyzerIDs: []uuid.UUID{analyzerX.ID, analyzerY.ID},
		From:        from, To: to,
		Force: true,
	}

	require.NoError(t, b.Backfill(ctx, payload))

	require.Len(t, enq.enqueued(), preEnqueued+4, "Force re-mints every window's id, so nothing collides with the earlier plain-id run")

	runs := backfillTestRuns(t, ctx, postgres.NewOpsRepository(pool), tn.Scope)
	require.Len(t, runs, 1)
	require.EqualValues(t, 4, runs[0].Processed)
	require.EqualValues(t, 0, runs[0].Skipped, "Force means nothing collides, so nothing is skipped")
	require.EqualValues(t, 0, runs[0].Failed)
	require.Equal(t, "success", runs[0].Status, "a Force run that skips nothing is a clean success")

	var detail struct {
		AlreadyEnqueued int32 `json:"already_enqueued"`
	}
	require.NoError(t, json.Unmarshal(runs[0].Detail, &detail))
	require.EqualValues(t, 0, detail.AlreadyEnqueued)
}

// TestBackfillEmptyAnalyzerListMeansActiveAnalyzersNotAll: with no
// AnalyzerIDs, an inactive analyzer of the credential's own provider is not
// enqueued — the empty list means "the credential's ACTIVE analyzers",
// never "every analyzer regardless of status" (Global Constraints: a
// required positional id list that is empty is NOT "all").
func TestBackfillEmptyAnalyzerListMeansActiveAnalyzersNotAll(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tn := testfixtures.NewTenant(t, ctx, pool, 15003)
	analyzers := postgres.NewAnalyzerRepository(pool)

	active := tn.Analyzers[0]   // OSOS, active by fixture default
	inactive := tn.Analyzers[2] // OSOS, same subtype — made inactive below
	require.Equal(t, active.ProviderSubtype, inactive.ProviderSubtype)
	// AdminScope (whole company), not tn.Scope: inactive (tn.Analyzers[2])
	// is under Buildings[1], outside tn.Scope's single-building grant. This
	// is test setup mutating a fixture row, not the thing under test.
	backfillTestSetInactive(t, ctx, analyzers, tn.AdminScope, inactive.ID)

	creds := integration.Credentials{CredentialID: uuid.New(), CompanyID: tn.Company.ID, Provider: integration.ProviderOSOS, Subtype: active.ProviderSubtype}
	src := &fakePlanner{provider: integration.ProviderOSOS, kinds: []model.ReadingKind{model.ReadingKindLoadProfile}, maxWindow: 30 * 24 * time.Hour}
	enq := newBackfillRealEnqueuer(t)
	clk := clock.NewFake(backfillTestNow)
	b := backfill.New(backfillTestDeps(pool, creds, src, enq, clk))

	from := backfillTestNow.Add(-10 * 24 * time.Hour)
	to := backfillTestNow.Add(-5 * 24 * time.Hour)
	payload := job.BackfillPayload{CompanyID: tn.Company.ID, CredentialID: creds.CredentialID, From: from, To: to} // AnalyzerIDs empty

	require.NoError(t, b.Backfill(ctx, payload))

	tasks := enq.enqueued()
	require.Len(t, tasks, 1, "only the active analyzer's single window is enqueued")

	wantKey := backfillWantTaskKey(t, tn.Company.ID, creds.CredentialID, active.ID, model.ReadingKindLoadProfile, job.Window{From: from, To: to})
	require.Equal(t, wantKey, backfillTaskKey(tasks[0]), "the enqueued task must be for the ACTIVE analyzer")

	runs := backfillTestRuns(t, ctx, postgres.NewOpsRepository(pool), tn.Scope)
	require.Len(t, runs, 1)
	require.EqualValues(t, 1, runs[0].Processed)
}

// TestBackfillMissingAnalyzerIDsRunPartialAndOnlyEnqueueVisible is fix
// round 1 / I2: AnalyzerIDs naming the credential's own active analyzer, a
// random uuid that names no analyzer at all, and another company's analyzer
// id must run "partial" (own analyzer's windows enqueue normally, the two
// bad ids each count failed, never silently skipped or silently
// succeeding), name BOTH missing ids in job_runs.detail.failures, and
// enqueue only the own analyzer's windows — never the cross-company one
// (SystemScope(p.CompanyID) narrows Analyzers.List by company, so the other
// tenant's analyzer id is invisible and reported missing exactly like the
// random uuid, not silently included).
func TestBackfillMissingAnalyzerIDsRunPartialAndOnlyEnqueueVisible(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tn := testfixtures.NewTenant(t, ctx, pool, 15007)
	other := testfixtures.NewTenant(t, ctx, pool, 15008)

	own := tn.Analyzers[0] // OSOS, building 0, tn's own company
	randomID := uuid.New()
	crossCompanyID := other.Analyzers[0].ID // visible to `other`, not to tn

	creds := integration.Credentials{CredentialID: uuid.New(), CompanyID: tn.Company.ID, Provider: integration.ProviderOSOS, Subtype: own.ProviderSubtype}
	src := &fakePlanner{provider: integration.ProviderOSOS, kinds: []model.ReadingKind{model.ReadingKindLoadProfile}, maxWindow: 30 * 24 * time.Hour}
	enq := newBackfillRealEnqueuer(t)
	clk := clock.NewFake(backfillTestNow)
	b := backfill.New(backfillTestDeps(pool, creds, src, enq, clk))

	from := backfillTestNow.Add(-10 * 24 * time.Hour)
	to := backfillTestNow.Add(-5 * 24 * time.Hour)
	payload := job.BackfillPayload{
		CompanyID: tn.Company.ID, CredentialID: creds.CredentialID,
		AnalyzerIDs: []uuid.UUID{own.ID, randomID, crossCompanyID},
		From:        from, To: to,
	}

	require.NoError(t, b.Backfill(ctx, payload))

	tasks := enq.enqueued()
	require.Len(t, tasks, 1, "only the own, visible analyzer's single window is enqueued")
	wantKey := backfillWantTaskKey(t, tn.Company.ID, creds.CredentialID, own.ID, model.ReadingKindLoadProfile, job.Window{From: from, To: to})
	require.Equal(t, wantKey, backfillTaskKey(tasks[0]))

	runs := backfillTestRuns(t, ctx, postgres.NewOpsRepository(pool), tn.Scope)
	require.Len(t, runs, 1)
	require.Equal(t, "partial", runs[0].Status, "one visible analyzer succeeded and two ids were missing: partial, not success or failed")
	require.EqualValues(t, 1, runs[0].Processed)
	require.EqualValues(t, 0, runs[0].Skipped)
	require.EqualValues(t, 2, runs[0].Failed, "the random id and the cross-company id each count failed")

	var detail struct {
		Failures []struct {
			AnalyzerID *uuid.UUID `json:"analyzer_id"`
			Reason     string     `json:"reason"`
		} `json:"failures"`
	}
	require.NoError(t, json.Unmarshal(runs[0].Detail, &detail))
	require.Len(t, detail.Failures, 2, "both missing ids must be named, never merged or dropped")
	named := make(map[uuid.UUID]bool, 2)
	for _, f := range detail.Failures {
		require.NotNil(t, f.AnalyzerID)
		named[*f.AnalyzerID] = true
	}
	require.True(t, named[randomID], "the random, non-existent id must be named in job_runs.detail.failures")
	require.True(t, named[crossCompanyID], "the other company's analyzer id must be named in job_runs.detail.failures")
}

// TestBackfillRejectsInvertedAndFutureRanges proves the brief's three range
// rules — From < To, To <= now, span <= 5 years — return
// integration.ErrMalformedPayload and never even open the credential or
// start a job run: a malformed request never attempted anything.
func TestBackfillRejectsInvertedAndFutureRanges(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tn := testfixtures.NewTenant(t, ctx, pool, 15004)
	clk := clock.NewFake(backfillTestNow)

	cases := []struct {
		name     string
		from, to time.Time
	}{
		{"inverted range", backfillTestNow.Add(-1 * time.Hour), backfillTestNow.Add(-2 * time.Hour)},
		{"equal from and to", backfillTestNow.Add(-1 * time.Hour), backfillTestNow.Add(-1 * time.Hour)},
		{"future to", backfillTestNow.Add(-24 * time.Hour), backfillTestNow.Add(24 * time.Hour)},
		{"span exceeds 5 years", backfillTestNow.Add(-6 * 365 * 24 * time.Hour), backfillTestNow},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps := backfill.Deps{
				Analyzers:   postgres.NewAnalyzerRepository(pool),
				Ops:         postgres.NewOpsRepository(pool),
				Credentials: panicCredentialOpener(t),
				Sources:     sourceMap{},
				Enqueuer:    panicEnqueuer{t: t},
				Clock:       clk,
				MaxRetry:    3,
			}
			b := backfill.New(deps)
			err := b.Backfill(ctx, job.BackfillPayload{CompanyID: tn.Company.ID, CredentialID: uuid.New(), From: tc.from, To: tc.to})
			require.Error(t, err)
			require.True(t, errors.Is(err, integration.ErrMalformedPayload), "got %v", err)
		})
	}

	runs := backfillTestRuns(t, ctx, postgres.NewOpsRepository(pool), tn.Scope)
	require.Empty(t, runs, "a malformed range must never start a job run")
}

// TestBackfillWarnsAboutAggregateHorizon: a From more than 30 days before
// now appends exactly one info operational message naming the R17
// aggregate horizon; a From within it appends none.
func TestBackfillWarnsAboutAggregateHorizon(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	ops := postgres.NewOpsRepository(pool)

	run := func(t *testing.T, seed int64, from, to time.Time) []model.OperationalMessage {
		tn := testfixtures.NewTenant(t, ctx, pool, seed)
		analyzer := tn.Analyzers[0]
		creds := integration.Credentials{CredentialID: uuid.New(), CompanyID: tn.Company.ID, Provider: integration.ProviderOSOS, Subtype: analyzer.ProviderSubtype}
		src := &fakePlanner{provider: integration.ProviderOSOS, kinds: []model.ReadingKind{model.ReadingKindLoadProfile}, maxWindow: 30 * 24 * time.Hour}
		enq := newBackfillRealEnqueuer(t)
		clk := clock.NewFake(backfillTestNow)
		b := backfill.New(backfillTestDeps(pool, creds, src, enq, clk))

		payload := job.BackfillPayload{CompanyID: tn.Company.ID, CredentialID: creds.CredentialID, AnalyzerIDs: []uuid.UUID{analyzer.ID}, From: from, To: to}
		require.NoError(t, b.Backfill(ctx, payload))

		messages, err := ops.ListMessages(ctx, tn.Scope, store.MessageFilter{})
		require.NoError(t, err)
		return messages
	}

	t.Run("range reaches before the horizon", func(t *testing.T) {
		t.Parallel()
		from := backfillTestNow.Add(-40 * 24 * time.Hour)
		to := backfillTestNow.Add(-35 * 24 * time.Hour)
		messages := run(t, 15005, from, to)

		require.Len(t, messages, 1)
		require.Equal(t, "job", messages[0].Kind)
		require.Equal(t, "info", messages[0].Status)
		require.Contains(t, messages[0].Message, "is not in the consumption aggregates until consumption.refresh runs for it (F3)")
	})

	t.Run("range stays within the horizon", func(t *testing.T) {
		t.Parallel()
		from := backfillTestNow.Add(-10 * 24 * time.Hour)
		to := backfillTestNow.Add(-5 * 24 * time.Hour)
		messages := run(t, 15006, from, to)

		require.Empty(t, messages, "a range entirely within the 30-day horizon gets no warning")
	})
}

// TestBackfillRefusesInactiveCredential is X-M3's Backfill half (final
// review B): see internal/ingest's TestFetchReadingsRefusesInactiveCredential
// for the full rationale. An inactive credential must stop before any
// window is planned or enqueued — the fakePlanner here would panic if
// FetchReadings were ever reached, and panicEnqueuer proves Enqueue is not.
func TestBackfillRefusesInactiveCredential(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tn := testfixtures.NewTenant(t, ctx, pool, 15010)

	creds := integration.Credentials{
		CredentialID: uuid.New(), CompanyID: tn.Company.ID, Provider: integration.ProviderOSOS,
		Subtype: "BackfillInactiveSub", IsActive: false,
	}
	src := &fakePlanner{provider: integration.ProviderOSOS, kinds: []model.ReadingKind{model.ReadingKindLoadProfile}, maxWindow: 30 * 24 * time.Hour}
	clk := clock.NewFake(backfillTestNow)

	deps := backfill.Deps{
		Analyzers: postgres.NewAnalyzerRepository(pool),
		Ops:       postgres.NewOpsRepository(pool),
		Credentials: credentialOpenerFunc(func(context.Context, store.Scope, uuid.UUID) (integration.Credentials, error) {
			return creds, nil
		}),
		Sources:  sourceMap{creds.Provider: src},
		Enqueuer: panicEnqueuer{t: t},
		Clock:    clk,
		MaxRetry: 3,
	}
	b := backfill.New(deps)

	from := backfillTestNow.Add(-10 * 24 * time.Hour)
	to := backfillTestNow.Add(-5 * 24 * time.Hour)
	err := b.Backfill(ctx, job.BackfillPayload{CompanyID: tn.Company.ID, CredentialID: creds.CredentialID, From: from, To: to})
	require.Error(t, err)
	require.ErrorIs(t, err, integration.ErrConfig, "an inactive credential must be ErrConfig, non-retryable via job.ClassifyForRetry")

	runs := backfillTestRuns(t, ctx, postgres.NewOpsRepository(pool), tn.Scope)
	require.Len(t, runs, 1)
	require.Equal(t, "failed", runs[0].Status)

	messages, merr := postgres.NewOpsRepository(pool).ListMessages(ctx, tn.Scope, store.MessageFilter{})
	require.NoError(t, merr)
	require.NotEmpty(t, messages, "the inactive-credential refusal must leave an operational message")
}
