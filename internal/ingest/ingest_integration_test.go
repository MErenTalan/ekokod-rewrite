//go:build integration

package ingest_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/ingest"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/admin"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

// ingestTestIstanbulMidnight is 2026-09-01T00:00:00 Europe/Istanbul in UTC
// (Istanbul has used a fixed UTC+3 offset, no DST, since 2016 — 02
// §1 — so this literal is exact and never needs the tzdata package to
// recompute).
var ingestTestIstanbulMidnight = time.Date(2026, 8, 31, 21, 0, 0, 0, time.UTC)

// ingestTestNow is comfortably after every fixed reading timestamp these
// tests use, so nothing trips RejectFuture by accident.
var ingestTestNow = time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)

// ingestTestRepos bundles one pool's worth of real postgres repositories —
// every dependency ingest.Deps needs beyond the fakes.
type ingestTestRepos struct {
	analyzers      *postgres.AnalyzerRepository
	readings       *postgres.ReadingRepository
	cursors        *postgres.CursorRepository
	anomalies      *postgres.AnomalyRepository
	ops            *postgres.OpsRepository
	series         *postgres.ProviderSeriesRepository
	adminIngestion *admin.IngestionRepository
	adminJournal   *admin.JournalRepository
}

func ingestTestNewRepos(pool *pgxpool.Pool) ingestTestRepos {
	return ingestTestRepos{
		analyzers:      postgres.NewAnalyzerRepository(pool),
		readings:       postgres.NewReadingRepository(pool),
		cursors:        postgres.NewCursorRepository(pool),
		anomalies:      postgres.NewAnomalyRepository(pool),
		ops:            postgres.NewOpsRepository(pool),
		series:         postgres.NewProviderSeriesRepository(pool),
		adminIngestion: admin.NewIngestionRepository(pool),
		adminJournal:   admin.NewJournalRepository(pool),
	}
}

// ingestTestNewService wires a Service against real repos, one fake
// adapter registered under creds.Provider, a fixed credential and the
// given clock/enqueuer/options.
func ingestTestNewService(t *testing.T, repos ingestTestRepos, creds integration.Credentials, src integration.Adapter, enq ingest.Enqueuer, clk clock.Clock, opts ingest.Options) *ingest.Service {
	t.Helper()
	deps := ingest.Deps{
		Analyzers: repos.analyzers, Readings: repos.readings, Cursors: repos.cursors,
		Anomalies: repos.anomalies, Ops: repos.ops, ProviderSeries: repos.series,
		AdminIngestion: repos.adminIngestion, AdminJournal: repos.adminJournal,
		Credentials: fixedCredentialOpener(creds), Sources: sourceMap{creds.Provider: src},
		Enqueuer: enq, Clock: clk, Log: testfixtures.DiscardLogger(),
	}
	svc, err := ingest.New(deps, opts)
	require.NoError(t, err)
	return svc
}

// ingestTestFailingAnalyzerRepo wraps a real store.AnalyzerRepository and
// forces Create to fail for one specific installation number — the "one
// analyzer failing does not abort the run" fixture (brief's option (b)).
type ingestTestFailingAnalyzerRepo struct {
	store.AnalyzerRepository
	failInstallation string
}

func (r *ingestTestFailingAnalyzerRepo) Create(ctx context.Context, s store.Scope, a model.Analyzer) (model.Analyzer, error) {
	if a.InstallationNumber == r.failInstallation {
		return model.Analyzer{}, fmt.Errorf("ingest_test: forced create failure for %s", a.InstallationNumber)
	}
	return r.AnalyzerRepository.Create(ctx, s, a)
}

// ingestTestFailingHook is an ingest.PostPersistHook that always fails.
type ingestTestFailingHook struct{}

func (ingestTestFailingHook) AfterPersist(context.Context, store.Scope, model.Analyzer, model.ReadingKind, time.Time, time.Time) error {
	return errors.New("ingest_test: hook boom")
}

// ingestTestQuarterHourly builds n consecutive load_profile readings 15
// minutes apart, active_import strictly increasing, starting at from.
func ingestTestQuarterHourly(analyzerID uuid.UUID, from time.Time, n int) []model.MeterReading {
	rows := make([]model.MeterReading, n)
	val := decimal.RequireFromString("1000.0000")
	step := decimal.RequireFromString("0.2500")
	for i := range n {
		tsN := from.Add(time.Duration(i) * 15 * time.Minute)
		rows[i] = readingFor(analyzerID, tsN.Format(time.RFC3339), model.ReadingKindLoadProfile, map[string]string{"active_import": val.String()})
		val = val.Add(step)
	}
	return rows
}

// ingestTestAllReadings reads every stored reading for analyzerID/kind over
// a window wide enough to cover every fixed timestamp these tests use.
func ingestTestAllReadings(t *testing.T, ctx context.Context, readings *postgres.ReadingRepository, sc store.Scope, analyzerID uuid.UUID, kind model.ReadingKind) []model.MeterReading {
	t.Helper()
	rows, err := readings.Range(ctx, sc, analyzerID, store.TimeRange{From: ts("2020-01-01T00:00:00Z"), To: ts("2030-01-01T00:00:00Z")}, kind)
	require.NoError(t, err)
	return rows
}

// ingestTestRunsForAnalyzer lists every job run of jobType whose scope
// names analyzerID, newest first (OpsListRuns' own order).
func ingestTestRunsForAnalyzer(t *testing.T, ctx context.Context, ops *postgres.OpsRepository, sc store.Scope, jobType string, analyzerID uuid.UUID) []model.JobRun {
	t.Helper()
	runs, err := ops.ListRuns(ctx, sc, store.JobRunFilter{JobType: &jobType, Page: store.Page{Limit: 200}})
	require.NoError(t, err)
	var out []model.JobRun
	for _, r := range runs {
		var scope struct {
			AnalyzerID uuid.UUID `json:"analyzer_id"`
		}
		if err := json.Unmarshal(r.Scope, &scope); err != nil {
			continue
		}
		if scope.AnalyzerID == analyzerID {
			out = append(out, r)
		}
	}
	return out
}

func ingestTestNewActiveAnalyzer(t *testing.T, ctx context.Context, analyzers *postgres.AnalyzerRepository, sc store.Scope, companyID, buildingID uuid.UUID, provider model.IntegrationProvider, subtype, installation string, multiplier decimal.Decimal, at time.Time) model.Analyzer {
	t.Helper()
	a, err := analyzers.Create(ctx, sc, model.Analyzer{
		CompanyID: companyID, BuildingID: &buildingID, Provider: provider, ProviderSubtype: subtype,
		InstallationNumber: installation, MeterMultiplier: multiplier, IsActive: true, CreatedAt: at, UpdatedAt: at,
	})
	require.NoError(t, err)
	return a
}

func ingestTestWindow(from time.Time, dur time.Duration) *job.Window {
	return &job.Window{From: from, To: from.Add(dur)}
}

// ingestTestSyncRunsForCredential lists every integration.sync_analyzers job
// run whose scope names credentialID — the sync-side counterpart of
// ingestTestRunsForAnalyzer, used instead of taking ListRuns' newest-first
// ordering on faith: job_runs.id is a UUID (StartRun mints it with
// uuid.New()), so "order by started_at desc, id" ties on started_at do NOT
// reliably tie-break to insertion order the way a bigserial id would.
// Filtering by this run's own credential_id sidesteps that entirely.
func ingestTestSyncRunsForCredential(t *testing.T, ctx context.Context, ops *postgres.OpsRepository, sc store.Scope, credentialID uuid.UUID) []model.JobRun {
	t.Helper()
	jobType := job.TypeIntegrationSyncAnalyzers
	runs, err := ops.ListRuns(ctx, sc, store.JobRunFilter{JobType: &jobType, Page: store.Page{Limit: 200}})
	require.NoError(t, err)
	var out []model.JobRun
	for _, r := range runs {
		var scope struct {
			CredentialID uuid.UUID `json:"credential_id"`
		}
		if err := json.Unmarshal(r.Scope, &scope); err != nil {
			continue
		}
		if scope.CredentialID == credentialID {
			out = append(out, r)
		}
	}
	return out
}

// ingestTestPlatformRun reads job_runs directly — AdminJournalRepository
// exposes no List/Get, since the dispatcher that writes through it has
// nothing to read back either (Global Constraints: the admin surface is
// closed to exactly what its caller needs) — for the one Dispatch test that
// needs to see the platform run it produced.
func ingestTestPlatformRun(t *testing.T, ctx context.Context, pool *pgxpool.Pool, jobType string) (status string, processed, skipped, failed int32, companyIDIsNull bool) {
	t.Helper()
	row := pool.QueryRow(ctx, `select status, processed, skipped, failed, company_id is null
		from job_runs where job_type = $1 order by started_at desc limit 1`, jobType)
	require.NoError(t, row.Scan(&status, &processed, &skipped, &failed, &companyIDIsNull))
	return status, processed, skipped, failed, companyIDIsNull
}

