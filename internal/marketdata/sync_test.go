package marketdata_test

// Fix round 1, finding I1 (task-12-fix1-findings.md): a mutation deleting
// either dedupPrices(prices) or dedupYekdem(yek) from sync.go's call sites
// passed every existing test — sync_integration_test.go's fixtures never
// hand back two rows sharing a key, and AdminMarketDataRepository's own
// real "refuse a duplicate key" rule (store.ErrConflict) would have turned
// a would-be silent duplicate into a hard error there anyway, masking the
// missing dedup call rather than proving it. These tests use a
// recordingMarket that, unlike the real repository, does NOT refuse a
// duplicate key — it just records whatever it was handed — so a missing
// dedup call is observable as "the fake received two rows for one key",
// not as an unrelated store error.
//
// This file is deliberately NOT integration-tagged (unlike
// sync_integration_test.go): it needs no Postgres, only the PriceSource/
// AdminMarketDataRepository/AdminJournalRepository interfaces, so it runs
// under the ordinary `go test ./internal/marketdata/...` gate and every
// -race run, not just the -tags=integration one.

import (
	"context"
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
)

// recordingMarket is an in-memory store.AdminMarketDataRepository that
// records exactly what it was handed, with none of the real repository's
// own "refuse a duplicate key" guard — see the file doc comment for why
// that matters for I1.
type recordingMarket struct {
	hourlyCalls [][]model.MarketPrice
	yekdemCalls [][]model.YekdemMonthly
}

func (m *recordingMarket) UpsertHourlyPrices(_ context.Context, prices []model.MarketPrice) (int64, error) {
	cp := make([]model.MarketPrice, len(prices))
	copy(cp, prices)
	m.hourlyCalls = append(m.hourlyCalls, cp)
	return int64(len(prices)), nil
}

func (m *recordingMarket) UpsertYekdem(_ context.Context, values []model.YekdemMonthly) (int64, error) {
	cp := make([]model.YekdemMonthly, len(values))
	copy(cp, values)
	m.yekdemCalls = append(m.yekdemCalls, cp)
	return int64(len(values)), nil
}

// allHourlyRows flattens every UpsertHourlyPrices call this recorder saw.
func (m *recordingMarket) allHourlyRows() []model.MarketPrice {
	var out []model.MarketPrice
	for _, c := range m.hourlyCalls {
		out = append(out, c...)
	}
	return out
}

// allYekdemRows flattens every UpsertYekdem call this recorder saw.
func (m *recordingMarket) allYekdemRows() []model.YekdemMonthly {
	var out []model.YekdemMonthly
	for _, c := range m.yekdemCalls {
		out = append(out, c...)
	}
	return out
}

// stubJournal is a minimal in-memory store.AdminJournalRepository — a
// second, independent fake from sync_integration_test.go's fakeJournal
// (that one is //go:build integration only, so it is not visible here; a
// plain `go test ./internal/marketdata/...` never compiles it, and
// `-tags=integration` compiles both files together, so the two fakes must
// not share a name).
type stubJournal struct {
	run      model.JobRun
	messages []model.OperationalMessage
}

func (j *stubJournal) StartPlatformRun(_ context.Context, run model.JobRun) (model.JobRun, error) {
	run.ID = uuid.New()
	run.Status = "running"
	j.run = run
	return run, nil
}

func (j *stubJournal) FinishPlatformRun(_ context.Context, id uuid.UUID, status string, processed, skipped, failed int32, errText *string, detail []byte, at time.Time) (model.JobRun, error) {
	if j.run.ID != id {
		return model.JobRun{}, store.ErrNotFound
	}
	j.run.Status = status
	j.run.Processed = processed
	j.run.Skipped = skipped
	j.run.Failed = failed
	j.run.Error = errText
	j.run.Detail = detail
	j.run.FinishedAt = &at
	return j.run, nil
}

func (j *stubJournal) AppendPlatformMessage(_ context.Context, m model.OperationalMessage) (model.OperationalMessage, error) {
	m.ID = int64(len(j.messages) + 1)
	j.messages = append(j.messages, m)
	return m, nil
}

// duplicatingSource is a marketdata.PriceSource whose HourlyPTF and
// YekdemUnitCost are supplied per test as plain closures, exactly like
// sync_integration_test.go's fakePriceSource but under a distinct name for
// the reason stubJournal's doc comment gives.
type duplicatingSource struct {
	hourly func(from, to time.Time) ([]model.MarketPrice, []integration.Warning, error)
	yekdem func(from, to time.Time) ([]model.YekdemMonthly, error)
}

