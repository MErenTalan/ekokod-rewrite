package consumption_test

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/lock"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/consumption"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

func discardLog() *slog.Logger { return slog.New(slog.DiscardHandler) }

var testIstanbul = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		panic(err)
	}
	return loc
}()

// recorder is a concurrency-safe append-only event log shared between
// fakeLocker and fakeAggregates so a test can assert the interleaved
// acquire/refresh/release order across every view in one place.
type recorder struct {
	mu     sync.Mutex
	events []string
}

func (r *recorder) add(s string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, s)
}

func (r *recorder) all() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.events...)
}

// fakeLease records its own release against rec, and (for the
// context-propagation test) the ctx.Err() it was released with.
type fakeLease struct {
	key string
	rec *recorder

	mu          sync.Mutex
	releaseCtxs []error
}

func (l *fakeLease) Release(ctx context.Context) error {
	l.rec.add("release:" + l.key)
	l.mu.Lock()
	l.releaseCtxs = append(l.releaseCtxs, ctx.Err())
	l.mu.Unlock()
	return nil
}

// fakeLocker records every Acquire against rec and fails (ErrNotAcquired)
// for any key in failKeys.
type fakeLocker struct {
	rec       *recorder
	failKeys  map[string]bool
	acquireMu sync.Mutex
	lastLease *fakeLease
}

func (f *fakeLocker) Acquire(_ context.Context, key string, _ time.Duration) (lock.Lease, error) {
	f.acquireMu.Lock()
	defer f.acquireMu.Unlock()
	if f.failKeys[key] {
		return nil, lock.ErrNotAcquired
	}
	f.rec.add("acquire:" + key)
	lease := &fakeLease{key: key, rec: f.rec}
	f.lastLease = lease
	return lease, nil
}

// fakeAggregates records every Refresh call, in order, against rec, and can
// be configured to fail for one view or to run a hook (onRefresh) — used by
// the cancelled-context test to cancel the handler's own ctx mid-refresh.
type fakeAggregates struct {
	rec       *recorder
	mu        sync.Mutex
	calls     []fakeRefreshCall
	failView  store.AggregateView
	failErr   error
	onRefresh func(view store.AggregateView)
}

type fakeRefreshCall struct {
	view store.AggregateView
	rng  store.TimeRange
}

func (f *fakeAggregates) Refresh(_ context.Context, view store.AggregateView, r store.TimeRange) error {
	f.mu.Lock()
	f.calls = append(f.calls, fakeRefreshCall{view: view, rng: r})
	f.mu.Unlock()
	if f.rec != nil {
		f.rec.add("refresh:" + string(view))
	}
	if f.onRefresh != nil {
		f.onRefresh(view)
	}
	if view == f.failView && f.failErr != nil {
		return f.failErr
	}
	return nil
}

func (f *fakeAggregates) viewsCalled() []store.AggregateView {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]store.AggregateView, len(f.calls))
	for i, c := range f.calls {
		out[i] = c.view
	}
	return out
}

func (f *fakeAggregates) rangeFor(view store.AggregateView) (store.TimeRange, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.calls {
		if c.view == view {
			return c.rng, true
		}
	}
	return store.TimeRange{}, false
}

// fakeEnqueuer is consumption.Enqueuer's test double: it only ever RECORDS
// the payload it is asked to re-enqueue — it never itself calls back into
// RefreshConsumption, which is what keeps every contention test in this
// file from being able to loop forever even by accident.
type fakeEnqueuer struct {
	mu    sync.Mutex
	calls []job.ConsumptionRefreshPayload
	err   error
}

func (f *fakeEnqueuer) EnqueueConsumptionRefresh(_ context.Context, p job.ConsumptionRefreshPayload) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, p)
	return f.err
}

func (f *fakeEnqueuer) calledWith() []job.ConsumptionRefreshPayload {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]job.ConsumptionRefreshPayload(nil), f.calls...)
}

// farFutureNow is a clock reading far enough past any test window below
// that NO view's policy horizon (even yearly's 5 years) ever clips it — the
// tests that assert exact, unclipped bucket ranges (finest-first order,
// per-view bucket expansion, lock ordering, error propagation) use this so
// R100(3)'s horizon clipping never interferes with what they are actually
// testing.
var farFutureNow = time.Date(2040, 1, 1, 0, 0, 0, 0, time.UTC)

