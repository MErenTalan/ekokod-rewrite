package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/internal/pgerr"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/sqlcgen"
)

// ProviderSeriesRepository implements store.ProviderSeriesRepository:
// provider_hourly_values, OSOS's own labelled hourly cross-check series
// (06 §2, removed-behaviour 23). NEVER read by consumption or billing — see
// repository.go's ProviderSeriesRepository doc.
//
// provider_hourly_values has no company_id — every method joins through
// analyzers, per repository.go's "ROWS WITHOUT company_id" header.
type ProviderSeriesRepository struct {
	q    *sqlcgen.Queries
	pool *pgxpool.Pool
}

// NewProviderSeriesRepository wraps pool in the generated query set.
func NewProviderSeriesRepository(pool *pgxpool.Pool) *ProviderSeriesRepository {
	return &ProviderSeriesRepository{q: sqlcgen.New(pool), pool: pool}
}

var _ store.ProviderSeriesRepository = (*ProviderSeriesRepository)(nil)

// providerHourlyStagingColumns is both the staging table's column list (in
// declaration order) and the CopyFrom column list, mirroring
// meterReadingsStagingColumns in readings.go for the same reason: the two
// must agree, and naming them once here is what keeps them from drifting
// apart. ingested_at is excluded — the upsert below stamps it with now(),
// exactly like meter_readings' own staging path.
var providerHourlyStagingColumns = []string{
	"analyzer_id", "ts", "active_consumption", "active_generation", "source_provider",
}

// f2seriesCreateProviderHourlyStaging mirrors provider_hourly_values'
// writable columns. Plain SQL rather than a migration or a sqlc query: the
// table exists for exactly one transaction, `on commit drop` cleans it up,
// and sqlc's schema catalogue must never see it (readings.go's
// createMeterReadingsStaging explains why in full).
const f2seriesCreateProviderHourlyStaging = `create temporary table provider_hourly_values_staging (
	analyzer_id         uuid          not null,
	ts                  timestamptz   not null,
	active_consumption  numeric(18,4),
	active_generation   numeric(18,4),
	source_provider     integration_provider not null
) on commit drop`

// f2seriesUpsertProviderHourlyFromStaging is UpsertHourly's one idempotent
// write, keyed on (analyzer_id, ts) — the same `on conflict … do update …
// returning (xmax = 0) as inserted` shape readings.go's
// upsertMeterReadingsFromStaging uses, for the same reasons: a PLAIN select
// (never `distinct on`), because UpsertHourly refuses the whole batch with
// store.ErrConflict, before the staging table even exists, the moment two
// rows in the caller's own slice share an (analyzer_id, ts) key (see
// f2seriesDuplicateKey).
//
// Defence in depth: the select joins analyzers again, on the SAME predicate
// ProviderHourlyVisibleAnalyzerIDs already locked ($1 company_id, $2/$3
// building branch, deleted_at is null) — params bound from the caller's own
// Scope, never trusted from the staging rows. This is redundant with the
// `for share` check above under normal operation; it exists so that if that
// check is ever weakened or bypassed, the write itself still cannot smuggle
// another tenant's row in. UpsertHourly additionally asserts the returned
// row count equals len(rows); a mismatch — this join silently dropping a row
// the Go-side check let through — rolls the whole batch back with
// store.ErrNotFound rather than reporting a partial success.
const f2seriesUpsertProviderHourlyFromStaging = `insert into provider_hourly_values (
	analyzer_id, ts, active_consumption, active_generation, source_provider
)
select
	s.analyzer_id, s.ts, s.active_consumption, s.active_generation, s.source_provider
from provider_hourly_values_staging s
join analyzers a on a.id = s.analyzer_id
	and a.company_id = $1
	and a.deleted_at is null
	and ($2::boolean or a.building_id = any($3::uuid[]))
on conflict (analyzer_id, ts) do update set
	active_consumption = excluded.active_consumption,
	active_generation  = excluded.active_generation,
	source_provider    = excluded.source_provider,
	ingested_at        = now()
returning (xmax = 0) as inserted`