// ---------------------------------------------------------------------------

// TestIngestionSameWindowTwiceProducesNoDuplicateRows is the F2 acceptance
// criterion "fetching the same window twice produces no duplicate rows".
func TestIngestionSameWindowTwiceProducesNoDuplicateRows(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tn := testfixtures.NewTenant(t, ctx, pool, 9101)
	repos := ingestTestNewRepos(pool)

	analyzer := tn.Analyzers[0] // osos, building0
	creds := integration.Credentials{CredentialID: uuid.New(), CompanyID: tn.Company.ID, Provider: integration.ProviderOSOS, Subtype: analyzer.ProviderSubtype}
	src := newFakeAdapter(integration.ProviderOSOS, 24*time.Hour, model.ReadingKindLoadProfile)
	clk := clock.NewFake(ingestTestNow)
	enq := newRecordingEnqueuer()
	svc := ingestTestNewService(t, repos, creds, src, enq, clk, ingest.Options{})

	window := ingestTestWindow(ingestTestIstanbulMidnight, 24*time.Hour)
	payload := job.FetchReadingsPayload{CompanyID: tn.Company.ID, CredentialID: creds.CredentialID, AnalyzerID: analyzer.ID, Kind: model.ReadingKindLoadProfile, Window: window}
	rows := ingestTestQuarterHourly(analyzer.ID, ingestTestIstanbulMidnight, 96)

	src.setSteps(fetchStep{result: integration.FetchResult{Readings: rows}})
	require.NoError(t, svc.FetchReadings(ctx, payload))
	require.Len(t, ingestTestAllReadings(t, ctx, repos.readings, tn.Scope, analyzer.ID, model.ReadingKindLoadProfile), 96)

	run1 := ingestTestRunsForAnalyzer(t, ctx, repos.ops, tn.Scope, job.TypeIntegrationFetchReadings, analyzer.ID)
	require.Len(t, run1, 1)
	require.EqualValues(t, 96, run1[0].Processed, "run 1: 96 inserted")

	clk.Advance(time.Minute)
	src.setSteps(fetchStep{result: integration.FetchResult{Readings: rows}})
	require.NoError(t, svc.FetchReadings(ctx, payload))
	require.Len(t, ingestTestAllReadings(t, ctx, repos.readings, tn.Scope, analyzer.ID, model.ReadingKindLoadProfile), 96,
		"re-ingesting the same window must upsert, never duplicate")

	run2 := ingestTestRunsForAnalyzer(t, ctx, repos.ops, tn.Scope, job.TypeIntegrationFetchReadings, analyzer.ID)
	require.Len(t, run2, 2)
	require.EqualValues(t, 96, run2[0].Processed, "run 2: 96 updated")

	anomalies, err := repos.anomalies.List(ctx, tn.Scope, store.AnomalyFilter{AnalyzerIDs: []uuid.UUID{analyzer.ID}})
	require.NoError(t, err)
	require.Empty(t, anomalies, "anomaly count is unchanged between runs")
}

// TestIngestionInterruptedFetchResumesFromCursor is the F2 acceptance
// criterion "interrupting a fetch and re-running resumes from the cursor and
// loses nothing".
func TestIngestionInterruptedFetchResumesFromCursor(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tn := testfixtures.NewTenant(t, ctx, pool, 9102)
	repos := ingestTestNewRepos(pool)

	analyzer := tn.Analyzers[0]
	creds := integration.Credentials{CredentialID: uuid.New(), CompanyID: tn.Company.ID, Provider: integration.ProviderOSOS, Subtype: analyzer.ProviderSubtype}
	// A wide MaxWindow so the 30-day first-run lookback collapses into ONE
	// chunk: the 00:00-05:45 / 06:00-23:45 split below is adapter-level
	// PAGINATION within that one chunk, not a chunk boundary.
	src := newFakeAdapter(integration.ProviderOSOS, 40*24*time.Hour, model.ReadingKindLoadProfile)
	clk := clock.NewFake(ingestTestIstanbulMidnight.Add(ingest.DefaultInitialLookback)) // from == ingestTestIstanbulMidnight exactly
	enq := newRecordingEnqueuer()
	opts := ingest.Options{} // InitialLookback defaults to 720h == ingest.DefaultInitialLookback
	svc := ingestTestNewService(t, repos, creds, src, enq, clk, opts)

	payload := job.FetchReadingsPayload{CompanyID: tn.Company.ID, CredentialID: creds.CredentialID, AnalyzerID: analyzer.ID, Kind: model.ReadingKindLoadProfile}

	page1End := ingestTestIstanbulMidnight.Add(5*time.Hour + 45*time.Minute)
	page1 := ingestTestQuarterHourly(analyzer.ID, ingestTestIstanbulMidnight, 24) // 00:00..05:45 inclusive
	src.setSteps(
		fetchStep{result: integration.FetchResult{Readings: page1, NextCursor: &page1End}},
		fetchStep{err: &integration.Error{Kind: integration.ErrUpstreamUnavailable, Provider: integration.ProviderOSOS, Op: "load_profiles"}},
	)

	err := svc.FetchReadings(ctx, payload)
	require.Error(t, err, "run 1 must return the page-2 failure")

	run1 := ingestTestRunsForAnalyzer(t, ctx, repos.ops, tn.Scope, job.TypeIntegrationFetchReadings, analyzer.ID)
	require.Len(t, run1, 1)
	require.Equal(t, "partial", run1[0].Status)

	cur1, err := repos.cursors.Get(ctx, tn.Scope, analyzer.ID, model.ReadingKindLoadProfile)
	require.NoError(t, err)
	require.NotNil(t, cur1.LastTs)
	require.True(t, cur1.LastTs.Equal(page1End), "the cursor never advances past data actually persisted (R34)")
	require.EqualValues(t, 1, cur1.ConsecutiveFailures)
	require.NotNil(t, cur1.LastError)

	// Swap the script: page 2 now succeeds, covering 06:00-23:45.
	page2Start := ingestTestIstanbulMidnight.Add(6 * time.Hour)
	page2 := ingestTestQuarterHourly(analyzer.ID, page2Start, 72) // 06:00..23:45 inclusive
	src.setSteps(fetchStep{result: integration.FetchResult{Readings: page2}})

	clk.Advance(time.Minute)
	require.NoError(t, svc.FetchReadings(ctx, payload))

	// The FIRST request of run 2 must resume exactly from the stored cursor.
	reqs := src.requestLog()
	require.True(t, reqs[len(reqs)-1].From.Equal(page1End), "run 2's first (only) request must resume exactly at the cursor")

	rows := ingestTestAllReadings(t, ctx, repos.readings, tn.Scope, analyzer.ID, model.ReadingKindLoadProfile)
	require.Len(t, rows, 96, "24 (page 1) + 72 (page 2) == 96 distinct rows, none duplicated")

	cur2, err := repos.cursors.Get(ctx, tn.Scope, analyzer.ID, model.ReadingKindLoadProfile)
	require.NoError(t, err)
	require.EqualValues(t, 0, cur2.ConsecutiveFailures)
}

