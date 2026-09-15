package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/internal/pgerr"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/sqlcgen"
)

// CarbonRepository implements store.CarbonRepository.
type CarbonRepository struct {
	q    *sqlcgen.Queries
	pool *pgxpool.Pool
}

// NewCarbonRepository builds a CarbonRepository on pool.
func NewCarbonRepository(pool *pgxpool.Pool) *CarbonRepository {
	return &CarbonRepository{q: sqlcgen.New(pool), pool: pool}
}

var _ store.CarbonRepository = (*CarbonRepository)(nil)

const (
	carbonListDefaultLimit = 100
	carbonListMaxLimit     = 1000
)

func carbonPageBounds(p store.Page) (limit, offset int32) {
	limit = p.Limit
	if limit <= 0 {
		limit = carbonListDefaultLimit
	}
	if limit > carbonListMaxLimit {
		limit = carbonListMaxLimit
	}
	offset = p.Offset
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

// --- emission_factors --------------------------------------------------

func emissionFactorFromRow(row sqlcgen.EmissionFactor) (model.EmissionFactor, error) {
	baseFactor, err := numericToDecimal(row.BaseFactor)
	if err != nil {
		return model.EmissionFactor{}, fmt.Errorf("emission_factors.base_factor: %w", err)
	}
	var scope *model.CarbonScope
	if row.Scope.Valid {
		cs := model.CarbonScope(row.Scope.CarbonScope)
		scope = &cs
	}
	return model.EmissionFactor{
		ID:            row.ID,
		CompanyID:     row.CompanyID,
		Key:           row.Key,
		Label:         row.Label,
		MainCategory:  row.MainCategory,
		SubCategories: row.SubCategories,
		CategoryPath:  row.CategoryPath,
		BaseFactor:    baseFactor,
		BaseUnit:      row.BaseUnit,
		FuelType:      row.FuelType,
		VehicleType:   row.VehicleType,
		Scope:         scope,
		IsoCategory:   row.IsoCategory,
		Status:        row.Status,
		Source:        row.Source,
		SourceYear:    row.SourceYear,
		SourceURL:     row.SourceUrl,
		CreatedAt:     row.CreatedAt.Time,
		UpdatedAt:     row.UpdatedAt.Time,
	}, nil
}

func carbonScopeParam(s *model.CarbonScope) sqlcgen.NullCarbonScope {
	if s == nil {
		return sqlcgen.NullCarbonScope{}
	}
	return sqlcgen.NullCarbonScope{CarbonScope: sqlcgen.CarbonScope(*s), Valid: true}
}

// carbonNonNilStrings turns a nil slice into an empty one: sub_categories and
// category_path are `not null default '{}'`, and this repository always
// writes an explicit value for both, so a nil model slice must become '{}',
// never SQL NULL.
func carbonNonNilStrings(ss []string) []string {
	if ss == nil {
		return []string{}
	}
	return ss
}

// Factor returns a platform factor or one of s.CompanyID's own. Another
// company's factor returns ErrNotFound.
func (r *CarbonRepository) Factor(ctx context.Context, s store.Scope, id uuid.UUID) (model.EmissionFactor, error) {
	if !s.Valid() {
		return model.EmissionFactor{}, store.ErrInvalidScope
	}
	companyID := s.CompanyID
	row, err := r.q.CarbonGetFactor(ctx, sqlcgen.CarbonGetFactorParams{ID: id, CompanyID: &companyID})
	if err != nil {
		return model.EmissionFactor{}, pgerr.Translate(r.pool, "get emission factor", err)
	}
	return emissionFactorFromRow(row)
}

// ListFactors returns s.CompanyID's own factors, and the platform catalogue
// too when f.IncludePlatform is set.
func (r *CarbonRepository) ListFactors(ctx context.Context, s store.Scope, f store.EmissionFactorFilter) ([]model.EmissionFactor, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	limit, offset := carbonPageBounds(f.Page)
	rows, err := r.q.CarbonListFactors(ctx, sqlcgen.CarbonListFactorsParams{
		CompanyID:       s.CompanyID,
		IncludePlatform: f.IncludePlatform,
		Keys:            f.Keys,
		MainCategory:    f.MainCategory,
		Scope:           carbonScopeParam(f.Scope),
		OffsetVal:       offset,
		LimitVal:        limit,
	})
	if err != nil {
		return nil, pgerr.Translate(r.pool, "list emission factors", err)
	}
	out := make([]model.EmissionFactor, 0, len(rows))
	for _, row := range rows {
		ef, err := emissionFactorFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, ef)
	}
	return out, nil
}

