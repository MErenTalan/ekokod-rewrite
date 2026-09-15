package admin

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/internal/pgerr"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/sqlcgen"
)

// CatalogueRepository implements store.AdminCatalogueRepository: the
// platform reference catalogues (national tariff schedule, platform
// emission factors and conversions, integration provider catalogue). See
// doc.go for why none of this can take a Scope.
type CatalogueRepository struct {
	q    *sqlcgen.Queries
	pool *pgxpool.Pool
}

// NewCatalogueRepository builds a CatalogueRepository on pool.
func NewCatalogueRepository(pool *pgxpool.Pool) *CatalogueRepository {
	return &CatalogueRepository{q: sqlcgen.New(pool), pool: pool}
}

var _ store.AdminCatalogueRepository = (*CatalogueRepository)(nil)

var catalogueZeroUUID = uuid.Nil

func catalogueIDOrZero(id uuid.UUID) uuid.UUID {
	if id == uuid.Nil {
		return catalogueZeroUUID
	}
	return id
}

// --- UpsertNationalTariffSchedule ---------------------------------------

type catalogueNationalTariffRecord struct {
	ID                uuid.UUID `json:"id"`
	EffectiveFrom     string    `json:"effective_from"`
	UserGroup         string    `json:"user_group"`
	VoltageLevel      string    `json:"voltage_level"`
	Term              string    `json:"term"`
	EnergyPrice       string    `json:"energy_price"`
	T1Price           *string   `json:"t1_price"`
	T2Price           *string   `json:"t2_price"`
	T3Price           *string   `json:"t3_price"`
	DistributionPrice string    `json:"distribution_price"`
	PowerPrice        *string   `json:"power_price"`
	OverusePrice      *string   `json:"overuse_price"`
	DailyThresholdKwh *string   `json:"daily_threshold_kwh"`
	VatRate           string    `json:"vat_rate"`
	Source            *string   `json:"source"`
}

// catalogueDecimalString formats a nullable decimal for JSON, keeping nil as nil (a
// SQL NULL through jsonb_to_recordset) rather than a zero value.
func catalogueDecimalString(d *decimal.Decimal) *string {
	if d == nil {
		return nil
	}
	s := d.String()
	return &s
}

// UpsertNationalTariffSchedule writes national_tariff_schedule, keyed on its
// unique (effective_from, user_group, voltage_level, term), and returns the
// number of rows written.
func (r *CatalogueRepository) UpsertNationalTariffSchedule(ctx context.Context, entries []model.NationalTariffScheduleEntry) (int64, error) {
	if len(entries) == 0 {
		return 0, nil
	}
	records := make([]catalogueNationalTariffRecord, len(entries))
	for i, e := range entries {
		records[i] = catalogueNationalTariffRecord{
			ID:                catalogueIDOrZero(e.ID),
			EffectiveFrom:     e.EffectiveFrom.Format("2006-01-02"),
			UserGroup:         string(e.UserGroup),
			VoltageLevel:      string(e.VoltageLevel),
			Term:              string(e.Term),
			EnergyPrice:       e.EnergyPrice.String(),
			T1Price:           catalogueDecimalString(e.T1Price),
			T2Price:           catalogueDecimalString(e.T2Price),
			T3Price:           catalogueDecimalString(e.T3Price),
			DistributionPrice: e.DistributionPrice.String(),
			PowerPrice:        catalogueDecimalString(e.PowerPrice),
			OverusePrice:      catalogueDecimalString(e.OverusePrice),
			DailyThresholdKwh: catalogueDecimalString(e.DailyThresholdKwh),
			VatRate:           e.VatRate.String(),
			Source:            e.Source,
		}
	}
	payload, err := json.Marshal(records)
	if err != nil {
		return 0, fmt.Errorf("marshal national tariff schedule entries: %w", err)
	}
	n, err := r.q.AdminUpsertNationalTariffSchedule(ctx, payload)
	if err != nil {
		return 0, pgerr.Translate(r.pool, "upsert national tariff schedule", err)
	}
	return n, nil
}

// --- UpsertPlatformFactor -------------------------------------------------

