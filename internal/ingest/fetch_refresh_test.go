//go:build integration

// R73: the ingest enqueue seam. These tests drive
// ingest.Service.FetchReadings through a real database exactly like
// ingest_integration_test.go's suite, adding a
// recordingConsumptionRefreshEnqueuer in place of a nil
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
// view asserted here: TestRefreshMaterialisesAnHourBelowTheWatermark's
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
	// firstAt is ~217 days before it, well past both the 29-day enqueue
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

	// R100(3): consumption_monthly's own policy start_offset is 1 year, and
	// February 2026 is only ~4 months before fetchRefreshTestNow (June
	// 2026) — comfortably INSIDE that policy's own live window, so a
	// refresher clocked at fetchRefreshTestNow would legitimately SKIP the
	// monthly view (the scheduled policy already covers it). This test's
	// whole point is proving the MANUAL refresh materialises a window the
	// policy has stopped touching, so the refresher here is clocked more
	// than a year past the window instead — independent of clk, which only
	// drives the ingest side's enqueue threshold above and is unaffected.
	refresher, err := consumption.NewRefresher(consumption.RefreshDeps{
		Aggregates: aggregateRepo,
		Locker:     fetchRefreshTestRedisLocker(t),
		Enqueuer:   newRecordingConsumptionRefreshEnqueuer(),
		Clock:      clock.NewFake(fetchRefreshTestNow.AddDate(2, 0, 0)),
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
// The affected range, not the requested window; both finish paths.
// ---------------------------------------------------------------------------

// TestFetchEnqueuesExactlyOneRefreshForTheAffectedRange proves the enqueued
// window is the affected range actually persisted, not the (much wider)
// requested window.
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
	// The enqueued To is the affected range's end WIDENED by one microsecond
	// via refreshRangeFor — acc.affectedTo is the inclusive last-reading
	// timestamp, but the payload's To is an exclusive bound.
	require.True(t, calls[0].To.Equal(secondAt.Add(time.Microsecond)), "want enqueued To == affected range end + 1us (not the requested window's bound, and not the bare inclusive timestamp), got %s", calls[0].To)
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
// R73: the threshold's two sides and its exact boundary.
// ---------------------------------------------------------------------------

// TestFetchDoesNotEnqueueWhenTheAffectedRangeIsEntirelyWithinTheLastTwentyNineDays
// is live data consumption_hourly's own real-time union already covers.
func TestFetchDoesNotEnqueueWhenTheAffectedRangeIsEntirelyWithinTheLastTwentyNineDays(t *testing.T) {
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

	require.Empty(t, refreshEnq.enqueued(), "an affected range entirely within the last 29 days needs no refresh")
}

// TestFetchEnqueuesWhenTheAffectedRangeStartsBeforeTwentyNineDaysAgo crosses
// the threshold and must enqueue.
func TestFetchEnqueuesWhenTheAffectedRangeStartsBeforeTwentyNineDaysAgo(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fx := fetchRefreshTestSetup(t, 9134)

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

	require.Len(t, refreshEnq.enqueued(), 1)
}

// TestFetchDoesNotEnqueueAtExactlyTwentyNineDaysBoundary and
// TestFetchEnqueuesJustPastTheTwentyNineDayBoundary pin the exact boundary
// (R100(6), amending R73's 30-day threshold down to 29 — one day of
// margin inside consumption_hourly's own 30-day policy window): an
// affected range starting EXACTLY 29 days before now is
// still "within" the policy window (the comparison is strict Before, not
// Before-or-equal) and does not enqueue; one microsecond earlier does.
// Microsecond, not nanosecond, because pgx/Postgres timestamptz columns are
// microsecond precision (see fetch.go's refreshRangeFor comment on the same
// trap).
func TestFetchDoesNotEnqueueAtExactlyTwentyNineDaysBoundary(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fx := fetchRefreshTestSetup(t, 9135)

	clk := clock.NewFake(fetchRefreshTestNow)
	src := newFakeAdapter(integration.ProviderOSOS, 20*24*time.Hour, model.ReadingKindLoadProfile)
	refreshEnq := newRecordingConsumptionRefreshEnqueuer()
	svc := fetchRefreshTestService(t, fx.repos, fx.creds, src, refreshEnq, clk, ingest.Options{ConsumptionRefreshEnabled: true})

	at := fetchRefreshTestNow.Add(-29 * 24 * time.Hour)
	window := ingestTestWindow(at.Add(-time.Hour), 2*time.Hour)
	rows := []model.MeterReading{readingFor(fx.analyzer.ID, at.Format(time.RFC3339), model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"})}
	src.setSteps(fetchStep{result: integration.FetchResult{Readings: rows}})

	payload := job.FetchReadingsPayload{CompanyID: fx.tenant.Company.ID, CredentialID: fx.creds.CredentialID, AnalyzerID: fx.analyzer.ID, Kind: model.ReadingKindLoadProfile, Window: window}
	require.NoError(t, svc.FetchReadings(ctx, payload))

	require.Empty(t, refreshEnq.enqueued(), "exactly 29 days ago is still within the policy window — the boundary is exclusive")
}

func TestFetchEnqueuesJustPastTheTwentyNineDayBoundary(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fx := fetchRefreshTestSetup(t, 9136)

	clk := clock.NewFake(fetchRefreshTestNow)
	src := newFakeAdapter(integration.ProviderOSOS, 20*24*time.Hour, model.ReadingKindLoadProfile)
	refreshEnq := newRecordingConsumptionRefreshEnqueuer()
	svc := fetchRefreshTestService(t, fx.repos, fx.creds, src, refreshEnq, clk, ingest.Options{ConsumptionRefreshEnabled: true})

	at := fetchRefreshTestNow.Add(-29 * 24 * time.Hour).Add(-time.Microsecond)
	window := ingestTestWindow(at.Add(-time.Hour), 2*time.Hour)
	rows := []model.MeterReading{readingFor(fx.analyzer.ID, at.Format(time.RFC3339Nano), model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"})}
	src.setSteps(fetchStep{result: integration.FetchResult{Readings: rows}})

	payload := job.FetchReadingsPayload{CompanyID: fx.tenant.Company.ID, CredentialID: fx.creds.CredentialID, AnalyzerID: fx.analyzer.ID, Kind: model.ReadingKindLoadProfile, Window: window}
	require.NoError(t, svc.FetchReadings(ctx, payload))

	require.Len(t, refreshEnq.enqueued(), 1, "one microsecond past the boundary must enqueue")
}

// ---------------------------------------------------------------------------
// R100(1): a run that persisted no load_profile rows never enqueues — every
// consumption continuous aggregate filters kind = 'load_profile' (migration
// 00005), so a run of any other kind touches rows the aggregates never
// read.
// ---------------------------------------------------------------------------

// TestFetchDoesNotEnqueueWhenTheRunPersistedNoLoadProfileRows drives a
// REAL "daily"-kind fetch run — not load_profile — well past the enqueue
// threshold, and proves it never enqueues a refresh even though rows were
// persisted and the affected range easily crosses the threshold: kind alone
// must gate the enqueue.
func TestFetchDoesNotEnqueueWhenTheRunPersistedNoLoadProfileRows(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fx := fetchRefreshTestSetup(t, 9145)

	clk := clock.NewFake(fetchRefreshTestNow)
	src := newFakeAdapter(integration.ProviderOSOS, 20*24*time.Hour, model.ReadingKindDaily)
	refreshEnq := newRecordingConsumptionRefreshEnqueuer()
	svc := fetchRefreshTestService(t, fx.repos, fx.creds, src, refreshEnq, clk, ingest.Options{ConsumptionRefreshEnabled: true})

	at := fetchRefreshTestNow.Add(-40 * 24 * time.Hour)
	window := ingestTestWindow(at.Add(-time.Hour), 2*time.Hour)
	rows := []model.MeterReading{readingFor(fx.analyzer.ID, at.Format(time.RFC3339), model.ReadingKindDaily, map[string]string{"active_import": "1000"})}
	src.setSteps(fetchStep{result: integration.FetchResult{Readings: rows}})

	payload := job.FetchReadingsPayload{CompanyID: fx.tenant.Company.ID, CredentialID: fx.creds.CredentialID, AnalyzerID: fx.analyzer.ID, Kind: model.ReadingKindDaily, Window: window}
	require.NoError(t, svc.FetchReadings(ctx, payload))

	require.Empty(t, refreshEnq.enqueued(), "a run that persisted only daily rows must never enqueue consumption.refresh")
}

// ---------------------------------------------------------------------------
// The partial-run path enqueues too.
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
	// Widened by one microsecond, same reasoning as
	// TestFetchEnqueuesExactlyOneRefreshForTheAffectedRange above.
	require.True(t, calls[0].To.Equal(page1To.Add(time.Microsecond)))
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

// ---------------------------------------------------------------------------
// refreshRangeFor's one-microsecond widening: the exclusive upper bound and
// the single-instant run.
// ---------------------------------------------------------------------------

// TestFetchEnqueuesTheHourlyBucketWhenTheLastReadingLandsOnAnHourBoundary
// proves the widening is load-bearing. A run's LAST persisted reading sits
// exactly on an hour boundary, below consumption_hourly's real-time
// watermark (moved forward first, exactly like aggregates_integration_test.go's
// TestRefreshMaterialisesAnHourBelowTheWatermark), and drives the REAL
// enqueue -> REAL refresher. Before refreshRangeFor widened the payload's
// To by one microsecond, RefreshConsumption's own
// energy.Bucket(Hourly, p.To.Add(-time.Nanosecond), loc).To trick would land
// back in the PREVIOUS hour, and the bucket that starts at the boundary
// reading's own hour — the one containing it as its first reading — would
// never be refreshed.
func TestFetchEnqueuesTheHourlyBucketWhenTheLastReadingLandsOnAnHourBoundary(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fx := fetchRefreshTestSetup(t, 9142)

	analyticsRepo := postgres.NewAnalyticsRepository(fx.pool)
	aggregateRepo := admin.NewAggregateRepository(fx.pool)

	now := time.Now().UTC()
	clk := clock.NewFake(now)

	// Move consumption_hourly's real-time watermark forward, exactly like
	// TestRefreshMaterialisesAnHourBelowTheWatermark: one recent reading,
	// then an explicit refresh reaching to ~now-1h.
	recentAt := now.Add(-2 * time.Hour)
	_, _, err := fx.repos.readings.BulkInsert(ctx, fx.tenant.Scope, []model.MeterReading{
		readingFor(fx.analyzer.ID, recentAt.Format(time.RFC3339), model.ReadingKindLoadProfile, map[string]string{"active_import": "1"}),
	})
	require.NoError(t, err)
	require.NoError(t, aggregateRepo.Refresh(ctx, store.ViewConsumptionHourly,
		store.TimeRange{From: now.Add(-400 * 24 * time.Hour), To: now.Add(-time.Hour)}))

	// A backfill ~200 days back whose run-ending reading sits exactly on an
	// hour boundary.
	base := now.Add(-200 * 24 * time.Hour).Truncate(time.Hour)
	firstAt := base.Add(-30 * time.Minute)

	src := newFakeAdapter(integration.ProviderOSOS, 40*24*time.Hour, model.ReadingKindLoadProfile)
	refreshEnq := newRecordingConsumptionRefreshEnqueuer()
	svc := fetchRefreshTestService(t, fx.repos, fx.creds, src, refreshEnq, clk, ingest.Options{ConsumptionRefreshEnabled: true})

	rows := []model.MeterReading{
		readingFor(fx.analyzer.ID, firstAt.Format(time.RFC3339), model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"}),
		readingFor(fx.analyzer.ID, base.Format(time.RFC3339), model.ReadingKindLoadProfile, map[string]string{"active_import": "1012.5"}),
	}
	src.setSteps(fetchStep{result: integration.FetchResult{Readings: rows}})

	window := &job.Window{From: firstAt.Add(-time.Hour), To: base.Add(time.Hour)}
	payload := job.FetchReadingsPayload{CompanyID: fx.tenant.Company.ID, CredentialID: fx.creds.CredentialID, AnalyzerID: fx.analyzer.ID, Kind: model.ReadingKindLoadProfile, Window: window}
	require.NoError(t, svc.FetchReadings(ctx, payload))

	calls := refreshEnq.enqueued()
	require.Len(t, calls, 1)
	require.True(t, calls[0].To.Equal(base.Add(time.Microsecond)),
		"enqueued To must be affectedTo (the boundary reading) widened by exactly one microsecond, got %s", calls[0].To)

	hourWindow := store.TimeRange{From: base, To: base.Add(time.Hour)}
	before, err := analyticsRepo.ConsumptionHourly(ctx, fx.tenant.AdminScope, []uuid.UUID{fx.analyzer.ID}, hourWindow)
	require.NoError(t, err)
	require.Empty(t, before, "the hour starting at the boundary reading is below the watermark and absent until refreshed")

	// R100(3): 200 days is comfortably past consumption_hourly's own 30-day
	// policy horizon regardless of exactly which "now" is used, so reusing
	// clk here (the same fake clock the ingest side above is wired to) is
	// safe — unlike the monthly-view tests in this file, no separate,
	// further-future clock is needed for this assertion to hold.
	refresher, err := consumption.NewRefresher(consumption.RefreshDeps{
		Aggregates: aggregateRepo,
		Locker:     fetchRefreshTestRedisLocker(t),
		Enqueuer:   newRecordingConsumptionRefreshEnqueuer(),
		Clock:      clk,
		LockTTL:    2 * time.Minute,
		Location:   normalize.Istanbul,
		Log:        testfixtures.DiscardLogger(),
	})
	require.NoError(t, err)
	require.NoError(t, refresher.RefreshConsumption(ctx, calls[0]))

	after, err := analyticsRepo.ConsumptionHourly(ctx, fx.tenant.AdminScope, []uuid.UUID{fx.analyzer.ID}, hourWindow)
	require.NoError(t, err)
	require.Len(t, after, 1, "the hour bucket containing the boundary reading as its first reading must be materialised after the refresh")
}

// TestFetchEnqueuesExactlyOneRefreshForASingleReadingRun proves that a
// run whose only persisted reading makes affectedFrom == affectedTo must
// still enqueue a valid job.ConsumptionRefreshPayload (From < To) — not
// silently swallow the enqueue via job.NewConsumptionRefreshTask's own
// "From must be before To" rejection, which an enqueue failure only ever
// logs as a warning. It also runs the real refresher against the enqueued
// payload and confirms the reading's (closed) month is materialised —
// cheap to add alongside the unit-level assertion since this file already
// drives the real pipeline.
func TestFetchEnqueuesExactlyOneRefreshForASingleReadingRun(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fx := fetchRefreshTestSetup(t, 9143)

	analyticsRepo := postgres.NewAnalyticsRepository(fx.pool)
	aggregateRepo := admin.NewAggregateRepository(fx.pool)

	clk := clock.NewFake(fetchRefreshTestNow)
	src := newFakeAdapter(integration.ProviderOSOS, 20*24*time.Hour, model.ReadingKindLoadProfile)
	refreshEnq := newRecordingConsumptionRefreshEnqueuer()
	svc := fetchRefreshTestService(t, fx.repos, fx.creds, src, refreshEnq, clk, ingest.Options{ConsumptionRefreshEnabled: true})

	// fetchRefreshTestNow is June 2026; 40 days back lands in May 2026 — a
	// month unambiguously closed relative to the real wall clock this test
	// actually runs under, same reasoning
	// TestABackfillTwoHundredDaysOldEntersTheAggregateAfterTheJobRuns uses
	// for February.
	at := fetchRefreshTestNow.Add(-40 * 24 * time.Hour)
	window := ingestTestWindow(at.Add(-time.Hour), 2*time.Hour)
	rows := []model.MeterReading{readingFor(fx.analyzer.ID, at.Format(time.RFC3339), model.ReadingKindLoadProfile, map[string]string{"active_import": "1000"})}
	src.setSteps(fetchStep{result: integration.FetchResult{Readings: rows}})

	payload := job.FetchReadingsPayload{CompanyID: fx.tenant.Company.ID, CredentialID: fx.creds.CredentialID, AnalyzerID: fx.analyzer.ID, Kind: model.ReadingKindLoadProfile, Window: window}
	require.NoError(t, svc.FetchReadings(ctx, payload))

	calls := refreshEnq.enqueued()
	require.Len(t, calls, 1, "a single-reading run must still enqueue exactly one valid refresh")
	require.True(t, calls[0].From.Equal(at))
	require.True(t, calls[0].To.Equal(at.Add(time.Microsecond)), "want To == From + 1 microsecond, got %s", calls[0].To)
	require.True(t, calls[0].From.Before(calls[0].To), "the enqueued payload must satisfy From < To")

	monthStart := time.Date(at.Year(), at.Month(), 1, 0, 0, 0, 0, normalize.Istanbul)
	monthWindow := store.TimeRange{From: monthStart.Add(-24 * time.Hour), To: monthStart.AddDate(0, 1, 0).Add(24 * time.Hour)}

	before, err := analyticsRepo.ConsumptionMonthly(ctx, fx.tenant.AdminScope, []uuid.UUID{fx.analyzer.ID}, monthWindow)
	require.NoError(t, err)
	require.Empty(t, before, "the closed month is absent until consumption.refresh actually runs")

	// R100(3): same reasoning as
	// TestABackfillTwoHundredDaysOldEntersTheAggregateAfterTheJobRuns above
	// — `at` is only 40 days before fetchRefreshTestNow, well inside
	// consumption_monthly's own 1-year policy window, so the refresher here
	// needs a clock more than a year past `at` for the manual refresh to
	// actually touch the monthly view.
	refresher, err := consumption.NewRefresher(consumption.RefreshDeps{
		Aggregates: aggregateRepo,
		Locker:     fetchRefreshTestRedisLocker(t),
		Enqueuer:   newRecordingConsumptionRefreshEnqueuer(),
		Clock:      clock.NewFake(fetchRefreshTestNow.AddDate(2, 0, 0)),
		LockTTL:    2 * time.Minute,
		Location:   normalize.Istanbul,
		Log:        testfixtures.DiscardLogger(),
	})
	require.NoError(t, err)
	require.NoError(t, refresher.RefreshConsumption(ctx, calls[0]))

	after, err := analyticsRepo.ConsumptionMonthly(ctx, fx.tenant.AdminScope, []uuid.UUID{fx.analyzer.ID}, monthWindow)
	require.NoError(t, err)
	require.Len(t, after, 1, "the single reading's month must be materialised after the refresh")
}

// ---------------------------------------------------------------------------
// failFetchRun on a cancelled context.
// ---------------------------------------------------------------------------

// cancelSensitiveConsumptionRefreshEnqueuer is a fixture:
// recordingConsumptionRefreshEnqueuer ignores ctx entirely, which cannot
// tell "enqueued with a live context" apart from "enqueued with the run's
// own, already-cancelled context". This records each call's ctx.Err() at
// call time alongside its payload.
type cancelSensitiveConsumptionRefreshEnqueuer struct {
	mu      sync.Mutex
	calls   []job.ConsumptionRefreshPayload
	ctxErrs []error
}

func (e *cancelSensitiveConsumptionRefreshEnqueuer) EnqueueConsumptionRefresh(ctx context.Context, p job.ConsumptionRefreshPayload) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.calls = append(e.calls, p)
	e.ctxErrs = append(e.ctxErrs, ctx.Err())
	return nil
}

var _ ingest.ConsumptionRefreshEnqueuer = (*cancelSensitiveConsumptionRefreshEnqueuer)(nil)

// TestFailFetchRunEnqueuesWithAnUncancelledContextAfterCancellation proves
// the enqueue survives run cancellation. The run's own ctx is cancelled
// right as page 2 is requested — after page 1 has already persisted rows
// past the threshold — forcing failFetchRun down the partial-failure path.
// It must still enqueue exactly one refresh for what page 1 persisted, and
// it must NOT be given the run's own (now cancelled) context, since
// enqueueing with a cancelled ctx would otherwise fail. A mutant that skips
// the enqueue when ctx.Err() != nil, or that passes ctx straight through
// instead of a context.WithoutCancel-derived one, turns this red.
func TestFailFetchRunEnqueuesWithAnUncancelledContextAfterCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	fx := fetchRefreshTestSetup(t, 9144)

	clk := clock.NewFake(fetchRefreshTestNow)
	src := newFakeAdapter(integration.ProviderOSOS, 41*24*time.Hour, model.ReadingKindLoadProfile)
	refreshEnq := &cancelSensitiveConsumptionRefreshEnqueuer{}
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
		// before fires exactly when page 2 is requested — after page 1's
		// rows are already fully persisted under a still-live ctx.
		fetchStep{
			before: cancel,
			err:    &integration.Error{Kind: integration.ErrUpstreamUnavailable, Provider: integration.ProviderOSOS, Op: "load_profiles"},
		},
	)

	window := &job.Window{From: chunkFrom, To: chunkTo}
	payload := job.FetchReadingsPayload{CompanyID: fx.tenant.Company.ID, CredentialID: fx.creds.CredentialID, AnalyzerID: fx.analyzer.ID, Kind: model.ReadingKindLoadProfile, Window: window}
	require.Error(t, svc.FetchReadings(ctx, payload), "the run must still report the page-2 failure")

	require.Len(t, refreshEnq.calls, 1, "a run cancelled mid-way must still enqueue for what it already persisted")
	require.True(t, refreshEnq.calls[0].From.Equal(page1From))
	require.True(t, refreshEnq.calls[0].To.Equal(page1To.Add(time.Microsecond)))
	require.NoError(t, refreshEnq.ctxErrs[0], "the enqueue must not be given the run's own cancelled context")
}