// UpsertFactor writes a COMPANY-OWNED factor, keyed on
// (coalesce(company_id, zero uuid), key). f.CompanyID must equal s.CompanyID,
// checked before any database call; an id naming another company's or the
// platform's factor is refused with ErrNotFound by the query itself (see
// carbon.sql).
func (r *CarbonRepository) UpsertFactor(ctx context.Context, s store.Scope, f model.EmissionFactor) (model.EmissionFactor, error) {
	if !s.Valid() {
		return model.EmissionFactor{}, store.ErrInvalidScope
	}
	if f.CompanyID == nil || *f.CompanyID != s.CompanyID {
		return model.EmissionFactor{}, store.ErrNotFound
	}
	row, err := r.q.CarbonUpsertFactor(ctx, sqlcgen.CarbonUpsertFactorParams{
		ID:            f.ID,
		CompanyID:     s.CompanyID,
		Key:           f.Key,
		Label:         f.Label,
		MainCategory:  f.MainCategory,
		SubCategories: carbonNonNilStrings(f.SubCategories),
		CategoryPath:  carbonNonNilStrings(f.CategoryPath),
		BaseFactor:    decimalToNumeric(f.BaseFactor),
		BaseUnit:      f.BaseUnit,
		FuelType:      f.FuelType,
		VehicleType:   f.VehicleType,
		Scope:         carbonScopeParam(f.Scope),
		IsoCategory:   f.IsoCategory,
		Status:        f.Status,
		Source:        f.Source,
		SourceYear:    f.SourceYear,
		SourceUrl:     f.SourceURL,
	})
	if err != nil {
		return model.EmissionFactor{}, pgerr.Translate(r.pool, "upsert emission factor", err)
	}
	return emissionFactorFromRow(row)
}

// --- emission_factor_conversions ---------------------------------------

// Conversions — Isolation: emission_factor_conversions has no company_id, so
// visibility is proven by fetching the parent factor first (CarbonGetFactor,
// whose own query carries the scope predicate): a factor id that does not
// exist, or is not visible, returns ErrNotFound instead of an ambiguous empty
// list.
func (r *CarbonRepository) Conversions(ctx context.Context, s store.Scope, factorID uuid.UUID) ([]model.EmissionFactorConversion, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	companyID := s.CompanyID
	if _, err := r.q.CarbonGetFactor(ctx, sqlcgen.CarbonGetFactorParams{ID: factorID, CompanyID: &companyID}); err != nil {
		return nil, pgerr.Translate(r.pool, "get emission factor", err)
	}
	rows, err := r.q.CarbonListConversions(ctx, sqlcgen.CarbonListConversionsParams{FactorID: factorID, CompanyID: &companyID})
	if err != nil {
		return nil, pgerr.Translate(r.pool, "list emission factor conversions", err)
	}
	out := make([]model.EmissionFactorConversion, 0, len(rows))
	for _, row := range rows {
		mult, err := numericToDecimal(row.Multiplier)
		if err != nil {
			return nil, fmt.Errorf("emission_factor_conversions.multiplier: %w", err)
		}
		out = append(out, model.EmissionFactorConversion{FactorID: row.FactorID, Unit: row.Unit, Multiplier: mult, Label: row.Label})
	}
	return out, nil
}

type carbonConversionRecord struct {
	Unit       string `json:"unit"`
	Multiplier string `json:"multiplier"`
	Label      string `json:"label"`
}

