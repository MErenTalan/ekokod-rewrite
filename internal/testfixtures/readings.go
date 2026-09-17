package testfixtures

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
)

// HourlyReadings returns load-profile readings every hour in [from, to) whose
// active_import register grows by perHour(ts) kWh per hour, starting at 1000.
// Inductive and capacitive registers grow by 20 % and 5 % of the active step.
func HourlyReadings(analyzerID uuid.UUID, from, to time.Time, perHour func(ts time.Time) decimal.Decimal) []model.MeterReading {
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
		inductive = inductive.Add(step.Mul(decimal.RequireFromString("0.2")))
		capacitive = capacitive.Add(step.Mul(decimal.RequireFromString("0.05")))
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
}

// MustUUID parses s or panics; for test tables keyed by id strings.
func MustUUID(s string) uuid.UUID { return uuid.MustParse(s) }