func validRefreshDeps(agg *fakeAggregates, locker lock.Locker, enq consumption.Enqueuer, now time.Time) consumption.RefreshDeps {
	return consumption.RefreshDeps{
		Aggregates: agg,
		Locker:     locker,
		Enqueuer:   enq,
		Clock:      clock.NewFake(now),
		LockTTL:    time.Minute,
		Location:   testIstanbul,
		Log:        discardLog(),
	}
}

// TestNewRefresherValidatesDeps proves every field is required.
func TestNewRefresherValidatesDeps(t *testing.T) {
	agg := &fakeAggregates{}
	locker := &fakeLocker{rec: &recorder{}}
	enq := &fakeEnqueuer{}
	valid := validRefreshDeps(agg, locker, enq, farFutureNow)

	t.Run("valid deps succeed", func(t *testing.T) {
		r, err := consumption.NewRefresher(valid)
		require.NoError(t, err)
		require.NotNil(t, r)
	})

	t.Run("nil Aggregates", func(t *testing.T) {
		d := valid
		d.Aggregates = nil
		_, err := consumption.NewRefresher(d)
		require.Error(t, err)
	})

	t.Run("nil Locker", func(t *testing.T) {
		d := valid
		d.Locker = nil
		_, err := consumption.NewRefresher(d)
		require.Error(t, err)
	})

	t.Run("nil Enqueuer", func(t *testing.T) {
		d := valid
		d.Enqueuer = nil
		_, err := consumption.NewRefresher(d)
		require.Error(t, err)
	})

	t.Run("nil Clock", func(t *testing.T) {
		d := valid
		d.Clock = nil
		_, err := consumption.NewRefresher(d)
		require.Error(t, err)
	})

	t.Run("zero LockTTL", func(t *testing.T) {
		d := valid
		d.LockTTL = 0
		_, err := consumption.NewRefresher(d)
		require.Error(t, err)
	})

	t.Run("negative LockTTL", func(t *testing.T) {
		d := valid
		d.LockTTL = -time.Second
		_, err := consumption.NewRefresher(d)
		require.Error(t, err)
	})

	t.Run("nil Location", func(t *testing.T) {
		d := valid
		d.Location = nil
		_, err := consumption.NewRefresher(d)
		require.Error(t, err)
	})

	t.Run("nil Log", func(t *testing.T) {
		d := valid
		d.Log = nil
		_, err := consumption.NewRefresher(d)
		require.Error(t, err)
	})
}

// TestRefreshConsumptionRefreshesViewsFinestFirst proves the four views are
// refreshed in exactly hourly, daily, monthly, yearly order.
func TestRefreshConsumptionRefreshesViewsFinestFirst(t *testing.T) {
	agg := &fakeAggregates{}
	locker := &fakeLocker{rec: &recorder{}}
	r, err := consumption.NewRefresher(validRefreshDeps(agg, locker, &fakeEnqueuer{}, farFutureNow))
	require.NoError(t, err)

	from := time.Date(2025, 3, 14, 10, 10, 0, 0, testIstanbul)
	to := time.Date(2025, 3, 14, 10, 50, 0, 0, testIstanbul)
	payload := job.ConsumptionRefreshPayload{CompanyID: uuid.New(), AnalyzerID: uuid.New(), From: from, To: to}

	require.NoError(t, r.RefreshConsumption(context.Background(), payload))

	require.Equal(t, []store.AggregateView{
		store.ViewConsumptionHourly,
		store.ViewConsumptionDaily,
		store.ViewConsumptionMonthly,
		store.ViewConsumptionYearly,
	}, agg.viewsCalled())
}

