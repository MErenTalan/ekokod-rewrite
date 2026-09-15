package postgres

// This file implements store.AnalyzerRepository — the "Buildings, analyzers
// and plants (migration 00003)" block of internal/store/repository.go.

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/internal/pgerr"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/sqlcgen"
)

const (
	analyzerPageDefaultLimit = 50
	analyzerPageMaxLimit     = 500
)

// AnalyzerRepository is the postgres store.AnalyzerRepository.
type AnalyzerRepository struct {
	q    *sqlcgen.Queries
	pool *pgxpool.Pool
}

var _ store.AnalyzerRepository = (*AnalyzerRepository)(nil)

// NewAnalyzerRepository builds an AnalyzerRepository over pool.
func NewAnalyzerRepository(pool *pgxpool.Pool) *AnalyzerRepository {
	return &AnalyzerRepository{q: sqlcgen.New(pool), pool: pool}
}

func analyzerFromRow(row sqlcgen.Analyzer) (model.Analyzer, error) {
	installedPower, err := numericToDecimalPtr(row.InstalledPowerKw)
	if err != nil {
		return model.Analyzer{}, err
	}
	meterMultiplier, err := numericToDecimal(row.MeterMultiplier)
	if err != nil {
		return model.Analyzer{}, err
	}
	latitude, err := numericToDecimalPtr(row.Latitude)
	if err != nil {
		return model.Analyzer{}, err
	}
	longitude, err := numericToDecimalPtr(row.Longitude)
	if err != nil {
		return model.Analyzer{}, err
	}
	return model.Analyzer{
		ID:                 row.ID,
		CompanyID:          row.CompanyID,
		BuildingID:         row.BuildingID,
		Provider:           model.IntegrationProvider(row.Provider),
		ProviderSubtype:    row.ProviderSubtype,
		InstallationNumber: row.InstallationNumber,
		CustomerName:       row.CustomerName,
		Address:            row.Address,
		Province:           row.Province,
		District:           row.District,
		Neighbourhood:      row.Neighbourhood,
		Street:             row.Street,
		TariffType:         row.TariffType,
		TariffKind:         row.TariffKind,
		InstallationKind:   row.InstallationKind,
		InstalledPowerKw:   installedPower,
		MeterNumber:        row.MeterNumber,
		MeterModel:         row.MeterModel,
		MeterMultiplier:    meterMultiplier,
		CounterpartyNo:     row.CounterpartyNo,
		MeteringPointName:  row.MeteringPointName,
		Latitude:           latitude,
		Longitude:          longitude,
		EtsoCode:           row.EtsoCode,
		DefinitionType:     row.DefinitionType,
		LastReadingAt:      tsPtr(row.LastReadingAt),
		IsActive:           row.IsActive,
		CreatedAt:          row.CreatedAt.Time,
		UpdatedAt:          row.UpdatedAt.Time,
		DeletedAt:          tsPtr(row.DeletedAt),
	}, nil
}

// Get implements store.AnalyzerRepository.Get.
func (r *AnalyzerRepository) Get(ctx context.Context, s store.Scope, id uuid.UUID) (model.Analyzer, error) {
	if !s.Valid() {
		return model.Analyzer{}, store.ErrInvalidScope
	}
	ids, all := s.BuildingFilter()
	row, err := r.q.AnalyzerGet(ctx, sqlcgen.AnalyzerGetParams{
		ID: id, CompanyID: s.CompanyID, AllBuildings: all, BuildingIds: ids,
	})
	if err != nil {
		return model.Analyzer{}, pgerr.Translate(r.pool, "get analyzer", err)
	}
	return analyzerFromRow(row)
}

// GetByInstallation implements store.AnalyzerRepository.GetByInstallation.
func (r *AnalyzerRepository) GetByInstallation(ctx context.Context, s store.Scope, provider model.IntegrationProvider, subtype, installationNumber string) (model.Analyzer, error) {
	if !s.Valid() {
		return model.Analyzer{}, store.ErrInvalidScope
	}
	ids, all := s.BuildingFilter()
	row, err := r.q.AnalyzerGetByInstallation(ctx, sqlcgen.AnalyzerGetByInstallationParams{
		Provider:           sqlcgen.IntegrationProvider(provider),
		ProviderSubtype:    subtype,
		InstallationNumber: installationNumber,
		CompanyID:          s.CompanyID,
		AllBuildings:       all,
		BuildingIds:        ids,
	})
	if err != nil {
		return model.Analyzer{}, pgerr.Translate(r.pool, "get analyzer by installation", err)
	}
	return analyzerFromRow(row)
}