// TestIngestionOneAnalyzerFailureDoesNotAbortTheRun is the F2 acceptance
// criterion "one analyzer failing does not abort the run; the job_runs row
// records processed/skipped/failed counts and the failure reason".
func TestIngestionOneAnalyzerFailureDoesNotAbortTheRun(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tn := testfixtures.NewTenant(t, ctx, pool, 9103)
	repos := ingestTestNewRepos(pool)

	t.Run("part A: sync continues past one point failure", func(t *testing.T) {
		now := ingestTestNow
		// OK-1 and OK-3 already exist and are ACTIVE, so the sync UPDATES
		// them (never touching IsActive — R27 governs creation only) and
		// they are enqueue-eligible; FAIL-2 does not exist yet, so the sync
		// CREATEs it, and that Create is the one made to fail.
		ok1 := ingestTestNewActiveAnalyzer(t, ctx, repos.analyzers, tn.Scope, tn.Company.ID, tn.Buildings[0].ID, model.IntegrationProviderOSOS, "SyncSub", "OK-1", decimal.NewFromInt(1), now)
		ok3 := ingestTestNewActiveAnalyzer(t, ctx, repos.analyzers, tn.Scope, tn.Company.ID, tn.Buildings[0].ID, model.IntegrationProviderOSOS, "SyncSub", "OK-3", decimal.NewFromInt(1), now)

		failing := &ingestTestFailingAnalyzerRepo{AnalyzerRepository: repos.analyzers, failInstallation: "FAIL-2"}
		creds := integration.Credentials{CredentialID: uuid.New(), CompanyID: tn.Company.ID, Provider: integration.ProviderOSOS, Subtype: "SyncSub"}
		src := newFakeAdapter(integration.ProviderOSOS, 24*time.Hour, model.ReadingKindLoadProfile)
		src.points = []integration.MeteringPoint{{InstallationNumber: "OK-1"}, {InstallationNumber: "FAIL-2"}, {InstallationNumber: "OK-3"}}
		clk := clock.NewFake(now)
		enq := newRecordingEnqueuer()

		deps := ingest.Deps{
			Analyzers: failing, Readings: repos.readings, Cursors: repos.cursors,
			Anomalies: repos.anomalies, Ops: repos.ops, ProviderSeries: repos.series,
			AdminIngestion: repos.adminIngestion, AdminJournal: repos.adminJournal,
			Credentials: fixedCredentialOpener(creds), Sources: sourceMap{creds.Provider: src},
			Enqueuer: enq, Clock: clk, Log: testfixtures.DiscardLogger(),
		}
		svc, err := ingest.New(deps, ingest.Options{})
		require.NoError(t, err)

		require.NoError(t, svc.SyncAnalyzers(ctx, job.SyncAnalyzersPayload{CompanyID: tn.Company.ID, CredentialID: creds.CredentialID}))

		runs, err := repos.ops.ListRuns(ctx, tn.Scope, store.JobRunFilter{JobType: ptrString(job.TypeIntegrationSyncAnalyzers), Page: store.Page{Limit: 5}})
		require.NoError(t, err)
		require.Len(t, runs, 1)
		require.Equal(t, "partial", runs[0].Status)
		require.EqualValues(t, 2, runs[0].Processed, "OK-1 and OK-3 were enqueued")
		require.EqualValues(t, 1, runs[0].Failed)
		require.Contains(t, string(runs[0].Detail), "FAIL-2")

		require.Len(t, enq.enqueued(), 2)

		require.NoError(t, ok1Untouched(t, ctx, repos.analyzers, tn.Scope, ok1.ID))
		require.NoError(t, ok1Untouched(t, ctx, repos.analyzers, tn.Scope, ok3.ID))
	})

	t.Run("part B: per-analyzer fetch isolation", func(t *testing.T) {
		now := ingestTestNow
		a := ingestTestNewActiveAnalyzer(t, ctx, repos.analyzers, tn.Scope, tn.Company.ID, tn.Buildings[0].ID, model.IntegrationProviderOSOS, "IsolationSub", "ISO-A", decimal.NewFromInt(1), now)
		b := ingestTestNewActiveAnalyzer(t, ctx, repos.analyzers, tn.Scope, tn.Company.ID, tn.Buildings[0].ID, model.IntegrationProviderOSOS, "IsolationSub", "ISO-B", decimal.NewFromInt(1), now)
		c := ingestTestNewActiveAnalyzer(t, ctx, repos.analyzers, tn.Scope, tn.Company.ID, tn.Buildings[0].ID, model.IntegrationProviderOSOS, "IsolationSub", "ISO-C", decimal.NewFromInt(1), now)

		creds := integration.Credentials{CredentialID: uuid.New(), CompanyID: tn.Company.ID, Provider: integration.ProviderOSOS, Subtype: "IsolationSub"}
		src := newFakeAdapter(integration.ProviderOSOS, 24*time.Hour, model.ReadingKindLoadProfile)
		clk := clock.NewFake(now)
		enq := newRecordingEnqueuer()
		svc := ingestTestNewService(t, repos, creds, src, enq, clk, ingest.Options{})

		window := ingestTestWindow(ingestTestIstanbulMidnight, 24*time.Hour)
		rowsA := ingestTestQuarterHourly(a.ID, ingestTestIstanbulMidnight, 4)
		rowsC := ingestTestQuarterHourly(c.ID, ingestTestIstanbulMidnight, 4)
		src.setStepsFor(a.ID, fetchStep{result: integration.FetchResult{Readings: rowsA}})
		src.setStepsFor(c.ID, fetchStep{result: integration.FetchResult{Readings: rowsC}})
		src.setStepsFor(b.ID, fetchStep{err: &integration.Error{Kind: integration.ErrUpstreamUnavailable, Provider: integration.ProviderOSOS, Op: "load_profiles"}})

		require.NoError(t, svc.FetchReadings(ctx, job.FetchReadingsPayload{CompanyID: tn.Company.ID, CredentialID: creds.CredentialID, AnalyzerID: a.ID, Kind: model.ReadingKindLoadProfile, Window: window}))
		require.Error(t, svc.FetchReadings(ctx, job.FetchReadingsPayload{CompanyID: tn.Company.ID, CredentialID: creds.CredentialID, AnalyzerID: b.ID, Kind: model.ReadingKindLoadProfile, Window: window}))
		require.NoError(t, svc.FetchReadings(ctx, job.FetchReadingsPayload{CompanyID: tn.Company.ID, CredentialID: creds.CredentialID, AnalyzerID: c.ID, Kind: model.ReadingKindLoadProfile, Window: window}))

		require.Len(t, ingestTestAllReadings(t, ctx, repos.readings, tn.Scope, a.ID, model.ReadingKindLoadProfile), 4)
		require.Len(t, ingestTestAllReadings(t, ctx, repos.readings, tn.Scope, c.ID, model.ReadingKindLoadProfile), 4)
		require.Empty(t, ingestTestAllReadings(t, ctx, repos.readings, tn.Scope, b.ID, model.ReadingKindLoadProfile))

		runA := ingestTestRunsForAnalyzer(t, ctx, repos.ops, tn.Scope, job.TypeIntegrationFetchReadings, a.ID)
		runB := ingestTestRunsForAnalyzer(t, ctx, repos.ops, tn.Scope, job.TypeIntegrationFetchReadings, b.ID)
		runC := ingestTestRunsForAnalyzer(t, ctx, repos.ops, tn.Scope, job.TypeIntegrationFetchReadings, c.ID)
		require.Len(t, runA, 1)
		require.Equal(t, "success", runA[0].Status)
		require.Len(t, runC, 1)
		require.Equal(t, "success", runC[0].Status)
		require.Len(t, runB, 1)
		require.Equal(t, "failed", runB[0].Status)
		require.EqualValues(t, 1, runB[0].Failed)
		require.NotNil(t, runB[0].Error)

		curA, err := repos.cursors.Get(ctx, tn.Scope, a.ID, model.ReadingKindLoadProfile)
		require.NoError(t, err)
		require.NotNil(t, curA.LastTs)
		curC, err := repos.cursors.Get(ctx, tn.Scope, c.ID, model.ReadingKindLoadProfile)
		require.NoError(t, err)
		require.NotNil(t, curC.LastTs)
		curB, err := repos.cursors.Get(ctx, tn.Scope, b.ID, model.ReadingKindLoadProfile)
		require.NoError(t, err)
		require.EqualValues(t, 1, curB.ConsecutiveFailures)
	})
}

// ok1Untouched is a small sanity read used by part A above to confirm the
// pre-seeded active analyzers are still there (and still visible) after the
// sync run touched them.
func ok1Untouched(t *testing.T, ctx context.Context, analyzers *postgres.AnalyzerRepository, sc store.Scope, id uuid.UUID) error {
	t.Helper()
	a, err := analyzers.Get(ctx, sc, id)
	if err != nil {
		return err
	}
	require.True(t, a.IsActive)
	return nil
}

