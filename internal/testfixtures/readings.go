package testfixtures

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
)

// HourlyReadings returns load-profile readings every hour in [from, to) whose
// active_import register grows by perHour(ts) kWh per hour, starting at 1000;
// inductive and capacitive registers grow by 20 % and 5 % of each step.
func HourlyReadings(analyzerID uuid.UUID, from, to time.Time, perHour func(ts time.Time) decimal.Decimal) []model.MeterReading {
	return HourlyReadingsReactive(analyzerID, from, to, perHour, "0.2", "0.05")
}

// HourlyReadingsReactive is HourlyReadings with explicit inductive and
// capacitive shares of each active step.
func HourlyReadingsReactive(analyzerID uuid.UUID, from, to time.Time, perHour func(ts time.Time) decimal.Decimal, inductiveShare, capacitiveShare string) []model.MeterReading {
	ind, capShare := decimal.RequireFromString(inductiveShare), decimal.RequireFromString(capacitiveShare)
	active := decimal.NewFromInt(1000)
	inductive, capacitive := decimal.Zero, decimal.Zero
	var out []model.MeterReading
	for ts := from; ts.Before(to); ts = ts.Add(time.Hour) {
		a, i, c := active, inductive, capacitive
		out = append(out, model.MeterReading{
			AnalyzerID: analyzerID, Ts: ts.UTC(), Kind: model.ReadingKindLoadProfile,
			ActiveImport: &a, ReactiveInductiveImport: &i, ReactiveCapacitiveImport: &c,
			MultiplierApplied: decimal.NewFromInt(1), SourceProvider: model.IntegrationProviderOSOS, IngestedAt: ts.UTC(),
		})
		step := perHour(ts)
		active = active.Add(step)
		inductive = inductive.Add(step.Mul(ind))
		capacitive = capacitive.Add(step.Mul(capShare))
	}
	return out
}

// Constant is a perHour function with a fixed rate.
func Constant(kwh string) func(time.Time) decimal.Decimal {
	v := decimal.RequireFromString(kwh)
	return func(time.Time) decimal.Decimal { return v }
}

// InsertReadings bulk-inserts rows for the company that owns them.
func InsertReadings(t *testing.T, ctx context.Context, pool *pgxpool.Pool, companyID uuid.UUID, rows []model.MeterReading) {
	t.Helper()
	const chunk = 5000
	repo := postgres.NewReadingRepository(pool)
	for start := 0; start < len(rows); start += chunk {
		_, _, err := repo.BulkInsert(ctx, store.SystemScope(companyID), rows[start:min(start+chunk, len(rows))])
		require.NoError(t, err)
	}
	if len(rows) == 0 {
		return
	}
	// A refresh policy may already have run in this database and moved the watermark past these
	// (historical) rows; below the watermark real-time aggregation shows nothing until a refresh
	// (00005's operator note). Refresh the real-time views over whole days around the data.
	from, to := rows[0].Ts, rows[0].Ts
	for _, r := range rows {
		from, to = minTime(from, r.Ts), maxTime(to, r.Ts)
	}
	from, to = from.Add(-48*time.Hour), to.Add(48*time.Hour)
	for _, view := range []string{"consumption_hourly", "consumption_daily"} {
		refreshAggregate(t, ctx, pool, view, from, to)
	}
}

// refreshAggregate waits out the policy job that may be refreshing the same view (SQLSTATE 55P03).
func refreshAggregate(t *testing.T, ctx context.Context, pool *pgxpool.Pool, view string, from, to time.Time) {
	t.Helper()
	var err error
	for range 100 {
		_, err = pool.Exec(ctx, `call refresh_continuous_aggregate($1, $2::timestamptz, $3::timestamptz)`, view, from, to)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "55P03" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.NoError(t, err, "refresh %s", view)
}

func minTime(a, b time.Time) time.Time {
	if b.Before(a) {
		return b
	}
	return a
}

func maxTime(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

// MustUUID parses s or panics; for test tables keyed by id strings.
func MustUUID(s string) uuid.UUID { return uuid.MustParse(s) }
