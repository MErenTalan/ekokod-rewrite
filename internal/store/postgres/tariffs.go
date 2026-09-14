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
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/internal/pgerr"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/sqlcgen"
)

// tariffDefaultPageLimit and tariffMaxPageLimit are this file's Page defaults,
// for every List method across tariffs.go: tariffs, templates, solar
// tariffs, the national schedule and icmal rows.
const (
	tariffDefaultPageLimit = 50
	tariffMaxPageLimit     = 500
)

// tariffPageLimits clamps a caller's Page to this file's bounds. A zero Limit
// means "the repository's default", never "unbounded".
func tariffPageLimits(p store.Page) (limit, offset int32) {
	limit = p.Limit
	if limit <= 0 {
		limit = tariffDefaultPageLimit
	}
	if limit > tariffMaxPageLimit {
		limit = tariffMaxPageLimit
	}
	return limit, p.Offset
}

// tariffDateToTime and tariffTimeToDate convert between a `date` column and
// time.Time for a NOT NULL date column. Only the calendar date carried by
// pgtype.Date is meaningful; the pair is deliberately not the numeric.go
// pair, which exists for `numeric` columns only.
func tariffDateToTime(d pgtype.Date) time.Time {
	if !d.Valid {
		return time.Time{}
	}
	return d.Time
}

func tariffTimeToDate(t time.Time) pgtype.Date {
	return pgtype.Date{Time: t, Valid: true}
}

// tariffDateToTimePtr and tariffTimePtrToDate are the nullable-date pair,
// used by BillRepository for bills.tariff_effective_from.
func tariffDateToTimePtr(d pgtype.Date) *time.Time {
	if !d.Valid {
		return nil
	}
	t := d.Time
	return &t
}

func tariffTimePtrToDate(t *time.Time) pgtype.Date {
	if t == nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: *t, Valid: true}
}

// tariffNullTimestamptz reports a nullable timestamptz as *time.Time,
// reused by every file in this package that needs the same nullable
// conversion (DeletedAt, ProcessedAt, NotifiedAt, …).
func tariffNullTimestamptz(ts pgtype.Timestamptz) *time.Time {
	if !ts.Valid {
		return nil
	}
	t := ts.Time
	return &t
}