func carbonConversionRecordsJSON(conversions []model.EmissionFactorConversion) ([]byte, error) {
	records := make([]carbonConversionRecord, len(conversions))
	for i, c := range conversions {
		records[i] = carbonConversionRecord{Unit: c.Unit, Multiplier: c.Multiplier.String(), Label: c.Label}
	}
	return json.Marshal(records)
}

// ReplaceConversions — Isolation: emission_factor_conversions has no
// company_id, so the parent factor is locked FOR SHARE inside this
// transaction (CarbonFactorOwnerForShare) before the delete+insert: a
// platform factor's id, or another company's, returns ErrNotFound and
// nothing is replaced.
func (r *CarbonRepository) ReplaceConversions(ctx context.Context, s store.Scope, factorID uuid.UUID, conversions []model.EmissionFactorConversion) error {
	if !s.Valid() {
		return store.ErrInvalidScope
	}
	payload, err := carbonConversionRecordsJSON(conversions)
	if err != nil {
		return fmt.Errorf("marshal conversions: %w", err)
	}
	return r.inTx(ctx, func(qtx *sqlcgen.Queries) error {
		owner, err := qtx.CarbonFactorOwnerForShare(ctx, factorID)
		if err != nil {
			return pgerr.Translate(r.pool, "lock emission factor", err)
		}
		if owner == nil || *owner != s.CompanyID {
			return store.ErrNotFound
		}
		if err := qtx.CarbonDeleteConversions(ctx, factorID); err != nil {
			return pgerr.Translate(r.pool, "delete emission factor conversions", err)
		}
		if len(conversions) == 0 {
			return nil
		}
		if err := qtx.CarbonInsertConversions(ctx, sqlcgen.CarbonInsertConversionsParams{
			FactorID: factorID, Conversions: payload,
		}); err != nil {
			return pgerr.Translate(r.pool, "insert emission factor conversions", err)
		}
		return nil
	})
}

// --- carbon_selected_activities -----------------------------------------

// SelectedActivities lists the activity keys a building tracks. Both
// company_id and building_id are stored directly on the table, so an
// invisible building simply yields no rows, matching this codebase's List
// convention ("an empty result is not an error"). The narrowing is the
// query's own predicate (CarbonListSelectedActivities), not
// Scope.AllowsBuilding: AllowsBuilding returns true for any id at all under
// AllBuildings, including another tenant's, so it must never be what stands
// between a caller and a row.
func (r *CarbonRepository) SelectedActivities(ctx context.Context, s store.Scope, buildingID uuid.UUID) ([]model.CarbonSelectedActivity, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	buildingIDs, allBuildings := s.BuildingFilter()
	rows, err := r.q.CarbonListSelectedActivities(ctx, sqlcgen.CarbonListSelectedActivitiesParams{
		CompanyID: s.CompanyID, BuildingID: buildingID, AllBuildings: allBuildings, BuildingIds: buildingIDs,
	})
	if err != nil {
		return nil, pgerr.Translate(r.pool, "list selected carbon activities", err)
	}
	out := make([]model.CarbonSelectedActivity, 0, len(rows))
	for _, row := range rows {
		out = append(out, model.CarbonSelectedActivity{CompanyID: row.CompanyID, BuildingID: row.BuildingID, ActivityKey: row.ActivityKey})
	}
	return out, nil
}