// TestIngestionNegativeIndexDeltaCreatesUnresolvedAnomaly is the F2
// acceptance criterion "a negative index delta creates an unresolved
// consumption_anomalies row".
func TestIngestionNegativeIndexDeltaCreatesUnresolvedAnomaly(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tn := testfixtures.NewTenant(t, ctx, pool, 9104)
	repos := ingestTestNewRepos(pool)

	analyzer := tn.Analyzers[0]
	creds := integration.Credentials{CredentialID: uuid.New(), CompanyID: tn.Company.ID, Provider: integration.ProviderOSOS, Subtype: analyzer.ProviderSubtype}
	src := newFakeAdapter(integration.ProviderOSOS, 24*time.Hour, model.ReadingKindLoadProfile)
	clk := clock.NewFake(ingestTestNow)
	enq := newRecordingEnqueuer()
	svc := ingestTestNewService(t, repos, creds, src, enq, clk, ingest.Options{})

	window := ingestTestWindow(ingestTestIstanbulMidnight, 2*time.Hour)
	payload := job.FetchReadingsPayload{CompanyID: tn.Company.ID, CredentialID: creds.CredentialID, AnalyzerID: analyzer.ID, Kind: model.ReadingKindLoadProfile, Window: window}
	rows := []model.MeterReading{
		readingFor(analyzer.ID, ingestTestIstanbulMidnight.Format(time.RFC3339), model.ReadingKindLoadProfile, map[string]string{"active_import": "1000.0000"}),
		readingFor(analyzer.ID, ingestTestIstanbulMidnight.Add(time.Hour).Format(time.RFC3339), model.ReadingKindLoadProfile, map[string]string{"active_import": "990.0000"}),
	}

	src.setSteps(fetchStep{result: integration.FetchResult{Readings: rows}})
	require.NoError(t, svc.FetchReadings(ctx, payload))
	require.Len(t, ingestTestAllReadings(t, ctx, repos.readings, tn.Scope, analyzer.ID, model.ReadingKindLoadProfile), 2, "the reading is stored even though it produced a negative delta")

	unresolved, err := repos.anomalies.List(ctx, tn.Scope, store.AnomalyFilter{AnalyzerIDs: []uuid.UUID{analyzer.ID}, Unresolved: true})
	require.NoError(t, err)
	require.Len(t, unresolved, 1)
	require.Equal(t, "negative_delta", unresolved[0].Reason)
	require.True(t, unresolved[0].PeriodStart.Equal(ingestTestIstanbulMidnight))
	require.True(t, unresolved[0].PeriodEnd.Equal(ingestTestIstanbulMidnight.Add(time.Hour)))
	require.Nil(t, unresolved[0].ResolvedAt)

	clk.Advance(time.Minute)
	src.setSteps(fetchStep{result: integration.FetchResult{Readings: rows}})
	require.NoError(t, svc.FetchReadings(ctx, payload))
	unresolvedAgain, err := repos.anomalies.List(ctx, tn.Scope, store.AnomalyFilter{AnalyzerIDs: []uuid.UUID{analyzer.ID}, Unresolved: true})
	require.NoError(t, err)
	require.Len(t, unresolvedAgain, 1, "re-fetching the same window must not create a second anomaly")

	resolver := tn.Users[model.UserRoleCompanyAdmin].ID
	_, err = repos.anomalies.Resolve(ctx, tn.Scope, unresolved[0].ID, resolver, "accepted", nil, clk.Now())
	require.NoError(t, err)

	clk.Advance(time.Minute)
	src.setSteps(fetchStep{result: integration.FetchResult{Readings: rows}})
	require.NoError(t, svc.FetchReadings(ctx, payload))

	stillUnresolved, err := repos.anomalies.List(ctx, tn.Scope, store.AnomalyFilter{AnalyzerIDs: []uuid.UUID{analyzer.ID}, Unresolved: true})
	require.NoError(t, err)
	require.Empty(t, stillUnresolved, "a resolved anomaly must never be resurrected")

	all, err := repos.anomalies.List(ctx, tn.Scope, store.AnomalyFilter{AnalyzerIDs: []uuid.UUID{analyzer.ID}})
	require.NoError(t, err)
	require.Len(t, all, 1, "still exactly one anomaly, now resolved")
}

// TestIngestionRejectionsAreCountedAndSurfaced: 2 future and 1
// negative-value reading -> skipped == 3, one warning message with
// metadata counts.
func TestIngestionRejectionsAreCountedAndSurfaced(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tn := testfixtures.NewTenant(t, ctx, pool, 9105)
	repos := ingestTestNewRepos(pool)

	analyzer := tn.Analyzers[0]
	creds := integration.Credentials{CredentialID: uuid.New(), CompanyID: tn.Company.ID, Provider: integration.ProviderOSOS, Subtype: analyzer.ProviderSubtype}
	src := newFakeAdapter(integration.ProviderOSOS, 24*time.Hour, model.ReadingKindLoadProfile)
	now := ingestTestNow
	clk := clock.NewFake(now)
	enq := newRecordingEnqueuer()
	svc := ingestTestNewService(t, repos, creds, src, enq, clk, ingest.Options{})

	window := ingestTestWindow(now.Add(-2*time.Hour), 4*time.Hour)
	payload := job.FetchReadingsPayload{CompanyID: tn.Company.ID, CredentialID: creds.CredentialID, AnalyzerID: analyzer.ID, Kind: model.ReadingKindLoadProfile, Window: window}

	rows := []model.MeterReading{
		readingFor(analyzer.ID, now.Add(20*time.Minute).Format(time.RFC3339), model.ReadingKindLoadProfile, map[string]string{"active_import": "5.0000"}),
		readingFor(analyzer.ID, now.Add(30*time.Minute).Format(time.RFC3339), model.ReadingKindLoadProfile, map[string]string{"active_import": "6.0000"}),
		readingFor(analyzer.ID, now.Add(-time.Hour).Format(time.RFC3339), model.ReadingKindLoadProfile, map[string]string{"active_import": "-1.0000"}),
	}
	src.setSteps(fetchStep{result: integration.FetchResult{Readings: rows}})
	require.NoError(t, svc.FetchReadings(ctx, payload))

	run := ingestTestRunsForAnalyzer(t, ctx, repos.ops, tn.Scope, job.TypeIntegrationFetchReadings, analyzer.ID)
	require.Len(t, run, 1)
	require.EqualValues(t, 3, run[0].Skipped)
	require.Equal(t, "success", run[0].Status)

	var detail struct {
		RejectionsByReason map[string]int32 `json:"rejections_by_reason"`
	}
	require.NoError(t, json.Unmarshal(run[0].Detail, &detail))
	require.EqualValues(t, 2, detail.RejectionsByReason[string(ingest.RejectFuture)])
	require.EqualValues(t, 1, detail.RejectionsByReason[string(ingest.RejectNegative)])

	messages, err := repos.ops.ListMessages(ctx, tn.Scope, store.MessageFilter{})
	require.NoError(t, err)
	var found bool
	for _, m := range messages {
		if m.Message == "3 readings rejected" {
			found = true
			require.Equal(t, "warning", m.Status)
			require.Equal(t, "job", m.Kind)
		}
	}
	require.True(t, found, "expected a '3 readings rejected' warning message")
}

// TestIngestionEmptyWindowDoesNotAdvanceCursor (R19).
func TestIngestionEmptyWindowDoesNotAdvanceCursor(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tn := testfixtures.NewTenant(t, ctx, pool, 9106)
	repos := ingestTestNewRepos(pool)

	analyzer := tn.Analyzers[0]
	creds := integration.Credentials{CredentialID: uuid.New(), CompanyID: tn.Company.ID, Provider: integration.ProviderOSOS, Subtype: analyzer.ProviderSubtype}
	src := newFakeAdapter(integration.ProviderOSOS, 24*time.Hour, model.ReadingKindLoadProfile) // no steps scripted: returns an empty page
	clk := clock.NewFake(ingestTestNow)
	enq := newRecordingEnqueuer()
	svc := ingestTestNewService(t, repos, creds, src, enq, clk, ingest.Options{})

	_, err := repos.cursors.Get(ctx, tn.Scope, analyzer.ID, model.ReadingKindLoadProfile)
	require.ErrorIs(t, err, store.ErrNotFound)

	window := ingestTestWindow(ingestTestIstanbulMidnight, 2*time.Hour)
	require.NoError(t, svc.FetchReadings(ctx, job.FetchReadingsPayload{CompanyID: tn.Company.ID, CredentialID: creds.CredentialID, AnalyzerID: analyzer.ID, Kind: model.ReadingKindLoadProfile, Window: window}))

	_, err = repos.cursors.Get(ctx, tn.Scope, analyzer.ID, model.ReadingKindLoadProfile)
	require.ErrorIs(t, err, store.ErrNotFound, "R19: an empty window never advances (or creates) the cursor")

	run := ingestTestRunsForAnalyzer(t, ctx, repos.ops, tn.Scope, job.TypeIntegrationFetchReadings, analyzer.ID)
	require.Len(t, run, 1)
	require.Equal(t, "success", run[0].Status)
	require.EqualValues(t, 0, run[0].Processed)
}

// TestIngestionFailureMessageContainsNoSecret: the adapter error's text
// includes the password fragment; job_runs.error, ingestion_cursors.last_error
// and every operational message for the company contain no fragment.
func TestIngestionFailureMessageContainsNoSecret(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tn := testfixtures.NewTenant(t, ctx, pool, 9107)
	repos := ingestTestNewRepos(pool)

	analyzer := tn.Analyzers[0]
	const secretValue = "sUp3rSecretPassw0rd!"
	creds := integration.Credentials{
		CredentialID: uuid.New(), CompanyID: tn.Company.ID, Provider: integration.ProviderOSOS, Subtype: analyzer.ProviderSubtype,
		Secret: integration.NewSecret([]byte(secretValue)),
	}
	src := newFakeAdapter(integration.ProviderOSOS, 24*time.Hour, model.ReadingKindLoadProfile)
	clk := clock.NewFake(ingestTestNow)
	enq := newRecordingEnqueuer()
	svc := ingestTestNewService(t, repos, creds, src, enq, clk, ingest.Options{})

	// The underlying cause wraps integration.ErrAuth (%w), the way a real
	// adapter would classify an upstream rejection — I3's proof needs a
	// cause that both leaks the secret in its text AND stays reachable via
	// errors.Is once wrapped for redaction.
	src.setSteps(fetchStep{err: fmt.Errorf("upstream rejected credentials for password %s: %w", secretValue, integration.ErrAuth)})

	window := ingestTestWindow(ingestTestIstanbulMidnight, 2*time.Hour)
	err := svc.FetchReadings(ctx, job.FetchReadingsPayload{CompanyID: tn.Company.ID, CredentialID: creds.CredentialID, AnalyzerID: analyzer.ID, Kind: model.ReadingKindLoadProfile, Window: window})
	require.Error(t, err)

	// I3: the error returned to the JOB LAYER (not just the stored
	// records checked below) must also be redacted, yet still classify —
	// job.ClassifyForRetry needs errors.Is to keep working through it.
	require.NotContains(t, err.Error(), secretValue, "I3: the error returned to the job layer must be redacted")
	require.ErrorIs(t, err, integration.ErrAuth, "I3: the redacted wrapper must still unwrap to the classified cause")

	run := ingestTestRunsForAnalyzer(t, ctx, repos.ops, tn.Scope, job.TypeIntegrationFetchReadings, analyzer.ID)
	require.Len(t, run, 1)
	require.NotNil(t, run[0].Error)
	require.NotContains(t, *run[0].Error, secretValue)

	cur, cerr := repos.cursors.Get(ctx, tn.Scope, analyzer.ID, model.ReadingKindLoadProfile)
	require.NoError(t, cerr)
	require.NotNil(t, cur.LastError)
	require.NotContains(t, *cur.LastError, secretValue)

	messages, merr := repos.ops.ListMessages(ctx, tn.Scope, store.MessageFilter{})
	require.NoError(t, merr)
	require.NotEmpty(t, messages)
	for _, m := range messages {
		require.NotContains(t, m.Message, secretValue)
		if m.Detail != nil {
			require.NotContains(t, *m.Detail, secretValue)
		}
	}
}