func tariffTimestamptz(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func tariffNullableTimestamptz(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

// ---------------------------------------------------------------------------
// TariffRepository
// ---------------------------------------------------------------------------

// TariffRepository implements store.TariffRepository.
type TariffRepository struct {
	q    *sqlcgen.Queries
	pool *pgxpool.Pool
}

// NewTariffRepository builds a TariffRepository over pool.
func NewTariffRepository(pool *pgxpool.Pool) *TariffRepository {
	return &TariffRepository{q: sqlcgen.New(pool), pool: pool}
}

var _ store.TariffRepository = (*TariffRepository)(nil)

// Get returns one tariff by id, scoped to the company and its visible buildings.
func (r *TariffRepository) Get(ctx context.Context, s store.Scope, id uuid.UUID) (model.Tariff, error) {
	if !s.Valid() {
		return model.Tariff{}, store.ErrInvalidScope
	}
	ids, all := s.BuildingFilter()
	row, err := r.q.TariffGet(ctx, sqlcgen.TariffGetParams{
		ID: id, CompanyID: s.CompanyID, AllBuildings: all, BuildingIds: ids,
	})
	if err != nil {
		return model.Tariff{}, pgerr.Translate(r.pool, "get tariff", err)
	}
	return tariffFromRow(row)
}

// List returns the Scope's visible tariffs matching f.
func (r *TariffRepository) List(ctx context.Context, s store.Scope, f store.TariffFilter) ([]model.Tariff, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	ids, all := s.BuildingFilter()
	limit, offset := tariffPageLimits(f.Page)
	filterIDs := f.IDs
	if filterIDs == nil {
		filterIDs = []uuid.UUID{}
	}
	var effectiveOn pgtype.Date
	if f.EffectiveOn != nil {
		effectiveOn = tariffTimeToDate(*f.EffectiveOn)
	}
	rows, err := r.q.TariffList(ctx, sqlcgen.TariffListParams{
		CompanyID: s.CompanyID, AllBuildings: all, BuildingIds: ids,
		Ids: filterIDs, BuildingID: f.BuildingID, CompanyWide: f.CompanyWide,
		EffectiveOn: effectiveOn, IncludeDeleted: f.IncludeDeleted,
		LimitVal: limit, OffsetVal: offset,
	})
	if err != nil {
		return nil, pgerr.Translate(r.pool, "list tariffs", err)
	}
	out := make([]model.Tariff, 0, len(rows))
	for _, row := range rows {
		t, err := tariffFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

// Create inserts a tariff for the Scope's company.
func (r *TariffRepository) Create(ctx context.Context, s store.Scope, t model.Tariff) (model.Tariff, error) {
	if !s.Valid() {
		return model.Tariff{}, store.ErrInvalidScope
	}
	if t.CompanyID != uuid.Nil && t.CompanyID != s.CompanyID {
		return model.Tariff{}, store.ErrNotFound
	}
	if !tariffBuildingWritable(s, t.BuildingID) {
		return model.Tariff{}, store.ErrNotFound
	}
	row, err := r.q.TariffCreate(ctx, sqlcgen.TariffCreateParams{
		CompanyID: s.CompanyID, BuildingID: t.BuildingID, Name: t.Name,
		EffectiveFrom: tariffTimeToDate(t.EffectiveFrom), Currency: sqlcgen.CurrencyCode(t.Currency),
		EnergyType: sqlcgen.EnergyType(t.EnergyType), VoltageLevel: sqlcgen.VoltageLevel(t.VoltageLevel),
		UserGroup: sqlcgen.DistributionUserGroup(t.UserGroup), PriceType: sqlcgen.PriceType(t.PriceType),
		Term: sqlcgen.TariffTerm(t.Term), SupplyCompany: sqlcgen.SupplyCompany(t.SupplyCompany),
		SingleTimePrice: decimalPtrToNumeric(t.SingleTimePrice), T1Price: decimalPtrToNumeric(t.T1Price),
		T2Price: decimalPtrToNumeric(t.T2Price), T3Price: decimalPtrToNumeric(t.T3Price),
		OverusePrice:                decimalPtrToNumeric(t.OverusePrice),
		OveruseThresholdKwhPerDay:   decimalPtrToNumeric(t.OveruseThresholdKwhPerDay),
		DistributionCost:            decimalToNumeric(t.DistributionCost),
		ReactivePowerPrice:          decimalToNumeric(t.ReactivePowerPrice),
		GreenEnergyPrice:            decimalPtrToNumeric(t.GreenEnergyPrice),
		GreenEnergyDistributionCost: decimalPtrToNumeric(t.GreenEnergyDistributionCost),
		ContractedPowerKw:           decimalPtrToNumeric(t.ContractedPowerKw),
		PowerUnitPrice:              decimalPtrToNumeric(t.PowerUnitPrice),
		GenerationUsage:             sqlcgen.GenerationUsage(t.GenerationUsage),
		GenerationPricePerKwh:       decimalPtrToNumeric(t.GenerationPricePerKwh),
		VatRate:                     decimalToNumeric(t.VatRate),
		UsePtfYekdem:                t.UsePtfYekdem,
		KbkEnergy:                   decimalPtrToNumeric(t.KbkEnergy),
		KbkT1:                       decimalPtrToNumeric(t.KbkT1),
		KbkT2:                       decimalPtrToNumeric(t.KbkT2),
		KbkT3:                       decimalPtrToNumeric(t.KbkT3),
		KbkPowerPrice:               decimalPtrToNumeric(t.KbkPowerPrice),
		KbkOverusePrice:             decimalPtrToNumeric(t.KbkOverusePrice),
		KbkReactivePower:            decimalPtrToNumeric(t.KbkReactivePower),
		KbkDistributionCostTlPerKwh: decimalPtrToNumeric(t.KbkDistributionCostTlPerKwh),
		UseManualYekdem:             t.UseManualYekdem,
		CreatedBy:                   t.CreatedBy,
		CreatedAt:                   tariffTimestamptz(t.CreatedAt),
	})
	if err != nil {
		return model.Tariff{}, pgerr.Translate(r.pool, "create tariff", err)
	}
	return tariffFromRow(row)
}

// Update fully replaces a tariff's fields by id.
func (r *TariffRepository) Update(ctx context.Context, s store.Scope, t model.Tariff) (model.Tariff, error) {
	if !s.Valid() {
		return model.Tariff{}, store.ErrInvalidScope
	}
	if !tariffBuildingWritable(s, t.BuildingID) {
		return model.Tariff{}, store.ErrNotFound
	}
	ids, all := s.BuildingFilter()
	row, err := r.q.TariffUpdate(ctx, sqlcgen.TariffUpdateParams{
		ID: t.ID, CompanyID: s.CompanyID, AllBuildings: all, BuildingIds: ids,
		BuildingID: t.BuildingID, Name: t.Name,
		EffectiveFrom: tariffTimeToDate(t.EffectiveFrom), Currency: sqlcgen.CurrencyCode(t.Currency),
		EnergyType: sqlcgen.EnergyType(t.EnergyType), VoltageLevel: sqlcgen.VoltageLevel(t.VoltageLevel),
		UserGroup: sqlcgen.DistributionUserGroup(t.UserGroup), PriceType: sqlcgen.PriceType(t.PriceType),
		Term: sqlcgen.TariffTerm(t.Term), SupplyCompany: sqlcgen.SupplyCompany(t.SupplyCompany),
		SingleTimePrice: decimalPtrToNumeric(t.SingleTimePrice), T1Price: decimalPtrToNumeric(t.T1Price),
		T2Price: decimalPtrToNumeric(t.T2Price), T3Price: decimalPtrToNumeric(t.T3Price),
		OverusePrice:                decimalPtrToNumeric(t.OverusePrice),
		OveruseThresholdKwhPerDay:   decimalPtrToNumeric(t.OveruseThresholdKwhPerDay),
		DistributionCost:            decimalToNumeric(t.DistributionCost),
		ReactivePowerPrice:          decimalToNumeric(t.ReactivePowerPrice),
		GreenEnergyPrice:            decimalPtrToNumeric(t.GreenEnergyPrice),
		GreenEnergyDistributionCost: decimalPtrToNumeric(t.GreenEnergyDistributionCost),
		ContractedPowerKw:           decimalPtrToNumeric(t.ContractedPowerKw),
		PowerUnitPrice:              decimalPtrToNumeric(t.PowerUnitPrice),
		GenerationUsage:             sqlcgen.GenerationUsage(t.GenerationUsage),
		GenerationPricePerKwh:       decimalPtrToNumeric(t.GenerationPricePerKwh),
		VatRate:                     decimalToNumeric(t.VatRate),
		UsePtfYekdem:                t.UsePtfYekdem,
		KbkEnergy:                   decimalPtrToNumeric(t.KbkEnergy),
		KbkT1:                       decimalPtrToNumeric(t.KbkT1),
		KbkT2:                       decimalPtrToNumeric(t.KbkT2),
		KbkT3:                       decimalPtrToNumeric(t.KbkT3),
		KbkPowerPrice:               decimalPtrToNumeric(t.KbkPowerPrice),
		KbkOverusePrice:             decimalPtrToNumeric(t.KbkOverusePrice),
		KbkReactivePower:            decimalPtrToNumeric(t.KbkReactivePower),
		KbkDistributionCostTlPerKwh: decimalPtrToNumeric(t.KbkDistributionCostTlPerKwh),
		UseManualYekdem:             t.UseManualYekdem,
		UpdatedAt:                   tariffTimestamptz(t.UpdatedAt),
	})
	if err != nil {
		return model.Tariff{}, pgerr.Translate(r.pool, "update tariff", err)
	}
	return tariffFromRow(row)
}

// SoftDelete stamps deleted_at on a tariff by id.
func (r *TariffRepository) SoftDelete(ctx context.Context, s store.Scope, id uuid.UUID, at time.Time) error {
	if !s.Valid() {
		return store.ErrInvalidScope
	}
	ids, all := s.BuildingFilter()
	n, err := r.q.TariffSoftDelete(ctx, sqlcgen.TariffSoftDeleteParams{
		DeletedAt: tariffTimestamptz(at), ID: id, CompanyID: s.CompanyID, AllBuildings: all, BuildingIds: ids,
	})
	if err != nil {
		return pgerr.Translate(r.pool, "soft delete tariff", err)
	}
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

// Effective resolves the greatest effective_from not after on: first among
// the building's own tariffs, falling back to the company-wide one. The
// building itself must be visible to the Scope FIRST — the one place this
// repository resolves to a company-wide row for a Scope that could not List
// or Get one directly (see repository.go's NULL building_id ruling).
func (r *TariffRepository) Effective(ctx context.Context, s store.Scope, buildingID uuid.UUID, on time.Time) (model.Tariff, error) {
	if !s.Valid() {
		return model.Tariff{}, store.ErrInvalidScope
	}
	ids, all := s.BuildingFilter()
	visible, err := r.q.TariffBuildingVisible(ctx, sqlcgen.TariffBuildingVisibleParams{
		BuildingID: buildingID, CompanyID: s.CompanyID, AllBuildings: all, BuildingIds: ids,
	})
	if err != nil {
		return model.Tariff{}, pgerr.Translate(r.pool, "check building visibility for effective tariff", err)
	}
	if !visible {
		return model.Tariff{}, store.ErrNotFound
	}

	onDate := tariffTimeToDate(on)
	row, err := r.q.TariffEffectiveForBuilding(ctx, sqlcgen.TariffEffectiveForBuildingParams{
		CompanyID: s.CompanyID, BuildingID: &buildingID, EffectiveOn: onDate,
	})
	if err == nil {
		return tariffFromRow(row)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return model.Tariff{}, pgerr.Translate(r.pool, "resolve effective tariff", err)
	}

	row, err = r.q.TariffEffectiveCompanyWide(ctx, sqlcgen.TariffEffectiveCompanyWideParams{
		CompanyID: s.CompanyID, EffectiveOn: onDate,
	})
	if err != nil {
		return model.Tariff{}, pgerr.Translate(r.pool, "resolve effective company-wide tariff", err)
	}
	return tariffFromRow(row)
}

// Taxes — Isolation: tariff_taxes has no company_id — join through tariffs.
func (r *TariffRepository) Taxes(ctx context.Context, s store.Scope, tariffID uuid.UUID) ([]model.TariffTax, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	if err := r.requireVisible(ctx, s, tariffID); err != nil {
		return nil, err
	}
	rows, err := r.q.TariffTaxList(ctx, tariffID)
	if err != nil {
		return nil, pgerr.Translate(r.pool, "list tariff taxes", err)
	}
	out := make([]model.TariffTax, 0, len(rows))
	for _, row := range rows {
		rate, err := numericToDecimal(row.Rate)
		if err != nil {
			return nil, fmt.Errorf("tariff_taxes.rate: %w", err)
		}
		out = append(out, model.TariffTax{ID: row.ID, TariffID: row.TariffID, Name: row.Name, Rate: rate, SortOrder: row.SortOrder})
	}
	return out, nil
}

// ReplaceTaxes swaps tariff_taxes in one transaction, after confirming the
// tariff is visible to the Scope.
func (r *TariffRepository) ReplaceTaxes(ctx context.Context, s store.Scope, tariffID uuid.UUID, taxes []model.TariffTax) ([]model.TariffTax, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	if err := r.requireVisible(ctx, s, tariffID); err != nil {
		return nil, err
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, pgerr.Translate(r.pool, "begin replace tariff taxes", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.q.WithTx(tx)

	if err := q.TariffTaxDeleteForTariff(ctx, tariffID); err != nil {
		return nil, pgerr.Translate(r.pool, "clear tariff taxes", err)
	}
	out := make([]model.TariffTax, 0, len(taxes))
	for _, tax := range taxes {
		row, err := q.TariffTaxInsert(ctx, sqlcgen.TariffTaxInsertParams{
			TariffID: tariffID, Name: tax.Name, Rate: decimalToNumeric(tax.Rate), SortOrder: tax.SortOrder,
		})
		if err != nil {
			return nil, pgerr.Translate(r.pool, "insert tariff tax", err)
		}
		rate, err := numericToDecimal(row.Rate)
		if err != nil {
			return nil, fmt.Errorf("tariff_taxes.rate: %w", err)
		}
		out = append(out, model.TariffTax{ID: row.ID, TariffID: row.TariffID, Name: row.Name, Rate: rate, SortOrder: row.SortOrder})
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, pgerr.Translate(r.pool, "commit replace tariff taxes", err)
	}
	return out, nil
}

// ManualYekdem — Isolation: tariff_manual_yekdem has no company_id — join
// through tariffs.
func (r *TariffRepository) ManualYekdem(ctx context.Context, s store.Scope, tariffID uuid.UUID) ([]model.TariffManualYekdem, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	if err := r.requireVisible(ctx, s, tariffID); err != nil {
		return nil, err
	}
	rows, err := r.q.TariffManualYekdemList(ctx, tariffID)
	if err != nil {
		return nil, pgerr.Translate(r.pool, "list tariff manual yekdem", err)
	}
	out := make([]model.TariffManualYekdem, 0, len(rows))
	for _, row := range rows {
		value, err := numericToDecimal(row.Value)
		if err != nil {
			return nil, fmt.Errorf("tariff_manual_yekdem.value: %w", err)
		}
		out = append(out, model.TariffManualYekdem{TariffID: row.TariffID, Year: row.Year, Month: row.Month, Value: value})
	}
	return out, nil
}

// ReplaceManualYekdem swaps tariff_manual_yekdem in one transaction, after
// confirming the tariff is visible to the Scope.
func (r *TariffRepository) ReplaceManualYekdem(ctx context.Context, s store.Scope, tariffID uuid.UUID, values []model.TariffManualYekdem) ([]model.TariffManualYekdem, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	if err := r.requireVisible(ctx, s, tariffID); err != nil {
		return nil, err
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, pgerr.Translate(r.pool, "begin replace tariff manual yekdem", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.q.WithTx(tx)

	if err := q.TariffManualYekdemDeleteForTariff(ctx, tariffID); err != nil {
		return nil, pgerr.Translate(r.pool, "clear tariff manual yekdem", err)
	}
	out := make([]model.TariffManualYekdem, 0, len(values))
	for _, v := range values {
		row, err := q.TariffManualYekdemInsert(ctx, sqlcgen.TariffManualYekdemInsertParams{
			TariffID: tariffID, Year: v.Year, Month: v.Month, Value: decimalToNumeric(v.Value),
		})
		if err != nil {
			return nil, pgerr.Translate(r.pool, "insert tariff manual yekdem", err)
		}
		value, err := numericToDecimal(row.Value)
		if err != nil {
			return nil, fmt.Errorf("tariff_manual_yekdem.value: %w", err)
		}
		out = append(out, model.TariffManualYekdem{TariffID: row.TariffID, Year: row.Year, Month: row.Month, Value: value})
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, pgerr.Translate(r.pool, "commit replace tariff manual yekdem", err)
	}
	return out, nil
}

// requireVisible is the shared isolation check for every tariff_taxes and
// tariff_manual_yekdem method: the tariff itself must be visible to the
// Scope, or the child is treated as belonging to no one.
func (r *TariffRepository) requireVisible(ctx context.Context, s store.Scope, tariffID uuid.UUID) error {
	ids, all := s.BuildingFilter()
	visible, err := r.q.TariffVisible(ctx, sqlcgen.TariffVisibleParams{
		ID: tariffID, CompanyID: s.CompanyID, AllBuildings: all, BuildingIds: ids,
	})
	if err != nil {
		return pgerr.Translate(r.pool, "check tariff visibility", err)
	}
	if !visible {
		return store.ErrNotFound
	}
	return nil
}

// tariffBuildingWritable reports whether a write naming buildingID (nil for
// a company-wide tariff) is authorised by s. A NULL building_id is
// deliberately allowed only for a Scope with AllBuildings, mirroring the
// read-side ruling: a narrow Scope may not create or move a tariff to
// company-wide standing.
func tariffBuildingWritable(s store.Scope, buildingID *uuid.UUID) bool {
	if buildingID == nil {
		return s.AllBuildings
	}
	return s.AllowsBuilding(*buildingID)
}

// tariffNumericFields lists every nullable `numeric` column on tariffs in
// declaration order, paired with a name for the wrapped error. Decoding them
// in a loop rather than as 20 repeated three-line blocks is not a style
// preference here: the block form was tried first and a single misplaced
// field silently swapped two prices with the same scale, which no test
// caught because both sides compiled and both were valid decimals. Walking
// this slice keeps the source-to-destination mapping in ONE place.
type tariffNumericField struct {
	name string
	src  pgtype.Numeric
	dst  **decimal.Decimal
}

func tariffFromRow(row sqlcgen.Tariff) (model.Tariff, error) {
	distributionCost, err := numericToDecimal(row.DistributionCost)
	if err != nil {
		return model.Tariff{}, fmt.Errorf("tariffs.distribution_cost: %w", err)
	}
	reactivePowerPrice, err := numericToDecimal(row.ReactivePowerPrice)
	if err != nil {
		return model.Tariff{}, fmt.Errorf("tariffs.reactive_power_price: %w", err)
	}
	vatRate, err := numericToDecimal(row.VatRate)
	if err != nil {
		return model.Tariff{}, fmt.Errorf("tariffs.vat_rate: %w", err)
	}

	out := model.Tariff{
		ID: row.ID, CompanyID: row.CompanyID, BuildingID: row.BuildingID, Name: row.Name,
		EffectiveFrom: tariffDateToTime(row.EffectiveFrom),
		Currency:      model.CurrencyCode(row.Currency),
		EnergyType:    model.EnergyType(row.EnergyType),
		VoltageLevel:  model.VoltageLevel(row.VoltageLevel),
		UserGroup:     model.DistributionUserGroup(row.UserGroup),
		PriceType:     model.PriceType(row.PriceType),
		Term:          model.TariffTerm(row.Term),
		SupplyCompany: model.SupplyCompany(row.SupplyCompany),

		DistributionCost:   distributionCost,
		ReactivePowerPrice: reactivePowerPrice,
		VatRate:            vatRate,

		GenerationUsage: model.GenerationUsage(row.GenerationUsage),
		UsePtfYekdem:    row.UsePtfYekdem,
		UseManualYekdem: row.UseManualYekdem,

		CreatedBy: row.CreatedBy,
		CreatedAt: row.CreatedAt.Time,
		UpdatedAt: row.UpdatedAt.Time,
		DeletedAt: tariffNullTimestamptz(row.DeletedAt),
	}

	for _, f := range []tariffNumericField{
		{"single_time_price", row.SingleTimePrice, &out.SingleTimePrice},
		{"t1_price", row.T1Price, &out.T1Price},
		{"t2_price", row.T2Price, &out.T2Price},
		{"t3_price", row.T3Price, &out.T3Price},
		{"overuse_price", row.OverusePrice, &out.OverusePrice},
		{"overuse_threshold_kwh_per_day", row.OveruseThresholdKwhPerDay, &out.OveruseThresholdKwhPerDay},
		{"green_energy_price", row.GreenEnergyPrice, &out.GreenEnergyPrice},
		{"green_energy_distribution_cost", row.GreenEnergyDistributionCost, &out.GreenEnergyDistributionCost},
		{"contracted_power_kw", row.ContractedPowerKw, &out.ContractedPowerKw},
		{"power_unit_price", row.PowerUnitPrice, &out.PowerUnitPrice},
		{"generation_price_per_kwh", row.GenerationPricePerKwh, &out.GenerationPricePerKwh},
		{"kbk_energy", row.KbkEnergy, &out.KbkEnergy},
		{"kbk_t1", row.KbkT1, &out.KbkT1},
		{"kbk_t2", row.KbkT2, &out.KbkT2},
		{"kbk_t3", row.KbkT3, &out.KbkT3},
		{"kbk_power_price", row.KbkPowerPrice, &out.KbkPowerPrice},
		{"kbk_overuse_price", row.KbkOverusePrice, &out.KbkOverusePrice},
		{"kbk_reactive_power", row.KbkReactivePower, &out.KbkReactivePower},
		{"kbk_distribution_cost_tl_per_kwh", row.KbkDistributionCostTlPerKwh, &out.KbkDistributionCostTlPerKwh},
	} {
		d, err := numericToDecimalPtr(f.src)
		if err != nil {
			return model.Tariff{}, fmt.Errorf("tariffs.%s: %w", f.name, err)
		}
		*f.dst = d
	}

	return out, nil
}

// ---------------------------------------------------------------------------
// TariffTemplateRepository
// ---------------------------------------------------------------------------

// TariffTemplateRepository implements store.TariffTemplateRepository.
//
// tariff_templates has no building_id: a template is a company-wide saved
// definition, so a Scope narrows it to the company and no further — the same
// shape PlantRepository uses for power_plants.
type TariffTemplateRepository struct {
	q    *sqlcgen.Queries
	pool *pgxpool.Pool
}

// NewTariffTemplateRepository builds a TariffTemplateRepository over pool.
func NewTariffTemplateRepository(pool *pgxpool.Pool) *TariffTemplateRepository {
	return &TariffTemplateRepository{q: sqlcgen.New(pool), pool: pool}
}

var _ store.TariffTemplateRepository = (*TariffTemplateRepository)(nil)

// Get returns one tariff template by id, scoped to the company.
func (r *TariffTemplateRepository) Get(ctx context.Context, s store.Scope, id uuid.UUID) (model.TariffTemplate, error) {
	if !s.Valid() {
		return model.TariffTemplate{}, store.ErrInvalidScope
	}
	row, err := r.q.TariffTemplateGet(ctx, sqlcgen.TariffTemplateGetParams{ID: id, CompanyID: s.CompanyID})
	if err != nil {
		return model.TariffTemplate{}, pgerr.Translate(r.pool, "get tariff template", err)
	}
	return tariffTemplateFromRow(row), nil
}

// List returns the company's templates matching f.
func (r *TariffTemplateRepository) List(ctx context.Context, s store.Scope, f store.TariffTemplateFilter) ([]model.TariffTemplate, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	limit, offset := tariffPageLimits(f.Page)
	rows, err := r.q.TariffTemplateList(ctx, sqlcgen.TariffTemplateListParams{
		CompanyID: s.CompanyID, NameContains: f.NameContains, IsDefault: f.IsDefault,
		LimitVal: limit, OffsetVal: offset,
	})
	if err != nil {
		return nil, pgerr.Translate(r.pool, "list tariff templates", err)
	}
	out := make([]model.TariffTemplate, 0, len(rows))
	for _, row := range rows {
		out = append(out, tariffTemplateFromRow(row))
	}
	return out, nil
}

// Create inserts a tariff template for the Scope's company.
func (r *TariffTemplateRepository) Create(ctx context.Context, s store.Scope, t model.TariffTemplate) (model.TariffTemplate, error) {
	if !s.Valid() {
		return model.TariffTemplate{}, store.ErrInvalidScope
	}
	if t.CompanyID != uuid.Nil && t.CompanyID != s.CompanyID {
		return model.TariffTemplate{}, store.ErrNotFound
	}
	row, err := r.q.TariffTemplateCreate(ctx, sqlcgen.TariffTemplateCreateParams{
		CompanyID: s.CompanyID, Name: t.Name, Description: t.Description, IsDefault: t.IsDefault,
		Payload: []byte(t.Payload), CreatedAt: tariffTimestamptz(t.CreatedAt),
	})
	if err != nil {
		return model.TariffTemplate{}, pgerr.Translate(r.pool, "create tariff template", err)
	}
	return tariffTemplateFromRow(row), nil
}

// Update fully replaces a template's fields by id.
func (r *TariffTemplateRepository) Update(ctx context.Context, s store.Scope, t model.TariffTemplate) (model.TariffTemplate, error) {
	if !s.Valid() {
		return model.TariffTemplate{}, store.ErrInvalidScope
	}
	row, err := r.q.TariffTemplateUpdate(ctx, sqlcgen.TariffTemplateUpdateParams{
		ID: t.ID, CompanyID: s.CompanyID, Name: t.Name, Description: t.Description,
		IsDefault: t.IsDefault, Payload: []byte(t.Payload), UpdatedAt: tariffTimestamptz(t.UpdatedAt),
	})
	if err != nil {
		return model.TariffTemplate{}, pgerr.Translate(r.pool, "update tariff template", err)
	}
	return tariffTemplateFromRow(row), nil
}

// Delete removes a template by id; tariff_templates has no deleted_at.
func (r *TariffTemplateRepository) Delete(ctx context.Context, s store.Scope, id uuid.UUID) error {
	if !s.Valid() {
		return store.ErrInvalidScope
	}
	n, err := r.q.TariffTemplateDelete(ctx, sqlcgen.TariffTemplateDeleteParams{ID: id, CompanyID: s.CompanyID})
	if err != nil {
		return pgerr.Translate(r.pool, "delete tariff template", err)
	}
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

func tariffTemplateFromRow(row sqlcgen.TariffTemplate) model.TariffTemplate {
	return model.TariffTemplate{
		ID: row.ID, CompanyID: row.CompanyID, Name: row.Name, Description: row.Description,
		IsDefault: row.IsDefault, Payload: row.Payload,
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
}

// ---------------------------------------------------------------------------
// SolarTariffRepository
// ---------------------------------------------------------------------------

// SolarTariffRepository implements store.SolarTariffRepository.
//
// solar_tariffs prices a plant, and power_plants has no building_id, so
// "visible to the Scope" here means company_id = s.CompanyID — the Scope's
// building branch narrows nothing further, exactly as for power_plants
// itself.
type SolarTariffRepository struct {
	q    *sqlcgen.Queries
	pool *pgxpool.Pool
}

// NewSolarTariffRepository builds a SolarTariffRepository over pool.
func NewSolarTariffRepository(pool *pgxpool.Pool) *SolarTariffRepository {
	return &SolarTariffRepository{q: sqlcgen.New(pool), pool: pool}
}

var _ store.SolarTariffRepository = (*SolarTariffRepository)(nil)

// Get returns one solar tariff by id, scoped to the company.
func (r *SolarTariffRepository) Get(ctx context.Context, s store.Scope, id uuid.UUID) (model.SolarTariff, error) {
	if !s.Valid() {
		return model.SolarTariff{}, store.ErrInvalidScope
	}
	row, err := r.q.SolarTariffGet(ctx, sqlcgen.SolarTariffGetParams{ID: id, CompanyID: s.CompanyID})
	if err != nil {
		return model.SolarTariff{}, pgerr.Translate(r.pool, "get solar tariff", err)
	}
	return solarTariffFromRow(row)
}

// List returns the company's solar tariffs matching f.
func (r *SolarTariffRepository) List(ctx context.Context, s store.Scope, f store.SolarTariffFilter) ([]model.SolarTariff, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	limit, offset := tariffPageLimits(f.Page)
	var effectiveOn pgtype.Date
	if f.EffectiveOn != nil {
		effectiveOn = tariffTimeToDate(*f.EffectiveOn)
	}
	rows, err := r.q.SolarTariffList(ctx, sqlcgen.SolarTariffListParams{
		CompanyID: s.CompanyID, PlantID: f.PlantID, EffectiveOn: effectiveOn,
		IncludeDeleted: f.IncludeDeleted, LimitVal: limit, OffsetVal: offset,
	})
	if err != nil {
		return nil, pgerr.Translate(r.pool, "list solar tariffs", err)
	}
	out := make([]model.SolarTariff, 0, len(rows))
	for _, row := range rows {
		st, err := solarTariffFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, nil
}

// Create inserts a solar tariff for a plant visible to the Scope.
func (r *SolarTariffRepository) Create(ctx context.Context, s store.Scope, t model.SolarTariff) (model.SolarTariff, error) {
	if !s.Valid() {
		return model.SolarTariff{}, store.ErrInvalidScope
	}
	if t.CompanyID != uuid.Nil && t.CompanyID != s.CompanyID {
		return model.SolarTariff{}, store.ErrNotFound
	}
	visible, err := r.q.SolarTariffPlantVisible(ctx, sqlcgen.SolarTariffPlantVisibleParams{PlantID: t.PlantID, CompanyID: s.CompanyID})
	if err != nil {
		return model.SolarTariff{}, pgerr.Translate(r.pool, "check plant visibility for solar tariff", err)
	}
	if !visible {
		return model.SolarTariff{}, store.ErrNotFound
	}
	row, err := r.q.SolarTariffCreate(ctx, sqlcgen.SolarTariffCreateParams{
		CompanyID: s.CompanyID, PlantID: t.PlantID, EffectiveFrom: tariffTimeToDate(t.EffectiveFrom),
		FeedInTariff: decimalToNumeric(t.FeedInTariff), PurchasePrice: decimalPtrToNumeric(t.PurchasePrice),
		Currency: sqlcgen.CurrencyCode(t.Currency), Notes: t.Notes, CreatedAt: tariffTimestamptz(t.CreatedAt),
	})
	if err != nil {
		return model.SolarTariff{}, pgerr.Translate(r.pool, "create solar tariff", err)
	}
	return solarTariffFromRow(row)
}

// SoftDelete stamps deleted_at on a solar tariff by id.
func (r *SolarTariffRepository) SoftDelete(ctx context.Context, s store.Scope, id uuid.UUID, at time.Time) error {
	if !s.Valid() {
		return store.ErrInvalidScope
	}
	n, err := r.q.SolarTariffSoftDelete(ctx, sqlcgen.SolarTariffSoftDeleteParams{
		DeletedAt: tariffTimestamptz(at), ID: id, CompanyID: s.CompanyID,
	})
	if err != nil {
		return pgerr.Translate(r.pool, "soft delete solar tariff", err)
	}
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

// Effective resolves by EffectiveFrom exactly as TariffRepository.Effective
// does, but with no company-wide fallback: solar_tariffs.plant_id is NOT
// NULL, so every row already names one plant.
func (r *SolarTariffRepository) Effective(ctx context.Context, s store.Scope, plantID uuid.UUID, on time.Time) (model.SolarTariff, error) {
	if !s.Valid() {
		return model.SolarTariff{}, store.ErrInvalidScope
	}
	visible, err := r.q.SolarTariffPlantVisible(ctx, sqlcgen.SolarTariffPlantVisibleParams{PlantID: plantID, CompanyID: s.CompanyID})
	if err != nil {
		return model.SolarTariff{}, pgerr.Translate(r.pool, "check plant visibility for effective solar tariff", err)
	}
	if !visible {
		return model.SolarTariff{}, store.ErrNotFound
	}
	row, err := r.q.SolarTariffEffective(ctx, sqlcgen.SolarTariffEffectiveParams{
		CompanyID: s.CompanyID, PlantID: plantID, EffectiveOn: tariffTimeToDate(on),
	})
	if err != nil {
		return model.SolarTariff{}, pgerr.Translate(r.pool, "resolve effective solar tariff", err)
	}
	return solarTariffFromRow(row)
}

func solarTariffFromRow(row sqlcgen.SolarTariff) (model.SolarTariff, error) {
	feedIn, err := numericToDecimal(row.FeedInTariff)
	if err != nil {
		return model.SolarTariff{}, fmt.Errorf("solar_tariffs.feed_in_tariff: %w", err)
	}
	purchasePrice, err := numericToDecimalPtr(row.PurchasePrice)
	if err != nil {
		return model.SolarTariff{}, fmt.Errorf("solar_tariffs.purchase_price: %w", err)
	}
	return model.SolarTariff{
		ID: row.ID, CompanyID: row.CompanyID, PlantID: row.PlantID,
		EffectiveFrom: tariffDateToTime(row.EffectiveFrom), FeedInTariff: feedIn, PurchasePrice: purchasePrice,
		Currency: model.CurrencyCode(row.Currency), Notes: row.Notes,
		CreatedAt: row.CreatedAt.Time, DeletedAt: tariffNullTimestamptz(row.DeletedAt),
	}, nil
}

// ---------------------------------------------------------------------------
// NationalTariffRepository
// ---------------------------------------------------------------------------

// NationalTariffRepository implements store.NationalTariffRepository.
//
// national_tariff_schedule is platform-wide (no company_id): the Scope is
// validated but narrows nothing. Writes go only through
// AdminCatalogueRepository.UpsertNationalTariffSchedule (Task 11b).
type NationalTariffRepository struct {
	q    *sqlcgen.Queries
	pool *pgxpool.Pool
}

// NewNationalTariffRepository builds a NationalTariffRepository over pool.
func NewNationalTariffRepository(pool *pgxpool.Pool) *NationalTariffRepository {
	return &NationalTariffRepository{q: sqlcgen.New(pool), pool: pool}
}

var _ store.NationalTariffRepository = (*NationalTariffRepository)(nil)

// List returns published national tariff schedule rows matching f. The
// Scope narrows nothing; it is validated because the table is platform-wide.
func (r *NationalTariffRepository) List(ctx context.Context, s store.Scope, f store.NationalTariffFilter) ([]model.NationalTariffScheduleEntry, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	limit, offset := tariffPageLimits(f.Page)
	var userGroup sqlcgen.NullDistributionUserGroup
	if f.UserGroup != nil {
		userGroup = sqlcgen.NullDistributionUserGroup{DistributionUserGroup: sqlcgen.DistributionUserGroup(*f.UserGroup), Valid: true}
	}
	var voltageLevel sqlcgen.NullVoltageLevel
	if f.VoltageLevel != nil {
		voltageLevel = sqlcgen.NullVoltageLevel{VoltageLevel: sqlcgen.VoltageLevel(*f.VoltageLevel), Valid: true}
	}
	var term sqlcgen.NullTariffTerm
	if f.Term != nil {
		term = sqlcgen.NullTariffTerm{TariffTerm: sqlcgen.TariffTerm(*f.Term), Valid: true}
	}
	var effectiveOn pgtype.Date
	if f.EffectiveOn != nil {
		effectiveOn = tariffTimeToDate(*f.EffectiveOn)
	}
	rows, err := r.q.NationalTariffList(ctx, sqlcgen.NationalTariffListParams{
		UserGroup: userGroup, VoltageLevel: voltageLevel, Term: term, EffectiveOn: effectiveOn,
		LimitVal: limit, OffsetVal: offset,
	})
	if err != nil {
		return nil, pgerr.Translate(r.pool, "list national tariff schedule", err)
	}
	out := make([]model.NationalTariffScheduleEntry, 0, len(rows))
	for _, row := range rows {
		e, err := nationalTariffFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

// Effective resolves the published schedule row in force on a date.
func (r *NationalTariffRepository) Effective(ctx context.Context, s store.Scope, group model.DistributionUserGroup, level model.VoltageLevel, term model.TariffTerm, on time.Time) (model.NationalTariffScheduleEntry, error) {
	if !s.Valid() {
		return model.NationalTariffScheduleEntry{}, store.ErrInvalidScope
	}
	row, err := r.q.NationalTariffEffective(ctx, sqlcgen.NationalTariffEffectiveParams{
		UserGroup: sqlcgen.DistributionUserGroup(group), VoltageLevel: sqlcgen.VoltageLevel(level),
		Term: sqlcgen.TariffTerm(term), EffectiveOn: tariffTimeToDate(on),
	})
	if err != nil {
		return model.NationalTariffScheduleEntry{}, pgerr.Translate(r.pool, "resolve effective national tariff", err)
	}
	return nationalTariffFromRow(row)
}

func nationalTariffFromRow(row sqlcgen.NationalTariffSchedule) (model.NationalTariffScheduleEntry, error) {
	energyPrice, err := numericToDecimal(row.EnergyPrice)
	if err != nil {
		return model.NationalTariffScheduleEntry{}, fmt.Errorf("national_tariff_schedule.energy_price: %w", err)
	}
	distributionPrice, err := numericToDecimal(row.DistributionPrice)
	if err != nil {
		return model.NationalTariffScheduleEntry{}, fmt.Errorf("national_tariff_schedule.distribution_price: %w", err)
	}
	vatRate, err := numericToDecimal(row.VatRate)
	if err != nil {
		return model.NationalTariffScheduleEntry{}, fmt.Errorf("national_tariff_schedule.vat_rate: %w", err)
	}
	t1, err := numericToDecimalPtr(row.T1Price)
	if err != nil {
		return model.NationalTariffScheduleEntry{}, fmt.Errorf("national_tariff_schedule.t1_price: %w", err)
	}
	t2, err := numericToDecimalPtr(row.T2Price)
	if err != nil {
		return model.NationalTariffScheduleEntry{}, fmt.Errorf("national_tariff_schedule.t2_price: %w", err)
	}
	t3, err := numericToDecimalPtr(row.T3Price)
	if err != nil {
		return model.NationalTariffScheduleEntry{}, fmt.Errorf("national_tariff_schedule.t3_price: %w", err)
	}
	powerPrice, err := numericToDecimalPtr(row.PowerPrice)
	if err != nil {
		return model.NationalTariffScheduleEntry{}, fmt.Errorf("national_tariff_schedule.power_price: %w", err)
	}
	overusePrice, err := numericToDecimalPtr(row.OverusePrice)
	if err != nil {
		return model.NationalTariffScheduleEntry{}, fmt.Errorf("national_tariff_schedule.overuse_price: %w", err)
	}
	dailyThreshold, err := numericToDecimalPtr(row.DailyThresholdKwh)
	if err != nil {
		return model.NationalTariffScheduleEntry{}, fmt.Errorf("national_tariff_schedule.daily_threshold_kwh: %w", err)
	}
	return model.NationalTariffScheduleEntry{
		ID: row.ID, EffectiveFrom: tariffDateToTime(row.EffectiveFrom),
		UserGroup: model.DistributionUserGroup(row.UserGroup), VoltageLevel: model.VoltageLevel(row.VoltageLevel),
		Term: model.TariffTerm(row.Term), EnergyPrice: energyPrice, T1Price: t1, T2Price: t2, T3Price: t3,
		DistributionPrice: distributionPrice, PowerPrice: powerPrice, OverusePrice: overusePrice,
		DailyThresholdKwh: dailyThreshold, VatRate: vatRate, Source: row.Source, CreatedAt: row.CreatedAt.Time,
	}, nil
}

// ---------------------------------------------------------------------------
// IcmalRepository
// ---------------------------------------------------------------------------

// IcmalRepository implements store.IcmalRepository.
//
// icmal_imports carries company_id but no building_id, so "visible to the
// Scope" for an import means company_id = s.CompanyID only. icmal_rows has
// neither and is reached only by joining through icmal_imports; a row's own
// BuildingID, when set, must additionally be a building visible to the
// Scope (see repository.go's ROWS WITHOUT company_id rules).
type IcmalRepository struct {
	q    *sqlcgen.Queries
	pool *pgxpool.Pool
}

// NewIcmalRepository builds an IcmalRepository over pool.
func NewIcmalRepository(pool *pgxpool.Pool) *IcmalRepository {
	return &IcmalRepository{q: sqlcgen.New(pool), pool: pool}
}

var _ store.IcmalRepository = (*IcmalRepository)(nil)

// CreateImport records an uploaded icmal file for the Scope's company.
func (r *IcmalRepository) CreateImport(ctx context.Context, s store.Scope, imp model.IcmalImport) (model.IcmalImport, error) {
	if !s.Valid() {
		return model.IcmalImport{}, store.ErrInvalidScope
	}
	if imp.CompanyID != uuid.Nil && imp.CompanyID != s.CompanyID {
		return model.IcmalImport{}, store.ErrNotFound
	}
	if imp.UploadedBy != nil {
		visible, err := r.q.IcmalUploaderVisible(ctx, sqlcgen.IcmalUploaderVisibleParams{UserID: *imp.UploadedBy, CompanyID: s.CompanyID})
		if err != nil {
			return model.IcmalImport{}, pgerr.Translate(r.pool, "check icmal uploader visibility", err)
		}
		if !visible {
			return model.IcmalImport{}, store.ErrNotFound
		}
	}
	status := imp.Status
	if status == "" {
		status = "pending"
	}
	row, err := r.q.IcmalImportCreate(ctx, sqlcgen.IcmalImportCreateParams{
		CompanyID: s.CompanyID, UploadedBy: imp.UploadedBy, FileName: imp.FileName,
		RowCount: imp.RowCount, Status: status, Result: []byte(imp.Result), CreatedAt: tariffTimestamptz(imp.CreatedAt),
	})
	if err != nil {
		return model.IcmalImport{}, pgerr.Translate(r.pool, "create icmal import", err)
	}
	return icmalImportFromRow(row), nil
}

// GetImport returns one icmal import by id, scoped to the company.
func (r *IcmalRepository) GetImport(ctx context.Context, s store.Scope, id uuid.UUID) (model.IcmalImport, error) {
	if !s.Valid() {
		return model.IcmalImport{}, store.ErrInvalidScope
	}
	row, err := r.q.IcmalImportGet(ctx, sqlcgen.IcmalImportGetParams{ID: id, CompanyID: s.CompanyID})
	if err != nil {
		return model.IcmalImport{}, pgerr.Translate(r.pool, "get icmal import", err)
	}
	return icmalImportFromRow(row), nil
}

// ListImports returns the company's icmal imports matching f.
func (r *IcmalRepository) ListImports(ctx context.Context, s store.Scope, f store.IcmalFilter) ([]model.IcmalImport, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	if f.Range != nil && !f.Range.Valid() {
		return nil, store.ErrInvalidRange
	}
	limit, offset := tariffPageLimits(f.Page)
	var from, to pgtype.Timestamptz
	if f.Range != nil {
		from, to = tariffTimestamptz(f.Range.From), tariffTimestamptz(f.Range.To)
	}
	rows, err := r.q.IcmalImportList(ctx, sqlcgen.IcmalImportListParams{
		CompanyID: s.CompanyID, Status: f.Status, RangeFrom: from, RangeTo: to,
		LimitVal: limit, OffsetVal: offset,
	})
	if err != nil {
		return nil, pgerr.Translate(r.pool, "list icmal imports", err)
	}
	out := make([]model.IcmalImport, 0, len(rows))
	for _, row := range rows {
		out = append(out, icmalImportFromRow(row))
	}
	return out, nil
}

// UpdateImportResult moves an import through pending -> analysed ->
// applied|rejected and stores the derived coefficients and warnings.
func (r *IcmalRepository) UpdateImportResult(ctx context.Context, s store.Scope, id uuid.UUID, status string, result []byte) (model.IcmalImport, error) {
	if !s.Valid() {
		return model.IcmalImport{}, store.ErrInvalidScope
	}
	row, err := r.q.IcmalImportUpdateResult(ctx, sqlcgen.IcmalImportUpdateResultParams{
		Status: status, Result: result, ID: id, CompanyID: s.CompanyID,
	})
	if err != nil {
		return model.IcmalImport{}, pgerr.Translate(r.pool, "update icmal import result", err)
	}
	return icmalImportFromRow(row), nil
}

// InsertRows — Isolation: icmal_rows has no company_id — join through
// icmal_imports. A row's BuildingID, when set, must be a building visible
// to the Scope; if any is not, the whole call is refused with ErrNotFound
// and nothing is written.
func (r *IcmalRepository) InsertRows(ctx context.Context, s store.Scope, importID uuid.UUID, rows []model.IcmalRow) (int64, error) {
	if !s.Valid() {
		return 0, store.ErrInvalidScope
	}
	visible, err := r.q.IcmalImportVisible(ctx, sqlcgen.IcmalImportVisibleParams{ID: importID, CompanyID: s.CompanyID})
	if err != nil {
		return 0, pgerr.Translate(r.pool, "check icmal import visibility", err)
	}
	if !visible {
		return 0, store.ErrNotFound
	}

	buildingIDs, all := s.BuildingFilter()
	checkIDs := distinctBuildingIDs(rows)
	if len(checkIDs) > 0 {
		count, err := r.q.IcmalVisibleBuildingCount(ctx, sqlcgen.IcmalVisibleBuildingCountParams{
			CompanyID: s.CompanyID, CheckIds: checkIDs, AllBuildings: all, BuildingIds: buildingIDs,
		})
		if err != nil {
			return 0, pgerr.Translate(r.pool, "check icmal row building visibility", err)
		}
		if count != int64(len(checkIDs)) {
			return 0, store.ErrNotFound
		}
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, pgerr.Translate(r.pool, "begin insert icmal rows", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.q.WithTx(tx)

	var n int64
	for _, row := range rows {
		_, err := q.IcmalRowInsert(ctx, sqlcgen.IcmalRowInsertParams{
			ImportID: importID, BuildingID: row.BuildingID, Period: row.Period, EtsoCode: row.EtsoCode,
			TotalKwh: decimalPtrToNumeric(row.TotalKwh), T0Kwh: decimalPtrToNumeric(row.T0Kwh),
			T1Kwh: decimalPtrToNumeric(row.T1Kwh), T2Kwh: decimalPtrToNumeric(row.T2Kwh), T3Kwh: decimalPtrToNumeric(row.T3Kwh),
			EnergyCharge: decimalPtrToNumeric(row.EnergyCharge), DistributionCharge: decimalPtrToNumeric(row.DistributionCharge),
			ReactiveCharge: decimalPtrToNumeric(row.ReactiveCharge), PowerCharge: decimalPtrToNumeric(row.PowerCharge),
			OveruseCharge: decimalPtrToNumeric(row.OveruseCharge), InductiveKvarh: decimalPtrToNumeric(row.InductiveKvarh),
			CapacitiveKvarh: decimalPtrToNumeric(row.CapacitiveKvarh), DemandKw: decimalPtrToNumeric(row.DemandKw),
			VatBase: decimalPtrToNumeric(row.VatBase), Vat: decimalPtrToNumeric(row.Vat), Btv: decimalPtrToNumeric(row.Btv),
			EnergyFund: decimalPtrToNumeric(row.EnergyFund), Trt: decimalPtrToNumeric(row.Trt),
			PriceDifference: decimalPtrToNumeric(row.PriceDifference), CorrectionAmount: decimalPtrToNumeric(row.CorrectionAmount),
			IsCancelled: row.IsCancelled, Term: row.Term, VoltageLevel: row.VoltageLevel, IsMultiTime: row.IsMultiTime,
			Raw: []byte(row.Raw),
		})
		if err != nil {
			return 0, pgerr.Translate(r.pool, "insert icmal row", err)
		}
		n++
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, pgerr.Translate(r.pool, "commit insert icmal rows", err)
	}
	return n, nil
}

// ListRows — Isolation: join icmal_rows through icmal_imports.
func (r *IcmalRepository) ListRows(ctx context.Context, s store.Scope, importID uuid.UUID, p store.Page) ([]model.IcmalRow, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	visible, err := r.q.IcmalImportVisible(ctx, sqlcgen.IcmalImportVisibleParams{ID: importID, CompanyID: s.CompanyID})
	if err != nil {
		return nil, pgerr.Translate(r.pool, "check icmal import visibility", err)
	}
	if !visible {
		return nil, store.ErrNotFound
	}
	limit, offset := tariffPageLimits(p)
	rows, err := r.q.IcmalRowList(ctx, sqlcgen.IcmalRowListParams{ImportID: importID, LimitVal: limit, OffsetVal: offset})
	if err != nil {
		return nil, pgerr.Translate(r.pool, "list icmal rows", err)
	}
	out := make([]model.IcmalRow, 0, len(rows))
	for _, row := range rows {
		ir, err := icmalRowFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, ir)
	}
	return out, nil
}

// distinctBuildingIDs collects the distinct, non-nil BuildingID values across
// a batch of icmal rows, for the one visibility-count round trip
// InsertRows makes rather than one per row.
func distinctBuildingIDs(rows []model.IcmalRow) []uuid.UUID {
	seen := make(map[uuid.UUID]struct{})
	out := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		if row.BuildingID == nil {
			continue
		}
		if _, ok := seen[*row.BuildingID]; ok {
			continue
		}
		seen[*row.BuildingID] = struct{}{}
		out = append(out, *row.BuildingID)
	}
	return out
}

func icmalImportFromRow(row sqlcgen.IcmalImport) model.IcmalImport {
	return model.IcmalImport{
		ID: row.ID, CompanyID: row.CompanyID, UploadedBy: row.UploadedBy, FileName: row.FileName,
		RowCount: row.RowCount, Status: row.Status, Result: row.Result, CreatedAt: row.CreatedAt.Time,
	}
}

func icmalRowFromRow(row sqlcgen.IcmalRow) (model.IcmalRow, error) {
	numFields := []tariffNumericField{}
	out := model.IcmalRow{
		ID: row.ID, ImportID: row.ImportID, BuildingID: row.BuildingID, Period: row.Period,
		EtsoCode: row.EtsoCode, IsCancelled: row.IsCancelled, Term: row.Term,
		VoltageLevel: row.VoltageLevel, IsMultiTime: row.IsMultiTime, Raw: row.Raw,
	}
	numFields = append(numFields,
		tariffNumericField{"total_kwh", row.TotalKwh, &out.TotalKwh},
		tariffNumericField{"t0_kwh", row.T0Kwh, &out.T0Kwh},
		tariffNumericField{"t1_kwh", row.T1Kwh, &out.T1Kwh},
		tariffNumericField{"t2_kwh", row.T2Kwh, &out.T2Kwh},
		tariffNumericField{"t3_kwh", row.T3Kwh, &out.T3Kwh},
		tariffNumericField{"energy_charge", row.EnergyCharge, &out.EnergyCharge},
		tariffNumericField{"distribution_charge", row.DistributionCharge, &out.DistributionCharge},
		tariffNumericField{"reactive_charge", row.ReactiveCharge, &out.ReactiveCharge},
		tariffNumericField{"power_charge", row.PowerCharge, &out.PowerCharge},
		tariffNumericField{"overuse_charge", row.OveruseCharge, &out.OveruseCharge},
		tariffNumericField{"inductive_kvarh", row.InductiveKvarh, &out.InductiveKvarh},
		tariffNumericField{"capacitive_kvarh", row.CapacitiveKvarh, &out.CapacitiveKvarh},
		tariffNumericField{"demand_kw", row.DemandKw, &out.DemandKw},
		tariffNumericField{"vat_base", row.VatBase, &out.VatBase},
		tariffNumericField{"vat", row.Vat, &out.Vat},
		tariffNumericField{"btv", row.Btv, &out.Btv},
		tariffNumericField{"energy_fund", row.EnergyFund, &out.EnergyFund},
		tariffNumericField{"trt", row.Trt, &out.Trt},
		tariffNumericField{"price_difference", row.PriceDifference, &out.PriceDifference},
		tariffNumericField{"correction_amount", row.CorrectionAmount, &out.CorrectionAmount},
	)
	for _, f := range numFields {
		d, err := numericToDecimalPtr(f.src)
		if err != nil {
			return model.IcmalRow{}, fmt.Errorf("icmal_rows.%s: %w", f.name, err)
		}
		*f.dst = d
	}
	return out, nil
}
