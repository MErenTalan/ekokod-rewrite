//go:build integration

package marketdata_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/normalize"
	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/marketdata"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/admin"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

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

// fakePriceSourceWithYekdemDrops additionally implements the OPTIONAL
// yekdemDropReporter capability sync.go type-asserts for (M2, fix round
// 1: "surface the drop count in the sync run's detail"), so a test can
// drive syncDetail.YekdemDropped without a real EPİAŞ payload containing
// an unparseable row.
type fakePriceSourceWithYekdemDrops struct {
	fakePriceSource
	dropped int32
}

func (f *fakePriceSourceWithYekdemDrops) YekdemDropped() int32 { return f.dropped }

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

// realJobRun is a job_runs row read directly off the database — not
// through store.AdminJournalRepository, which exposes no read method — so
// a test can inspect exactly what Syncer, via the real
// admin.JournalRepository, actually persisted.
type realJobRun struct {
	ID         uuid.UUID
	CompanyID  *uuid.UUID
	Status     string
	Processed  int32
	Skipped    int32
	Failed     int32
	Error      *string
	Scope      []byte
	Detail     []byte
	StartedAt  time.Time
	FinishedAt *time.Time
}

// fetchJobRunByID reads one job_runs row by id.
func fetchJobRunByID(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) realJobRun {
	t.Helper()
	var r realJobRun
	err := pool.QueryRow(ctx, `
		select id, company_id, status, processed, skipped, failed, error, scope, detail, started_at, finished_at
		from job_runs where id = $1`, id).
		Scan(&r.ID, &r.CompanyID, &r.Status, &r.Processed, &r.Skipped, &r.Failed, &r.Error, &r.Scope, &r.Detail, &r.StartedAt, &r.FinishedAt)
	require.NoError(t, err)
	return r
}

// fetchLatestJobRun reads the most recently started job_runs row for
// jobType. Ties in started_at (this package's tests run against a fixed
// clock.Fake, so two runs in the same test can share one instant) are
// broken by id, which is fine here: every test that calls this more than
// once asserts something true of EITHER run it could produce.
func fetchLatestJobRun(t *testing.T, ctx context.Context, pool *pgxpool.Pool, jobType string) realJobRun {
	t.Helper()
	var id uuid.UUID
	err := pool.QueryRow(ctx, `
		select id from job_runs where job_type = $1
		order by started_at desc, id desc
		limit 1`, jobType).Scan(&id)
	require.NoError(t, err)
	return fetchJobRunByID(t, ctx, pool, id)
}

// realOperationalMessage is an operational_messages row read directly off
// the database, the same way realJobRun is.
type realOperationalMessage struct {
	ID        int64
	CompanyID *uuid.UUID
	Kind      string
	Category  string
	Status    string
	Message   string
	Metadata  []byte
	CreatedAt time.Time
}

// fetchOperationalMessages reads every operational_messages row for
// category, ordered by id (bigserial: insertion order). That ordering is
// for THIS TEST's own determinism only — Syncer (sync.go) never reads
// operational_messages back, so nothing in the code under test relies on
// any read-back ordering here.
func fetchOperationalMessages(t *testing.T, ctx context.Context, pool *pgxpool.Pool, category string) []realOperationalMessage {
	t.Helper()
	rows, err := pool.Query(ctx, `
		select id, company_id, kind, category, status, message, metadata, created_at
		from operational_messages where category = $1
		order by id asc`, category)
	require.NoError(t, err)
	defer rows.Close()
	var out []realOperationalMessage
	for rows.Next() {
		var m realOperationalMessage
		require.NoError(t, rows.Scan(&m.ID, &m.CompanyID, &m.Kind, &m.Category, &m.Status, &m.Message, &m.Metadata, &m.CreatedAt))
		out = append(out, m)
	}
	require.NoError(t, rows.Err())
	return out
}

// TestSyncPricesIsIdempotent: running the same window twice converges
// rather than duplicating, and the platform run row carries no company —
// asserted here directly on the real job_runs row admin.JournalRepository
// wrote.
func TestSyncPricesIsIdempotent(t *testing.T) {
	t.Parallel()
	pool := testfixtures.NewIsolatedDB(t)
	market := admin.NewMarketDataRepository(pool)
	journal := admin.NewJournalRepository(pool)

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

	run := fetchLatestJobRun(t, ctx, pool, "epias.sync_prices")
	require.Nil(t, run.CompanyID, "a platform run's company_id must be NULL")
	require.Equal(t, "success", run.Status)
}