// TestIngestionUsesSystemScopeAndCannotTouchAnotherTenant: a payload with
// company A and tenant B's analyzer id -> ErrNotFound, no rows written to
// either.
func TestIngestionUsesSystemScopeAndCannotTouchAnotherTenant(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tnA := testfixtures.NewTenant(t, ctx, pool, 9108)
	tnB := testfixtures.NewTenant(t, ctx, pool, 9109)
	repos := ingestTestNewRepos(pool)

	creds := integration.Credentials{CredentialID: uuid.New(), CompanyID: tnA.Company.ID, Provider: integration.ProviderOSOS, Subtype: tnA.Analyzers[0].ProviderSubtype}
	src := newFakeAdapter(integration.ProviderOSOS, 24*time.Hour, model.ReadingKindLoadProfile)
	clk := clock.NewFake(ingestTestNow)
	enq := newRecordingEnqueuer()
	svc := ingestTestNewService(t, repos, creds, src, enq, clk, ingest.Options{})

	targetAnalyzer := tnB.Analyzers[0] // provider osos, belongs to tenant B only
	window := ingestTestWindow(ingestTestIstanbulMidnight, 2*time.Hour)
	err := svc.FetchReadings(ctx, job.FetchReadingsPayload{
		CompanyID: tnA.Company.ID, CredentialID: creds.CredentialID, AnalyzerID: targetAnalyzer.ID,
		Kind: model.ReadingKindLoadProfile, Window: window,
	})
	require.ErrorIs(t, err, store.ErrNotFound)

	require.Empty(t, ingestTestAllReadings(t, ctx, repos.readings, tnB.AdminScope, targetAnalyzer.ID, model.ReadingKindLoadProfile),
		"tenant B's analyzer must be untouched by a company-A-scoped job")
	require.Empty(t, src.requestLog(), "the adapter must never even be called for an analyzer outside the scope")
}

// TestIngestionMultiplierChangeIsAppliedAndReported.
func TestIngestionMultiplierChangeIsAppliedAndReported(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tn := testfixtures.NewTenant(t, ctx, pool, 9110)
	repos := ingestTestNewRepos(pool)

	analyzer := tn.Analyzers[0] // MeterMultiplier fixture value: 40.000000
	creds := integration.Credentials{CredentialID: uuid.New(), CompanyID: tn.Company.ID, Provider: integration.ProviderOSOS, Subtype: analyzer.ProviderSubtype}
	src := newFakeAdapter(integration.ProviderOSOS, 24*time.Hour, model.ReadingKindLoadProfile)
	clk := clock.NewFake(ingestTestNow)
	enq := newRecordingEnqueuer()
	svc := ingestTestNewService(t, repos, creds, src, enq, clk, ingest.Options{})

	newMultiplier := decimal.RequireFromString("45.000000")
	rows := []model.MeterReading{
		readingFor(analyzer.ID, ingestTestIstanbulMidnight.Format(time.RFC3339), model.ReadingKindLoadProfile, map[string]string{"active_import": "10.0000"}),
	}
	window := ingestTestWindow(ingestTestIstanbulMidnight, 2*time.Hour)
	src.setSteps(fetchStep{result: integration.FetchResult{
		Readings:           rows,
		ResolvedMultiplier: &integration.ResolvedMultiplier{Value: newMultiplier, Source: integration.MultiplierFromLastEndex},
	}})

	require.NoError(t, svc.FetchReadings(ctx, job.FetchReadingsPayload{CompanyID: tn.Company.ID, CredentialID: creds.CredentialID, AnalyzerID: analyzer.ID, Kind: model.ReadingKindLoadProfile, Window: window}))

	updated, err := repos.analyzers.Get(ctx, tn.Scope, analyzer.ID)
	require.NoError(t, err)
	require.True(t, updated.MeterMultiplier.Equal(newMultiplier))

	messages, err := repos.ops.ListMessages(ctx, tn.Scope, store.MessageFilter{})
	require.NoError(t, err)
	var found bool
	for _, m := range messages {
		if m.Message == "meter multiplier changed" {
			found = true
			require.Equal(t, "warning", m.Status)
		}
	}
	require.True(t, found)
}

// TestDispatchEnqueuesOneSyncPerActiveMeterCredential (isolar credential
// skipped; platform run row with company_id NULL).
func TestDispatchEnqueuesOneSyncPerActiveMeterCredential(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tn := testfixtures.NewTenant(t, ctx, pool, 9111)
	repos := ingestTestNewRepos(pool)

	ososDefID := uuid.New()
	isolarDefID := uuid.New()
	ososCredID := uuid.New()
	isolarCredID := uuid.New()
	_, err := pool.Exec(ctx, `insert into integration_definitions (id, provider, subtype) values ($1,'osos','Dispatch'), ($2,'isolar','Dispatch')`, ososDefID, isolarDefID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `insert into integration_credentials (id, company_id, definition_id, is_active) values ($1,$2,$3,true), ($4,$2,$5,true)`,
		ososCredID, tn.Company.ID, ososDefID, isolarCredID, isolarDefID)
	require.NoError(t, err)

	enq := newRecordingEnqueuer()
	clk := clock.NewFake(ingestTestNow)
	deps := ingest.Deps{
		Analyzers: repos.analyzers, Readings: repos.readings, Cursors: repos.cursors,
		Anomalies: repos.anomalies, Ops: repos.ops, ProviderSeries: repos.series,
		AdminIngestion: repos.adminIngestion, AdminJournal: repos.adminJournal,
		Credentials: fixedCredentialOpener(integration.Credentials{}), Sources: sourceMap{},
		Enqueuer: enq, Clock: clk, Log: testfixtures.DiscardLogger(),
	}
	svc, err := ingest.New(deps, ingest.Options{})
	require.NoError(t, err)

	require.NoError(t, svc.Dispatch(ctx))

	tasks := enq.enqueued()
	require.Len(t, tasks, 1, "the isolar credential must be skipped — it is not a meter provider")
	require.Equal(t, job.TypeIntegrationSyncAnalyzers, tasks[0].Type())
	p, err := job.DecodeSyncAnalyzers(tasks[0])
	require.NoError(t, err)
	require.Equal(t, ososCredID, p.CredentialID)

	status, processed, skipped, failed, companyNull := ingestTestPlatformRun(t, ctx, pool, job.TypeIntegrationSyncDispatch)
	require.Equal(t, "success", status)
	require.EqualValues(t, 1, processed)
	require.EqualValues(t, 0, skipped)
	require.EqualValues(t, 0, failed)
	require.True(t, companyNull, "a platform run's company_id is NULL")
}