// TestRefreshConsumptionExpandsEachViewToItsOwnBuckets proves each view
// receives its own level-expanded range, not the raw payload window and not
// another level's expansion, when now is far enough away that R100(3)'s
// policy-horizon clip never trims it.
func TestRefreshConsumptionExpandsEachViewToItsOwnBuckets(t *testing.T) {
	agg := &fakeAggregates{}
	locker := &fakeLocker{rec: &recorder{}}
	r, err := consumption.NewRefresher(validRefreshDeps(agg, locker, &fakeEnqueuer{}, farFutureNow))
	require.NoError(t, err)

	from := time.Date(2025, 3, 14, 10, 10, 0, 0, testIstanbul)
	to := time.Date(2025, 3, 14, 10, 50, 0, 0, testIstanbul)
	payload := job.ConsumptionRefreshPayload{CompanyID: uuid.New(), AnalyzerID: uuid.New(), From: from, To: to}

	require.NoError(t, r.RefreshConsumption(context.Background(), payload))
	require.Len(t, agg.calls, 4)

	want := map[store.AggregateView]store.TimeRange{
		store.ViewConsumptionHourly: {
			From: time.Date(2025, 3, 14, 10, 0, 0, 0, testIstanbul),
			To:   time.Date(2025, 3, 14, 11, 0, 0, 0, testIstanbul),
		},
		store.ViewConsumptionDaily: {
			From: time.Date(2025, 3, 14, 0, 0, 0, 0, testIstanbul),
			To:   time.Date(2025, 3, 15, 0, 0, 0, 0, testIstanbul),
		},
		store.ViewConsumptionMonthly: {
			From: time.Date(2025, 3, 1, 0, 0, 0, 0, testIstanbul),
			To:   time.Date(2025, 4, 1, 0, 0, 0, 0, testIstanbul),
		},
		store.ViewConsumptionYearly: {
			From: time.Date(2025, 1, 1, 0, 0, 0, 0, testIstanbul),
			To:   time.Date(2026, 1, 1, 0, 0, 0, 0, testIstanbul),
		},
	}

	for _, call := range agg.calls {
		wantRange, ok := want[call.view]
		require.True(t, ok, "unexpected view %s", call.view)
		require.True(t, wantRange.From.Equal(call.rng.From), "%s From: want %v, got %v", call.view, wantRange.From, call.rng.From)
		require.True(t, wantRange.To.Equal(call.rng.To), "%s To: want %v, got %v", call.view, wantRange.To, call.rng.To)
	}
}

// TestRefreshConsumptionHoldsOneGlobalLockAroundTheWholeRefresh proves
// R100(4): a SINGLE "consumption.refresh" lock is acquired once, before any
// view is touched, and released once, after every view has run — never one
// lock per view.
func TestRefreshConsumptionHoldsOneGlobalLockAroundTheWholeRefresh(t *testing.T) {
	rec := &recorder{}
	agg := &fakeAggregates{rec: rec}
	locker := &fakeLocker{rec: rec}
	r, err := consumption.NewRefresher(validRefreshDeps(agg, locker, &fakeEnqueuer{}, farFutureNow))
	require.NoError(t, err)

	from := time.Date(2025, 6, 1, 0, 0, 0, 0, testIstanbul)
	to := from.Add(time.Hour)
	payload := job.ConsumptionRefreshPayload{CompanyID: uuid.New(), From: from, To: to}

	require.NoError(t, r.RefreshConsumption(context.Background(), payload))

	require.Equal(t, []string{
		"acquire:consumption.refresh",
		"refresh:consumption_hourly",
		"refresh:consumption_daily",
		"refresh:consumption_monthly",
		"refresh:consumption_yearly",
		"release:consumption.refresh",
	}, rec.all())
}