// TestSyncPricesReportsMissingHoursWithoutFilling: 23 of 24 hours published
// → 23 rows written, one warning platform message, the run is partial, and
// no row exists for the missing hour — it is reported, never fabricated.
// Also covers the jsonb round-trip for FinishPlatformRun's detail column,
// StartPlatformRun's scope column, and AppendPlatformMessage's metadata
// column, all read back off the real database.
func TestSyncPricesReportsMissingHoursWithoutFilling(t *testing.T) {
	t.Parallel()
	pool := testfixtures.NewIsolatedDB(t)
	market := admin.NewMarketDataRepository(pool)
	journal := admin.NewJournalRepository(pool)

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

	run := fetchLatestJobRun(t, ctx, pool, "epias.sync_prices")
	require.Equal(t, "partial", run.Status)
	require.Nil(t, run.CompanyID, "a platform run's company_id must be NULL")

	// jsonb round-trip: StartPlatformRun's scope column records the window
	// this run actually requested (sync.go's syncScope). syncScope itself
	// is unexported, so this mirrors its json tags rather than importing
	// the type.
	var scope struct {
		PTFFrom time.Time `json:"ptf_from"`
		PTFTo   time.Time `json:"ptf_to"`
	}
	require.NoError(t, json.Unmarshal(run.Scope, &scope))
	require.True(t, from.Equal(scope.PTFFrom), "scope.ptf_from must round-trip")
	require.True(t, to.Equal(scope.PTFTo), "scope.ptf_to must round-trip")

	// jsonb round-trip: FinishPlatformRun's detail column (sync.go's
	// syncDetail) — one missing hour, no YEKDEM drop on this run.
	var detail struct {
		MissingDays   int   `json:"missing_days"`
		MissingHours  int   `json:"missing_hours"`
		YekdemDropped int32 `json:"yekdem_dropped,omitempty"`
	}
	require.NoError(t, json.Unmarshal(run.Detail, &detail))
	require.Equal(t, 1, detail.MissingDays)
	require.Equal(t, 1, detail.MissingHours)
	require.Zero(t, detail.YekdemDropped)

	// Platform messages are readable off operational_messages. Ordering:
	// this run writes exactly one message, and Syncer never reads
	// operational_messages back at all (see fetchOperationalMessages'
	// doc comment) — there is no ordering the code relies on to state.
	messages := fetchOperationalMessages(t, ctx, pool, "market-prices")
	require.Len(t, messages, 1)
	require.Nil(t, messages[0].CompanyID)
	require.Equal(t, "job", messages[0].Kind)
	require.Equal(t, "warning", messages[0].Status)
	require.Equal(t, "1 PTF hours missing", messages[0].Message)

	// jsonb round-trip: AppendPlatformMessage's metadata column
	// ({"days": missing}, sync.go's appendMissingHoursMessage).
	var meta struct {
		Days map[string][]time.Time `json:"days"`
	}
	require.NoError(t, json.Unmarshal(messages[0].Metadata, &meta))
	dayKey := missingHour.In(normalize.Istanbul).Format("2006-01-02")
	require.Contains(t, meta.Days, dayKey)
	require.Len(t, meta.Days[dayKey], 1)
	require.True(t, missingHour.Equal(meta.Days[dayKey][0]), "the missing hour itself must round-trip through metadata")
}

// TestSyncPricesBackfillWindow: an explicit 60-day window writes every
// day, not just the default trailing window.
func TestSyncPricesBackfillWindow(t *testing.T) {
	t.Parallel()
	pool := testfixtures.NewIsolatedDB(t)
	market := admin.NewMarketDataRepository(pool)
	journal := admin.NewJournalRepository(pool)

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

	run := fetchLatestJobRun(t, ctx, pool, "epias.sync_prices")
	require.Equal(t, "success", run.Status)
	require.Nil(t, run.CompanyID, "a platform run's company_id must be NULL")
}

// TestSyncPricesRecordsYekdemDroppedInDetail proves the jsonb round-trip
// for syncDetail.YekdemDropped specifically in the non-zero case (M2, fix
// round 1): when Source reports dropped YEKDEM rows via the optional
// yekdemDropReporter capability, that count is written into
// FinishPlatformRun's detail jsonb and reads back unchanged.
func TestSyncPricesRecordsYekdemDroppedInDetail(t *testing.T) {
	t.Parallel()
	pool := testfixtures.NewIsolatedDB(t)
	market := admin.NewMarketDataRepository(pool)
	journal := admin.NewJournalRepository(pool)

	from := time.Date(2026, 3, 1, 0, 0, 0, 0, normalize.Istanbul).UTC()
	to := time.Date(2026, 3, 2, 0, 0, 0, 0, normalize.Istanbul).UTC()

	src := &fakePriceSourceWithYekdemDrops{
		fakePriceSource: fakePriceSource{
			hourly: func(f, t time.Time) ([]model.MarketPrice, []integration.Warning, error) {
				return fullDayPrices(f, t), nil, nil
			},
			yekdem: fixedYekdem,
		},
		dropped: 3,
	}
	syncer := marketdata.New(src, market, journal, clock.NewFake(to), testfixtures.DiscardLogger())

	ctx := context.Background()
	require.NoError(t, syncer.SyncPrices(ctx, job.SyncPricesPayload{Window: &job.Window{From: from, To: to}}))

	run := fetchLatestJobRun(t, ctx, pool, "epias.sync_prices")
	require.Equal(t, "success", run.Status)

	var detail struct {
		MissingDays   int   `json:"missing_days"`
		MissingHours  int   `json:"missing_hours"`
		YekdemDropped int32 `json:"yekdem_dropped,omitempty"`
	}
	require.NoError(t, json.Unmarshal(run.Detail, &detail))
	require.Equal(t, int32(3), detail.YekdemDropped, "a nonzero YekdemDropped count must round-trip through the jsonb detail column")
}