// ReplaceSelectedActivities — Isolation: the building is locked FOR SHARE
// inside this transaction (CarbonBuildingVisibleForShare) before the
// delete+insert: a building the Scope cannot see returns ErrNotFound and
// nothing is replaced.
func (r *CarbonRepository) ReplaceSelectedActivities(ctx context.Context, s store.Scope, buildingID uuid.UUID, keys []string) error {
	if !s.Valid() {
		return store.ErrInvalidScope
	}
	buildingIDs, allBuildings := s.BuildingFilter()
	return r.inTx(ctx, func(qtx *sqlcgen.Queries) error {
		if _, err := qtx.CarbonBuildingVisibleForShare(ctx, sqlcgen.CarbonBuildingVisibleForShareParams{
			BuildingID: buildingID, CompanyID: s.CompanyID, AllBuildings: allBuildings, BuildingIds: buildingIDs,
		}); err != nil {
			return pgerr.Translate(r.pool, "lock building", err)
		}
		if err := qtx.CarbonDeleteSelectedActivities(ctx, sqlcgen.CarbonDeleteSelectedActivitiesParams{
			CompanyID: s.CompanyID, BuildingID: buildingID,
		}); err != nil {
			return pgerr.Translate(r.pool, "delete selected carbon activities", err)
		}
		if len(keys) == 0 {
			return nil
		}
		if err := qtx.CarbonInsertSelectedActivities(ctx, sqlcgen.CarbonInsertSelectedActivitiesParams{
			CompanyID: s.CompanyID, BuildingID: buildingID, Keys: keys,
		}); err != nil {
			return pgerr.Translate(r.pool, "insert selected carbon activities", err)
		}
		return nil
	})
}

// --- carbon_activities ---------------------------------------------------

func carbonActivityFromRow(row sqlcgen.CarbonActivity) (model.CarbonActivity, error) {
	quantity, err := numericToDecimal(row.Quantity)
	if err != nil {
		return model.CarbonActivity{}, fmt.Errorf("carbon_activities.quantity: %w", err)
	}
	factorValue, err := numericToDecimalPtr(row.FactorValue)
	if err != nil {
		return model.CarbonActivity{}, fmt.Errorf("carbon_activities.factor_value: %w", err)
	}
	conversionMultiplier, err := numericToDecimal(row.ConversionMultiplier)
	if err != nil {
		return model.CarbonActivity{}, fmt.Errorf("carbon_activities.conversion_multiplier: %w", err)
	}
	emission, err := numericToDecimal(row.EmissionKgco2e)
	if err != nil {
		return model.CarbonActivity{}, fmt.Errorf("carbon_activities.emission_kgco2e: %w", err)
	}
	return model.CarbonActivity{
		ID:                   row.ID,
		CompanyID:            row.CompanyID,
		BuildingID:           row.BuildingID,
		MainCategory:         row.MainCategory,
		SubCategory:          row.SubCategory,
		ActivityType:         row.ActivityType,
		PeriodStart:          row.PeriodStart.Time,
		PeriodEnd:            row.PeriodEnd.Time,
		Quantity:             quantity,
		Unit:                 row.Unit,
		FactorID:             row.FactorID,
		FactorKey:            row.FactorKey,
		FactorValue:          factorValue,
		ConversionMultiplier: conversionMultiplier,
		EmissionKgco2e:       emission,
		Scope:                model.CarbonScope(row.Scope),
		IsoCategory:          row.IsoCategory,
		Description:          row.Description,
		Details:              row.Details,
		Status:               model.CarbonStatus(row.Status),
		IsAutomated:          row.IsAutomated,
		CreatedBy:            row.CreatedBy,
		CreatedAt:            row.CreatedAt.Time,
		UpdatedAt:            row.UpdatedAt.Time,
	}, nil
}

// Activity returns one carbon activity by id, scoped to s.
func (r *CarbonRepository) Activity(ctx context.Context, s store.Scope, id uuid.UUID) (model.CarbonActivity, error) {
	if !s.Valid() {
		return model.CarbonActivity{}, store.ErrInvalidScope
	}
	buildingIDs, allBuildings := s.BuildingFilter()
	row, err := r.q.CarbonGetActivity(ctx, sqlcgen.CarbonGetActivityParams{
		ID: id, CompanyID: s.CompanyID, AllBuildings: allBuildings, BuildingIds: buildingIDs,
	})
	if err != nil {
		return model.CarbonActivity{}, pgerr.Translate(r.pool, "get carbon activity", err)
	}
	return carbonActivityFromRow(row)
}