// TestRefreshConsumptionContentionReEnqueuesAndReturnsNil proves R100(4):
// when the global lock cannot be acquired (lock.ErrNotAcquired), the
// handler re-enqueues the SAME payload through deps.Enqueuer and returns
// nil — no error, no retry consumed, and no view is ever touched.
func TestRefreshConsumptionContentionReEnqueuesAndReturnsNil(t *testing.T) {
	agg := &fakeAggregates{}
	locker := &fakeLocker{rec: &recorder{}, failKeys: map[string]bool{"consumption.refresh": true}}
	enq := &fakeEnqueuer{}
	r, err := consumption.NewRefresher(validRefreshDeps(agg, locker, enq, farFutureNow))
	require.NoError(t, err)

	from := time.Date(2025, 6, 1, 0, 0, 0, 0, testIstanbul)
	to := from.Add(time.Hour)
	payload := job.ConsumptionRefreshPayload{CompanyID: uuid.New(), AnalyzerID: uuid.New(), From: from, To: to}

	err = r.RefreshConsumption(context.Background(), payload)
	require.NoError(t, err, "lock contention must return nil, never an error — it must never consume asynq's retry budget")
	require.Empty(t, agg.calls, "no view may be touched when the global lock was not acquired")

	calls := enq.calledWith()
	require.Len(t, calls, 1)
	require.Equal(t, payload.CompanyID, calls[0].CompanyID)
	require.Equal(t, payload.AnalyzerID, calls[0].AnalyzerID)
	require.True(t, payload.From.Equal(calls[0].From))
	require.True(t, payload.To.Equal(calls[0].To))
}

// TestRefreshConsumptionContentionSurfacesAReEnqueueFailure proves that if
// the re-enqueue itself fails, RefreshConsumption returns a plain
// (retryable) error rather than silently swallowing the failure and
// returning nil — the one case where losing the window entirely would be
// worse than falling back to asynq's ordinary retry.
func TestRefreshConsumptionContentionSurfacesAReEnqueueFailure(t *testing.T) {
	agg := &fakeAggregates{}
	locker := &fakeLocker{rec: &recorder{}, failKeys: map[string]bool{"consumption.refresh": true}}
	reEnqueueErr := errors.New("boom: re-enqueue failed")
	enq := &fakeEnqueuer{err: reEnqueueErr}
	r, err := consumption.NewRefresher(validRefreshDeps(agg, locker, enq, farFutureNow))
	require.NoError(t, err)

	from := time.Date(2025, 6, 1, 0, 0, 0, 0, testIstanbul)
	to := from.Add(time.Hour)
	payload := job.ConsumptionRefreshPayload{CompanyID: uuid.New(), From: from, To: to}

	err = r.RefreshConsumption(context.Background(), payload)
	require.Error(t, err)
	require.ErrorIs(t, err, reEnqueueErr)
	require.False(t, errors.Is(err, asynq.SkipRetry), "a failed re-enqueue is retryable, not a permanent skip")
}

// TestRefreshConsumptionReleasesLockWithAnUncancelledContext proves R100(5):
// the lease is released through a context.WithoutCancel-derived context —
// cancelling the handler's OWN ctx mid-refresh must never stop the release
// from going through, and the release must observe a live (non-cancelled)
// context.
func TestRefreshConsumptionReleasesLockWithAnUncancelledContext(t *testing.T) {
	rec := &recorder{}
	locker := &fakeLocker{rec: rec}
	ctx, cancel := context.WithCancel(context.Background())
	agg := &fakeAggregates{rec: rec, onRefresh: func(view store.AggregateView) {
		if view == store.ViewConsumptionHourly {
			cancel()
		}
	}}
	r, err := consumption.NewRefresher(validRefreshDeps(agg, locker, &fakeEnqueuer{}, farFutureNow))
	require.NoError(t, err)

	from := time.Date(2025, 6, 1, 0, 0, 0, 0, testIstanbul)
	to := from.Add(time.Hour)
	payload := job.ConsumptionRefreshPayload{CompanyID: uuid.New(), From: from, To: to}

	require.NoError(t, r.RefreshConsumption(ctx, payload))

	require.NotNil(t, locker.lastLease)
	locker.lastLease.mu.Lock()
	defer locker.lastLease.mu.Unlock()
	require.Len(t, locker.lastLease.releaseCtxs, 1)
	require.NoError(t, locker.lastLease.releaseCtxs[0],
		"Release must be called with an uncancelled context even though the handler's own ctx was cancelled mid-refresh")
}