// TestIngestionHooksRunBeforeCursorAdvance: a hook that fails leaves the
// cursor untouched, and the run returns the error.
func TestIngestionHooksRunBeforeCursorAdvance(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tn := testfixtures.NewTenant(t, ctx, pool, 9112)
	repos := ingestTestNewRepos(pool)

	analyzer := tn.Analyzers[0]
	creds := integration.Credentials{CredentialID: uuid.New(), CompanyID: tn.Company.ID, Provider: integration.ProviderOSOS, Subtype: analyzer.ProviderSubtype}
	src := newFakeAdapter(integration.ProviderOSOS, 24*time.Hour, model.ReadingKindLoadProfile)
	clk := clock.NewFake(ingestTestNow)
	enq := newRecordingEnqueuer()

	deps := ingest.Deps{
		Analyzers: repos.analyzers, Readings: repos.readings, Cursors: repos.cursors,
		Anomalies: repos.anomalies, Ops: repos.ops, ProviderSeries: repos.series,
		AdminIngestion: repos.adminIngestion, AdminJournal: repos.adminJournal,
		Credentials: fixedCredentialOpener(creds), Sources: sourceMap{creds.Provider: src},
		Enqueuer: enq, Clock: clk, Log: testfixtures.DiscardLogger(),
		Hooks: map[model.IntegrationProvider][]ingest.PostPersistHook{model.IntegrationProviderOSOS: {ingestTestFailingHook{}}},
	}
	svc, err := ingest.New(deps, ingest.Options{})
	require.NoError(t, err)

	rows := ingestTestQuarterHourly(analyzer.ID, ingestTestIstanbulMidnight, 4)
	window := ingestTestWindow(ingestTestIstanbulMidnight, 2*time.Hour)
	src.setSteps(fetchStep{result: integration.FetchResult{Readings: rows}})

	err = svc.FetchReadings(ctx, job.FetchReadingsPayload{CompanyID: tn.Company.ID, CredentialID: creds.CredentialID, AnalyzerID: analyzer.ID, Kind: model.ReadingKindLoadProfile, Window: window})
	require.Error(t, err)
	require.Contains(t, err.Error(), "hook boom")

	cur, cerr := repos.cursors.Get(ctx, tn.Scope, analyzer.ID, model.ReadingKindLoadProfile)
	require.NoError(t, cerr, "RecordFailure still runs (the ordinary failure protocol), but RecordSuccess must not")
	require.Nil(t, cur.LastTs, "the cursor's high-water mark is untouched by a hook failure")

	// The rows themselves WERE persisted before the hook ran — only the
	// cursor is held back.
	require.Len(t, ingestTestAllReadings(t, ctx, repos.readings, tn.Scope, analyzer.ID, model.ReadingKindLoadProfile), 4)
}

// TestIngestionExplicitWindowNeverAdvancesCursorPastAGap is I1: an explicit
// p.Window fetch may move the live cursor forward only when it is
// contiguous with what the cursor already covers. A window that starts
// AFTER the cursor — a gap the cursor has not covered yet — must leave the
// cursor exactly where it was, so the next cursor-driven run still asks for
// the gap instead of skipping straight past it.
func TestIngestionExplicitWindowNeverAdvancesCursorPastAGap(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tn := testfixtures.NewTenant(t, ctx, pool, 9113)
	repos := ingestTestNewRepos(pool)

	analyzer := tn.Analyzers[0]
	creds := integration.Credentials{CredentialID: uuid.New(), CompanyID: tn.Company.ID, Provider: integration.ProviderOSOS, Subtype: analyzer.ProviderSubtype}
	src := newFakeAdapter(integration.ProviderOSOS, 24*time.Hour, model.ReadingKindLoadProfile)
	// The clock must sit comfortably AFTER day 6 (below), or day 6's own
	// readings would trip RejectFuture (R12) instead of exercising I1.
	clk := clock.NewFake(ingestTestIstanbulMidnight.Add(7 * 24 * time.Hour))
	enq := newRecordingEnqueuer()
	svc := ingestTestNewService(t, repos, creds, src, enq, clk, ingest.Options{})

	// Day 1: an explicit window with no stored cursor yet — always allowed
	// to advance (resolveFetchWindow's "no stored cursor" case).
	day1 := ingestTestIstanbulMidnight
	day1Rows := ingestTestQuarterHourly(analyzer.ID, day1, 4) // 00:00..00:45
	src.setSteps(fetchStep{result: integration.FetchResult{Readings: day1Rows}})
	require.NoError(t, svc.FetchReadings(ctx, job.FetchReadingsPayload{
		CompanyID: tn.Company.ID, CredentialID: creds.CredentialID, AnalyzerID: analyzer.ID,
		Kind: model.ReadingKindLoadProfile, Window: ingestTestWindow(day1, time.Hour),
	}))
	cur1, err := repos.cursors.Get(ctx, tn.Scope, analyzer.ID, model.ReadingKindLoadProfile)
	require.NoError(t, err)
	require.NotNil(t, cur1.LastTs)
	day1LastTs := *cur1.LastTs

	// Day 6: an explicit window that STARTS AFTER the cursor (days 2-5 were
	// never fetched — a gap). I1: the rows are still persisted, but the
	// cursor must NOT move.
	day6 := day1.Add(5 * 24 * time.Hour)
	day6Rows := ingestTestQuarterHourly(analyzer.ID, day6, 4)
	clk.Advance(time.Minute)
	src.setSteps(fetchStep{result: integration.FetchResult{Readings: day6Rows}})
	require.NoError(t, svc.FetchReadings(ctx, job.FetchReadingsPayload{
		CompanyID: tn.Company.ID, CredentialID: creds.CredentialID, AnalyzerID: analyzer.ID,
		Kind: model.ReadingKindLoadProfile, Window: ingestTestWindow(day6, time.Hour),
	}))

	cur2, err := repos.cursors.Get(ctx, tn.Scope, analyzer.ID, model.ReadingKindLoadProfile)
	require.NoError(t, err)
	require.True(t, cur2.LastTs.Equal(day1LastTs), "I1: an explicit window past a gap must never advance the cursor")

	rows := ingestTestAllReadings(t, ctx, repos.readings, tn.Scope, analyzer.ID, model.ReadingKindLoadProfile)
	require.Len(t, rows, 8, "both windows' rows are stored even though only day 1's moved the cursor")

	// A subsequent CURSOR-DRIVEN run (Window nil) must resume exactly from
	// the cursor — i.e. it still asks for the gap (days 2-5), not day 6.
	startIdx := len(src.requestLog())
	clk.Advance(time.Minute)
	src.setSteps(fetchStep{result: integration.FetchResult{}})
	require.NoError(t, svc.FetchReadings(ctx, job.FetchReadingsPayload{
		CompanyID: tn.Company.ID, CredentialID: creds.CredentialID, AnalyzerID: analyzer.ID, Kind: model.ReadingKindLoadProfile,
	}))
	reqs := src.requestLog()
	require.True(t, len(reqs) > startIdx)
	require.True(t, reqs[startIdx].From.Equal(day1LastTs), "the next cursor-driven run must still request the gap, not resume past it")
}

// TestIngestionRowsStampedWithAnotherAnalyzerAreRejected is I2: a page an
// adapter returns for one analyzer's request is trusted to carry that
// analyzer's own rows only after an attribution check — a row stamped with
// a DIFFERENT, real analyzer's ID (same company, same provider) must never
// be persisted under either identity, and must never influence the
// REQUESTED analyzer's cursor.
func TestIngestionRowsStampedWithAnotherAnalyzerAreRejected(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tn := testfixtures.NewTenant(t, ctx, pool, 9114)
	repos := ingestTestNewRepos(pool)

	now := ingestTestNow
	target := ingestTestNewActiveAnalyzer(t, ctx, repos.analyzers, tn.Scope, tn.Company.ID, tn.Buildings[0].ID, model.IntegrationProviderOSOS, "AttribSub", "ATTRIB-TARGET", decimal.NewFromInt(1), now)
	impostor := ingestTestNewActiveAnalyzer(t, ctx, repos.analyzers, tn.Scope, tn.Company.ID, tn.Buildings[0].ID, model.IntegrationProviderOSOS, "AttribSub", "ATTRIB-IMPOSTOR", decimal.NewFromInt(1), now)

	creds := integration.Credentials{CredentialID: uuid.New(), CompanyID: tn.Company.ID, Provider: integration.ProviderOSOS, Subtype: "AttribSub"}
	src := newFakeAdapter(integration.ProviderOSOS, 24*time.Hour, model.ReadingKindLoadProfile)
	clk := clock.NewFake(now)
	enq := newRecordingEnqueuer()
	svc := ingestTestNewService(t, repos, creds, src, enq, clk, ingest.Options{})

	correctRows := ingestTestQuarterHourly(target.ID, ingestTestIstanbulMidnight, 2) // 00:00, 00:15
	// The wrong-analyzer rows carry LATER timestamps than the correct ones:
	// if I2's attribution check were missing, they would wrongly advance
	// target's cursor past what target itself actually received.
	wrongRows := ingestTestQuarterHourly(impostor.ID, ingestTestIstanbulMidnight.Add(2*time.Hour), 2) // 02:00, 02:15

	window := ingestTestWindow(ingestTestIstanbulMidnight, 4*time.Hour)
	mixed := append(append([]model.MeterReading{}, correctRows...), wrongRows...)
	src.setSteps(fetchStep{result: integration.FetchResult{Readings: mixed}})

	require.NoError(t, svc.FetchReadings(ctx, job.FetchReadingsPayload{
		CompanyID: tn.Company.ID, CredentialID: creds.CredentialID, AnalyzerID: target.ID,
		Kind: model.ReadingKindLoadProfile, Window: window,
	}))

	require.Len(t, ingestTestAllReadings(t, ctx, repos.readings, tn.Scope, target.ID, model.ReadingKindLoadProfile), 2,
		"only the correctly-stamped rows are persisted for the fetched analyzer")
	require.Empty(t, ingestTestAllReadings(t, ctx, repos.readings, tn.Scope, impostor.ID, model.ReadingKindLoadProfile),
		"I2: nothing is EVER written for the impostor analyzer — a row stamped with its ID is rejected outright")

	cur, err := repos.cursors.Get(ctx, tn.Scope, target.ID, model.ReadingKindLoadProfile)
	require.NoError(t, err)
	require.NotNil(t, cur.LastTs)
	require.True(t, cur.LastTs.Equal(ingestTestIstanbulMidnight.Add(15*time.Minute)),
		"I2: the cursor reflects only the correctly-attributed rows, not the wrong-analyzer rows' later timestamps")

	run := ingestTestRunsForAnalyzer(t, ctx, repos.ops, tn.Scope, job.TypeIntegrationFetchReadings, target.ID)
	require.Len(t, run, 1)
	var detail struct {
		RejectionsByReason map[string]int32 `json:"rejections_by_reason"`
	}
	require.NoError(t, json.Unmarshal(run[0].Detail, &detail))
	require.EqualValues(t, 2, detail.RejectionsByReason[string(ingest.RejectWrongAnalyzer)])
}

