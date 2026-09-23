package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/internal/pgerr"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/sqlcgen"
)

var istanbulDays = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		panic(err)
	}
	return loc
}()

// dayWindows turns Istanbul days into [from, to) windows and checks every
// row timestamp falls inside one (R278: a day is replaced, never merged).
func dayWindows(days []time.Time, stamps []time.Time) ([][2]time.Time, error) {
	windows := make([][2]time.Time, 0, len(days))
	for _, d := range days {
		l := d.In(istanbulDays)
		from := time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, istanbulDays)
		windows = append(windows, [2]time.Time{from, from.AddDate(0, 0, 1)})
	}
	for _, ts := range stamps {
		inside := false
		for _, w := range windows {
			if !ts.Before(w[0]) && ts.Before(w[1]) {
				inside = true
				break
			}
		}
		if !inside {
			return nil, store.ErrInvalidRange
		}
	}
	return windows, nil
}

// solarPlantVisible chooses ErrNotFound for an empty answer; never a data gate.
func solarPlantVisible(ctx context.Context, q *sqlcgen.Queries, pool *pgxpool.Pool, s store.Scope, plantID uuid.UUID) error {
	ok, err := q.ProductionPlantVisible(ctx, sqlcgen.ProductionPlantVisibleParams{PlantID: plantID, CompanyID: s.CompanyID})
	if err != nil {
		return pgerr.Translate(pool, "plant visible", err)
	}
	if !ok {
		return store.ErrNotFound
	}
	return nil
}

// ProductionTotalsRepository implements store.ProductionTotalsRepository.
type ProductionTotalsRepository struct {
	q    *sqlcgen.Queries
	pool *pgxpool.Pool
}

// NewProductionTotalsRepository wraps pool.
func NewProductionTotalsRepository(pool *pgxpool.Pool) *ProductionTotalsRepository {
	return &ProductionTotalsRepository{q: sqlcgen.New(pool), pool: pool}
}

var _ store.ProductionTotalsRepository = (*ProductionTotalsRepository)(nil)