// UpsertPlatformFactor writes a PLATFORM emission factor (company_id NULL),
// keyed on (coalesce(company_id, zero uuid), key). It never touches a
// company-owned factor: the query hard-codes company_id null in the
// inserted row, so the conflict target can only ever match another
// company_id-null row (see admin_catalogue.sql). A factor whose CompanyID
// is non-nil is refused with ErrNotFound before any database call.
func (r *CatalogueRepository) UpsertPlatformFactor(ctx context.Context, f model.EmissionFactor) (model.EmissionFactor, error) {
	if f.CompanyID != nil {
		return model.EmissionFactor{}, store.ErrNotFound
	}
	var scope sqlcgen.NullCarbonScope
	if f.Scope != nil {
		scope = sqlcgen.NullCarbonScope{CarbonScope: sqlcgen.CarbonScope(*f.Scope), Valid: true}
	}
	subCats := f.SubCategories
	if subCats == nil {
		subCats = []string{}
	}
	catPath := f.CategoryPath
	if catPath == nil {
		catPath = []string{}
	}
	row, err := r.q.AdminUpsertPlatformFactor(ctx, sqlcgen.AdminUpsertPlatformFactorParams{
		ID:            catalogueIDOrZero(f.ID),
		Key:           f.Key,
		Label:         f.Label,
		MainCategory:  f.MainCategory,
		SubCategories: subCats,
		CategoryPath:  catPath,
		BaseFactor:    decimalToNumeric(f.BaseFactor),
		BaseUnit:      f.BaseUnit,
		FuelType:      f.FuelType,
		VehicleType:   f.VehicleType,
		Scope:         scope,
		IsoCategory:   f.IsoCategory,
		Status:        f.Status,
		Source:        f.Source,
		SourceYear:    f.SourceYear,
		SourceUrl:     f.SourceURL,
	})
	if err != nil {
		return model.EmissionFactor{}, pgerr.Translate(r.pool, "upsert platform emission factor", err)
	}
	return catalogueEmissionFactorFromRow(row)
}

func catalogueEmissionFactorFromRow(row sqlcgen.EmissionFactor) (model.EmissionFactor, error) {
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
		ID: row.ID, CompanyID: row.CompanyID, Key: row.Key, Label: row.Label,
		MainCategory: row.MainCategory, SubCategories: row.SubCategories, CategoryPath: row.CategoryPath,
		BaseFactor: baseFactor, BaseUnit: row.BaseUnit, FuelType: row.FuelType, VehicleType: row.VehicleType,
		Scope: scope, IsoCategory: row.IsoCategory, Status: row.Status, Source: row.Source,
		SourceYear: row.SourceYear, SourceURL: row.SourceUrl,
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}, nil
}

// --- ReplacePlatformConversions -------------------------------------------

// ReplacePlatformConversions swaps the conversions of a PLATFORM factor in
// one transaction. The factor is locked FOR SHARE (AdminPlatformFactorOwnerForShare,
// which only ever matches a company_id-null row) before the delete+insert: a
// company-owned factor's id returns ErrNotFound and nothing is replaced.
func (r *CatalogueRepository) ReplacePlatformConversions(ctx context.Context, factorID uuid.UUID, conversions []model.EmissionFactorConversion) error {
	records := make([]catalogueConversionRecord, len(conversions))
	for i, c := range conversions {
		records[i] = catalogueConversionRecord{Unit: c.Unit, Multiplier: c.Multiplier.String(), Label: c.Label}
	}
	payload, err := json.Marshal(records)
	if err != nil {
		return fmt.Errorf("marshal platform conversions: %w", err)
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return pgerr.Translate(r.pool, "begin transaction", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := sqlcgen.New(tx)
	if _, err := qtx.AdminPlatformFactorOwnerForShare(ctx, factorID); err != nil {
		return pgerr.Translate(r.pool, "lock platform emission factor", err)
	}
	if err := qtx.AdminDeleteFactorConversions(ctx, factorID); err != nil {
		return pgerr.Translate(r.pool, "delete platform emission factor conversions", err)
	}
	if len(conversions) > 0 {
		if err := qtx.AdminInsertFactorConversions(ctx, sqlcgen.AdminInsertFactorConversionsParams{
			FactorID: factorID, Conversions: payload,
		}); err != nil {
			return pgerr.Translate(r.pool, "insert platform emission factor conversions", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return pgerr.Translate(r.pool, "commit transaction", err)
	}
	return nil
}

type catalogueConversionRecord struct {
	Unit       string `json:"unit"`
	Multiplier string `json:"multiplier"`
	Label      string `json:"label"`
}

// --- UpsertIntegrationDefinitions ------------------------------------------

type catalogueIntegrationDefinitionRecord struct {
	ID        uuid.UUID       `json:"id"`
	Provider  string          `json:"provider"`
	Subtype   string          `json:"subtype"`
	Endpoints json.RawMessage `json:"endpoints"`
}

// UpsertIntegrationDefinitions writes integration_definitions, keyed on its
// unique (provider, subtype), and returns the number of rows written.
func (r *CatalogueRepository) UpsertIntegrationDefinitions(ctx context.Context, defs []model.IntegrationDefinition) (int64, error) {
	if len(defs) == 0 {
		return 0, nil
	}
	records := make([]catalogueIntegrationDefinitionRecord, len(defs))
	for i, d := range defs {
		endpoints := d.Endpoints
		if endpoints == nil {
			endpoints = json.RawMessage("{}")
		}
		records[i] = catalogueIntegrationDefinitionRecord{
			ID: catalogueIDOrZero(d.ID), Provider: string(d.Provider), Subtype: d.Subtype, Endpoints: endpoints,
		}
	}
	payload, err := json.Marshal(records)
	if err != nil {
		return 0, fmt.Errorf("marshal integration definitions: %w", err)
	}
	n, err := r.q.AdminUpsertIntegrationDefinitions(ctx, payload)
	if err != nil {
		return 0, pgerr.Translate(r.pool, "upsert integration definitions", err)
	}
	return n, nil
}
