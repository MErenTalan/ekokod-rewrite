//go:build integration

package marketdata_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/normalize"
	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/marketdata"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/admin"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

// fakeJournal is an in-memory store.AdminJournalRepository.
//
// SPEC GAP (see task-12-report.md): the task-12 brief's own text for this
// file names `admin.NewJournalRepository`, the real Postgres-backed
// implementation. It does not exist on this task's base — Task 12's base
// is F2 Waves A+B only (Tasks 1-4), and store.AdminJournalRepository
// (job_runs/operational_messages writes with company_id NULL) is
// implemented by Task 11, in Wave D, which has not run yet; only
// audit.go, auth.go and marketdata.go exist under
// internal/store/postgres/admin today. internal/store/postgres/admin is
// also not this task's file ownership (it owns internal/integration/epias
// and internal/marketdata only) — writing journal.go here would collide
// with Task 11's own implementation at rebase time.
//
// This fake is the documented workaround: Syncer itself (sync.go) is
// written against the store.AdminJournalRepository INTERFACE only, so it
// is completely unaffected by which implementation a caller wires in: once
// this branch is rebased onto a base that has Task 11's
// admin.NewJournalRepository, that constructor drops in here unchanged and
// this fake (and this comment) can be deleted. Until then, this is what
// lets the market-data write path (admin.NewMarketDataRepository, real
// Postgres, via NewIsolatedDB) be genuinely integration-tested while the
// journal half is exercised against a working substitute rather than not
// exercised at all.
type fakeJournal struct {
	mu       sync.Mutex
	runs     map[uuid.UUID]model.JobRun
	messages []model.OperationalMessage
}

func newFakeJournal() *fakeJournal {
	return &fakeJournal{runs: make(map[uuid.UUID]model.JobRun)}
}

func (f *fakeJournal) StartPlatformRun(_ context.Context, run model.JobRun) (model.JobRun, error) {
	if run.CompanyID != nil {
		return model.JobRun{}, store.ErrNotFound
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	run.ID = uuid.New()
	run.Status = "running"
	f.runs[run.ID] = run
	return run, nil
}

func (f *fakeJournal) FinishPlatformRun(_ context.Context, id uuid.UUID, status string, processed, skipped, failed int32, errText *string, detail []byte, at time.Time) (model.JobRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	run, ok := f.runs[id]
	if !ok {
		return model.JobRun{}, store.ErrNotFound
	}
	run.Status = status
	run.Processed = processed
	run.Skipped = skipped
	run.Failed = failed
	run.Error = errText
	run.Detail = detail
	run.FinishedAt = &at
	f.runs[id] = run
	return run, nil
}

func (f *fakeJournal) AppendPlatformMessage(_ context.Context, m model.OperationalMessage) (model.OperationalMessage, error) {
	if m.CompanyID != nil {
		return model.OperationalMessage{}, store.ErrNotFound
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	m.ID = int64(len(f.messages) + 1)
	f.messages = append(f.messages, m)
	return m, nil
}

func (f *fakeJournal) lastRun() model.JobRun {
	f.mu.Lock()
	defer f.mu.Unlock()
	var latest model.JobRun
	for _, r := range f.runs {
		if r.StartedAt.After(latest.StartedAt) || latest.ID == uuid.Nil {
			latest = r
		}
	}
	return latest
}

func (f *fakeJournal) allMessages() []model.OperationalMessage {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]model.OperationalMessage, len(f.messages))
	copy(out, f.messages)
	return out
}

// fakePriceSource is a hand-rolled marketdata.PriceSource: HourlyPTF and
// YekdemUnitCost are supplied per test as plain functions so each test
// controls exactly what "EPİAŞ" returns without touching the network.
type fakePriceSource struct {
	hourly func(from, to time.Time) ([]model.MarketPrice, []integration.Warning, error)
	yekdem func(from, to time.Time) ([]model.YekdemMonthly, error)
}

func (f *fakePriceSource) HourlyPTF(_ context.Context, from, to time.Time) ([]model.MarketPrice, []integration.Warning, error) {
	return f.hourly(from, to)
}

func (f *fakePriceSource) YekdemUnitCost(_ context.Context, from, to time.Time) ([]model.YekdemMonthly, error) {
	if f.yekdem == nil {
		return nil, nil
	}
	return f.yekdem(from, to)
}

// fullDayPrices returns one model.MarketPrice per hour in [from, to).
func fullDayPrices(from, to time.Time) []model.MarketPrice {
	var prices []model.MarketPrice
	for h := from; h.Before(to); h = h.Add(time.Hour) {
		prices = append(prices, model.MarketPrice{Ts: h, PTF: decimal.RequireFromString("1000.0000")})
	}
	return prices
}

func fixedYekdem(_, _ time.Time) ([]model.YekdemMonthly, error) {
	return []model.YekdemMonthly{{Year: 2026, Month: 1, Value: decimal.RequireFromString("100.0000")}}, nil
}

