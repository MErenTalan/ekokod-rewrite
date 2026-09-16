//go:build integration

// Task 11b (R73, I-12, I-13): the ingest enqueue seam. These tests drive
// ingest.Service.FetchReadings through a real database exactly like
// ingest_integration_test.go's suite, adding a
// recordingConsumptionRefreshEnqueuer in place of the nil F2 left
// Deps.ConsumptionRefresh, so the enqueue call site's gating (nil seam,
// config.ConsumptionRefreshEnabled, the affected-range threshold) and its
// choice of window (the affected range, never the requested one) are all
// observable without a real asynq broker.
package ingest_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/ingest"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/normalize"
	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/lock"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/consumption"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/admin"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

// fetchRefreshTestNow anchors every threshold-focused test below: readings
// are placed at fixed offsets from it, never from time.Now().
var fetchRefreshTestNow = time.Date(2026, time.June, 15, 12, 0, 0, 0, time.UTC)

// recordingConsumptionRefreshEnqueuer is an ingest.ConsumptionRefreshEnqueuer
// that records every payload it is asked to enqueue. failErr, when set,
// makes every call fail instead — TestAnEnqueueFailureDoesNotFailTheFetchRun's
// fixture.
type recordingConsumptionRefreshEnqueuer struct {
	mu      sync.Mutex
	calls   []job.ConsumptionRefreshPayload
	failErr error
}

func newRecordingConsumptionRefreshEnqueuer() *recordingConsumptionRefreshEnqueuer {
	return &recordingConsumptionRefreshEnqueuer{}
}

func (e *recordingConsumptionRefreshEnqueuer) EnqueueConsumptionRefresh(_ context.Context, p job.ConsumptionRefreshPayload) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.failErr != nil {
		return e.failErr
	}
	e.calls = append(e.calls, p)
	return nil
}

func (e *recordingConsumptionRefreshEnqueuer) enqueued() []job.ConsumptionRefreshPayload {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]job.ConsumptionRefreshPayload, len(e.calls))
	copy(out, e.calls)
	return out
}

var _ ingest.ConsumptionRefreshEnqueuer = (*recordingConsumptionRefreshEnqueuer)(nil)

// fetchRefreshTestFixture bundles everything one test needs: the pool (for
// repositories fetchRefreshTestSetup does not already wrap, like
// AnalyticsRepository and AdminAggregateRepository), the tenant, its first
// analyzer and a fixed OSOS credential for it.
type fetchRefreshTestFixture struct {
	pool     *pgxpool.Pool
	repos    ingestTestRepos
	tenant   testfixtures.Tenant
	analyzer model.Analyzer
	creds    integration.Credentials
}

// fetchRefreshTestSetup builds one isolated tenant, its real repositories
// and a fixed OSOS credential for analyzer Analyzers[0] — the same shape
// ingest_integration_test.go's own tests share, factored out here because
// every test below needs it.
func fetchRefreshTestSetup(t *testing.T, seed int64) fetchRefreshTestFixture {
	t.Helper()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tn := testfixtures.NewTenant(t, ctx, pool, seed)
	repos := ingestTestNewRepos(pool)
	analyzer := tn.Analyzers[0]
	creds := integration.Credentials{
		CredentialID: uuid.New(),
		CompanyID:    tn.Company.ID,
		Provider:     integration.ProviderOSOS,
		Subtype:      analyzer.ProviderSubtype,
	}
	return fetchRefreshTestFixture{pool: pool, repos: repos, tenant: tn, analyzer: analyzer, creds: creds}
}

// fetchRefreshTestService wires an ingest.Service against real repos, one
// fake adapter, a fixed credential, and refreshEnq in place of F2's nil
// Deps.ConsumptionRefresh.
func fetchRefreshTestService(t *testing.T, repos ingestTestRepos, creds integration.Credentials, src integration.Adapter, refreshEnq ingest.ConsumptionRefreshEnqueuer, clk clock.Clock, opts ingest.Options) *ingest.Service {
	t.Helper()
	deps := ingest.Deps{
		Analyzers: repos.analyzers, Readings: repos.readings, Cursors: repos.cursors,
		Anomalies: repos.anomalies, Ops: repos.ops, ProviderSeries: repos.series,
		AdminIngestion: repos.adminIngestion, AdminJournal: repos.adminJournal,
		Credentials: fixedCredentialOpener(creds), Sources: sourceMap{creds.Provider: src},
		Enqueuer: newRecordingEnqueuer(), ConsumptionRefresh: refreshEnq, Clock: clk, Log: testfixtures.DiscardLogger(),
	}
	svc, err := ingest.New(deps, opts)
	require.NoError(t, err)
	return svc
}