// TestSyncAnalyzersFailureMessageContainsNoSecret is I4: SyncAnalyzers'
// redaction had no test coverage at all — this covers all three of its
// failure surfaces (Verify, DiscoverMeteringPoints, and a per-point
// failed_points reason), matching what
// TestIngestionFailureMessageContainsNoSecret already pins for
// FetchReadings.
func TestSyncAnalyzersFailureMessageContainsNoSecret(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tn := testfixtures.NewTenant(t, ctx, pool, 9115)
	repos := ingestTestNewRepos(pool)
	const secretValue = "sUp3rSyncSecretPassw0rd!"

	t.Run("Verify", func(t *testing.T) {
		creds := integration.Credentials{
			CredentialID: uuid.New(), CompanyID: tn.Company.ID, Provider: integration.ProviderOSOS, Subtype: "SyncVerifySub",
			Secret: integration.NewSecret([]byte(secretValue)),
		}
		src := newFakeAdapter(integration.ProviderOSOS, 24*time.Hour, model.ReadingKindLoadProfile)
		src.verifyErr = fmt.Errorf("verify rejected password %s: %w", secretValue, integration.ErrAuth)
		clk := clock.NewFake(ingestTestNow)
		enq := newRecordingEnqueuer()
		svc := ingestTestNewService(t, repos, creds, src, enq, clk, ingest.Options{})

		err := svc.SyncAnalyzers(ctx, job.SyncAnalyzersPayload{CompanyID: tn.Company.ID, CredentialID: creds.CredentialID})
		require.Error(t, err)
		require.NotContains(t, err.Error(), secretValue, "I3-for-sync: the returned error must be redacted too")
		require.ErrorIs(t, err, integration.ErrAuth)

		runs := ingestTestSyncRunsForCredential(t, ctx, repos.ops, tn.Scope, creds.CredentialID)
		require.Len(t, runs, 1)
		require.NotNil(t, runs[0].Error)
		require.NotContains(t, *runs[0].Error, secretValue)

		messages, merr := repos.ops.ListMessages(ctx, tn.Scope, store.MessageFilter{})
		require.NoError(t, merr)
		for _, m := range messages {
			require.NotContains(t, m.Message, secretValue)
		}
	})

	t.Run("Discover", func(t *testing.T) {
		creds := integration.Credentials{
			CredentialID: uuid.New(), CompanyID: tn.Company.ID, Provider: integration.ProviderOSOS, Subtype: "SyncDiscoverSub",
			Secret: integration.NewSecret([]byte(secretValue)),
		}
		src := newFakeAdapter(integration.ProviderOSOS, 24*time.Hour, model.ReadingKindLoadProfile)
		src.discoverErr = fmt.Errorf("discovery failed, password %s rejected: %w", secretValue, integration.ErrUpstreamUnavailable)
		clk := clock.NewFake(ingestTestNow)
		enq := newRecordingEnqueuer()
		svc := ingestTestNewService(t, repos, creds, src, enq, clk, ingest.Options{})

		err := svc.SyncAnalyzers(ctx, job.SyncAnalyzersPayload{CompanyID: tn.Company.ID, CredentialID: creds.CredentialID})
		require.Error(t, err)
		require.NotContains(t, err.Error(), secretValue)
		require.ErrorIs(t, err, integration.ErrUpstreamUnavailable)

		runs := ingestTestSyncRunsForCredential(t, ctx, repos.ops, tn.Scope, creds.CredentialID)
		require.Len(t, runs, 1)
		require.NotNil(t, runs[0].Error)
		require.NotContains(t, *runs[0].Error, secretValue)
	})

	t.Run("failed_points", func(t *testing.T) {
		creds := integration.Credentials{
			CredentialID: uuid.New(), CompanyID: tn.Company.ID, Provider: integration.ProviderOSOS, Subtype: "SyncPointsSub",
			Secret: integration.NewSecret([]byte(secretValue)),
		}
		src := newFakeAdapter(integration.ProviderOSOS, 24*time.Hour, model.ReadingKindLoadProfile)
		src.points = []integration.MeteringPoint{{InstallationNumber: "LEAK-1"}}
		leaking := &ingestTestSecretLeakingAnalyzerRepo{AnalyzerRepository: repos.analyzers, failInstallation: "LEAK-1", secret: secretValue}
		clk := clock.NewFake(ingestTestNow)
		enq := newRecordingEnqueuer()

		deps := ingest.Deps{
			Analyzers: leaking, Readings: repos.readings, Cursors: repos.cursors,
			Anomalies: repos.anomalies, Ops: repos.ops, ProviderSeries: repos.series,
			AdminIngestion: repos.adminIngestion, AdminJournal: repos.adminJournal,
			Credentials: fixedCredentialOpener(creds), Sources: sourceMap{creds.Provider: src},
			Enqueuer: enq, Clock: clk, Log: testfixtures.DiscardLogger(),
		}
		svc, err := ingest.New(deps, ingest.Options{})
		require.NoError(t, err)

		require.NoError(t, svc.SyncAnalyzers(ctx, job.SyncAnalyzersPayload{CompanyID: tn.Company.ID, CredentialID: creds.CredentialID}))

		runs := ingestTestSyncRunsForCredential(t, ctx, repos.ops, tn.Scope, creds.CredentialID)
		require.Len(t, runs, 1)
		require.Contains(t, string(runs[0].Detail), "LEAK-1")
		require.NotContains(t, string(runs[0].Detail), secretValue, "I4: a failed_points reason must be redacted")
	})
}

// TestSyncMultiplierChangeIsAppliedAndReported is I5/R27: a multiplier
// change SyncAnalyzers discovers (not FetchReadings) must be applied AND
// reported with the same "meter multiplier changed" warning shape the
// fetch path uses (TestIngestionMultiplierChangeIsAppliedAndReported).
func TestSyncMultiplierChangeIsAppliedAndReported(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tn := testfixtures.NewTenant(t, ctx, pool, 9116)
	repos := ingestTestNewRepos(pool)

	now := ingestTestNow
	existing := ingestTestNewActiveAnalyzer(t, ctx, repos.analyzers, tn.Scope, tn.Company.ID, tn.Buildings[0].ID, model.IntegrationProviderOSOS, "SyncMultSub", "MULT-1", decimal.NewFromInt(1), now)

	creds := integration.Credentials{CredentialID: uuid.New(), CompanyID: tn.Company.ID, Provider: integration.ProviderOSOS, Subtype: "SyncMultSub"}
	src := newFakeAdapter(integration.ProviderOSOS, 24*time.Hour, model.ReadingKindLoadProfile)
	newMultiplier := decimal.RequireFromString("60.000000")
	src.points = []integration.MeteringPoint{{InstallationNumber: "MULT-1", MeterMultiplier: &newMultiplier}}
	clk := clock.NewFake(now)
	enq := newRecordingEnqueuer()
	svc := ingestTestNewService(t, repos, creds, src, enq, clk, ingest.Options{})

	require.NoError(t, svc.SyncAnalyzers(ctx, job.SyncAnalyzersPayload{CompanyID: tn.Company.ID, CredentialID: creds.CredentialID}))

	updated, err := repos.analyzers.Get(ctx, tn.Scope, existing.ID)
	require.NoError(t, err)
	require.True(t, updated.MeterMultiplier.Equal(newMultiplier), "I5/R27: sync applies a reported multiplier change")

	messages, merr := repos.ops.ListMessages(ctx, tn.Scope, store.MessageFilter{})
	require.NoError(t, merr)
	var found bool
	for _, m := range messages {
		if m.Message == "meter multiplier changed" {
			found = true
			require.Equal(t, "warning", m.Status)
		}
	}
	require.True(t, found, "I5/R27: sync must report the multiplier change, same as the fetch path")
}