// TestSyncPricesIsIdempotent: running the same window twice converges
// rather than duplicating, and the platform run rows carry no company —
// asserted here on the fakeJournal's own recorded run (see fakeJournal's
// doc comment for why AdminJournalRepository is a fake on this base).
func TestSyncPricesIsIdempotent(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	market := admin.NewMarketDataRepository(pool)
	journal := newFakeJournal()

	from := time.Date(2026, 1, 5, 0, 0, 0, 0, normalize.Istanbul).UTC()
	to := time.Date(2026, 1, 6, 0, 0, 0, 0, normalize.Istanbul).UTC()

	src := &fakePriceSource{
		hourly: func(f, t time.Time) ([]model.MarketPrice, []integration.Warning, error) {
			return fullDayPrices(f, t), nil, nil
		},
		yekdem: fixedYekdem,
	}
	syncer := marketdata.New(src, market, journal, clock.NewFake(to), testfixtures.DiscardLogger())

	ctx := context.Background()
	require.NoError(t, syncer.SyncPrices(ctx, job.SyncPricesPayload{Window: &job.Window{From: from, To: to}}))

	var n1 int
	require.NoError(t, pool.QueryRow(ctx, `select count(*) from market_prices_hourly`).Scan(&n1))
	require.Equal(t, 24, n1)

	require.NoError(t, syncer.SyncPrices(ctx, job.SyncPricesPayload{Window: &job.Window{From: from, To: to}}))

	var n2 int
	require.NoError(t, pool.QueryRow(ctx, `select count(*) from market_prices_hourly`).Scan(&n2))
	require.Equal(t, n1, n2, "re-syncing the same window must converge, not duplicate")

	run := journal.lastRun()
	require.Nil(t, run.CompanyID, "a platform run's company_id must be NULL")
	require.Equal(t, "success", run.Status)
}

// TestSyncPricesReportsMissingHoursWithoutFilling: 23 of 24 hours published
// → 23 rows written, one warning platform message, the run is partial, and
// no row exists for the missing hour — it is reported, never fabricated.
func TestSyncPricesReportsMissingHoursWithoutFilling(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	market := admin.NewMarketDataRepository(pool)
	journal := newFakeJournal()

	from := time.Date(2026, 1, 7, 0, 0, 0, 0, normalize.Istanbul).UTC()
	to := time.Date(2026, 1, 8, 0, 0, 0, 0, normalize.Istanbul).UTC()
	missingHour := from.Add(15 * time.Hour)

	src := &fakePriceSource{
		hourly: func(f, t time.Time) ([]model.MarketPrice, []integration.Warning, error) {
			all := fullDayPrices(f, t)
			out := all[:0]
			for _, p := range all {
				if p.Ts.Equal(missingHour) {
					continue
				}
				out = append(out, p)
			}
			return out, nil, nil
		},
		yekdem: fixedYekdem,
	}
	syncer := marketdata.New(src, market, journal, clock.NewFake(to), testfixtures.DiscardLogger())

	ctx := context.Background()
	require.NoError(t, syncer.SyncPrices(ctx, job.SyncPricesPayload{Window: &job.Window{From: from, To: to}}))

	var n int
	require.NoError(t, pool.QueryRow(ctx, `select count(*) from market_prices_hourly where ts >= $1 and ts < $2`, from, to).Scan(&n))
	require.Equal(t, 23, n)

	var missingCount int
	require.NoError(t, pool.QueryRow(ctx, `select count(*) from market_prices_hourly where ts = $1`, missingHour).Scan(&missingCount))
	require.Equal(t, 0, missingCount, "the missing hour must have no row at all, not a fabricated one")

	run := journal.lastRun()
	require.Equal(t, "partial", run.Status)

	messages := journal.allMessages()
	require.Len(t, messages, 1)
	require.Equal(t, "job", messages[0].Kind)
	require.Equal(t, "market-prices", messages[0].Category)
	require.Equal(t, "warning", messages[0].Status)
	require.Equal(t, "1 PTF hours missing", messages[0].Message)
}

// TestSyncPricesBackfillWindow: an explicit 60-day window writes every
// day, not just the default trailing window.
func TestSyncPricesBackfillWindow(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	market := admin.NewMarketDataRepository(pool)
	journal := newFakeJournal()

	from := time.Date(2020, 6, 1, 0, 0, 0, 0, normalize.Istanbul).UTC()
	to := time.Date(2020, 7, 31, 0, 0, 0, 0, normalize.Istanbul).UTC() // 60 days

	src := &fakePriceSource{
		hourly: func(f, t time.Time) ([]model.MarketPrice, []integration.Warning, error) {
			return fullDayPrices(f, t), nil, nil
		},
		yekdem: fixedYekdem,
	}
	syncer := marketdata.New(src, market, journal, clock.NewFake(to), testfixtures.DiscardLogger())

	ctx := context.Background()
	require.NoError(t, syncer.SyncPrices(ctx, job.SyncPricesPayload{Window: &job.Window{From: from, To: to}}))

	var n int
	require.NoError(t, pool.QueryRow(ctx, `select count(*) from market_prices_hourly where ts >= $1 and ts < $2`, from, to).Scan(&n))
	require.Equal(t, 60*24, n)

	// date_trunc('day', ts) alone truncates in the session's timezone
	// (UTC), not Europe/Istanbul — since Istanbul is UTC+3, that would
	// split every stored hour's UTC instant across the wrong calendar-day
	// boundary and over-count by one day at the window's edges. "AT TIME
	// ZONE 'Europe/Istanbul'" converts to Istanbul wall-clock time first,
	// matching how the rest of this codebase (and MissingHours) buckets
	// days.
	var days int
	require.NoError(t, pool.QueryRow(ctx,
		`select count(distinct date_trunc('day', ts at time zone 'Europe/Istanbul')) from market_prices_hourly where ts >= $1 and ts < $2`,
		from, to).Scan(&days))
	require.Equal(t, 60, days, "every day of the explicit window must have been written")

	run := journal.lastRun()
	require.Equal(t, "success", run.Status)
}
