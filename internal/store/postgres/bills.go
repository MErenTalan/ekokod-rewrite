package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/internal/pgerr"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/sqlcgen"
)

// billDefaultPageLimit and billMaxPageLimit are this file's Page defaults.
const (
	billDefaultPageLimit = 50
	billMaxPageLimit     = 500
)

func billPageLimits(p store.Page) (limit, offset int32) {
	limit = p.Limit
	if limit <= 0 {
		limit = billDefaultPageLimit
	}
	if limit > billMaxPageLimit {
		limit = billMaxPageLimit
	}
	offset = p.Offset
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

// errBillCannotSetSuperseded is returned by UpdateStatus for an attempt to
// set BillStatusSuperseded directly. It is not one of the store sentinels:
// this is a caller programming error, not a scope or existence question, and
// wrapping it as ErrNotFound or ErrConflict would make it indistinguishable
// from either at the call site.
var errBillCannotSetSuperseded = errors.New("bill status superseded may only be set by Supersede")

// errBillSupersedeMismatch is returned by Supersede when the replacement
// bill's (scope, subject, period) does not match the bill being replaced.
// Like errBillCannotSetSuperseded, this is a caller programming error —
// recomputing bill X with a payload that names a different scope, subject or
// period — not a scope or existence question, so it is not one of the store
// sentinels either.
var errBillSupersedeMismatch = errors.New("supersede replacement must keep the replaced bill's scope, subject and period")

// BillRepository implements store.BillRepository.
type BillRepository struct {
	q    *sqlcgen.Queries
	pool *pgxpool.Pool
}

// NewBillRepository builds a BillRepository over pool.
func NewBillRepository(pool *pgxpool.Pool) *BillRepository {
	return &BillRepository{q: sqlcgen.New(pool), pool: pool}
}

var _ store.BillRepository = (*BillRepository)(nil)

// Get returns one bill by id, scoped to the company and its visible buildings.
func (r *BillRepository) Get(ctx context.Context, s store.Scope, id uuid.UUID) (model.Bill, error) {
	if !s.Valid() {
		return model.Bill{}, store.ErrInvalidScope
	}
	ids, all := s.BuildingFilter()
	row, err := r.q.BillGet(ctx, sqlcgen.BillGetParams{ID: id, CompanyID: s.CompanyID, AllBuildings: all, BuildingIds: ids})
	if err != nil {
		return model.Bill{}, pgerr.Translate(r.pool, "get bill", err)
	}
	return billFromRow(row)
}

// List returns the Scope's visible bills matching f.
func (r *BillRepository) List(ctx context.Context, s store.Scope, f store.BillFilter) ([]model.Bill, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	ids, all := s.BuildingFilter()
	limit, offset := billPageLimits(f.Page)
	filterIDs := f.IDs
	if filterIDs == nil {
		filterIDs = []uuid.UUID{}
	}
	statuses := make([]string, 0, len(f.Statuses))
	for _, st := range f.Statuses {
		statuses = append(statuses, string(st))
	}
	var billScope sqlcgen.NullBillScope
	if f.BillScope != nil {
		billScope = sqlcgen.NullBillScope{BillScope: sqlcgen.BillScope(*f.BillScope), Valid: true}
	}
	rows, err := r.q.BillList(ctx, sqlcgen.BillListParams{
		CompanyID: s.CompanyID, AllBuildings: all, BuildingIds: ids, Ids: filterIDs,
		BuildingID: f.BuildingID, AnalyzerID: f.AnalyzerID, BillScope: billScope, PeriodKey: f.PeriodKey,
		Statuses: statuses, IncludeSuperseded: f.IncludeSuperseded, LimitVal: limit, OffsetVal: offset,
	})
	if err != nil {
		return nil, pgerr.Translate(r.pool, "list bills", err)
	}
	out := make([]model.Bill, 0, len(rows))
	for _, row := range rows {
		b, err := billFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, nil
}

// Current returns the one non-superseded bill for a scope and period.
func (r *BillRepository) Current(ctx context.Context, s store.Scope, billScope model.BillScope, subjectID uuid.UUID, periodKey string) (model.Bill, error) {
	if !s.Valid() {
		return model.Bill{}, store.ErrInvalidScope
	}
	ids, all := s.BuildingFilter()
	row, err := r.q.BillCurrent(ctx, sqlcgen.BillCurrentParams{
		CompanyID: s.CompanyID, AllBuildings: all, BuildingIds: ids,
		BillScope: sqlcgen.BillScope(billScope), SubjectID: &subjectID, PeriodKey: periodKey,
	})
	if err != nil {
		return model.Bill{}, pgerr.Translate(r.pool, "get current bill", err)
	}
	return billFromRow(row)
}

// Create inserts a bill, its lines and its members in one transaction, after
// confirming every referenced id is visible to the Scope. See Create's
// Isolation comment in repository.go.
func (r *BillRepository) Create(ctx context.Context, s store.Scope, b model.Bill, lines []model.BillLine, members []uuid.UUID) (model.Bill, error) {
	if !s.Valid() {
		return model.Bill{}, store.ErrInvalidScope
	}
	if b.CompanyID != uuid.Nil && b.CompanyID != s.CompanyID {
		return model.Bill{}, store.ErrNotFound
	}
	if !billBuildingWritable(s, b.BuildingID) {
		return model.Bill{}, store.ErrNotFound
	}
	if err := r.requireAnalyzersVisible(ctx, s, b.AnalyzerID, members); err != nil {
		return model.Bill{}, err
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return model.Bill{}, pgerr.Translate(r.pool, "begin create bill", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.q.WithTx(tx)

	row, err := billInsert(ctx, q, r.pool, s, b)
	if err != nil {
		return model.Bill{}, err
	}
	if err := billInsertLinesAndMembers(ctx, q, r.pool, s, row.ID, lines, members); err != nil {
		return model.Bill{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.Bill{}, pgerr.Translate(r.pool, "commit create bill", err)
	}
	return billFromRow(row)
}

// Supersede marks replacing superseded and inserts b, its lines and its
// members, all in one transaction. See repository.go's §14 comment.
func (r *BillRepository) Supersede(ctx context.Context, s store.Scope, replacing uuid.UUID, b model.Bill, lines []model.BillLine, members []uuid.UUID, at time.Time) (model.Bill, error) {
	if !s.Valid() {
		return model.Bill{}, store.ErrInvalidScope
	}
	if b.CompanyID != uuid.Nil && b.CompanyID != s.CompanyID {
		return model.Bill{}, store.ErrNotFound
	}
	if !billBuildingWritable(s, b.BuildingID) {
		return model.Bill{}, store.ErrNotFound
	}
	if err := r.requireAnalyzersVisible(ctx, s, b.AnalyzerID, members); err != nil {
		return model.Bill{}, err
	}

	ids, all := s.BuildingFilter()
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return model.Bill{}, pgerr.Translate(r.pool, "begin supersede bill", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.q.WithTx(tx)

	old, err := q.BillMarkSuperseded(ctx, sqlcgen.BillMarkSupersededParams{
		UpdatedAt: tariffTimestamptz(at), ID: replacing, CompanyID: s.CompanyID, AllBuildings: all, BuildingIds: ids,
	})
	if err != nil {
		return model.Bill{}, pgerr.Translate(r.pool, "mark bill superseded", err)
	}
	// Folded minor (fix round 1): the replacement must keep the replaced
	// bill's (scope, subject, period) — Supersede is a recomputation of ONE
	// bill's history, not a way to attach an unrelated bill's row where
	// another one used to be.
	if old.Scope != sqlcgen.BillScope(b.Scope) || old.SubjectID != billSubjectID(s, b) || old.PeriodKey != b.PeriodKey {
		return model.Bill{}, errBillSupersedeMismatch
	}

	row, err := billInsert(ctx, q, r.pool, s, b)
	if err != nil {
		return model.Bill{}, err
	}
	if err := billInsertLinesAndMembers(ctx, q, r.pool, s, row.ID, lines, members); err != nil {
		return model.Bill{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.Bill{}, pgerr.Translate(r.pool, "commit supersede bill", err)
	}
	return billFromRow(row)
}

// billSubjectID computes coalesce(analyzer_id, building_id, company_id)
// exactly as the bills table's own unique index and BillCurrent/
// BillMarkSuperseded do, so a replacement's subject can be compared against
// the replaced bill's stored one. s.CompanyID, not b.CompanyID, is the
// fallback: it is s.CompanyID that Create/Supersede actually stores.
func billSubjectID(s store.Scope, b model.Bill) uuid.UUID {
	switch {
	case b.AnalyzerID != nil:
		return *b.AnalyzerID
	case b.BuildingID != nil:
		return *b.BuildingID
	default:
		return s.CompanyID
	}
}

// UpdateStatus moves a bill between draft, issued and flagged.
func (r *BillRepository) UpdateStatus(ctx context.Context, s store.Scope, id uuid.UUID, status model.BillStatus, flagReason *string, at time.Time) (model.Bill, error) {
	if !s.Valid() {
		return model.Bill{}, store.ErrInvalidScope
	}
	if status == model.BillStatusSuperseded {
		return model.Bill{}, errBillCannotSetSuperseded
	}
	ids, all := s.BuildingFilter()
	row, err := r.q.BillUpdateStatus(ctx, sqlcgen.BillUpdateStatusParams{
		Status: sqlcgen.BillStatus(status), FlagReason: flagReason, UpdatedAt: tariffTimestamptz(at),
		ID: id, CompanyID: s.CompanyID, AllBuildings: all, BuildingIds: ids,
	})
	if err != nil {
		return model.Bill{}, pgerr.Translate(r.pool, "update bill status", err)
	}
	return billFromRow(row)
}

// SetPDFPath records where the rendered invoice was stored.
func (r *BillRepository) SetPDFPath(ctx context.Context, s store.Scope, id uuid.UUID, path string) error {
	if !s.Valid() {
		return store.ErrInvalidScope
	}
	ids, all := s.BuildingFilter()
	n, err := r.q.BillSetPDFPath(ctx, sqlcgen.BillSetPDFPathParams{
		PdfPath: &path, ID: id, CompanyID: s.CompanyID, AllBuildings: all, BuildingIds: ids,
	})
	if err != nil {
		return pgerr.Translate(r.pool, "set bill pdf path", err)
	}
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

// Lines — Isolation: bill_lines has no company_id — join through bills.
func (r *BillRepository) Lines(ctx context.Context, s store.Scope, billID uuid.UUID) ([]model.BillLine, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	if err := r.requireVisible(ctx, s, billID); err != nil {
		return nil, err
	}
	ids, all := s.BuildingFilter()
	rows, err := r.q.BillLineList(ctx, sqlcgen.BillLineListParams{
		BillID: billID, CompanyID: s.CompanyID, AllBuildings: all, BuildingIds: ids,
	})
	if err != nil {
		return nil, pgerr.Translate(r.pool, "list bill lines", err)
	}
	out := make([]model.BillLine, 0, len(rows))
	for _, row := range rows {
		l, err := billLineFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, nil
}

// Members — Isolation: bill_members has no company_id — join through bills.
func (r *BillRepository) Members(ctx context.Context, s store.Scope, billID uuid.UUID) ([]model.BillMember, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	if err := r.requireVisible(ctx, s, billID); err != nil {
		return nil, err
	}
	ids, all := s.BuildingFilter()
	rows, err := r.q.BillMemberList(ctx, sqlcgen.BillMemberListParams{
		BillID: billID, CompanyID: s.CompanyID, AllBuildings: all, BuildingIds: ids,
	})
	if err != nil {
		return nil, pgerr.Translate(r.pool, "list bill members", err)
	}
	out := make([]model.BillMember, 0, len(rows))
	for _, row := range rows {
		out = append(out, model.BillMember{BillID: row.BillID, AnalyzerID: row.AnalyzerID})
	}
	return out, nil
}

// HourlyDetail — Isolation: bill_hourly_detail has no company_id — join
// through bills.
func (r *BillRepository) HourlyDetail(ctx context.Context, s store.Scope, billID uuid.UUID) ([]model.BillHourlyDetail, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	if err := r.requireVisible(ctx, s, billID); err != nil {
		return nil, err
	}
	ids, all := s.BuildingFilter()
	rows, err := r.q.BillHourlyDetailList(ctx, sqlcgen.BillHourlyDetailListParams{
		BillID: billID, CompanyID: s.CompanyID, AllBuildings: all, BuildingIds: ids,
	})
	if err != nil {
		return nil, pgerr.Translate(r.pool, "list bill hourly detail", err)
	}
	out := make([]model.BillHourlyDetail, 0, len(rows))
	for _, row := range rows {
		d, err := billHourlyDetailFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

// ReplaceHourlyDetail — Isolation: join bill_hourly_detail through bills.
func (r *BillRepository) ReplaceHourlyDetail(ctx context.Context, s store.Scope, billID uuid.UUID, rows []model.BillHourlyDetail) (int64, error) {
	if !s.Valid() {
		return 0, store.ErrInvalidScope
	}
	if err := r.requireVisible(ctx, s, billID); err != nil {
		return 0, err
	}

	ids, all := s.BuildingFilter()
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, pgerr.Translate(r.pool, "begin replace bill hourly detail", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.q.WithTx(tx)

	if err := q.BillHourlyDetailDeleteForBill(ctx, sqlcgen.BillHourlyDetailDeleteForBillParams{
		BillID: billID, CompanyID: s.CompanyID, AllBuildings: all, BuildingIds: ids,
	}); err != nil {
		return 0, pgerr.Translate(r.pool, "clear bill hourly detail", err)
	}
	var n int64
	for _, d := range rows {
		if err := q.BillHourlyDetailInsert(ctx, sqlcgen.BillHourlyDetailInsertParams{
			BillID: billID, Ts: tariffTimestamptz(d.Ts), Consumption: decimalToNumeric(d.Consumption),
			Ptf: decimalToNumeric(d.PTF), Yekdem: decimalToNumeric(d.Yekdem), Kbk: decimalToNumeric(d.Kbk),
			UnitPrice: decimalToNumeric(d.UnitPrice), Cost: decimalToNumeric(d.Cost),
			CompanyID: s.CompanyID, AllBuildings: all, BuildingIds: ids,
		}); err != nil {
			return 0, pgerr.Translate(r.pool, "insert bill hourly detail", err)
		}
		n++
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, pgerr.Translate(r.pool, "commit replace bill hourly detail", err)
	}
	return n, nil
}

func (r *BillRepository) requireVisible(ctx context.Context, s store.Scope, billID uuid.UUID) error {
	ids, all := s.BuildingFilter()
	visible, err := r.q.BillVisible(ctx, sqlcgen.BillVisibleParams{ID: billID, CompanyID: s.CompanyID, AllBuildings: all, BuildingIds: ids})
	if err != nil {
		return pgerr.Translate(r.pool, "check bill visibility", err)
	}
	if !visible {
		return store.ErrNotFound
	}
	return nil
}

// requireAnalyzersVisible checks, in ONE round trip, that every analyzer id
// a bill write touches — its own AnalyzerID plus every member — belongs to
// the Scope's company and one of its visible buildings. The whole write is
// refused if any is not.
func (r *BillRepository) requireAnalyzersVisible(ctx context.Context, s store.Scope, analyzerID *uuid.UUID, members []uuid.UUID) error {
	seen := make(map[uuid.UUID]struct{}, len(members)+1)
	if analyzerID != nil {
		seen[*analyzerID] = struct{}{}
	}
	for _, m := range members {
		seen[m] = struct{}{}
	}
	if len(seen) == 0 {
		return nil
	}
	check := make([]uuid.UUID, 0, len(seen))
	for id := range seen {
		check = append(check, id)
	}
	buildingIDs, all := s.BuildingFilter()
	count, err := r.q.BillCountVisibleAnalyzers(ctx, sqlcgen.BillCountVisibleAnalyzersParams{
		CompanyID: s.CompanyID, AnalyzerIds: check, AllBuildings: all, BuildingIds: buildingIDs,
	})
	if err != nil {
		return pgerr.Translate(r.pool, "check bill analyzer visibility", err)
	}
	if count != int64(len(check)) {
		return store.ErrNotFound
	}
	return nil
}

// billBuildingWritable mirrors tariffBuildingWritable: a NULL building_id
// (a company-level bill) may be written only by a Scope with AllBuildings.
//
// This is a FAST PRE-CHECK, not the guard: it decides the narrow-Scope
// business rule above, but it cannot tell whether a non-nil buildingID
// belongs to this Scope's company at all (Scope.AllowsBuilding's own doc
// comment explains why — an AllBuildings Scope answers true for ANY id).
// Critical Finding 1 (task-11a fix round 1): BillCreate validates the stored
// building_id (and tariff_id) itself, in SQL, against the Scope's company,
// deleted_at and building branch — see queries/bills.sql — so the write is
// refused even if this Go-side check were skipped entirely.
func billBuildingWritable(s store.Scope, buildingID *uuid.UUID) bool {
	if buildingID == nil {
		return s.AllBuildings
	}
	return s.AllowsBuilding(*buildingID)
}

func billInsert(ctx context.Context, q *sqlcgen.Queries, pool *pgxpool.Pool, s store.Scope, b model.Bill) (sqlcgen.Bill, error) {
	ids, all := s.BuildingFilter()
	row, err := q.BillCreate(ctx, sqlcgen.BillCreateParams{
		CompanyID: s.CompanyID, AllBuildings: all, BuildingIds: ids, BuildingID: b.BuildingID, AnalyzerID: b.AnalyzerID,
		BillScope: sqlcgen.BillScope(b.Scope), PeriodKey: b.PeriodKey,
		PeriodStart: tariffTimestamptz(b.PeriodStart), PeriodEnd: tariffTimestamptz(b.PeriodEnd),
		DaysInPeriod: b.DaysInPeriod, TariffID: b.TariffID, TariffEffectiveFrom: tariffTimePtrToDate(b.TariffEffectiveFrom),
		ActiveImport: decimalToNumeric(b.ActiveImport), T1Kwh: decimalToNumeric(b.T1Kwh), T2Kwh: decimalToNumeric(b.T2Kwh),
		T3Kwh: decimalToNumeric(b.T3Kwh), InductiveKvarh: decimalToNumeric(b.InductiveKvarh),
		CapacitiveKvarh: decimalToNumeric(b.CapacitiveKvarh), ActiveExport: decimalToNumeric(b.ActiveExport),
		NetConsumption: decimalToNumeric(b.NetConsumption), LowTierKwh: decimalToNumeric(b.LowTierKwh),
		HighTierKwh: decimalToNumeric(b.HighTierKwh), MaxDemandKw: decimalPtrToNumeric(b.MaxDemandKw),
		TieredApplied: b.TieredApplied, IndexStart: []byte(b.IndexStart), IndexEnd: []byte(b.IndexEnd),
		EffectiveEnergyPrice: decimalPtrToNumeric(b.EffectiveEnergyPrice), LowTierPrice: decimalPtrToNumeric(b.LowTierPrice),
		HighTierPrice: decimalPtrToNumeric(b.HighTierPrice), EnergyCost: decimalToNumeric(b.EnergyCost),
		DistributionCost: decimalToNumeric(b.DistributionCost), GreenEnergyCost: decimalToNumeric(b.GreenEnergyCost),
		PowerCost: decimalToNumeric(b.PowerCost), DemandOverrunCost: decimalToNumeric(b.DemandOverrunCost),
		ReactivePenalty: decimalToNumeric(b.ReactivePenalty), OtherTaxesCost: decimalToNumeric(b.OtherTaxesCost),
		VatBase: decimalToNumeric(b.VatBase), VatCost: decimalToNumeric(b.VatCost),
		GenerationCredit: decimalToNumeric(b.GenerationCredit), TotalCost: decimalToNumeric(b.TotalCost),
		InductiveRatio: decimalPtrToNumeric(b.InductiveRatio), CapacitiveRatio: decimalPtrToNumeric(b.CapacitiveRatio),
		InductiveThreshold: decimalPtrToNumeric(b.InductiveThreshold), CapacitiveThreshold: decimalPtrToNumeric(b.CapacitiveThreshold),
		ReactivePenaltyApplied: b.ReactivePenaltyApplied, ReactivePowerPrice: decimalPtrToNumeric(b.ReactivePowerPrice),
		GenerationUsage: sqlcgen.GenerationUsage(b.GenerationUsage), GenerationPricePerKwh: decimalPtrToNumeric(b.GenerationPricePerKwh),
		PtfYekdemUsed: b.PtfYekdemUsed, PtfHoursMatched: b.PtfHoursMatched, PtfHoursMissing: b.PtfHoursMissing,
		PtfAverage: decimalPtrToNumeric(b.PtfAverage), YekdemUsed: decimalPtrToNumeric(b.YekdemUsed),
		Status: sqlcgen.BillStatus(b.Status), FlagReason: b.FlagReason, PdfPath: b.PdfPath,
		ComputedAt: tariffTimestamptz(b.ComputedAt), CreatedAt: tariffTimestamptz(b.CreatedAt),
	})
	if err != nil {
		return sqlcgen.Bill{}, pgerr.Translate(pool, "create bill", err)
	}
	return row, nil
}

// billInsertLinesAndMembers writes lines and members under billID.
//
// Important Finding 1: BillLineInsert and BillMemberInsert both re-validate
// billID against the Scope themselves (queries/bills.sql), so this join
// through bills holds even if the caller's own requireVisible /
// requireAnalyzersVisible pre-checks were ever skipped — that is why s is
// threaded all the way down here rather than trusting the caller's checks
// alone.
func billInsertLinesAndMembers(ctx context.Context, q *sqlcgen.Queries, pool *pgxpool.Pool, s store.Scope, billID uuid.UUID, lines []model.BillLine, members []uuid.UUID) error {
	ids, all := s.BuildingFilter()
	for _, line := range lines {
		if _, err := q.BillLineInsert(ctx, sqlcgen.BillLineInsertParams{
			BillID: billID, Code: line.Code, Label: line.Label, Quantity: decimalPtrToNumeric(line.Quantity),
			Unit: line.Unit, UnitPrice: decimalPtrToNumeric(line.UnitPrice), RatePct: decimalPtrToNumeric(line.RatePct),
			Amount: decimalToNumeric(line.Amount), SortOrder: line.SortOrder,
			CompanyID: s.CompanyID, AllBuildings: all, BuildingIds: ids,
		}); err != nil {
			return pgerr.Translate(pool, "insert bill line", err)
		}
	}
	for _, analyzerID := range members {
		if _, err := q.BillMemberInsert(ctx, sqlcgen.BillMemberInsertParams{
			BillID: billID, AnalyzerID: analyzerID, CompanyID: s.CompanyID, AllBuildings: all, BuildingIds: ids,
		}); err != nil {
			return pgerr.Translate(pool, "insert bill member", err)
		}
	}
	return nil
}

func billFromRow(row sqlcgen.Bill) (model.Bill, error) {
	activeImport, err := numericToDecimal(row.ActiveImport)
	if err != nil {
		return model.Bill{}, fmt.Errorf("bills.active_import: %w", err)
	}
	t1Kwh, err := numericToDecimal(row.T1Kwh)
	if err != nil {
		return model.Bill{}, fmt.Errorf("bills.t1_kwh: %w", err)
	}
	t2Kwh, err := numericToDecimal(row.T2Kwh)
	if err != nil {
		return model.Bill{}, fmt.Errorf("bills.t2_kwh: %w", err)
	}
	t3Kwh, err := numericToDecimal(row.T3Kwh)
	if err != nil {
		return model.Bill{}, fmt.Errorf("bills.t3_kwh: %w", err)
	}
	inductiveKvarh, err := numericToDecimal(row.InductiveKvarh)
	if err != nil {
		return model.Bill{}, fmt.Errorf("bills.inductive_kvarh: %w", err)
	}
	capacitiveKvarh, err := numericToDecimal(row.CapacitiveKvarh)
	if err != nil {
		return model.Bill{}, fmt.Errorf("bills.capacitive_kvarh: %w", err)
	}
	activeExport, err := numericToDecimal(row.ActiveExport)
	if err != nil {
		return model.Bill{}, fmt.Errorf("bills.active_export: %w", err)
	}
	netConsumption, err := numericToDecimal(row.NetConsumption)
	if err != nil {
		return model.Bill{}, fmt.Errorf("bills.net_consumption: %w", err)
	}
	lowTierKwh, err := numericToDecimal(row.LowTierKwh)
	if err != nil {
		return model.Bill{}, fmt.Errorf("bills.low_tier_kwh: %w", err)
	}
	highTierKwh, err := numericToDecimal(row.HighTierKwh)
	if err != nil {
		return model.Bill{}, fmt.Errorf("bills.high_tier_kwh: %w", err)
	}
	energyCost, err := numericToDecimal(row.EnergyCost)
	if err != nil {
		return model.Bill{}, fmt.Errorf("bills.energy_cost: %w", err)
	}
	distributionCost, err := numericToDecimal(row.DistributionCost)
	if err != nil {
		return model.Bill{}, fmt.Errorf("bills.distribution_cost: %w", err)
	}
	greenEnergyCost, err := numericToDecimal(row.GreenEnergyCost)
	if err != nil {
		return model.Bill{}, fmt.Errorf("bills.green_energy_cost: %w", err)
	}
	powerCost, err := numericToDecimal(row.PowerCost)
	if err != nil {
		return model.Bill{}, fmt.Errorf("bills.power_cost: %w", err)
	}
	demandOverrunCost, err := numericToDecimal(row.DemandOverrunCost)
	if err != nil {
		return model.Bill{}, fmt.Errorf("bills.demand_overrun_cost: %w", err)
	}
	reactivePenalty, err := numericToDecimal(row.ReactivePenalty)
	if err != nil {
		return model.Bill{}, fmt.Errorf("bills.reactive_penalty: %w", err)
	}
	otherTaxesCost, err := numericToDecimal(row.OtherTaxesCost)
	if err != nil {
		return model.Bill{}, fmt.Errorf("bills.other_taxes_cost: %w", err)
	}
	vatBase, err := numericToDecimal(row.VatBase)
	if err != nil {
		return model.Bill{}, fmt.Errorf("bills.vat_base: %w", err)
	}
	vatCost, err := numericToDecimal(row.VatCost)
	if err != nil {
		return model.Bill{}, fmt.Errorf("bills.vat_cost: %w", err)
	}
	generationCredit, err := numericToDecimal(row.GenerationCredit)
	if err != nil {
		return model.Bill{}, fmt.Errorf("bills.generation_credit: %w", err)
	}
	totalCost, err := numericToDecimal(row.TotalCost)
	if err != nil {
		return model.Bill{}, fmt.Errorf("bills.total_cost: %w", err)
	}

	out := model.Bill{
		ID: row.ID, CompanyID: row.CompanyID, BuildingID: row.BuildingID, AnalyzerID: row.AnalyzerID,
		Scope: model.BillScope(row.Scope), PeriodStart: row.PeriodStart.Time, PeriodEnd: row.PeriodEnd.Time,
		PeriodKey: row.PeriodKey, DaysInPeriod: row.DaysInPeriod, TariffID: row.TariffID,
		TariffEffectiveFrom: tariffDateToTimePtr(row.TariffEffectiveFrom),

		ActiveImport: activeImport, T1Kwh: t1Kwh, T2Kwh: t2Kwh, T3Kwh: t3Kwh,
		InductiveKvarh: inductiveKvarh, CapacitiveKvarh: capacitiveKvarh, ActiveExport: activeExport,
		NetConsumption: netConsumption, LowTierKwh: lowTierKwh, HighTierKwh: highTierKwh,
		TieredApplied: row.TieredApplied, IndexStart: row.IndexStart, IndexEnd: row.IndexEnd,

		EnergyCost: energyCost, DistributionCost: distributionCost, GreenEnergyCost: greenEnergyCost,
		PowerCost: powerCost, DemandOverrunCost: demandOverrunCost, ReactivePenalty: reactivePenalty,
		OtherTaxesCost: otherTaxesCost, VatBase: vatBase, VatCost: vatCost, GenerationCredit: generationCredit,
		TotalCost: totalCost,

		ReactivePenaltyApplied: row.ReactivePenaltyApplied,
		GenerationUsage:        model.GenerationUsage(row.GenerationUsage),
		PtfYekdemUsed:          row.PtfYekdemUsed, PtfHoursMatched: row.PtfHoursMatched, PtfHoursMissing: row.PtfHoursMissing,

		Status: model.BillStatus(row.Status), FlagReason: row.FlagReason, PdfPath: row.PdfPath,
		ComputedAt: row.ComputedAt.Time, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}

	for _, f := range []tariffNumericField{
		{"max_demand_kw", row.MaxDemandKw, &out.MaxDemandKw},
		{"effective_energy_price", row.EffectiveEnergyPrice, &out.EffectiveEnergyPrice},
		{"low_tier_price", row.LowTierPrice, &out.LowTierPrice},
		{"high_tier_price", row.HighTierPrice, &out.HighTierPrice},
		{"inductive_ratio", row.InductiveRatio, &out.InductiveRatio},
		{"capacitive_ratio", row.CapacitiveRatio, &out.CapacitiveRatio},
		{"inductive_threshold", row.InductiveThreshold, &out.InductiveThreshold},
		{"capacitive_threshold", row.CapacitiveThreshold, &out.CapacitiveThreshold},
		{"reactive_power_price", row.ReactivePowerPrice, &out.ReactivePowerPrice},
		{"generation_price_per_kwh", row.GenerationPricePerKwh, &out.GenerationPricePerKwh},
		{"ptf_average", row.PtfAverage, &out.PtfAverage},
		{"yekdem_used", row.YekdemUsed, &out.YekdemUsed},
	} {
		d, err := numericToDecimalPtr(f.src)
		if err != nil {
			return model.Bill{}, fmt.Errorf("bills.%s: %w", f.name, err)
		}
		*f.dst = d
	}

	return out, nil
}

func billLineFromRow(row sqlcgen.BillLine) (model.BillLine, error) {
	amount, err := numericToDecimal(row.Amount)
	if err != nil {
		return model.BillLine{}, fmt.Errorf("bill_lines.amount: %w", err)
	}
	out := model.BillLine{
		ID: row.ID, BillID: row.BillID, Code: row.Code, Label: row.Label,
		Amount: amount, SortOrder: row.SortOrder, Unit: row.Unit,
	}
	for _, f := range []tariffNumericField{
		{"quantity", row.Quantity, &out.Quantity},
		{"unit_price", row.UnitPrice, &out.UnitPrice},
		{"rate_pct", row.RatePct, &out.RatePct},
	} {
		d, err := numericToDecimalPtr(f.src)
		if err != nil {
			return model.BillLine{}, fmt.Errorf("bill_lines.%s: %w", f.name, err)
		}
		*f.dst = d
	}
	return out, nil
}

func billHourlyDetailFromRow(row sqlcgen.BillHourlyDetail) (model.BillHourlyDetail, error) {
	consumption, err := numericToDecimal(row.Consumption)
	if err != nil {
		return model.BillHourlyDetail{}, fmt.Errorf("bill_hourly_detail.consumption: %w", err)
	}
	ptf, err := numericToDecimal(row.Ptf)
	if err != nil {
		return model.BillHourlyDetail{}, fmt.Errorf("bill_hourly_detail.ptf: %w", err)
	}
	yekdem, err := numericToDecimal(row.Yekdem)
	if err != nil {
		return model.BillHourlyDetail{}, fmt.Errorf("bill_hourly_detail.yekdem: %w", err)
	}
	kbk, err := numericToDecimal(row.Kbk)
	if err != nil {
		return model.BillHourlyDetail{}, fmt.Errorf("bill_hourly_detail.kbk: %w", err)
	}
	unitPrice, err := numericToDecimal(row.UnitPrice)
	if err != nil {
		return model.BillHourlyDetail{}, fmt.Errorf("bill_hourly_detail.unit_price: %w", err)
	}
	cost, err := numericToDecimal(row.Cost)
	if err != nil {
		return model.BillHourlyDetail{}, fmt.Errorf("bill_hourly_detail.cost: %w", err)
	}
	return model.BillHourlyDetail{
		BillID: row.BillID, Ts: row.Ts.Time, Consumption: consumption, PTF: ptf, Yekdem: yekdem,
		Kbk: kbk, UnitPrice: unitPrice, Cost: cost,
	}, nil
}
