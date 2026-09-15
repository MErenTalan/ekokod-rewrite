package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/internal/pgerr"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/sqlcgen"
)

// ReadingRepository implements store.ReadingRepository: the billing-grade
// surface that reads meter_readings directly and NEVER a continuous
// aggregate (04-data-model.md §4.3; see AnalyticsRepository for the
// materialised reads).
//
// meter_readings has no company_id — every method joins through analyzers,
// per repository.go's "ROWS WITHOUT company_id" header.
type ReadingRepository struct {
	q    *sqlcgen.Queries
	pool *pgxpool.Pool
}

// NewReadingRepository wraps pool in the generated query set.
func NewReadingRepository(pool *pgxpool.Pool) *ReadingRepository {
	return &ReadingRepository{q: sqlcgen.New(pool), pool: pool}
}

var _ store.ReadingRepository = (*ReadingRepository)(nil)

// meterReadingsStagingColumns is both the staging table's column list (in
// declaration order) and the CopyFrom column list: the two must agree, and
// naming them once here is what keeps them from drifting apart.
var meterReadingsStagingColumns = []string{
	"analyzer_id", "ts", "kind",
	"active_import", "reactive_inductive_import", "reactive_capacitive_import",
	"t1_import", "t2_import", "t3_import",
	"active_export", "reactive_inductive_export", "reactive_capacitive_export",
	"t1_export", "t2_export", "t3_export",
	"max_demand_kw", "meter_serial", "multiplier_applied", "source_provider", "raw",
}

// createMeterReadingsStaging mirrors meter_readings' writable columns (every
// column but ingested_at, which the upsert below stamps with now()). It is
// plain SQL rather than a migration: the table exists for exactly one
// transaction, `on commit drop` cleans it up, and sqlc's schema catalogue
// must never see it — a temp table sqlc believes exists permanently is a
// worse trap than one it does not know about at all.
const createMeterReadingsStaging = `create temporary table meter_readings_staging (
	analyzer_id                uuid          not null,
	ts                         timestamptz   not null,
	kind                       reading_kind  not null,
	active_import              numeric(18,4),
	reactive_inductive_import  numeric(18,4),
	reactive_capacitive_import numeric(18,4),
	t1_import                  numeric(18,4),
	t2_import                  numeric(18,4),
	t3_import                  numeric(18,4),
	active_export              numeric(18,4),
	reactive_inductive_export  numeric(18,4),
	reactive_capacitive_export numeric(18,4),
	t1_export                  numeric(18,4),
	t2_export                  numeric(18,4),
	t3_export                  numeric(18,4),
	max_demand_kw              numeric(14,4),
	meter_serial               text,
	multiplier_applied         numeric(12,6) not null default 1,
	source_provider            integration_provider not null,
	raw                        jsonb
) on commit drop`

// upsertMeterReadingsFromStaging is the one idempotent write 04-data-model.md
// §14 requires: `insert … on conflict (analyzer_id, ts, kind) do update`.
//
// This is a PLAIN select, not `distinct on`: BulkInsert refuses the whole
// batch with store.ErrConflict, before the staging table even exists, the
// moment two rows in the CALLER'S SLICE share an (analyzer_id, ts, kind) key
// (see readingDuplicateKey). A `distinct on` here would silently pick an
// arbitrary winner between two batched rows with different register values —
// unacceptable for a billing-grade register — so once the Go-side check
// guarantees the staging table itself never holds two rows with the same key,
// keeping `distinct on` would be dead weight hiding that guarantee, not
// reinforcing it. `returning (xmax = 0) as inserted` is the standard,
// reliable way to tell which outcome each row took: a freshly inserted row's
// xmax is still 0, an updated row's is the current transaction's id.
const upsertMeterReadingsFromStaging = `insert into meter_readings (
	analyzer_id, ts, kind,
	active_import, reactive_inductive_import, reactive_capacitive_import,
	t1_import, t2_import, t3_import,
	active_export, reactive_inductive_export, reactive_capacitive_export,
	t1_export, t2_export, t3_export,
	max_demand_kw, meter_serial, multiplier_applied, source_provider, raw
)
select
	analyzer_id, ts, kind,
	active_import, reactive_inductive_import, reactive_capacitive_import,
	t1_import, t2_import, t3_import,
	active_export, reactive_inductive_export, reactive_capacitive_export,
	t1_export, t2_export, t3_export,
	max_demand_kw, meter_serial, multiplier_applied, source_provider, raw
from meter_readings_staging
on conflict (analyzer_id, ts, kind) do update set
	active_import              = excluded.active_import,
	reactive_inductive_import  = excluded.reactive_inductive_import,
	reactive_capacitive_import = excluded.reactive_capacitive_import,
	t1_import                  = excluded.t1_import,
	t2_import                  = excluded.t2_import,
	t3_import                  = excluded.t3_import,
	active_export              = excluded.active_export,
	reactive_inductive_export  = excluded.reactive_inductive_export,
	reactive_capacitive_export = excluded.reactive_capacitive_export,
	t1_export                  = excluded.t1_export,
	t2_export                  = excluded.t2_export,
	t3_export                  = excluded.t3_export,
	max_demand_kw              = excluded.max_demand_kw,
	meter_serial               = excluded.meter_serial,
	multiplier_applied         = excluded.multiplier_applied,
	source_provider            = excluded.source_provider,
	ingested_at                = now(),
	raw                        = excluded.raw
returning (xmax = 0) as inserted`