// List implements store.AnalyzerRepository.List.
func (r *AnalyzerRepository) List(ctx context.Context, s store.Scope, f store.AnalyzerFilter) ([]model.Analyzer, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	ids, all := s.BuildingFilter()
	providers := make([]string, len(f.Providers))
	for i, p := range f.Providers {
		providers[i] = string(p)
	}
	rows, err := r.q.AnalyzerList(ctx, sqlcgen.AnalyzerListParams{
		CompanyID:        s.CompanyID,
		AllBuildings:     all,
		BuildingIds:      ids,
		FilterIds:        uuidsOrEmpty(f.IDs),
		FilterBuildingID: f.BuildingID,
		Unassigned:       f.Unassigned,
		Providers:        providers,
		IsActive:         f.IsActive,
		IncludeDeleted:   f.IncludeDeleted,
		PageOffset:       f.Page.Offset,
		PageLimit:        pageLimit(f.Page, analyzerPageDefaultLimit, analyzerPageMaxLimit),
	})
	if err != nil {
		return nil, pgerr.Translate(r.pool, "list analyzers", err)
	}
	out := make([]model.Analyzer, 0, len(rows))
	for _, row := range rows {
		a, err := analyzerFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}

// Create implements store.AnalyzerRepository.Create.
//
// Wave F integration: a Go-level buildingVisible pre-check used to run here,
// non-transactionally, before the insert. It is gone — AnalyzerCreate's own
// WHERE clause (queries/analyzers.sql) embeds the identical check (the
// building_id FK against company_id/building_ids/deleted_at, plus the
// nil-building-id-only-under-AllBuildings rule) inside the write statement
// itself, so a refused write inserts zero rows and the :one scan's
// pgx.ErrNoRows becomes store.ErrNotFound through pgerr.Translate — the same
// error the old Go-level check returned. TestAnalyzerRepositoryCreateRefusesA-
// ForeignBuilding now exercises that embedded SQL check directly.
func (r *AnalyzerRepository) Create(ctx context.Context, s store.Scope, a model.Analyzer) (model.Analyzer, error) {
	if !s.Valid() {
		return model.Analyzer{}, store.ErrInvalidScope
	}
	if a.CompanyID != s.CompanyID {
		return model.Analyzer{}, store.ErrNotFound
	}
	id := a.ID
	if id == uuid.Nil {
		id = uuid.New()
	}
	buildingIDs, allBuildings := s.BuildingFilter()
	row, err := r.q.AnalyzerCreate(ctx, sqlcgen.AnalyzerCreateParams{
		ID:                 id,
		CompanyID:          s.CompanyID,
		BuildingID:         a.BuildingID,
		AllBuildings:       allBuildings,
		BuildingIds:        buildingIDs,
		Provider:           sqlcgen.IntegrationProvider(a.Provider),
		ProviderSubtype:    a.ProviderSubtype,
		InstallationNumber: a.InstallationNumber,
		CustomerName:       a.CustomerName,
		Address:            a.Address,
		Province:           a.Province,
		District:           a.District,
		Neighbourhood:      a.Neighbourhood,
		Street:             a.Street,
		TariffType:         a.TariffType,
		TariffKind:         a.TariffKind,
		InstallationKind:   a.InstallationKind,
		InstalledPowerKw:   decimalPtrToNumeric(a.InstalledPowerKw),
		MeterNumber:        a.MeterNumber,
		MeterModel:         a.MeterModel,
		MeterMultiplier:    decimalToNumeric(a.MeterMultiplier),
		CounterpartyNo:     a.CounterpartyNo,
		MeteringPointName:  a.MeteringPointName,
		Latitude:           decimalPtrToNumeric(a.Latitude),
		Longitude:          decimalPtrToNumeric(a.Longitude),
		EtsoCode:           a.EtsoCode,
		DefinitionType:     a.DefinitionType,
		IsActive:           a.IsActive,
		At:                 tsOrNow(a.CreatedAt),
	})
	if err != nil {
		return model.Analyzer{}, pgerr.Translate(r.pool, "create analyzer", err)
	}
	return analyzerFromRow(row)
}

// Update implements store.AnalyzerRepository.Update.
//
// Wave F integration: the same Go-level buildingVisible pre-check removed
// from Create is gone here too — AnalyzerUpdate's WHERE clause embeds two
// building_id checks in one statement (see queries/analyzers.sql): the row's
// CURRENT building_id gates ordinary row visibility, and an embedded
// exists(...) clause validates the NEW value being written the same way
// AnalyzerCreate's insert does. TestAnalyzerRepositoryUpdateRefusesAForeign-
// Building now exercises that embedded SQL check directly.
func (r *AnalyzerRepository) Update(ctx context.Context, s store.Scope, a model.Analyzer) (model.Analyzer, error) {
	if !s.Valid() {
		return model.Analyzer{}, store.ErrInvalidScope
	}
	if a.CompanyID != s.CompanyID {
		return model.Analyzer{}, store.ErrNotFound
	}
	ids, all := s.BuildingFilter()
	row, err := r.q.AnalyzerUpdate(ctx, sqlcgen.AnalyzerUpdateParams{
		BuildingID:        a.BuildingID,
		CustomerName:      a.CustomerName,
		Address:           a.Address,
		Province:          a.Province,
		District:          a.District,
		Neighbourhood:     a.Neighbourhood,
		Street:            a.Street,
		TariffType:        a.TariffType,
		TariffKind:        a.TariffKind,
		InstallationKind:  a.InstallationKind,
		InstalledPowerKw:  decimalPtrToNumeric(a.InstalledPowerKw),
		MeterNumber:       a.MeterNumber,
		MeterModel:        a.MeterModel,
		MeterMultiplier:   decimalToNumeric(a.MeterMultiplier),
		CounterpartyNo:    a.CounterpartyNo,
		MeteringPointName: a.MeteringPointName,
		Latitude:          decimalPtrToNumeric(a.Latitude),
		Longitude:         decimalPtrToNumeric(a.Longitude),
		EtsoCode:          a.EtsoCode,
		DefinitionType:    a.DefinitionType,
		IsActive:          a.IsActive,
		UpdatedAt:         ts(a.UpdatedAt),
		ID:                a.ID,
		CompanyID:         s.CompanyID,
		AllBuildings:      all,
		BuildingIds:       ids,
	})
	if err != nil {
		return model.Analyzer{}, pgerr.Translate(r.pool, "update analyzer", err)
	}
	return analyzerFromRow(row)
}

// SoftDelete implements store.AnalyzerRepository.SoftDelete.
func (r *AnalyzerRepository) SoftDelete(ctx context.Context, s store.Scope, id uuid.UUID, at time.Time) error {
	if !s.Valid() {
		return store.ErrInvalidScope
	}
	ids, all := s.BuildingFilter()
	n, err := r.q.AnalyzerSoftDelete(ctx, sqlcgen.AnalyzerSoftDeleteParams{
		DeletedAt: ts(at), ID: id, CompanyID: s.CompanyID, AllBuildings: all, BuildingIds: ids,
	})
	if err != nil {
		return pgerr.Translate(r.pool, "soft delete analyzer", err)
	}
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

// TouchLastReading advances last_reading_at, never retreats it: GREATEST
// against the stored value (or -infinity if unset) makes an out-of-order
// backfill unable to make an analyzer look staler than it is.
func (r *AnalyzerRepository) TouchLastReading(ctx context.Context, s store.Scope, id uuid.UUID, at time.Time) error {
	if !s.Valid() {
		return store.ErrInvalidScope
	}
	ids, all := s.BuildingFilter()
	n, err := r.q.AnalyzerTouchLastReading(ctx, sqlcgen.AnalyzerTouchLastReadingParams{
		At: ts(at), ID: id, CompanyID: s.CompanyID, AllBuildings: all, BuildingIds: ids,
	})
	if err != nil {
		return pgerr.Translate(r.pool, "touch analyzer last reading", err)
	}
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}