// ReplaceDays deletes the plant's rows inside days and inserts rows, in one transaction.
func (r *ProductionTotalsRepository) ReplaceDays(ctx context.Context, s store.Scope, plantID uuid.UUID, days []time.Time, rows []model.PlantProductionTotal) (int, error) {
	if !s.Valid() {
		return 0, store.ErrInvalidScope
	}
	const op = "replace production totals"
	stamps := make([]time.Time, len(rows))
	seen := make(map[int64]bool, len(rows))
	for i, row := range rows {
		if row.PlantID != plantID {
			return 0, store.ErrNotFound
		}
		stamps[i] = row.Ts
		if seen[row.Ts.UnixNano()] {
			return 0, fmt.Errorf("%w: duplicate production total ts=%s", store.ErrConflict, row.Ts.Format(time.RFC3339))
		}
		seen[row.Ts.UnixNano()] = true
	}
	windows, err := dayWindows(days, stamps)
	if err != nil {
		return 0, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, pgerr.Translate(r.pool, op, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.q.WithTx(tx)
	if _, err := q.SolarPlantVisibleForShare(ctx, sqlcgen.SolarPlantVisibleForShareParams{PlantID: plantID, CompanyID: s.CompanyID}); err != nil {
		return 0, pgerr.Translate(r.pool, op, err)
	}
	for _, w := range windows {
		if _, err := q.TotalsDeleteRange(ctx, sqlcgen.TotalsDeleteRangeParams{
			PlantID: plantID, CompanyID: s.CompanyID, FromTs: ts(w[0]), ToTs: ts(w[1]),
		}); err != nil {
			return 0, pgerr.Translate(r.pool, op, err)
		}
	}
	n, err := tx.CopyFrom(ctx, pgx.Identifier{"plant_production_totals"},
		[]string{"plant_id", "ts", "production_kwh", "active_power_kw", "basis"},
		pgx.CopyFromSlice(len(rows), func(i int) ([]any, error) {
			row := rows[i]
			return []any{row.PlantID, row.Ts, decimalPtrToNumeric(row.ProductionKwh), decimalPtrToNumeric(row.ActivePowerKw), row.Basis}, nil
		}))
	if err != nil {
		return 0, pgerr.Translate(r.pool, op, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, pgerr.Translate(r.pool, op, err)
	}
	return int(n), nil
}

// Range reads totals over r, scoped through power_plants in the query.
func (r *ProductionTotalsRepository) Range(ctx context.Context, s store.Scope, plantID uuid.UUID, tr store.TimeRange) ([]model.PlantProductionTotal, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	if !tr.Valid() {
		return nil, store.ErrInvalidRange
	}
	rows, err := r.q.TotalsRange(ctx, sqlcgen.TotalsRangeParams{PlantID: plantID, CompanyID: s.CompanyID, FromTs: ts(tr.From), ToTs: ts(tr.To)})
	if err != nil {
		return nil, pgerr.Translate(r.pool, "production totals range", err)
	}
	if len(rows) == 0 {
		if err := solarPlantVisible(ctx, r.q, r.pool, s, plantID); err != nil {
			return nil, err
		}
	}
	out := make([]model.PlantProductionTotal, 0, len(rows))
	for _, row := range rows {
		kwh, err := numericToDecimalPtr(row.ProductionKwh)
		if err != nil {
			return nil, err
		}
		kw, err := numericToDecimalPtr(row.ActivePowerKw)
		if err != nil {
			return nil, err
		}
		out = append(out, model.PlantProductionTotal{PlantID: row.PlantID, Ts: row.Ts.Time, ProductionKwh: kwh, ActivePowerKw: kw, Basis: row.Basis})
	}
	return out, nil
}

// FirstDay is the Istanbul day of the plant's earliest total, or nil.
func (r *ProductionTotalsRepository) FirstDay(ctx context.Context, s store.Scope, plantID uuid.UUID) (*time.Time, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	first, err := r.q.TotalsFirstTs(ctx, sqlcgen.TotalsFirstTsParams{PlantID: plantID, CompanyID: s.CompanyID})
	if err != nil {
		return nil, pgerr.Translate(r.pool, "production totals first day", err)
	}
	if !first.Valid {
		return nil, solarPlantVisible(ctx, r.q, r.pool, s, plantID)
	}
	l := first.Time.In(istanbulDays)
	day := time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, istanbulDays)
	return &day, nil
}

// ReplaceDeviceDays is ReplaceDays for device rows (R278).
func (r *ProductionRepository) ReplaceDeviceDays(ctx context.Context, s store.Scope, plantID uuid.UUID, days []time.Time, rows []model.PlantProduction) (int, error) {
	if !s.Valid() {
		return 0, store.ErrInvalidScope
	}
	const op = "replace device production"
	stamps := make([]time.Time, len(rows))
	for i, row := range rows {
		if row.PlantID != plantID {
			return 0, store.ErrNotFound
		}
		stamps[i] = row.Ts
	}
	if dup, ok := productionDuplicateKey(rows); ok {
		return 0, fmt.Errorf("%w: duplicate production key ts=%s device_id=%s", store.ErrConflict, dup.Ts.Format(time.RFC3339), dup.DeviceID)
	}
	windows, err := dayWindows(days, stamps)
	if err != nil {
		return 0, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, pgerr.Translate(r.pool, op, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.q.WithTx(tx)
	if _, err := q.SolarPlantVisibleForShare(ctx, sqlcgen.SolarPlantVisibleForShareParams{PlantID: plantID, CompanyID: s.CompanyID}); err != nil {
		return 0, pgerr.Translate(r.pool, op, err)
	}
	if len(rows) > 0 {
		pairPlants, pairDevices := distinctPlantDevicePairs(rows)
		valid, err := q.ProductionValidDevicePairCount(ctx, sqlcgen.ProductionValidDevicePairCountParams{PlantIds: pairPlants, DeviceIds: pairDevices})
		if err != nil {
			return 0, pgerr.Translate(r.pool, op, err)
		}
		if valid != int64(len(pairPlants)) {
			return 0, store.ErrNotFound
		}
	}
	for _, w := range windows {
		if _, err := q.DeviceProductionDeleteRange(ctx, sqlcgen.DeviceProductionDeleteRangeParams{
			PlantID: plantID, CompanyID: s.CompanyID, FromTs: ts(w[0]), ToTs: ts(w[1]),
		}); err != nil {
			return 0, pgerr.Translate(r.pool, op, err)
		}
	}
	n, err := tx.CopyFrom(ctx, pgx.Identifier{"plant_production"}, plantProductionStagingColumns,
		pgx.CopyFromSlice(len(rows), func(i int) ([]any, error) {
			row := rows[i]
			source := row.Source
			if source == "" {
				source = "isolar"
			}
			return []any{row.PlantID, row.Ts, row.DeviceID,
				decimalPtrToNumeric(row.ProductionKwh), decimalPtrToNumeric(row.ActivePowerKw), decimalPtrToNumeric(row.EfficiencyPct),
				decimalPtrToNumeric(row.IrradianceWm2), decimalPtrToNumeric(row.ModuleTempC), decimalPtrToNumeric(row.AmbientTempC), source}, nil
		}))
	if err != nil {
		return 0, pgerr.Translate(r.pool, op, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, pgerr.Translate(r.pool, op, err)
	}
	return int(n), nil
}

func withDeviceSnapshot(d model.PlantDevice, at pgtype.Timestamptz, faultStatus *int32, power, today, month, year, total pgtype.Numeric) (model.PlantDevice, error) {
	d.SnapshotAt, d.FaultStatus = tsPtr(at), faultStatus
	for _, pair := range []struct {
		dst **decimal.Decimal
		src pgtype.Numeric
	}{{&d.ActivePowerKw, power}, {&d.YieldTodayKwh, today}, {&d.YieldMonthKwh, month}, {&d.YieldYearKwh, year}, {&d.YieldTotalKwh, total}} {
		v, err := numericToDecimalPtr(pair.src)
		if err != nil {
			return model.PlantDevice{}, err
		}
		*pair.dst = v
	}
	return d, nil
}

// UpdateDeviceSnapshot stores a device's realtime snapshot (R282).
func (r *PlantRepository) UpdateDeviceSnapshot(ctx context.Context, s store.Scope, d model.PlantDevice) error {
	if !s.Valid() {
		return store.ErrInvalidScope
	}
	n, err := r.q.PlantDeviceSnapshotUpdate(ctx, sqlcgen.PlantDeviceSnapshotUpdateParams{
		SnapshotAt: tsPtrOrZero(d.SnapshotAt), FaultStatus: d.FaultStatus,
		ActivePowerKw: decimalPtrToNumeric(d.ActivePowerKw), YieldTodayKwh: decimalPtrToNumeric(d.YieldTodayKwh),
		YieldMonthKwh: decimalPtrToNumeric(d.YieldMonthKwh), YieldYearKwh: decimalPtrToNumeric(d.YieldYearKwh),
		YieldTotalKwh: decimalPtrToNumeric(d.YieldTotalKwh), Status: d.Status,
		ID: d.ID, PlantID: d.PlantID, CompanyID: s.CompanyID,
	})
	if err != nil {
		return pgerr.Translate(r.pool, "update device snapshot", err)
	}
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

// SetSyncState records the last sync time and its closed error code (nil = ok).
func (r *PlantRepository) SetSyncState(ctx context.Context, s store.Scope, plantID uuid.UUID, at time.Time, errCode *string) error {
	if !s.Valid() {
		return store.ErrInvalidScope
	}
	n, err := r.q.PlantSetSyncState(ctx, sqlcgen.PlantSetSyncStateParams{At: ts(at), ErrorCode: errCode, ID: plantID, CompanyID: s.CompanyID})
	if err != nil {
		return pgerr.Translate(r.pool, "set plant sync state", err)
	}
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

// SetIsolarLink writes the link fields (nil clears them) and total capacity.
func (r *PlantRepository) SetIsolarLink(ctx context.Context, s store.Scope, p model.PowerPlant) (model.PowerPlant, error) {
	if !s.Valid() {
		return model.PowerPlant{}, store.ErrInvalidScope
	}
	row, err := r.q.PlantSetIsolarLink(ctx, sqlcgen.PlantSetIsolarLinkParams{
		IsolarPsID: p.IsolarPsID, IsolarPsKey: p.IsolarPsKey, IsolarPsName: p.IsolarPsName,
		IsolarInstalledKw: decimalPtrToNumeric(p.IsolarInstalledKw), IsolarLinkedAt: tsPtrOrZero(p.IsolarLinkedAt),
		IsolarCredentialID: p.IsolarCredentialID, TotalCapacityKw: decimalPtrToNumeric(p.TotalCapacityKw),
		ID: p.ID, CompanyID: s.CompanyID,
	})
	if err != nil {
		return model.PowerPlant{}, pgerr.Translate(r.pool, "set isolar link", err)
	}
	return plantFromRow(row)
}

// ListLinked lists the scope company's iSolar-linked plants.
func (r *PlantRepository) ListLinked(ctx context.Context, s store.Scope) ([]model.PowerPlant, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	rows, err := r.q.PlantListLinked(ctx, s.CompanyID)
	if err != nil {
		return nil, pgerr.Translate(r.pool, "list linked plants", err)
	}
	out := make([]model.PowerPlant, 0, len(rows))
	for _, row := range rows {
		p, err := plantFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

// FaultRepository implements store.FaultRepository.
type FaultRepository struct {
	q    *sqlcgen.Queries
	pool *pgxpool.Pool
}

// NewFaultRepository wraps pool.
func NewFaultRepository(pool *pgxpool.Pool) *FaultRepository {
	return &FaultRepository{q: sqlcgen.New(pool), pool: pool}
}

var _ store.FaultRepository = (*FaultRepository)(nil)

// Upsert stores faults; a fault of an invisible plant is refused.
func (r *FaultRepository) Upsert(ctx context.Context, s store.Scope, faults []model.PlantFault) (int, error) {
	if !s.Valid() {
		return 0, store.ErrInvalidScope
	}
	total := 0
	for _, f := range faults {
		n, err := r.q.FaultUpsert(ctx, sqlcgen.FaultUpsertParams{
			PlantID: f.PlantID, Ref: f.Ref, Code: f.Code, Name: f.Name, Level: f.Level, Type: f.Type,
			DeviceName: f.DeviceName, OccurredAt: ts(f.OccurredAt), ClosedAt: tsPtrOrZero(f.ClosedAt),
			FirstSeenAt: tsOrNow(f.FirstSeenAt), CompanyID: s.CompanyID,
		})
		if err != nil {
			return total, pgerr.Translate(r.pool, "upsert fault", err)
		}
		if n == 0 {
			return total, store.ErrNotFound
		}
		total += int(n)
	}
	return total, nil
}

func faultFromRow(row sqlcgen.PlantFault) model.PlantFault {
	return model.PlantFault{PlantID: row.PlantID, Ref: row.Ref, Code: row.Code, Name: row.Name, Level: row.Level, Type: row.Type,
		DeviceName: row.DeviceName, OccurredAt: row.OccurredAt.Time, ClosedAt: tsPtr(row.ClosedAt), FirstSeenAt: row.FirstSeenAt.Time}
}

// List pages a plant's faults, newest first, with the total.
func (r *FaultRepository) List(ctx context.Context, s store.Scope, plantID uuid.UUID, p store.Page) ([]model.PlantFault, int, error) {
	if !s.Valid() {
		return nil, 0, store.ErrInvalidScope
	}
	rows, err := r.q.FaultList(ctx, sqlcgen.FaultListParams{PlantID: plantID, CompanyID: s.CompanyID, PageLimit: pageLimit(p, 50, 500), PageOffset: p.Offset})
	if err != nil {
		return nil, 0, pgerr.Translate(r.pool, "list faults", err)
	}
	count, err := r.q.FaultCount(ctx, sqlcgen.FaultCountParams{PlantID: plantID, CompanyID: s.CompanyID})
	if err != nil {
		return nil, 0, pgerr.Translate(r.pool, "count faults", err)
	}
	if count == 0 {
		if err := solarPlantVisible(ctx, r.q, r.pool, s, plantID); err != nil {
			return nil, 0, err
		}
	}
	out := make([]model.PlantFault, len(rows))
	for i, row := range rows {
		out[i] = faultFromRow(row)
	}
	return out, int(count), nil
}

// Unforwarded lists faults since `since` whose ref was never claimed.
func (r *FaultRepository) Unforwarded(ctx context.Context, s store.Scope, plantID uuid.UUID, since time.Time) ([]model.PlantFault, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	rows, err := r.q.FaultUnforwarded(ctx, sqlcgen.FaultUnforwardedParams{PlantID: plantID, CompanyID: s.CompanyID, Since: ts(since)})
	if err != nil {
		return nil, pgerr.Translate(r.pool, "unforwarded faults", err)
	}
	if len(rows) == 0 {
		if err := solarPlantVisible(ctx, r.q, r.pool, s, plantID); err != nil {
			return nil, err
		}
	}
	out := make([]model.PlantFault, len(rows))
	for i, row := range rows {
		out[i] = faultFromRow(row)
	}
	return out, nil
}

// Claim marks refs forwarded and returns only the ones this call inserted (R287).
func (r *FaultRepository) Claim(ctx context.Context, s store.Scope, plantID uuid.UUID, refs []string) ([]string, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	if len(refs) == 0 {
		return nil, nil
	}
	got, err := r.q.FaultClaim(ctx, sqlcgen.FaultClaimParams{PlantID: plantID, Refs: refs, CompanyID: s.CompanyID})
	if err != nil {
		return nil, pgerr.Translate(r.pool, "claim faults", err)
	}
	if len(got) == 0 {
		if err := solarPlantVisible(ctx, r.q, r.pool, s, plantID); err != nil {
			return nil, err
		}
	}
	return got, nil
}

// Release undoes a claim after a failed send, so the next run retries.
func (r *FaultRepository) Release(ctx context.Context, s store.Scope, plantID uuid.UUID, refs []string) error {
	if !s.Valid() {
		return store.ErrInvalidScope
	}
	if _, err := r.q.FaultRelease(ctx, sqlcgen.FaultReleaseParams{PlantID: plantID, CompanyID: s.CompanyID, Refs: refs}); err != nil {
		return pgerr.Translate(r.pool, "release faults", err)
	}
	return nil
}

// Prune deletes faults and forwarded refs older than before; returns faults deleted.
func (r *FaultRepository) Prune(ctx context.Context, s store.Scope, before time.Time) (int, error) {
	if !s.Valid() {
		return 0, store.ErrInvalidScope
	}
	n, err := r.q.FaultPrune(ctx, sqlcgen.FaultPruneParams{CompanyID: s.CompanyID, Before: ts(before)})
	if err != nil {
		return 0, pgerr.Translate(r.pool, "prune faults", err)
	}
	if _, err := r.q.FaultForwardedPrune(ctx, sqlcgen.FaultForwardedPruneParams{CompanyID: s.CompanyID, Before: ts(before)}); err != nil {
		return 0, pgerr.Translate(r.pool, "prune forwarded alarms", err)
	}
	return int(n), nil
}