// BulkInsert implements store.ReadingRepository.BulkInsert.
//
// It is the one write in this file that does not go through sqlc: a per-call
// temporary table cannot appear in sqlc's schema catalogue (see
// createMeterReadingsStaging), so the whole COPY-then-upsert runs as plain
// SQL against a *pgxpool.Tx. The visibility check that guards it, however, IS
// a generated query (ReadingVisibleAnalyzerIDs) — there is nothing dynamic
// about it.
func (r *ReadingRepository) BulkInsert(ctx context.Context, s store.Scope, rows []model.MeterReading) (inserted, updated int, err error) {
	if !s.Valid() {
		return 0, 0, store.ErrInvalidScope
	}
	if len(rows) == 0 {
		return 0, 0, nil
	}

	const op = "reading bulk insert"
	buildingIDs, allBuildings := s.BuildingFilter()

	// A duplicate (analyzer_id, ts, kind) inside the CALLER'S OWN batch is
	// refused loudly, before any database round trip: two rows with the same
	// key but different register values would otherwise reach
	// `on conflict … do update` with no way to know which one Postgres would
	// pick last (see the controller ruling recorded on
	// store.ReadingRepository.BulkInsert). F2's ingestion deduplicates
	// upstream; this is the last line of defence.
	if dup, ok := readingDuplicateKey(rows); ok {
		return 0, 0, fmt.Errorf("%w: duplicate reading key analyzer_id=%s ts=%s kind=%s",
			store.ErrConflict, dup.AnalyzerID, dup.Ts.Format(time.RFC3339Nano), dup.Kind)
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
	// transaction, so a concurrent reassignment or soft-delete cannot open a
	// gap between this check and the write below — the reason this is a
	// locking SELECT and not the plain COUNT(*) a first draft used (which
	// cannot take FOR SHARE at all: Postgres rejects FOR UPDATE/SHARE
	// combined with an aggregate).
	visibleIDs, err := r.q.WithTx(tx).ReadingVisibleAnalyzerIDs(ctx, sqlcgen.ReadingVisibleAnalyzerIDsParams{
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

	if _, err := tx.Exec(ctx, createMeterReadingsStaging); err != nil {
		return 0, 0, pgerr.Translate(r.pool, op, err)
	}

	_, err = tx.CopyFrom(ctx, pgx.Identifier{"meter_readings_staging"}, meterReadingsStagingColumns,
		pgx.CopyFromSlice(len(rows), func(i int) ([]any, error) {
			row := rows[i]
			return []any{
				row.AnalyzerID,
				row.Ts,
				string(row.Kind),
				decimalPtrToNumeric(row.ActiveImport),
				decimalPtrToNumeric(row.ReactiveInductiveImport),
				decimalPtrToNumeric(row.ReactiveCapacitiveImport),
				decimalPtrToNumeric(row.T1Import),
				decimalPtrToNumeric(row.T2Import),
				decimalPtrToNumeric(row.T3Import),
				decimalPtrToNumeric(row.ActiveExport),
				decimalPtrToNumeric(row.ReactiveInductiveExport),
				decimalPtrToNumeric(row.ReactiveCapacitiveExport),
				decimalPtrToNumeric(row.T1Export),
				decimalPtrToNumeric(row.T2Export),
				decimalPtrToNumeric(row.T3Export),
				decimalPtrToNumeric(row.MaxDemandKw),
				row.MeterSerial,
				decimalToNumeric(row.MultiplierApplied),
				string(row.SourceProvider),
				readingRawJSONOrNil(row.Raw),
			}, nil
		}))
	if err != nil {
		return 0, 0, pgerr.Translate(r.pool, op, err)
	}

	upsertRows, err := tx.Query(ctx, upsertMeterReadingsFromStaging)
	if err != nil {
		return 0, 0, pgerr.Translate(r.pool, op, err)
	}
	inserted, updated, err = timeseriesScanUpsertCounts(upsertRows)
	if err != nil {
		return 0, 0, pgerr.Translate(r.pool, op, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, 0, pgerr.Translate(r.pool, op, err)
	}
	return inserted, updated, nil
}

// Range implements store.ReadingRepository.Range.
//
// The scope is carried by ReadingRange's OWN join through analyzers, not by
// a separate check run first: this method runs that query unconditionally
// and consults requireAnalyzerVisible only to choose an error when it comes
// back empty, so a bug in the (redundant) existence check can never be the
// reason foreign data is or is not returned — the query's own predicate is.
func (r *ReadingRepository) Range(ctx context.Context, s store.Scope, analyzerID uuid.UUID, tr store.TimeRange, kind model.ReadingKind) ([]model.MeterReading, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	if !tr.Valid() {
		return nil, store.ErrInvalidRange
	}
	const op = "reading range"
	buildingIDs, allBuildings := s.BuildingFilter()

	rows, err := r.q.ReadingRange(ctx, sqlcgen.ReadingRangeParams{
		AnalyzerID:   analyzerID,
		Kind:         sqlcgen.ReadingKind(kind),
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
		// Zero rows either means the analyzer is not visible, or it is
		// visible with nothing in this window — disambiguate.
		if err := r.requireAnalyzerVisible(ctx, s, analyzerID, op); err != nil {
			return nil, err
		}
		return []model.MeterReading{}, nil
	}
	out := make([]model.MeterReading, len(rows))
	for i, row := range rows {
		mr, err := readingFromRow(row)
		if err != nil {
			return nil, pgerr.Translate(r.pool, op, err)
		}
		out[i] = mr
	}
	return out, nil
}

// BoundaryReadings implements store.ReadingRepository.BoundaryReadings.
func (r *ReadingRepository) BoundaryReadings(ctx context.Context, s store.Scope, analyzerID uuid.UUID, kind model.ReadingKind, start, end time.Time) (startReading, endReading *model.MeterReading, err error) {
	if !s.Valid() {
		return nil, nil, store.ErrInvalidScope
	}
	const op = "reading boundary"

	startReading, err = r.boundaryAtOrBefore(ctx, s, analyzerID, kind, start, op)
	if err != nil {
		return nil, nil, err
	}
	endReading, err = r.boundaryAtOrBefore(ctx, s, analyzerID, kind, end, op)
	if err != nil {
		return nil, nil, err
	}
	return startReading, endReading, nil
}

// boundaryAtOrBefore, like Range, runs ReadingBoundaryAtOrBefore's own
// scoped query first and unconditionally, and calls requireAnalyzerVisible
// only when it comes back empty.
func (r *ReadingRepository) boundaryAtOrBefore(ctx context.Context, s store.Scope, analyzerID uuid.UUID, kind model.ReadingKind, at time.Time, op string) (*model.MeterReading, error) {
	buildingIDs, allBuildings := s.BuildingFilter()
	row, err := r.q.ReadingBoundaryAtOrBefore(ctx, sqlcgen.ReadingBoundaryAtOrBeforeParams{
		AnalyzerID:   analyzerID,
		Kind:         sqlcgen.ReadingKind(kind),
		At:           timeseriesToTimestamptz(at),
		CompanyID:    s.CompanyID,
		AllBuildings: allBuildings,
		BuildingIds:  buildingIDs,
	})
	if err != nil {
		translated := pgerr.Translate(r.pool, op, err)
		if !timeseriesIsNotFound(translated) {
			return nil, translated
		}
		// Zero rows: disambiguate "not visible" (ErrNotFound) from "visible,
		// nothing at or before `at`" (nil — 02-domain-rules.md §3.1: a real,
		// nil-shaped absence, not an error).
		if err := r.requireAnalyzerVisible(ctx, s, analyzerID, op); err != nil {
			return nil, err
		}
		return nil, nil
	}
	mr, err := readingFromRow(row)
	if err != nil {
		return nil, pgerr.Translate(r.pool, op, err)
	}
	return &mr, nil
}

// Latest implements store.ReadingRepository.Latest.
func (r *ReadingRepository) Latest(ctx context.Context, s store.Scope, analyzerID uuid.UUID, tr store.TimeRange, kind model.ReadingKind) (*model.MeterReading, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	if !tr.Valid() {
		return nil, store.ErrInvalidRange
	}
	const op = "reading latest"
	buildingIDs, allBuildings := s.BuildingFilter()

	row, err := r.q.ReadingLatest(ctx, sqlcgen.ReadingLatestParams{
		AnalyzerID:   analyzerID,
		Kind:         sqlcgen.ReadingKind(kind),
		FromTs:       timeseriesToTimestamptz(tr.From),
		ToTs:         timeseriesToTimestamptz(tr.To),
		CompanyID:    s.CompanyID,
		AllBuildings: allBuildings,
		BuildingIds:  buildingIDs,
	})
	if err != nil {
		translated := pgerr.Translate(r.pool, op, err)
		if !timeseriesIsNotFound(translated) {
			return nil, translated
		}
		if err := r.requireAnalyzerVisible(ctx, s, analyzerID, op); err != nil {
			return nil, err
		}
		return nil, nil
	}
	mr, err := readingFromRow(row)
	if err != nil {
		return nil, pgerr.Translate(r.pool, op, err)
	}
	return &mr, nil
}

// requireAnalyzerVisible returns store.ErrNotFound when analyzerID is not
// visible to s — missing, another tenant's, or outside the Scope's
// buildings. It exists only to CHOOSE AN ERROR after a properly-scoped data
// query has already come back empty (Range, boundaryAtOrBefore, Latest); it
// is never the sole gate on whether data is returned, and BulkInsert (which
// checks a whole batch at once, with a row lock) does not use it at all.
func (r *ReadingRepository) requireAnalyzerVisible(ctx context.Context, s store.Scope, analyzerID uuid.UUID, op string) error {
	buildingIDs, allBuildings := s.BuildingFilter()
	visible, err := r.q.ReadingAnalyzerVisible(ctx, sqlcgen.ReadingAnalyzerVisibleParams{
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

// readingFromRow converts a generated MeterReading row to the domain model,
// through the one audited numeric pair (internal/store/postgres/numeric.go).
func readingFromRow(row sqlcgen.MeterReading) (model.MeterReading, error) {
	activeImport, err := numericToDecimalPtr(row.ActiveImport)
	if err != nil {
		return model.MeterReading{}, err
	}
	reactiveInductiveImport, err := numericToDecimalPtr(row.ReactiveInductiveImport)
	if err != nil {
		return model.MeterReading{}, err
	}
	reactiveCapacitiveImport, err := numericToDecimalPtr(row.ReactiveCapacitiveImport)
	if err != nil {
		return model.MeterReading{}, err
	}
	t1Import, err := numericToDecimalPtr(row.T1Import)
	if err != nil {
		return model.MeterReading{}, err
	}
	t2Import, err := numericToDecimalPtr(row.T2Import)
	if err != nil {
		return model.MeterReading{}, err
	}
	t3Import, err := numericToDecimalPtr(row.T3Import)
	if err != nil {
		return model.MeterReading{}, err
	}
	activeExport, err := numericToDecimalPtr(row.ActiveExport)
	if err != nil {
		return model.MeterReading{}, err
	}
	reactiveInductiveExport, err := numericToDecimalPtr(row.ReactiveInductiveExport)
	if err != nil {
		return model.MeterReading{}, err
	}
	reactiveCapacitiveExport, err := numericToDecimalPtr(row.ReactiveCapacitiveExport)
	if err != nil {
		return model.MeterReading{}, err
	}
	t1Export, err := numericToDecimalPtr(row.T1Export)
	if err != nil {
		return model.MeterReading{}, err
	}
	t2Export, err := numericToDecimalPtr(row.T2Export)
	if err != nil {
		return model.MeterReading{}, err
	}
	t3Export, err := numericToDecimalPtr(row.T3Export)
	if err != nil {
		return model.MeterReading{}, err
	}
	maxDemandKw, err := numericToDecimalPtr(row.MaxDemandKw)
	if err != nil {
		return model.MeterReading{}, err
	}
	multiplierApplied, err := numericToDecimal(row.MultiplierApplied)
	if err != nil {
		return model.MeterReading{}, err
	}

	return model.MeterReading{
		AnalyzerID:               row.AnalyzerID,
		Ts:                       row.Ts.Time,
		Kind:                     model.ReadingKind(row.Kind),
		ActiveImport:             activeImport,
		ReactiveInductiveImport:  reactiveInductiveImport,
		ReactiveCapacitiveImport: reactiveCapacitiveImport,
		T1Import:                 t1Import,
		T2Import:                 t2Import,
		T3Import:                 t3Import,
		ActiveExport:             activeExport,
		ReactiveInductiveExport:  reactiveInductiveExport,
		ReactiveCapacitiveExport: reactiveCapacitiveExport,
		T1Export:                 t1Export,
		T2Export:                 t2Export,
		T3Export:                 t3Export,
		MaxDemandKw:              maxDemandKw,
		MeterSerial:              row.MeterSerial,
		MultiplierApplied:        multiplierApplied,
		SourceProvider:           model.IntegrationProvider(row.SourceProvider),
		IngestedAt:               row.IngestedAt.Time,
		Raw:                      row.Raw,
	}, nil
}

// --- small helpers shared by every BulkInsert in this package ------------

// readingDistinctUUIDs returns ids with duplicates removed, order-preserving on
// first occurrence. Every batch-scope visibility check in this file counts
// against the DISTINCT set, so that a batch repeating one analyzer many times
// still requires exactly one visible row, never a coincidentally-matching
// count.
func readingDistinctUUIDs(ids []uuid.UUID) []uuid.UUID {
	seen := make(map[uuid.UUID]struct{}, len(ids))
	out := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// timeseriesScanUpsertCounts consumes the `returning (xmax = 0) as inserted` rows every
// staging-table upsert in this package produces and splits them into
// inserted/updated counts — the REAL counts BulkInsert promises, never
// estimated.
func timeseriesScanUpsertCounts(rows pgx.Rows) (inserted, updated int, err error) {
	defer rows.Close()
	for rows.Next() {
		var isInsert bool
		if err := rows.Scan(&isInsert); err != nil {
			return 0, 0, err
		}
		if isInsert {
			inserted++
		} else {
			updated++
		}
	}
	if err := rows.Err(); err != nil {
		return 0, 0, err
	}
	return inserted, updated, nil
}

// timeseriesToTimestamptz converts a time.Time known to be non-zero (every TimeRange
// bound is, once Valid() has passed) to the generated parameter type.
func timeseriesToTimestamptz(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

// readingRawJSONOrNil turns an empty/nil json.RawMessage into an untyped nil, so
// pgx's CopyFrom encodes it as SQL NULL rather than an empty jsonb value.
func readingRawJSONOrNil(raw []byte) any {
	if len(raw) == 0 {
		return nil
	}
	return raw
}

// timeseriesIsNotFound reports whether err — already through pgerr.Translate — is
// store.ErrNotFound. Every BoundaryReadings/Latest-shaped method in this
// package uses it to tell "no row in this window" (nil, no error) from
// "the id itself is not visible" (which its own explicit visibility check
// already turned into ErrNotFound before the query ran).
func timeseriesIsNotFound(err error) bool { return errors.Is(err, store.ErrNotFound) }

// readingDuplicateKey reports the first row in rows whose
// (analyzer_id, ts, kind) key repeats an earlier row's, and true — or a zero
// MeterReading and false when every key in the batch is unique. BulkInsert
// uses it to refuse the whole batch loudly, before the staging table exists,
// rather than let `on conflict … do update` silently pick whichever of the
// two rows Postgres processes last as the stored value for a billing-grade
// register (controller ruling, task-10 fix round 1).
func readingDuplicateKey(rows []model.MeterReading) (model.MeterReading, bool) {
	type key struct {
		analyzerID uuid.UUID
		ts         int64
		kind       model.ReadingKind
	}
	seen := make(map[key]struct{}, len(rows))
	for _, row := range rows {
		k := key{row.AnalyzerID, row.Ts.UnixNano(), row.Kind}
		if _, ok := seen[k]; ok {
			return row, true
		}
		seen[k] = struct{}{}
	}
	return model.MeterReading{}, false
}
