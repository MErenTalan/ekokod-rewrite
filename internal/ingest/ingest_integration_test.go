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

	src.setSteps(fetchStep{err: fmt.Errorf("upstream rejected credentials for password %s", secretValue)})

	window := ingestTestWindow(ingestTestIstanbulMidnight, 2*time.Hour)
	err := svc.FetchReadings(ctx, job.FetchReadingsPayload{CompanyID: tn.Company.ID, CredentialID: creds.CredentialID, AnalyzerID: analyzer.ID, Kind: model.ReadingKindLoadProfile, Window: window})
	require.Error(t, err)

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