// TestSyncPricesRecordsFailedRunOnSourceError exercises Syncer's failure
// path (Syncer.fail) against the real journal: a fetch error surfaces as
// a 'failed' platform run with the error text stored and readable back,
// and the platform run's company_id stays NULL even on failure.
func TestSyncPricesRecordsFailedRunOnSourceError(t *testing.T) {
	t.Parallel()
	pool := testfixtures.NewIsolatedDB(t)
	market := admin.NewMarketDataRepository(pool)
	journal := admin.NewJournalRepository(pool)

	from := time.Date(2026, 2, 1, 0, 0, 0, 0, normalize.Istanbul).UTC()
	to := time.Date(2026, 2, 2, 0, 0, 0, 0, normalize.Istanbul).UTC()

	wantErr := errors.New("epias: simulated fetch failure")
	src := &fakePriceSource{
		hourly: func(_, _ time.Time) ([]model.MarketPrice, []integration.Warning, error) {
			return nil, nil, wantErr
		},
	}
	syncer := marketdata.New(src, market, journal, clock.NewFake(to), testfixtures.DiscardLogger())

	ctx := context.Background()
	err := syncer.SyncPrices(ctx, job.SyncPricesPayload{Window: &job.Window{From: from, To: to}})
	require.ErrorIs(t, err, wantErr)

	run := fetchLatestJobRun(t, ctx, pool, "epias.sync_prices")
	require.Nil(t, run.CompanyID, "a platform run's company_id must be NULL even on failure")
	require.Equal(t, "failed", run.Status)
	require.NotNil(t, run.Error)
	require.Contains(t, *run.Error, "simulated fetch failure")
}

// TestPlatformJournalStatusValuesRoundTrip proves that every status value
// Syncer ever writes to a platform run — 'running' (StartPlatformRun's
// fixed insert status), then 'success', 'partial' or 'failed'
// (FinishPlatformRun) — is accepted by job_runs.status and read back
// unchanged, both via the repository's own returned row and a direct
// re-read off the database (so a value merely echoed by RETURNING,
// without ever really being persisted, cannot pass this test).
func TestPlatformJournalStatusValuesRoundTrip(t *testing.T) {
	t.Parallel()
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	journal := admin.NewJournalRepository(pool)

	for _, status := range []string{"success", "partial", "failed"} {
		started, err := journal.StartPlatformRun(ctx, model.JobRun{JobType: "epias.sync_prices"})
		require.NoError(t, err)
		require.Equal(t, "running", started.Status)
		require.Equal(t, "running", fetchJobRunByID(t, ctx, pool, started.ID).Status,
			"'running' must be accepted by the DB and read back, not just echoed by the insert's own RETURNING")

		var errText *string
		if status == "failed" {
			msg := "epias: simulated failure"
			errText = &msg
		}
		finished, err := journal.FinishPlatformRun(ctx, started.ID, status, 1, 0, 0, errText, []byte(`{}`), time.Now().UTC())
		require.NoError(t, err)
		require.Equal(t, status, finished.Status)

		reread := fetchJobRunByID(t, ctx, pool, started.ID)
		require.Equal(t, status, reread.Status, "%q must be accepted by the DB and read back", status)
		if status == "failed" {
			require.NotNil(t, reread.Error)
			require.Equal(t, "epias: simulated failure", *reread.Error)
		}
	}
}

// TestJournalFinishPlatformRunCannotFinishATenantRun proves (d): the real
// admin.JournalRepository's FinishPlatformRun can never finish a TENANT's
// job run, even given that run's own id — AdminFinishPlatformRun's SQL
// (admin_journal.sql) only ever matches a company_id-NULL row.
func TestJournalFinishPlatformRunCannotFinishATenantRun(t *testing.T) {
	t.Parallel()
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenant := testfixtures.NewTenant(t, ctx, pool, 912004)
	opsRepo := postgres.NewOpsRepository(pool)
	journal := admin.NewJournalRepository(pool)

	companyID := tenant.Company.ID
	tenantRun, err := opsRepo.StartRun(ctx, tenant.Scope, model.JobRun{CompanyID: &companyID, JobType: "epias.sync_prices"})
	require.NoError(t, err)

	_, err = journal.FinishPlatformRun(ctx, tenantRun.ID, "success", 5, 0, 0, nil, nil, time.Now().UTC())
	require.ErrorIs(t, err, store.ErrNotFound)

	stillRunning, err := opsRepo.GetRun(ctx, tenant.Scope, tenantRun.ID)
	require.NoError(t, err)
	require.Equal(t, "running", stillRunning.Status, "the refused platform finish must not have touched the tenant's run")
}