func (d *duplicatingSource) HourlyPTF(_ context.Context, from, to time.Time) ([]model.MarketPrice, []integration.Warning, error) {
	return d.hourly(from, to)
}

func (d *duplicatingSource) YekdemUnitCost(_ context.Context, from, to time.Time) ([]model.YekdemMonthly, error) {
	return d.yekdem(from, to)
}

// TestSyncPricesDedupesDuplicateHourBeforeUpsert (I1): when HourlyPTF hands
// back two rows for the same Ts (a provider defect, or two overlapping
// chunks — see dedupPrices' own doc comment in sync.go), Syncer must send
// UpsertHourlyPrices exactly one row for that Ts, keeping the FIRST
// occurrence — dedupPrices' documented rule. Deleting the `prices =
// dedupPrices(prices)` call in sync.go makes this FAIL: the fake would
// then receive two rows for the same Ts, so the "exactly one row" count
// assertion below fails (see task-12-report.md's Fix round 1 section for
// the recorded failure output).
func TestSyncPricesDedupesDuplicateHourBeforeUpsert(t *testing.T) {
	from := time.Date(2026, 3, 1, 0, 0, 0, 0, normalize.Istanbul).UTC()
	to := time.Date(2026, 3, 1, 1, 0, 0, 0, normalize.Istanbul).UTC() // one hour window

	first := decimal.RequireFromString("1000.0000")
	second := decimal.RequireFromString("2000.0000")

	market := &recordingMarket{}
	journal := &stubJournal{}
	src := &duplicatingSource{
		hourly: func(f, t time.Time) ([]model.MarketPrice, []integration.Warning, error) {
			return []model.MarketPrice{
				{Ts: f, PTF: first},  // first occurrence — dedupPrices keeps this one
				{Ts: f, PTF: second}, // duplicate Ts, later in the slice — must be dropped
			}, nil, nil
		},
		yekdem: func(time.Time, time.Time) ([]model.YekdemMonthly, error) { return nil, nil },
	}
	syncer := marketdata.New(src, market, journal, clock.NewFake(to), nil)

	require.NoError(t, syncer.SyncPrices(context.Background(), job.SyncPricesPayload{Window: &job.Window{From: from, To: to}}))

	rows := market.allHourlyRows()
	require.Len(t, rows, 1, "UpsertHourlyPrices must receive exactly one row per Ts")
	require.True(t, rows[0].Ts.Equal(from))
	require.True(t, first.Equal(rows[0].PTF), "the FIRST occurrence for a duplicated Ts must win (dedupPrices' documented rule), got %s", rows[0].PTF)
}

// TestSyncPricesDedupesDuplicateYekdemMonthBeforeUpsert (I1): the same
// proof as above for YEKDEM — dedupYekdem keeps the first (Year, Month)
// occurrence. Deleting `yek = dedupYekdem(yek)` in sync.go makes this
// FAIL.
func TestSyncPricesDedupesDuplicateYekdemMonthBeforeUpsert(t *testing.T) {
	from := time.Date(2026, 3, 1, 0, 0, 0, 0, normalize.Istanbul).UTC()
	to := time.Date(2026, 3, 1, 1, 0, 0, 0, normalize.Istanbul).UTC()

	first := decimal.RequireFromString("111.1111")
	second := decimal.RequireFromString("222.2222")

	market := &recordingMarket{}
	journal := &stubJournal{}
	src := &duplicatingSource{
		hourly: func(f, t time.Time) ([]model.MarketPrice, []integration.Warning, error) { return nil, nil, nil },
		yekdem: func(time.Time, time.Time) ([]model.YekdemMonthly, error) {
			return []model.YekdemMonthly{
				{Year: 2026, Month: 3, Value: first},  // first occurrence — dedupYekdem keeps this one
				{Year: 2026, Month: 3, Value: second}, // duplicate (Year, Month) — must be dropped
			}, nil
		},
	}
	syncer := marketdata.New(src, market, journal, clock.NewFake(to), nil)

	require.NoError(t, syncer.SyncPrices(context.Background(), job.SyncPricesPayload{Window: &job.Window{From: from, To: to}}))

	rows := market.allYekdemRows()
	require.Len(t, rows, 1, "UpsertYekdem must receive exactly one row per (Year, Month)")
	require.Equal(t, int16(2026), rows[0].Year)
	require.Equal(t, int16(3), rows[0].Month)
	require.True(t, first.Equal(rows[0].Value), "the FIRST occurrence for a duplicated (Year, Month) must win (dedupYekdem's documented rule), got %s", rows[0].Value)
}