// TestIngestionMultiplierUpdateFailureDoesNotReportOrMutate is M1: a failed
// Analyzers.Update on a resolved multiplier change must not be reported as
// "meter multiplier changed" (it did not happen), must not leave the
// in-memory analyzer mutated for the rest of the run, and must not fail the
// whole run — the page's readings are still persisted.
func TestIngestionMultiplierUpdateFailureDoesNotReportOrMutate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tn := testfixtures.NewTenant(t, ctx, pool, 9117)
	repos := ingestTestNewRepos(pool)

	analyzer := tn.Analyzers[0] // MeterMultiplier fixture value: 40.000000
	creds := integration.Credentials{CredentialID: uuid.New(), CompanyID: tn.Company.ID, Provider: integration.ProviderOSOS, Subtype: analyzer.ProviderSubtype}
	src := newFakeAdapter(integration.ProviderOSOS, 24*time.Hour, model.ReadingKindLoadProfile)
	clk := clock.NewFake(ingestTestNow)
	enq := newRecordingEnqueuer()

	failing := &ingestTestFailingUpdateAnalyzerRepo{AnalyzerRepository: repos.analyzers}
	deps := ingest.Deps{
		Analyzers: failing, Readings: repos.readings, Cursors: repos.cursors,
		Anomalies: repos.anomalies, Ops: repos.ops, ProviderSeries: repos.series,
		AdminIngestion: repos.adminIngestion, AdminJournal: repos.adminJournal,
		Credentials: fixedCredentialOpener(creds), Sources: sourceMap{creds.Provider: src},
		Enqueuer: enq, Clock: clk, Log: testfixtures.DiscardLogger(),
	}
	svc, err := ingest.New(deps, ingest.Options{})
	require.NoError(t, err)

	newMultiplier := decimal.RequireFromString("45.000000")
	rows := []model.MeterReading{
		readingFor(analyzer.ID, ingestTestIstanbulMidnight.Format(time.RFC3339), model.ReadingKindLoadProfile, map[string]string{"active_import": "10.0000"}),
	}
	window := ingestTestWindow(ingestTestIstanbulMidnight, 2*time.Hour)
	src.setSteps(fetchStep{result: integration.FetchResult{
		Readings:           rows,
		ResolvedMultiplier: &integration.ResolvedMultiplier{Value: newMultiplier, Source: integration.MultiplierFromLastEndex},
	}})

	require.NoError(t, svc.FetchReadings(ctx, job.FetchReadingsPayload{CompanyID: tn.Company.ID, CredentialID: creds.CredentialID, AnalyzerID: analyzer.ID, Kind: model.ReadingKindLoadProfile, Window: window}),
		"M1: a failed multiplier update must not fail the whole run")

	require.Len(t, ingestTestAllReadings(t, ctx, repos.readings, tn.Scope, analyzer.ID, model.ReadingKindLoadProfile), 1)

	stillOld, gerr := repos.analyzers.Get(ctx, tn.Scope, analyzer.ID)
	require.NoError(t, gerr)
	require.False(t, stillOld.MeterMultiplier.Equal(newMultiplier), "M1: a failed Update must never be silently treated as if it changed the multiplier")

	messages, merr := repos.ops.ListMessages(ctx, tn.Scope, store.MessageFilter{})
	require.NoError(t, merr)
	for _, m := range messages {
		require.NotEqual(t, "meter multiplier changed", m.Message, "M1: no 'meter multiplier changed' message on a failed update")
	}
}

// TestIngestionHookReceivesActualPersistedPageRange is M2: PostPersistHook
// must receive [minTs, maxTs] of the rows actually persisted THAT PAGE, not
// the chunk's outer [From, To) — two pages of one chunk, scripted with
// disjoint ranges, must produce two DIFFERENT hook calls.
func TestIngestionHookReceivesActualPersistedPageRange(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tn := testfixtures.NewTenant(t, ctx, pool, 9118)
	repos := ingestTestNewRepos(pool)

	analyzer := tn.Analyzers[0]
	creds := integration.Credentials{CredentialID: uuid.New(), CompanyID: tn.Company.ID, Provider: integration.ProviderOSOS, Subtype: analyzer.ProviderSubtype}
	src := newFakeAdapter(integration.ProviderOSOS, 24*time.Hour, model.ReadingKindLoadProfile)
	clk := clock.NewFake(ingestTestNow)
	enq := newRecordingEnqueuer()
	spy := &ingestTestSpyHook{}

	deps := ingest.Deps{
		Analyzers: repos.analyzers, Readings: repos.readings, Cursors: repos.cursors,
		Anomalies: repos.anomalies, Ops: repos.ops, ProviderSeries: repos.series,
		AdminIngestion: repos.adminIngestion, AdminJournal: repos.adminJournal,
		Credentials: fixedCredentialOpener(creds), Sources: sourceMap{creds.Provider: src},
		Enqueuer: enq, Clock: clk, Log: testfixtures.DiscardLogger(),
		Hooks: map[model.IntegrationProvider][]ingest.PostPersistHook{model.IntegrationProviderOSOS: {spy}},
	}
	svc, err := ingest.New(deps, ingest.Options{})
	require.NoError(t, err)

	window := ingestTestWindow(ingestTestIstanbulMidnight, 24*time.Hour) // one chunk (MaxWindow == 24h)
	page1From := ingestTestIstanbulMidnight
	page1 := ingestTestQuarterHourly(analyzer.ID, page1From, 2) // 00:00, 00:15
	page1NextCursor := page1From.Add(15 * time.Minute)
	page2From := ingestTestIstanbulMidnight.Add(10 * time.Hour)
	page2 := ingestTestQuarterHourly(analyzer.ID, page2From, 2) // 10:00, 10:15
	src.setSteps(
		fetchStep{result: integration.FetchResult{Readings: page1, NextCursor: &page1NextCursor}},
		fetchStep{result: integration.FetchResult{Readings: page2}},
	)

	require.NoError(t, svc.FetchReadings(ctx, job.FetchReadingsPayload{CompanyID: tn.Company.ID, CredentialID: creds.CredentialID, AnalyzerID: analyzer.ID, Kind: model.ReadingKindLoadProfile, Window: window}))

	calls := spy.callsSoFar()
	require.Len(t, calls, 2)
	require.True(t, calls[0].From.Equal(page1From), "hook call 1: page 1's own min, not the chunk's From")
	require.True(t, calls[0].To.Equal(page1From.Add(15*time.Minute)), "hook call 1: page 1's own max")
	require.True(t, calls[1].From.Equal(page2From), "hook call 2: page 2's own min, not the chunk's (unchanged) From")
	require.True(t, calls[1].To.Equal(page2From.Add(15*time.Minute)), "hook call 2: page 2's own max")
}

// TestIngestionWarningMessagesEmittedInSortedCodeOrder is M5: distinct
// adapter warning codes must become operational messages in a fixed,
// deterministic (sorted) order — acc.warnings is a Go map, so without a
// sort the emission order would vary from run to run.
func TestIngestionWarningMessagesEmittedInSortedCodeOrder(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tn := testfixtures.NewTenant(t, ctx, pool, 9119)
	repos := ingestTestNewRepos(pool)

	analyzer := tn.Analyzers[0]
	creds := integration.Credentials{CredentialID: uuid.New(), CompanyID: tn.Company.ID, Provider: integration.ProviderOSOS, Subtype: analyzer.ProviderSubtype}
	src := newFakeAdapter(integration.ProviderOSOS, 24*time.Hour, model.ReadingKindLoadProfile)
	clk := clock.NewFake(ingestTestNow)
	enq := newRecordingEnqueuer()
	svc := ingestTestNewService(t, repos, creds, src, enq, clk, ingest.Options{})

	rows := ingestTestQuarterHourly(analyzer.ID, ingestTestIstanbulMidnight, 1)
	window := ingestTestWindow(ingestTestIstanbulMidnight, time.Hour)
	src.setSteps(fetchStep{result: integration.FetchResult{
		Readings: rows,
		Warnings: []integration.Warning{{Code: "zzz_code"}, {Code: "aaa_code"}, {Code: "mmm_code"}},
	}})

	require.NoError(t, svc.FetchReadings(ctx, job.FetchReadingsPayload{CompanyID: tn.Company.ID, CredentialID: creds.CredentialID, AnalyzerID: analyzer.ID, Kind: model.ReadingKindLoadProfile, Window: window}))

	messages, err := repos.ops.ListMessages(ctx, tn.Scope, store.MessageFilter{})
	require.NoError(t, err)
	byCode := map[string]model.OperationalMessage{}
	for _, m := range messages {
		if m.Message == "aaa_code" || m.Message == "mmm_code" || m.Message == "zzz_code" {
			byCode[m.Message] = m
		}
	}
	require.Len(t, byCode, 3)
	require.True(t, byCode["aaa_code"].ID < byCode["mmm_code"].ID, "M5: warning messages must be emitted in sorted code order")
	require.True(t, byCode["mmm_code"].ID < byCode["zzz_code"].ID, "M5: warning messages must be emitted in sorted code order")
}
