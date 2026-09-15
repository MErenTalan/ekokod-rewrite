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

func fixedCredentialOpener(creds integration.Credentials) credentialOpenerFunc {
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

// recordingEnqueuer is an ingest.Enqueuer that records every task it
// accepts and returns asynq.ErrTaskIDConflict for one it has already seen.
//
// It keys "already seen" on task.Type()+payload rather than trying to read
// the asynq.TaskID option the production Enqueuer would dedup on: the
// *asynq.Task the client library builds carries its Option values in an
// UNEXPORTED field (asynq.Task.opts — see NewTask/EnqueueContext in
// github.com/hibiken/asynq/asynq.go and client.go), so a fake enqueuer
// outside that package cannot read the real TaskID back off task at all.
// job.FetchReadingsPayload's JSON encoding happens to carry AnalyzerID,
// Kind and Window.From/To — the same fields integFetchReadingsTaskID's
// deterministic ID is a function of — so "same type+payload" and "same
// TaskID" collide on the same pairs of calls for this payload shape, which
// is what makes this key a faithful stand-in for exercising Backfill's own
// enqueue loop and its asynq.ErrTaskIDConflict -> skipped handling.
//
// It does NOT verify the real integFetchReadingsTaskID formula itself
// (a different formula producing "same key iff same real TaskID" would
// pass every test in this file unchanged) — that formula is proved
// directly, as a pure function in package job, by
// TestFetchTaskWithWindowHasDeterministicTaskID in
// internal/job/integration_test.go (fix round 1 / I1).
type recordingEnqueuer struct {
	mu    sync.Mutex
	tasks []*asynq.Task
	seen  map[string]bool
}

func newRecordingEnqueuer() *recordingEnqueuer { return &recordingEnqueuer{seen: map[string]bool{}} }

func recordingEnqueuerKey(task *asynq.Task) string { return task.Type() + ":" + string(task.Payload()) }

func (e *recordingEnqueuer) Enqueue(_ context.Context, task *asynq.Task, _ ...asynq.Option) (*asynq.TaskInfo, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	key := recordingEnqueuerKey(task)
	if e.seen[key] {
		return nil, asynq.ErrTaskIDConflict
	}
	e.seen[key] = true
	e.tasks = append(e.tasks, task)
	return &asynq.TaskInfo{ID: uuid.NewString(), Type: task.Type(), Queue: "low"}, nil
}

func (e *recordingEnqueuer) enqueued() []*asynq.Task {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]*asynq.Task, len(e.tasks))
	copy(out, e.tasks)
	return out
}

// markSeen pre-seeds a set of (payload-derived) keys as already enqueued —
// the resumability fixture: a previous, partial backfill run already
// queued (or still retains, per job.NewFetchReadingsTask's 30-day
// Retention) these exact analyzer/kind/window tasks.
func (e *recordingEnqueuer) markSeen(keys ...string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, k := range keys {
		e.seen[k] = true
	}
}

var _ ingest.Enqueuer = (*recordingEnqueuer)(nil)

// backfillTestTaskKey builds the exact key recordingEnqueuer would compute
// for one analyzer/kind/window fetch_readings task, via the real
// job.NewFetchReadingsTask — never a hand-rolled formula that could drift
// from production.
func backfillTestTaskKey(t *testing.T, companyID, credentialID, analyzerID uuid.UUID, kind model.ReadingKind, w job.Window) string {
	t.Helper()
	win := w
	task, err := job.NewFetchReadingsTask(
		job.FetchReadingsPayload{CompanyID: companyID, CredentialID: credentialID, AnalyzerID: analyzerID, Kind: kind, Window: &win},
		job.TaskOptions{MaxRetry: 0},
	)
	require.NoError(t, err)
	return recordingEnqueuerKey(task)
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
	enq := newRecordingEnqueuer()
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
		key := recordingEnqueuerKey(task)
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
// pre-existing ones as skipped, never failed.
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

	// analyzerX's two windows are already "enqueued" from an earlier,
	// partial run of this exact backfill request.
	windows := backfill.Windows(from, to, maxWindow)
	require.Len(t, windows, 2, "45 days at a 30-day max is 2 windows")
	enq := newRecordingEnqueuer()
	for _, w := range windows {
		enq.markSeen(backfillTestTaskKey(t, tn.Company.ID, creds.CredentialID, analyzerX.ID, model.ReadingKindLoadProfile, w))
	}

	b := backfill.New(backfillTestDeps(pool, creds, src, enq, clk))
	payload := job.BackfillPayload{
		CompanyID: tn.Company.ID, CredentialID: creds.CredentialID,
		AnalyzerIDs: []uuid.UUID{analyzerX.ID, analyzerY.ID},
		From:        from, To: to,
	}

	require.NoError(t, b.Backfill(ctx, payload))

	require.Len(t, enq.enqueued(), 2, "only analyzerY's 2 new windows are actually enqueued")

	runs := backfillTestRuns(t, ctx, postgres.NewOpsRepository(pool), tn.Scope)
	require.Len(t, runs, 1)
	require.EqualValues(t, 2, runs[0].Processed, "analyzerY's 2 new windows")
	require.EqualValues(t, 2, runs[0].Skipped, "analyzerX's 2 already-queued windows, counted skipped not failed")
	require.EqualValues(t, 0, runs[0].Failed, "a task-ID conflict is resumability working as intended, never a failure")
	require.Equal(t, "success", runs[0].Status)
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
	enq := newRecordingEnqueuer()
	clk := clock.NewFake(backfillTestNow)
	b := backfill.New(backfillTestDeps(pool, creds, src, enq, clk))

	from := backfillTestNow.Add(-10 * 24 * time.Hour)
	to := backfillTestNow.Add(-5 * 24 * time.Hour)
	payload := job.BackfillPayload{CompanyID: tn.Company.ID, CredentialID: creds.CredentialID, From: from, To: to} // AnalyzerIDs empty

	require.NoError(t, b.Backfill(ctx, payload))

	tasks := enq.enqueued()
	require.Len(t, tasks, 1, "only the active analyzer's single window is enqueued")

	wantKey := backfillTestTaskKey(t, tn.Company.ID, creds.CredentialID, active.ID, model.ReadingKindLoadProfile, job.Window{From: from, To: to})
	require.Equal(t, wantKey, recordingEnqueuerKey(tasks[0]), "the enqueued task must be for the ACTIVE analyzer")

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
	enq := newRecordingEnqueuer()
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
	wantKey := backfillTestTaskKey(t, tn.Company.ID, creds.CredentialID, own.ID, model.ReadingKindLoadProfile, job.Window{From: from, To: to})
	require.Equal(t, wantKey, recordingEnqueuerKey(tasks[0]))

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
				Enqueuer:    newRecordingEnqueuer(),
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
		enq := newRecordingEnqueuer()
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