// ListActivities returns the recorded activities matching f, scoped to s.
func (r *CarbonRepository) ListActivities(ctx context.Context, s store.Scope, f store.CarbonActivityFilter) ([]model.CarbonActivity, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	buildingIDs, allBuildings := s.BuildingFilter()
	limit, offset := carbonPageBounds(f.Page)

	// scopes/statuses go over the wire as text[]: pgx has no array codec for
	// a custom enum type without explicit registration (see carbon.sql).
	var scopes, statuses []string
	if len(f.Scopes) > 0 {
		scopes = make([]string, len(f.Scopes))
		for i, sc := range f.Scopes {
			scopes[i] = string(sc)
		}
	}
	if len(f.Statuses) > 0 {
		statuses = make([]string, len(f.Statuses))
		for i, st := range f.Statuses {
			statuses[i] = string(st)
		}
	}

	rows, err := r.q.CarbonListActivities(ctx, sqlcgen.CarbonListActivitiesParams{
		CompanyID:         s.CompanyID,
		AllBuildings:      allBuildings,
		BuildingIds:       buildingIDs,
		FilterBuildingIds: f.BuildingIDs,
		Scopes:            scopes,
		Statuses:          statuses,
		ActivityType:      f.ActivityType,
		PeriodFrom:        carbonDateParam(f.PeriodFrom),
		PeriodTo:          carbonDateParam(f.PeriodTo),
		IsAutomated:       f.IsAutomated,
		OffsetVal:         offset,
		LimitVal:          limit,
	})
	if err != nil {
		return nil, pgerr.Translate(r.pool, "list carbon activities", err)
	}
	out := make([]model.CarbonActivity, 0, len(rows))
	for _, row := range rows {
		a, err := carbonActivityFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}

func carbonDateParam(t *time.Time) pgtype.Date {
	if t == nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: *t, Valid: true}
}

// CreateActivity — Isolation: the building must be visible to s, checked
// atomically by the insert's own `where exists (...)` clause (carbon.sql).
// a.CompanyID must equal s.CompanyID, checked first.
func (r *CarbonRepository) CreateActivity(ctx context.Context, s store.Scope, a model.CarbonActivity) (model.CarbonActivity, error) {
	if !s.Valid() {
		return model.CarbonActivity{}, store.ErrInvalidScope
	}
	if a.CompanyID != s.CompanyID {
		return model.CarbonActivity{}, store.ErrNotFound
	}
	buildingIDs, allBuildings := s.BuildingFilter()
	row, err := r.q.CarbonCreateActivity(ctx, sqlcgen.CarbonCreateActivityParams{
		ID: a.ID, CompanyID: s.CompanyID, BuildingID: a.BuildingID,
		MainCategory: a.MainCategory, SubCategory: a.SubCategory, ActivityType: a.ActivityType,
		PeriodStart: pgtype.Date{Time: a.PeriodStart, Valid: true}, PeriodEnd: pgtype.Date{Time: a.PeriodEnd, Valid: true},
		Quantity: decimalToNumeric(a.Quantity), Unit: a.Unit,
		FactorID: a.FactorID, FactorKey: a.FactorKey, FactorValue: decimalPtrToNumeric(a.FactorValue),
		ConversionMultiplier: decimalToNumeric(a.ConversionMultiplier), EmissionKgco2e: decimalToNumeric(a.EmissionKgco2e),
		Scope: sqlcgen.CarbonScope(a.Scope), IsoCategory: a.IsoCategory, Description: a.Description,
		Details: a.Details, Status: sqlcgen.CarbonStatus(a.Status), CreatedBy: a.CreatedBy,
		AllBuildings: allBuildings, BuildingIds: buildingIDs,
	})
	if err != nil {
		return model.CarbonActivity{}, pgerr.Translate(r.pool, "create carbon activity", err)
	}
	return carbonActivityFromRow(row)
}

