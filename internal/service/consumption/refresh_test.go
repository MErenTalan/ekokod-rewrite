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

// fakeLease records its own release against rec.
type fakeLease struct {
	key string
	rec *recorder
}

func (l *fakeLease) Release(context.Context) error {
	l.rec.add("release:" + l.key)
	return nil
}

// fakeLocker records every Acquire against rec and fails (ErrNotAcquired)
// for any key in failKeys.
type fakeLocker struct {
	rec       *recorder
	failKeys  map[string]bool
	acquireMu sync.Mutex
}

func (f *fakeLocker) Acquire(_ context.Context, key string, _ time.Duration) (lock.Lease, error) {
	f.acquireMu.Lock()
	defer f.acquireMu.Unlock()
	if f.failKeys[key] {
		return nil, lock.ErrNotAcquired
	}
	f.rec.add("acquire:" + key)
	return &fakeLease{key: key, rec: f.rec}, nil
}

// fakeAggregates records every Refresh call, in order, against rec, and can
// be configured to fail for one view.
type fakeAggregates struct {
	rec      *recorder
	mu       sync.Mutex
	calls    []fakeRefreshCall
	failView store.AggregateView
	failErr  error
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

func validRefreshDeps(agg *fakeAggregates, locker lock.Locker) consumption.RefreshDeps {
	return consumption.RefreshDeps{
		Aggregates: agg,
		Locker:     locker,
		LockTTL:    time.Minute,
		Location:   testIstanbul,
		Log:        discardLog(),
	}
}

// TestNewRefresherValidatesDeps proves every field is required.
func TestNewRefresherValidatesDeps(t *testing.T) {
	agg := &fakeAggregates{}
	locker := &fakeLocker{rec: &recorder{}}
	valid := validRefreshDeps(agg, locker)

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
	r, err := consumption.NewRefresher(validRefreshDeps(agg, locker))
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
// another level's expansion.
func TestRefreshConsumptionExpandsEachViewToItsOwnBuckets(t *testing.T) {
	agg := &fakeAggregates{}
	locker := &fakeLocker{rec: &recorder{}}
	r, err := consumption.NewRefresher(validRefreshDeps(agg, locker))
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

// TestRefreshConsumptionHoldsALockAroundEachViewsRefresh proves a lock is
// acquired immediately before, and released immediately after, each view's
// own Refresh call — never held across views, never released early.
func TestRefreshConsumptionHoldsALockAroundEachViewsRefresh(t *testing.T) {
	rec := &recorder{}
	agg := &fakeAggregates{rec: rec}
	locker := &fakeLocker{rec: rec}
	r, err := consumption.NewRefresher(validRefreshDeps(agg, locker))
	require.NoError(t, err)

	from := time.Date(2025, 6, 1, 0, 0, 0, 0, testIstanbul)
	to := from.Add(time.Hour)
	payload := job.ConsumptionRefreshPayload{CompanyID: uuid.New(), From: from, To: to}

	require.NoError(t, r.RefreshConsumption(context.Background(), payload))

	require.Equal(t, []string{
		"acquire:consumption.refresh:consumption_hourly",
		"refresh:consumption_hourly",
		"release:consumption.refresh:consumption_hourly",
		"acquire:consumption.refresh:consumption_daily",
		"refresh:consumption_daily",
		"release:consumption.refresh:consumption_daily",
		"acquire:consumption.refresh:consumption_monthly",
		"refresh:consumption_monthly",
		"release:consumption.refresh:consumption_monthly",
		"acquire:consumption.refresh:consumption_yearly",
		"refresh:consumption_yearly",
		"release:consumption.refresh:consumption_yearly",
	}, rec.all())
}

// TestRefreshConsumptionLockNotAcquiredReturnsErrorAndSkipsRefresh proves
// that when Acquire fails with lock.ErrNotAcquired, RefreshConsumption
// returns an error wrapping it and never calls Refresh for that view.
func TestRefreshConsumptionLockNotAcquiredReturnsErrorAndSkipsRefresh(t *testing.T) {
	rec := &recorder{}
	agg := &fakeAggregates{rec: rec}
	locker := &fakeLocker{rec: rec, failKeys: map[string]bool{"consumption.refresh:consumption_hourly": true}}
	r, err := consumption.NewRefresher(validRefreshDeps(agg, locker))
	require.NoError(t, err)

	from := time.Date(2025, 6, 1, 0, 0, 0, 0, testIstanbul)
	to := from.Add(time.Hour)
	payload := job.ConsumptionRefreshPayload{CompanyID: uuid.New(), From: from, To: to}

	err = r.RefreshConsumption(context.Background(), payload)
	require.Error(t, err)
	require.ErrorIs(t, err, lock.ErrNotAcquired)
	require.Empty(t, agg.calls, "Refresh must never be called for a view whose lock was not acquired")
	require.False(t, errors.Is(err, asynq.SkipRetry), "a lock failure is retryable, not a permanent skip")
}

// TestRefreshConsumptionErrorOnOneViewAbortsTheRestButReleasesItsLease
// proves a refresh error on the daily view stops monthly/yearly from
// running, while the daily lease is still released.
func TestRefreshConsumptionErrorOnOneViewAbortsTheRestButReleasesItsLease(t *testing.T) {
	rec := &recorder{}
	failErr := errors.New("boom: daily refresh failed")
	agg := &fakeAggregates{rec: rec, failView: store.ViewConsumptionDaily, failErr: failErr}
	locker := &fakeLocker{rec: rec}
	r, err := consumption.NewRefresher(validRefreshDeps(agg, locker))
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

	require.Contains(t, rec.all(), "release:consumption.refresh:consumption_daily",
		"the daily lease must be released even though its own refresh failed")
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
			r, err := consumption.NewRefresher(validRefreshDeps(agg, locker))
			require.NoError(t, err)

			err = r.RefreshConsumption(context.Background(), payload)
			require.Error(t, err)
			require.ErrorIs(t, err, asynq.SkipRetry)
			require.Empty(t, agg.calls)
		})
	}
}