// TestRefreshConsumptionErrorOnOneViewAbortsTheRestButReleasesTheLock
// proves a refresh error on the daily view stops monthly/yearly from
// running, while the single global lease is still released.
func TestRefreshConsumptionErrorOnOneViewAbortsTheRestButReleasesTheLock(t *testing.T) {
	rec := &recorder{}
	failErr := errors.New("boom: daily refresh failed")
	agg := &fakeAggregates{rec: rec, failView: store.ViewConsumptionDaily, failErr: failErr}
	locker := &fakeLocker{rec: rec}
	r, err := consumption.NewRefresher(validRefreshDeps(agg, locker, &fakeEnqueuer{}, farFutureNow))
	require.NoError(t, err)

	from := time.Date(2025, 6, 1, 0, 0, 0, 0, testIstanbul)
	to := from.Add(time.Hour)
	payload := job.ConsumptionRefreshPayload{CompanyID: uuid.New(), From: from, To: to}

	err = r.RefreshConsumption(context.Background(), payload)
	require.Error(t, err)
	require.ErrorIs(t, err, failErr)

	require.Equal(t, []store.AggregateView{
		store.ViewConsumptionHourly,
		store.ViewConsumptionDaily,
	}, agg.viewsCalled(), "monthly and yearly must never be attempted once daily fails")

	require.Contains(t, rec.all(), "release:consumption.refresh",
		"the single global lease must be released even though a view's own refresh failed")
}

// TestRefreshConsumptionRejectsInvalidPayloadsWithSkipRetry proves an
// invalid payload (nil company, zero or reversed times) is rejected before
// any lock is acquired or any view is touched, wrapped with
// asynq.SkipRetry: no retry will ever make an invalid payload valid.
func TestRefreshConsumptionRejectsInvalidPayloadsWithSkipRetry(t *testing.T) {
	from := time.Date(2025, 6, 1, 0, 0, 0, 0, testIstanbul)
	to := from.Add(time.Hour)

	cases := map[string]job.ConsumptionRefreshPayload{
		"nil company":   {CompanyID: uuid.Nil, From: from, To: to},
		"zero From":     {CompanyID: uuid.New(), From: time.Time{}, To: to},
		"zero To":       {CompanyID: uuid.New(), From: from, To: time.Time{}},
		"From equal To": {CompanyID: uuid.New(), From: from, To: from},
		"From after To": {CompanyID: uuid.New(), From: to, To: from},
	}

	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			agg := &fakeAggregates{}
			locker := &fakeLocker{rec: &recorder{}}
			r, err := consumption.NewRefresher(validRefreshDeps(agg, locker, &fakeEnqueuer{}, farFutureNow))
			require.NoError(t, err)

			err = r.RefreshConsumption(context.Background(), payload)
			require.Error(t, err)
			require.ErrorIs(t, err, asynq.SkipRetry)
			require.Empty(t, agg.calls)
		})
	}
}

// ---------------------------------------------------------------------------
// R100(3): a view is refreshed only for the part of the window older than
// its own policy start_offset (hourly 30d, daily 90d, monthly 1y, yearly
// 5y), clipped to whole buckets; a view with nothing older is skipped
// entirely.
// ---------------------------------------------------------------------------

// refreshNow anchors every policy-horizon test below: windows are placed at
// fixed offsets from it, never from time.Now().
var refreshNow = time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)

// TestRefreshConsumptionSkipsEveryViewWhenWindowIsEntirelyInsideHourlyPolicy
// proves a window entirely inside the hourly policy window (< 30 days old,
// therefore also inside daily/monthly/yearly's wider windows) refreshes
// NOTHING: every view's scheduled policy already covers it live.
func TestRefreshConsumptionSkipsEveryViewWhenWindowIsEntirelyInsideHourlyPolicy(t *testing.T) {
	agg := &fakeAggregates{}
	locker := &fakeLocker{rec: &recorder{}}
	r, err := consumption.NewRefresher(validRefreshDeps(agg, locker, &fakeEnqueuer{}, refreshNow))
	require.NoError(t, err)

	from := refreshNow.Add(-5 * 24 * time.Hour)
	to := from.Add(2 * time.Hour)
	payload := job.ConsumptionRefreshPayload{CompanyID: uuid.New(), From: from, To: to}

	require.NoError(t, r.RefreshConsumption(context.Background(), payload))
	require.Empty(t, agg.calls, "a window entirely inside the hourly policy window must refresh nothing")
}