// UpdateActivity is a full replace by id. building_id is not updatable: an
// activity stays under the building it was created in.
func (r *CarbonRepository) UpdateActivity(ctx context.Context, s store.Scope, a model.CarbonActivity) (model.CarbonActivity, error) {
	if !s.Valid() {
		return model.CarbonActivity{}, store.ErrInvalidScope
	}
	if a.CompanyID != s.CompanyID {
		return model.CarbonActivity{}, store.ErrNotFound
	}
	buildingIDs, allBuildings := s.BuildingFilter()
	row, err := r.q.CarbonUpdateActivity(ctx, sqlcgen.CarbonUpdateActivityParams{
		MainCategory: a.MainCategory, SubCategory: a.SubCategory, ActivityType: a.ActivityType,
		PeriodStart: pgtype.Date{Time: a.PeriodStart, Valid: true}, PeriodEnd: pgtype.Date{Time: a.PeriodEnd, Valid: true},
		Quantity: decimalToNumeric(a.Quantity), Unit: a.Unit,
		FactorID: a.FactorID, FactorKey: a.FactorKey, FactorValue: decimalPtrToNumeric(a.FactorValue),
		ConversionMultiplier: decimalToNumeric(a.ConversionMultiplier), EmissionKgco2e: decimalToNumeric(a.EmissionKgco2e),
		Scope: sqlcgen.CarbonScope(a.Scope), IsoCategory: a.IsoCategory, Description: a.Description,
		Details: a.Details, Status: sqlcgen.CarbonStatus(a.Status),
		ID: a.ID, CompanyID: s.CompanyID, AllBuildings: allBuildings, BuildingIds: buildingIDs,
	})
	if err != nil {
		return model.CarbonActivity{}, pgerr.Translate(r.pool, "update carbon activity", err)
	}
	return carbonActivityFromRow(row)
}