// UpsertHourly implements store.ProviderSeriesRepository.UpsertHourly.
//
// It follows readings.go's BulkInsert pattern exactly: a per-call temporary
// table cannot appear in sqlc's schema catalogue, so the whole
// COPY-then-upsert runs as plain SQL against a *pgxpool.Tx. The visibility
// check that guards it, ProviderHourlyVisibleAnalyzerIDs, IS a generated
// query — there is nothing dynamic about it, and it is THE tenant predicate
// for this write, evaluated once per batch (never embedded in the upsert SQL
// itself, unlike GenerationRepository.SetAnchor, which writes exactly one
// analyzer's row).
func (r *ProviderSeriesRepository) UpsertHourly(ctx context.Context, s store.Scope, rows []model.ProviderHourlyValue) (inserted, updated int, err error) {
	if !s.Valid() {
		return 0, 0, store.ErrInvalidScope
	}
	if len(rows) == 0 {
		return 0, 0, nil
	}

	const op = "provider hourly upsert"
	buildingIDs, allBuildings := s.BuildingFilter()

	// A duplicate (analyzer_id, ts) inside the CALLER'S OWN batch is refused
	// loudly, before any database round trip — the Global Constraints
	// "batches are all-or-nothing, duplicate keys refused" rule, applied the
	// same way readingDuplicateKey applies it to BulkInsert.
	if dup, ok := f2seriesDuplicateKey(rows); ok {
		return 0, 0, fmt.Errorf("%w: duplicate provider hourly key analyzer_id=%s ts=%s",
			store.ErrConflict, dup.AnalyzerID, dup.Ts.Format(time.RFC3339Nano))
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, 0, pgerr.Translate(r.pool, op, err)
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op once committed

	analyzerIDs := make([]uuid.UUID, len(rows))
	for i, row := range rows {
		analyzerIDs[i] = row.AnalyzerID
	}
	distinctAnalyzerIDs := readingDistinctUUIDs(analyzerIDs)

	// `for share` locks every visible analyzer row for the rest of this
	// transaction, closing the same TOCTOU gap ReadingVisibleAnalyzerIDs
	// closes for BulkInsert.
	visibleIDs, err := r.q.WithTx(tx).ProviderHourlyVisibleAnalyzerIDs(ctx, sqlcgen.ProviderHourlyVisibleAnalyzerIDsParams{
		AnalyzerIds:  distinctAnalyzerIDs,
		CompanyID:    s.CompanyID,
		AllBuildings: allBuildings,
		BuildingIds:  buildingIDs,
	})
	if err != nil {
		return 0, 0, pgerr.Translate(r.pool, op, err)
	}
	if len(visibleIDs) != len(distinctAnalyzerIDs) {
		// At least one row's analyzer is not visible to the Scope: the whole
		// batch is refused and nothing is written (the deferred Rollback
		// above does that).
		return 0, 0, store.ErrNotFound
	}

	if _, err := tx.Exec(ctx, f2seriesCreateProviderHourlyStaging); err != nil {
		return 0, 0, pgerr.Translate(r.pool, op, err)
	}

	_, err = tx.CopyFrom(ctx, pgx.Identifier{"provider_hourly_values_staging"}, providerHourlyStagingColumns,
		pgx.CopyFromSlice(len(rows), func(i int) ([]any, error) {
			row := rows[i]
			return []any{
				row.AnalyzerID,
				row.Ts,
				decimalPtrToNumeric(row.ActiveConsumption),
				decimalPtrToNumeric(row.ActiveGeneration),
				string(row.SourceProvider),
			}, nil
		}))
	if err != nil {
		return 0, 0, pgerr.Translate(r.pool, op, err)
	}

	upsertRows, err := tx.Query(ctx, f2seriesUpsertProviderHourlyFromStaging, s.CompanyID, allBuildings, buildingIDs)
	if err != nil {
		return 0, 0, pgerr.Translate(r.pool, op, err)
	}
	inserted, updated, err = timeseriesScanUpsertCounts(upsertRows)
	if err != nil {
		return 0, 0, pgerr.Translate(r.pool, op, err)
	}
	if inserted+updated != len(rows) {
		// Defence in depth: the upsert's own analyzers join (see
		// f2seriesUpsertProviderHourlyFromStaging) returned fewer rows than
		// the batch had — some row's analyzer failed the join's own
		// company/building/deleted_at predicate even though the earlier
		// ProviderHourlyVisibleAnalyzerIDs check let it through. Refuse the
		// whole batch rather than report a partial success; the deferred
		// Rollback above discards everything written so far.
		return 0, 0, store.ErrNotFound
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, 0, pgerr.Translate(r.pool, op, err)
	}
	return inserted, updated, nil
}

// HourlyRange implements store.ProviderSeriesRepository.HourlyRange.
//
// Like ReadingRepository.Range, the scope is carried by
// ProviderHourlyRange's OWN join through analyzers, and
// f2seriesRequireAnalyzerVisible is consulted only to choose an error when
// the scoped query comes back empty.
func (r *ProviderSeriesRepository) HourlyRange(ctx context.Context, s store.Scope, analyzerID uuid.UUID, tr store.TimeRange) ([]model.ProviderHourlyValue, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	if !tr.Valid() {
		return nil, store.ErrInvalidRange
	}
	const op = "provider hourly range"
	buildingIDs, allBuildings := s.BuildingFilter()

	rows, err := r.q.ProviderHourlyRange(ctx, sqlcgen.ProviderHourlyRangeParams{
		AnalyzerID:   analyzerID,
		FromTs:       timeseriesToTimestamptz(tr.From),
		ToTs:         timeseriesToTimestamptz(tr.To),
		CompanyID:    s.CompanyID,
		AllBuildings: allBuildings,
		BuildingIds:  buildingIDs,
	})
	if err != nil {
		return nil, pgerr.Translate(r.pool, op, err)
	}
	if len(rows) == 0 {
		if err := r.requireAnalyzerVisible(ctx, s, analyzerID, op); err != nil {
			return nil, err
		}
		return []model.ProviderHourlyValue{}, nil
	}
	out := make([]model.ProviderHourlyValue, len(rows))
	for i, row := range rows {
		phv, err := f2seriesFromRow(row)
		if err != nil {
			return nil, pgerr.Translate(r.pool, op, err)
		}
		out[i] = phv
	}
	return out, nil
}

// requireAnalyzerVisible returns store.ErrNotFound when analyzerID is not
// visible to s — missing, another tenant's, or outside the Scope's
// buildings. Mirrors ReadingRepository.requireAnalyzerVisible exactly: used
// only to choose an error after HourlyRange's own scoped query has already
// come back empty, never the sole gate on whether data is returned.
func (r *ProviderSeriesRepository) requireAnalyzerVisible(ctx context.Context, s store.Scope, analyzerID uuid.UUID, op string) error {
	buildingIDs, allBuildings := s.BuildingFilter()
	visible, err := r.q.ProviderHourlyAnalyzerVisible(ctx, sqlcgen.ProviderHourlyAnalyzerVisibleParams{
		AnalyzerID:   analyzerID,
		CompanyID:    s.CompanyID,
		AllBuildings: allBuildings,
		BuildingIds:  buildingIDs,
	})
	if err != nil {
		return pgerr.Translate(r.pool, op, err)
	}
	if !visible {
		return store.ErrNotFound
	}
	return nil
}

// f2seriesFromRow converts a generated ProviderHourlyValue row to the
// domain model, through the one audited numeric pair
// (internal/store/postgres/numeric.go).
func f2seriesFromRow(row sqlcgen.ProviderHourlyValue) (model.ProviderHourlyValue, error) {
	activeConsumption, err := numericToDecimalPtr(row.ActiveConsumption)
	if err != nil {
		return model.ProviderHourlyValue{}, err
	}
	activeGeneration, err := numericToDecimalPtr(row.ActiveGeneration)
	if err != nil {
		return model.ProviderHourlyValue{}, err
	}
	return model.ProviderHourlyValue{
		AnalyzerID:        row.AnalyzerID,
		Ts:                row.Ts.Time,
		ActiveConsumption: activeConsumption,
		ActiveGeneration:  activeGeneration,
		SourceProvider:    model.IntegrationProvider(row.SourceProvider),
		IngestedAt:        row.IngestedAt.Time,
	}, nil
}

// f2seriesDuplicateKey reports the first row in rows whose (analyzer_id, ts)
// key repeats an earlier row's, and true — or a zero ProviderHourlyValue and
// false when every key in the batch is unique. Mirrors readingDuplicateKey
// in readings.go exactly, at this table's (shorter) key.
func f2seriesDuplicateKey(rows []model.ProviderHourlyValue) (model.ProviderHourlyValue, bool) {
	type key struct {
		analyzerID uuid.UUID
		ts         int64
	}
	seen := make(map[key]struct{}, len(rows))
	for _, row := range rows {
		k := key{row.AnalyzerID, row.Ts.UnixNano()}
		if _, ok := seen[k]; ok {
			return row, true
		}
		seen[k] = struct{}{}
	}
	return model.ProviderHourlyValue{}, false
}