// TestRefreshConsumptionTwoHundredDayOldWindowRefreshesHourlyAndDailyOnly
// proves a 200-day-old window is older than hourly's (30d) and daily's
// (90d) horizons but NEWER than monthly's (1y) and yearly's (5y) — so only
// hourly and daily are refreshed.
func TestRefreshConsumptionTwoHundredDayOldWindowRefreshesHourlyAndDailyOnly(t *testing.T) {
	agg := &fakeAggregates{}
	locker := &fakeLocker{rec: &recorder{}}
	r, err := consumption.NewRefresher(validRefreshDeps(agg, locker, &fakeEnqueuer{}, refreshNow))
	require.NoError(t, err)

	from := refreshNow.Add(-200 * 24 * time.Hour)
	to := from.Add(2 * time.Hour)
	payload := job.ConsumptionRefreshPayload{CompanyID: uuid.New(), From: from, To: to}

	require.NoError(t, r.RefreshConsumption(context.Background(), payload))
	require.Equal(t, []store.AggregateView{
		store.ViewConsumptionHourly,
		store.ViewConsumptionDaily,
	}, agg.viewsCalled(), "a 200-day-old window must refresh hourly and daily but not monthly or yearly")
}

// TestRefreshConsumptionTwoYearOldWindowRefreshesEverythingButYearly proves
// a 2-year-old window is older than hourly (30d), daily (90d) and monthly
// (1y) horizons but NEWER than yearly's (5y) — so hourly, daily and monthly
// refresh, and yearly is skipped.
func TestRefreshConsumptionTwoYearOldWindowRefreshesEverythingButYearly(t *testing.T) {
	agg := &fakeAggregates{}
	locker := &fakeLocker{rec: &recorder{}}
	r, err := consumption.NewRefresher(validRefreshDeps(agg, locker, &fakeEnqueuer{}, refreshNow))
	require.NoError(t, err)

	from := refreshNow.Add(-2 * 365 * 24 * time.Hour)
	to := from.Add(2 * time.Hour)
	payload := job.ConsumptionRefreshPayload{CompanyID: uuid.New(), From: from, To: to}

	require.NoError(t, r.RefreshConsumption(context.Background(), payload))
	require.Equal(t, []store.AggregateView{
		store.ViewConsumptionHourly,
		store.ViewConsumptionDaily,
		store.ViewConsumptionMonthly,
	}, agg.viewsCalled(), "a 2-year-old window must refresh hourly, daily and monthly but not yearly")
}

// TestRefreshConsumptionClipsAWindowStraddlingTheHourlyHorizon proves the
// "clipped to whole buckets" half of R100(3): a window spanning from 40
// days ago to 10 days ago straddles the hourly policy's 30-day horizon —
// the refreshed range must stop at the horizon's own hour bucket, not
// extend all the way to the window's original (much newer) To.
func TestRefreshConsumptionClipsAWindowStraddlingTheHourlyHorizon(t *testing.T) {
	agg := &fakeAggregates{}
	locker := &fakeLocker{rec: &recorder{}}
	r, err := consumption.NewRefresher(validRefreshDeps(agg, locker, &fakeEnqueuer{}, refreshNow))
	require.NoError(t, err)

	from := refreshNow.Add(-40 * 24 * time.Hour)
	to := refreshNow.Add(-10 * 24 * time.Hour)
	payload := job.ConsumptionRefreshPayload{CompanyID: uuid.New(), From: from, To: to}

	require.NoError(t, r.RefreshConsumption(context.Background(), payload))

	rng, ok := agg.rangeFor(store.ViewConsumptionHourly)
	require.True(t, ok, "the hourly view must still be refreshed for its older part")

	horizon := refreshNow.Add(-30 * 24 * time.Hour).In(testIstanbul).Truncate(time.Hour)
	require.True(t, rng.To.Equal(horizon), "hourly refresh must stop at the policy horizon's own bucket, want %v got %v", horizon, rng.To)
	require.True(t, rng.To.Before(to.In(testIstanbul)), "the clipped range must not reach the window's original (too recent) To")
}