// DeleteActivity removes one carbon activity by id, scoped to s.
func (r *CarbonRepository) DeleteActivity(ctx context.Context, s store.Scope, id uuid.UUID) error {
	if !s.Valid() {
		return store.ErrInvalidScope
	}
	buildingIDs, allBuildings := s.BuildingFilter()
	n, err := r.q.CarbonDeleteActivity(ctx, sqlcgen.CarbonDeleteActivityParams{
		ID: id, CompanyID: s.CompanyID, AllBuildings: allBuildings, BuildingIds: buildingIDs,
	})
	if err != nil {
		return pgerr.Translate(r.pool, "delete carbon activity", err)
	}
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

// UpsertAutomatedActivity is the derived-from-meter-data path, keyed on the
// partial unique index (building_id, activity_type, period_start) where
// is_automated.
func (r *CarbonRepository) UpsertAutomatedActivity(ctx context.Context, s store.Scope, a model.CarbonActivity) (model.CarbonActivity, error) {
	if !s.Valid() {
		return model.CarbonActivity{}, store.ErrInvalidScope
	}
	if a.CompanyID != s.CompanyID {
		return model.CarbonActivity{}, store.ErrNotFound
	}
	buildingIDs, allBuildings := s.BuildingFilter()
	row, err := r.q.CarbonUpsertAutomatedActivity(ctx, sqlcgen.CarbonUpsertAutomatedActivityParams{
		CompanyID: s.CompanyID, BuildingID: a.BuildingID,
		MainCategory: a.MainCategory, SubCategory: a.SubCategory, ActivityType: a.ActivityType,
		PeriodStart: pgtype.Date{Time: a.PeriodStart, Valid: true}, PeriodEnd: pgtype.Date{Time: a.PeriodEnd, Valid: true},
		Quantity: decimalToNumeric(a.Quantity), Unit: a.Unit,
		FactorID: a.FactorID, FactorKey: a.FactorKey, FactorValue: decimalPtrToNumeric(a.FactorValue),
		ConversionMultiplier: decimalToNumeric(a.ConversionMultiplier), EmissionKgco2e: decimalToNumeric(a.EmissionKgco2e),
		Scope: sqlcgen.CarbonScope(a.Scope), IsoCategory: a.IsoCategory, Description: a.Description,
		Details: a.Details, Status: sqlcgen.CarbonStatus(a.Status), CreatedBy: a.CreatedBy,
		AllBuildings: allBuildings, BuildingIds: buildingIDs,
	})
	if err != nil {
		return model.CarbonActivity{}, pgerr.Translate(r.pool, "upsert automated carbon activity", err)
	}
	return carbonActivityFromRow(row)
}

// SetActivityStatus moves an activity through pending/approved/rejected.
func (r *CarbonRepository) SetActivityStatus(ctx context.Context, s store.Scope, id uuid.UUID, status model.CarbonStatus, at time.Time) (model.CarbonActivity, error) {
	if !s.Valid() {
		return model.CarbonActivity{}, store.ErrInvalidScope
	}
	buildingIDs, allBuildings := s.BuildingFilter()
	row, err := r.q.CarbonSetActivityStatus(ctx, sqlcgen.CarbonSetActivityStatusParams{
		Status: sqlcgen.CarbonStatus(status), At: pgtype.Timestamptz{Time: at, Valid: true},
		ID: id, CompanyID: s.CompanyID, AllBuildings: allBuildings, BuildingIds: buildingIDs,
	})
	if err != nil {
		return model.CarbonActivity{}, pgerr.Translate(r.pool, "set carbon activity status", err)
	}
	return carbonActivityFromRow(row)
}

// --- carbon_reports -------------------------------------------------------

func carbonReportFromRow(row sqlcgen.CarbonReport) model.CarbonReport {
	return model.CarbonReport{
		ID: row.ID, CompanyID: row.CompanyID, BuildingID: row.BuildingID,
		Name: row.Name, ReportType: row.ReportType, Period: row.Period,
		Payload: row.Payload, PdfPath: row.PdfPath, CreatedAt: row.CreatedAt.Time,
	}
}

// CreateReport stores a generated GHG or ISO carbon report.
func (r *CarbonRepository) CreateReport(ctx context.Context, s store.Scope, rep model.CarbonReport) (model.CarbonReport, error) {
	if !s.Valid() {
		return model.CarbonReport{}, store.ErrInvalidScope
	}
	if rep.CompanyID != s.CompanyID {
		return model.CarbonReport{}, store.ErrNotFound
	}
	buildingIDs, allBuildings := s.BuildingFilter()
	row, err := r.q.CarbonCreateReport(ctx, sqlcgen.CarbonCreateReportParams{
		ID: rep.ID, CompanyID: s.CompanyID, BuildingID: rep.BuildingID,
		Name: rep.Name, ReportType: rep.ReportType, Period: rep.Period,
		Payload: rep.Payload, PdfPath: rep.PdfPath,
		AllBuildings: allBuildings, BuildingIds: buildingIDs,
	})
	if err != nil {
		return model.CarbonReport{}, pgerr.Translate(r.pool, "create carbon report", err)
	}
	return carbonReportFromRow(row), nil
}

// ListReports returns a company's carbon reports, optionally narrowed to one building.
func (r *CarbonRepository) ListReports(ctx context.Context, s store.Scope, buildingID *uuid.UUID, p store.Page) ([]model.CarbonReport, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	buildingIDs, allBuildings := s.BuildingFilter()
	limit, offset := carbonPageBounds(p)
	rows, err := r.q.CarbonListReports(ctx, sqlcgen.CarbonListReportsParams{
		CompanyID: s.CompanyID, AllBuildings: allBuildings, BuildingIds: buildingIDs,
		BuildingID: buildingID, OffsetVal: offset, LimitVal: limit,
	})
	if err != nil {
		return nil, pgerr.Translate(r.pool, "list carbon reports", err)
	}
	out := make([]model.CarbonReport, 0, len(rows))
	for _, row := range rows {
		out = append(out, carbonReportFromRow(row))
	}
	return out, nil
}

// inTx runs fn inside one transaction on r.pool, committing on a nil error
// and rolling back otherwise. It is a METHOD, not a package-level function,
// specifically so its name is namespaced to *CarbonRepository and cannot
// collide with a sibling task's identically-named helper on its own
// repository type in the same package.
func (r *CarbonRepository) inTx(ctx context.Context, fn func(qtx *sqlcgen.Queries) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return pgerr.Translate(r.pool, "begin transaction", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := sqlcgen.New(tx)
	if err := fn(qtx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return pgerr.Translate(r.pool, "commit transaction", err)
	}
	return nil
}