// fetchRefreshTestRedisLocker connects a real lock.Redis to an isolated
// redis container, mirroring internal/service/consumption's own
// refresh_integration_test.go fixture.
func fetchRefreshTestRedisLocker(t *testing.T) lock.Locker {
	t.Helper()
	uri := testfixtures.StartRedis(t)
	opts, err := goredis.ParseURL(uri)
	require.NoError(t, err)
	client := goredis.NewClient(opts)
	t.Cleanup(func() { _ = client.Close() })
	return lock.NewRedis(client)
}

// ---------------------------------------------------------------------------
// R73: the phase's operational promise made true.
// ---------------------------------------------------------------------------

// TestABackfillTwoHundredDaysOldEntersTheAggregateAfterTheJobRuns drives a
// real fetch of readings ~200 days old through the ingest pipeline, captures
// the consumption.refresh payload it enqueues, and proves that running
// consumption.Refresher.RefreshConsumption on that exact payload — with a
// real AdminAggregateRepository and a real Redis locker — is what makes the
// bucket appear.
//
// consumption_monthly (materialized_only), not consumption_hourly, is the
// view asserted here: Task 6's TestRefreshMaterialisesAnHourBelowTheWatermark
// pattern requires first moving the real-time watermark forward with a
// SEPARATE recent reading, which would muddy this test's one point (that
// consumption.refresh, and only consumption.refresh, is what makes a
// backfilled bucket appear) with an unrelated fixture. A closed month is
// materialized-only end to end, so "absent, then present" isolates exactly
// the promise R73 makes — the same reasoning
// TestRefreshConsumptionMaterialisesAClosedMonthBelowThePolicyWindow in
// internal/service/consumption already uses.
func TestABackfillTwoHundredDaysOldEntersTheAggregateAfterTheJobRuns(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fx := fetchRefreshTestSetup(t, 9130)

	analyticsRepo := postgres.NewAnalyticsRepository(fx.pool)
	aggregateRepo := admin.NewAggregateRepository(fx.pool)

	src := newFakeAdapter(integration.ProviderOSOS, 40*24*time.Hour, model.ReadingKindLoadProfile)
	// fetchRefreshTestNow's June 2026 is comfortably after February 2026 —
	// firstAt is ~217 days before it, well past both the 30-day enqueue
	// threshold and any refresh-policy watermark.
	clk := clock.NewFake(fetchRefreshTestNow)
	refreshEnq := newRecordingConsumptionRefreshEnqueuer()
	svc := fetchRefreshTestService(t, fx.repos, fx.creds, src, refreshEnq, clk, ingest.Options{ConsumptionRefreshEnabled: true})

	monthStart := time.Date(2026, time.February, 1, 0, 0, 0, 0, normalize.Istanbul)
	firstAt := time.Date(2026, time.February, 10, 12, 0, 0, 0, normalize.Istanbul)
	secondAt := time.Date(2026, time.February, 20, 12, 0, 0, 0, normalize.Istanbul)
	monthWindow := store.TimeRange{From: monthStart.Add(-24 * time.Hour), To: monthStart.AddDate(0, 1, 0).Add(24 * time.Hour)}

	rows := []model.MeterReading{
		readingFor(fx.analyzer.ID, firstAt.Format(time.RFC3339), model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
		readingFor(fx.analyzer.ID, secondAt.Format(time.RFC3339), model.ReadingKindLoadProfile, map[string]string{"active_import": "1012.5"}),
	}
	src.setSteps(fetchStep{result: integration.FetchResult{Readings: rows}})

	window := &job.Window{From: monthStart, To: monthStart.AddDate(0, 1, 0)}
	payload := job.FetchReadingsPayload{CompanyID: fx.tenant.Company.ID, CredentialID: fx.creds.CredentialID, AnalyzerID: fx.analyzer.ID, Kind: model.ReadingKindLoadProfile, Window: window}
	require.NoError(t, svc.FetchReadings(ctx, payload))

	calls := refreshEnq.enqueued()
	require.Len(t, calls, 1, "a backfill of a range past the threshold enqueues exactly one refresh")

	before, err := analyticsRepo.ConsumptionMonthly(ctx, fx.tenant.AdminScope, []uuid.UUID{fx.analyzer.ID}, monthWindow)
	require.NoError(t, err)
	require.Empty(t, before, "the closed month is absent until consumption.refresh actually runs — enqueueing alone must not materialise it")

	refresher, err := consumption.NewRefresher(consumption.RefreshDeps{
		Aggregates: aggregateRepo,
		Locker:     fetchRefreshTestRedisLocker(t),
		LockTTL:    2 * time.Minute,
		Location:   normalize.Istanbul,
		Log:        testfixtures.DiscardLogger(),
	})
	require.NoError(t, err)
	require.NoError(t, refresher.RefreshConsumption(ctx, calls[0]))

	after, err := analyticsRepo.ConsumptionMonthly(ctx, fx.tenant.AdminScope, []uuid.UUID{fx.analyzer.ID}, monthWindow)
	require.NoError(t, err)
	require.Len(t, after, 1)
	require.True(t, decimal.RequireFromString("12.5").Equal(*after[0].ActiveConsumption),
		"want 12.5, got %s", after[0].ActiveConsumption.String())
}

// ---------------------------------------------------------------------------
// The affected range, not the requested window; both finish paths (I-12).
// ---------------------------------------------------------------------------

// TestFetchEnqueuesExactlyOneRefreshForTheAffectedRange proves the enqueued
// window is the affected range actually persisted, not the (much wider)
// requested window — I-12's "the window enqueued is the affected range".
func TestFetchEnqueuesExactlyOneRefreshForTheAffectedRange(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fx := fetchRefreshTestSetup(t, 9131)

	clk := clock.NewFake(fetchRefreshTestNow)
	src := newFakeAdapter(integration.ProviderOSOS, 20*24*time.Hour, model.ReadingKindLoadProfile)
	refreshEnq := newRecordingConsumptionRefreshEnqueuer()
	svc := fetchRefreshTestService(t, fx.repos, fx.creds, src, refreshEnq, clk, ingest.Options{ConsumptionRefreshEnabled: true})

	window := ingestTestWindow(fetchRefreshTestNow.Add(-70*24*time.Hour), 20*24*time.Hour)
	firstAt := fetchRefreshTestNow.Add(-65 * 24 * time.Hour)
	secondAt := fetchRefreshTestNow.Add(-64 * 24 * time.Hour)
	rows := []model.MeterReading{
		readingFor(fx.analyzer.ID, firstAt.Format(time.RFC3339), model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
		readingFor(fx.analyzer.ID, secondAt.Format(time.RFC3339), model.ReadingKindLoadProfile, map[string]string{"active_import": "1010"}),
	}
	src.setSteps(fetchStep{result: integration.FetchResult{Readings: rows}})

	payload := job.FetchReadingsPayload{CompanyID: fx.tenant.Company.ID, CredentialID: fx.creds.CredentialID, AnalyzerID: fx.analyzer.ID, Kind: model.ReadingKindLoadProfile, Window: window}
	require.NoError(t, svc.FetchReadings(ctx, payload))

	calls := refreshEnq.enqueued()
	require.Len(t, calls, 1)
	require.True(t, calls[0].From.Equal(firstAt), "want enqueued From == affected range start, got %s", calls[0].From)
	require.True(t, calls[0].To.Equal(secondAt), "want enqueued To == affected range end (not the requested window's bound), got %s", calls[0].To)
	require.Equal(t, fx.tenant.Company.ID, calls[0].CompanyID)
	require.Equal(t, fx.analyzer.ID, calls[0].AnalyzerID)
}

// TestFetchDoesNotEnqueueWhenNothingWasPersisted proves an empty affected
// range never enqueues.
func TestFetchDoesNotEnqueueWhenNothingWasPersisted(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fx := fetchRefreshTestSetup(t, 9132)

	clk := clock.NewFake(fetchRefreshTestNow)
	src := newFakeAdapter(integration.ProviderOSOS, 20*24*time.Hour, model.ReadingKindLoadProfile)
	refreshEnq := newRecordingConsumptionRefreshEnqueuer()
	svc := fetchRefreshTestService(t, fx.repos, fx.creds, src, refreshEnq, clk, ingest.Options{ConsumptionRefreshEnabled: true})

	window := ingestTestWindow(fetchRefreshTestNow.Add(-70*24*time.Hour), 20*24*time.Hour)
	src.setSteps(fetchStep{result: integration.FetchResult{}})

	payload := job.FetchReadingsPayload{CompanyID: fx.tenant.Company.ID, CredentialID: fx.creds.CredentialID, AnalyzerID: fx.analyzer.ID, Kind: model.ReadingKindLoadProfile, Window: window}
	require.NoError(t, svc.FetchReadings(ctx, payload))

	require.Empty(t, refreshEnq.enqueued())
}

// ---------------------------------------------------------------------------
// R73/I-13: the threshold's two sides and its exact boundary.
// ---------------------------------------------------------------------------

// TestFetchDoesNotEnqueueWhenTheAffectedRangeIsEntirelyWithinTheLastThirtyDays
// is live data consumption_hourly's own real-time union already covers.
func TestFetchDoesNotEnqueueWhenTheAffectedRangeIsEntirelyWithinTheLastThirtyDays(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fx := fetchRefreshTestSetup(t, 9133)

	clk := clock.NewFake(fetchRefreshTestNow)
	src := newFakeAdapter(integration.ProviderOSOS, 20*24*time.Hour, model.ReadingKindLoadProfile)
	refreshEnq := newRecordingConsumptionRefreshEnqueuer()
	svc := fetchRefreshTestService(t, fx.repos, fx.creds, src, refreshEnq, clk, ingest.Options{ConsumptionRefreshEnabled: true})

	at := fetchRefreshTestNow.Add(-10 * 24 * time.Hour)
	window := ingestTestWindow(at.Add(-time.Hour), 2*time.Hour)
	rows := []model.MeterReading{readingFor(fx.analyzer.ID, at.Format(time.RFC3339), model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"})}
	src.setSteps(fetchStep{result: integration.FetchResult{Readings: rows}})

	payload := job.FetchReadingsPayload{CompanyID: fx.tenant.Company.ID, CredentialID: fx.creds.CredentialID, AnalyzerID: fx.analyzer.ID, Kind: model.ReadingKindLoadProfile, Window: window}
	require.NoError(t, svc.FetchReadings(ctx, payload))

	require.Empty(t, refreshEnq.enqueued(), "an affected range entirely within the last 30 days needs no refresh")
}

// TestFetchEnqueuesWhenTheAffectedRangeStartsBeforeThirtyDaysAgo crosses the
// threshold and must enqueue.
func TestFetchEnqueuesWhenTheAffectedRangeStartsBeforeThirtyDaysAgo(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fx := fetchRefreshTestSetup(t, 9134)

	clk := clock.NewFake(fetchRefreshTestNow)
	src := newFakeAdapter(integration.ProviderOSOS, 20*24*time.Hour, model.ReadingKindLoadProfile)
	refreshEnq := newRecordingConsumptionRefreshEnqueuer()
	svc := fetchRefreshTestService(t, fx.repos, fx.creds, src, refreshEnq, clk, ingest.Options{ConsumptionRefreshEnabled: true})

	at := fetchRefreshTestNow.Add(-31 * 24 * time.Hour)
	window := ingestTestWindow(at.Add(-time.Hour), 2*time.Hour)
	rows := []model.MeterReading{readingFor(fx.analyzer.ID, at.Format(time.RFC3339), model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"})}
	src.setSteps(fetchStep{result: integration.FetchResult{Readings: rows}})

	payload := job.FetchReadingsPayload{CompanyID: fx.tenant.Company.ID, CredentialID: fx.creds.CredentialID, AnalyzerID: fx.analyzer.ID, Kind: model.ReadingKindLoadProfile, Window: window}
	require.NoError(t, svc.FetchReadings(ctx, payload))

	require.Len(t, refreshEnq.enqueued(), 1)
}

// TestFetchDoesNotEnqueueAtExactlyThirtyDaysBoundary and
// TestFetchEnqueuesJustPastTheThirtyDayBoundary pin the exact boundary: an
// affected range starting EXACTLY 30 days before now is still "within" the
// policy window (the comparison is strict Before, not Before-or-equal) and
// does not enqueue; one microsecond earlier does. Microsecond, not
// nanosecond, because pgx/Postgres timestamptz columns are microsecond
// precision (see fetch.go's M1 comment on the same trap).
func TestFetchDoesNotEnqueueAtExactlyThirtyDaysBoundary(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fx := fetchRefreshTestSetup(t, 9135)

	clk := clock.NewFake(fetchRefreshTestNow)
	src := newFakeAdapter(integration.ProviderOSOS, 20*24*time.Hour, model.ReadingKindLoadProfile)
	refreshEnq := newRecordingConsumptionRefreshEnqueuer()
	svc := fetchRefreshTestService(t, fx.repos, fx.creds, src, refreshEnq, clk, ingest.Options{ConsumptionRefreshEnabled: true})

	at := fetchRefreshTestNow.Add(-30 * 24 * time.Hour)
	window := ingestTestWindow(at.Add(-time.Hour), 2*time.Hour)
	rows := []model.MeterReading{readingFor(fx.analyzer.ID, at.Format(time.RFC3339), model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"})}
	src.setSteps(fetchStep{result: integration.FetchResult{Readings: rows}})

	payload := job.FetchReadingsPayload{CompanyID: fx.tenant.Company.ID, CredentialID: fx.creds.CredentialID, AnalyzerID: fx.analyzer.ID, Kind: model.ReadingKindLoadProfile, Window: window}
	require.NoError(t, svc.FetchReadings(ctx, payload))

	require.Empty(t, refreshEnq.enqueued(), "exactly 30 days ago is still within the policy window — the boundary is exclusive")
}

func TestFetchEnqueuesJustPastTheThirtyDayBoundary(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fx := fetchRefreshTestSetup(t, 9136)

	clk := clock.NewFake(fetchRefreshTestNow)
	src := newFakeAdapter(integration.ProviderOSOS, 20*24*time.Hour, model.ReadingKindLoadProfile)
	refreshEnq := newRecordingConsumptionRefreshEnqueuer()
	svc := fetchRefreshTestService(t, fx.repos, fx.creds, src, refreshEnq, clk, ingest.Options{ConsumptionRefreshEnabled: true})

	at := fetchRefreshTestNow.Add(-30 * 24 * time.Hour).Add(-time.Microsecond)
	window := ingestTestWindow(at.Add(-time.Hour), 2*time.Hour)
	rows := []model.MeterReading{readingFor(fx.analyzer.ID, at.Format(time.RFC3339Nano), model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"})}
	src.setSteps(fetchStep{result: integration.FetchResult{Readings: rows}})

	payload := job.FetchReadingsPayload{CompanyID: fx.tenant.Company.ID, CredentialID: fx.creds.CredentialID, AnalyzerID: fx.analyzer.ID, Kind: model.ReadingKindLoadProfile, Window: window}
	require.NoError(t, svc.FetchReadings(ctx, payload))

	require.Len(t, refreshEnq.enqueued(), 1, "one microsecond past the boundary must enqueue")
}

// ---------------------------------------------------------------------------
// I-12: the partial-run path enqueues too.
// ---------------------------------------------------------------------------

// TestFailFetchRunStillEnqueuesWhenSomeRowsWerePersistedBeforeTheFailure
// scripts a first page that succeeds (persisting rows past the threshold)
// and a second page that fails, forcing failFetchRun — which must still
// enqueue for what the first page actually persisted.
func TestFailFetchRunStillEnqueuesWhenSomeRowsWerePersistedBeforeTheFailure(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fx := fetchRefreshTestSetup(t, 9137)

	clk := clock.NewFake(fetchRefreshTestNow)
	src := newFakeAdapter(integration.ProviderOSOS, 41*24*time.Hour, model.ReadingKindLoadProfile)
	refreshEnq := newRecordingConsumptionRefreshEnqueuer()
	svc := fetchRefreshTestService(t, fx.repos, fx.creds, src, refreshEnq, clk, ingest.Options{ConsumptionRefreshEnabled: true})

	chunkFrom := fetchRefreshTestNow.Add(-90 * 24 * time.Hour)
	chunkTo := fetchRefreshTestNow.Add(-50 * 24 * time.Hour)
	page1From := chunkFrom.Add(24 * time.Hour)
	page1To := page1From.Add(time.Hour)
	nextCursor := page1To.Add(time.Hour)

	page1Rows := []model.MeterReading{
		readingFor(fx.analyzer.ID, page1From.Format(time.RFC3339), model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
		readingFor(fx.analyzer.ID, page1To.Format(time.RFC3339), model.ReadingKindLoadProfile, map[string]string{"active_import": "1005"}),
	}
	src.setSteps(
		fetchStep{result: integration.FetchResult{Readings: page1Rows, NextCursor: &nextCursor}},
		fetchStep{err: &integration.Error{Kind: integration.ErrUpstreamUnavailable, Provider: integration.ProviderOSOS, Op: "load_profiles"}},
	)

	window := &job.Window{From: chunkFrom, To: chunkTo}
	payload := job.FetchReadingsPayload{CompanyID: fx.tenant.Company.ID, CredentialID: fx.creds.CredentialID, AnalyzerID: fx.analyzer.ID, Kind: model.ReadingKindLoadProfile, Window: window}
	require.Error(t, svc.FetchReadings(ctx, payload), "the run must still report the page-2 failure")

	calls := refreshEnq.enqueued()
	require.Len(t, calls, 1, "a partial run that persisted rows before failing must still enqueue for what it actually persisted")
	require.True(t, calls[0].From.Equal(page1From))
	require.True(t, calls[0].To.Equal(page1To))
}

// TestFailFetchRunDoesNotEnqueueWhenNothingWasEverPersisted: the run fails
// before persisting anything, so acc.affectedFrom is nil and there is
// nothing to refresh.
func TestFailFetchRunDoesNotEnqueueWhenNothingWasEverPersisted(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fx := fetchRefreshTestSetup(t, 9138)

	clk := clock.NewFake(fetchRefreshTestNow)
	src := newFakeAdapter(integration.ProviderOSOS, 41*24*time.Hour, model.ReadingKindLoadProfile)
	refreshEnq := newRecordingConsumptionRefreshEnqueuer()
	svc := fetchRefreshTestService(t, fx.repos, fx.creds, src, refreshEnq, clk, ingest.Options{ConsumptionRefreshEnabled: true})

	src.setSteps(fetchStep{err: &integration.Error{Kind: integration.ErrUpstreamUnavailable, Provider: integration.ProviderOSOS, Op: "load_profiles"}})

	window := ingestTestWindow(fetchRefreshTestNow.Add(-90*24*time.Hour), 40*24*time.Hour)
	payload := job.FetchReadingsPayload{CompanyID: fx.tenant.Company.ID, CredentialID: fx.creds.CredentialID, AnalyzerID: fx.analyzer.ID, Kind: model.ReadingKindLoadProfile, Window: window}
	require.Error(t, svc.FetchReadings(ctx, payload))

	require.Empty(t, refreshEnq.enqueued())
}

// ---------------------------------------------------------------------------
// An enqueue failure is a warning, never a run failure; the config gate.
// ---------------------------------------------------------------------------

// TestAnEnqueueFailureDoesNotFailTheFetchRun: the enqueuer always errors,
// but the run — which had already succeeded by the time the enqueue call
// happens — must still report success, with a warning message recorded.
func TestAnEnqueueFailureDoesNotFailTheFetchRun(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fx := fetchRefreshTestSetup(t, 9139)

	clk := clock.NewFake(fetchRefreshTestNow)
	src := newFakeAdapter(integration.ProviderOSOS, 20*24*time.Hour, model.ReadingKindLoadProfile)
	refreshEnq := newRecordingConsumptionRefreshEnqueuer()
	refreshEnq.failErr = errors.New("fetch_refresh_test: forced enqueue failure")
	svc := fetchRefreshTestService(t, fx.repos, fx.creds, src, refreshEnq, clk, ingest.Options{ConsumptionRefreshEnabled: true})

	at := fetchRefreshTestNow.Add(-31 * 24 * time.Hour)
	window := ingestTestWindow(at.Add(-time.Hour), 2*time.Hour)
	rows := []model.MeterReading{readingFor(fx.analyzer.ID, at.Format(time.RFC3339), model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"})}
	src.setSteps(fetchStep{result: integration.FetchResult{Readings: rows}})

	payload := job.FetchReadingsPayload{CompanyID: fx.tenant.Company.ID, CredentialID: fx.creds.CredentialID, AnalyzerID: fx.analyzer.ID, Kind: model.ReadingKindLoadProfile, Window: window}
	require.NoError(t, svc.FetchReadings(ctx, payload), "an enqueue failure must never surface as the run's own error")

	runs := ingestTestRunsForAnalyzer(t, ctx, fx.repos.ops, fx.tenant.Scope, job.TypeIntegrationFetchReadings, fx.analyzer.ID)
	require.Len(t, runs, 1)
	require.Equal(t, "success", runs[0].Status)

	msgs, err := fx.repos.ops.ListMessages(ctx, fx.tenant.Scope, store.MessageFilter{Statuses: []string{"warning"}, Page: store.Page{Limit: 50}})
	require.NoError(t, err)
	found := false
	for _, m := range msgs {
		if m.Message == "consumption.refresh enqueue failed" {
			found = true
		}
	}
	require.True(t, found, "an enqueue failure must be recorded as a warning operational message")
}

// TestConsumptionRefreshEnabledFalseDisablesTheEnqueueEntirely: the config
// gate must be honoured even when everything else (a wired enqueuer, an
// affected range past the threshold) says "enqueue".
func TestConsumptionRefreshEnabledFalseDisablesTheEnqueueEntirely(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fx := fetchRefreshTestSetup(t, 9140)

	clk := clock.NewFake(fetchRefreshTestNow)
	src := newFakeAdapter(integration.ProviderOSOS, 20*24*time.Hour, model.ReadingKindLoadProfile)
	refreshEnq := newRecordingConsumptionRefreshEnqueuer()
	svc := fetchRefreshTestService(t, fx.repos, fx.creds, src, refreshEnq, clk, ingest.Options{ConsumptionRefreshEnabled: false})

	at := fetchRefreshTestNow.Add(-31 * 24 * time.Hour)
	window := ingestTestWindow(at.Add(-time.Hour), 2*time.Hour)
	rows := []model.MeterReading{readingFor(fx.analyzer.ID, at.Format(time.RFC3339), model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"})}
	src.setSteps(fetchStep{result: integration.FetchResult{Readings: rows}})

	payload := job.FetchReadingsPayload{CompanyID: fx.tenant.Company.ID, CredentialID: fx.creds.CredentialID, AnalyzerID: fx.analyzer.ID, Kind: model.ReadingKindLoadProfile, Window: window}
	require.NoError(t, svc.FetchReadings(ctx, payload))

	require.Empty(t, refreshEnq.enqueued(), "EKOKOD_CONSUMPTION_REFRESH_ENABLED=false must disable the enqueue entirely")
}

// ---------------------------------------------------------------------------
// (a) the threshold must be evaluated against Deps.Clock, never time.Now().
// ---------------------------------------------------------------------------

// TestFetchUsesTheDepsClockNotWallClockForTheEnqueueThreshold pins a fake
// "now" far in the past (2015) with a reading only 12 days before it — well
// within the 30-day policy window relative to the FAKE clock, so the
// correct implementation never enqueues. A mutant that read time.Now()
// instead would compare that same reading against the REAL wall clock
// (~2026), see an affected range over a decade old, and wrongly enqueue —
// which is exactly what this test would catch.
func TestFetchUsesTheDepsClockNotWallClockForTheEnqueueThreshold(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fx := fetchRefreshTestSetup(t, 9141)

	fakeNow := time.Date(2015, time.January, 1, 0, 0, 0, 0, time.UTC)
	clk := clock.NewFake(fakeNow)
	src := newFakeAdapter(integration.ProviderOSOS, 20*24*time.Hour, model.ReadingKindLoadProfile)
	refreshEnq := newRecordingConsumptionRefreshEnqueuer()
	svc := fetchRefreshTestService(t, fx.repos, fx.creds, src, refreshEnq, clk, ingest.Options{ConsumptionRefreshEnabled: true})

	at := fakeNow.Add(-12 * 24 * time.Hour)
	window := ingestTestWindow(at.Add(-time.Hour), 2*time.Hour)
	rows := []model.MeterReading{readingFor(fx.analyzer.ID, at.Format(time.RFC3339), model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"})}
	src.setSteps(fetchStep{result: integration.FetchResult{Readings: rows}})

	payload := job.FetchReadingsPayload{CompanyID: fx.tenant.Company.ID, CredentialID: fx.creds.CredentialID, AnalyzerID: fx.analyzer.ID, Kind: model.ReadingKindLoadProfile, Window: window}
	require.NoError(t, svc.FetchReadings(ctx, payload))

	require.Empty(t, refreshEnq.enqueued(), "the threshold must be evaluated against the FAKE clock's now, not time.Now()")
}
