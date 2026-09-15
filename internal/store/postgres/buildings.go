package postgres

// This file implements store.BuildingRepository — the "Buildings, analyzers
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
	buildingPageDefaultLimit = 50
	buildingPageMaxLimit     = 500
)

// BuildingRepository is the postgres store.BuildingRepository.
type BuildingRepository struct {
	q    *sqlcgen.Queries
	pool *pgxpool.Pool
}

var _ store.BuildingRepository = (*BuildingRepository)(nil)

// NewBuildingRepository builds a BuildingRepository over pool.
func NewBuildingRepository(pool *pgxpool.Pool) *BuildingRepository {
	return &BuildingRepository{q: sqlcgen.New(pool), pool: pool}
}

func buildingFromRow(row sqlcgen.Building) (model.Building, error) {
	latitude, err := numericToDecimalPtr(row.Latitude)
	if err != nil {
		return model.Building{}, err
	}
	longitude, err := numericToDecimalPtr(row.Longitude)
	if err != nil {
		return model.Building{}, err
	}
	totalArea, err := numericToDecimalPtr(row.TotalAreaM2)
	if err != nil {
		return model.Building{}, err
	}
	return model.Building{
		ID:                row.ID,
		CompanyID:         row.CompanyID,
		Name:              row.Name,
		Address:           row.Address,
		Latitude:          latitude,
		Longitude:         longitude,
		Floors:            row.Floors,
		PersonnelCount:    row.PersonnelCount,
		TotalAreaM2:       totalArea,
		Sector:            row.Sector,
		ResponsibleUserID: row.ResponsibleUserID,
		BillCutoffDay:     row.BillCutoffDay,
		CreatedAt:         row.CreatedAt.Time,
		UpdatedAt:         row.UpdatedAt.Time,
		DeletedAt:         tsPtr(row.DeletedAt),
	}, nil
}

// Get implements store.BuildingRepository.Get.
func (r *BuildingRepository) Get(ctx context.Context, s store.Scope, id uuid.UUID) (model.Building, error) {
	if !s.Valid() {
		return model.Building{}, store.ErrInvalidScope
	}
	ids, all := s.BuildingFilter()
	row, err := r.q.BuildingGetScoped(ctx, sqlcgen.BuildingGetScopedParams{
		ID: id, CompanyID: s.CompanyID, AllBuildings: all, BuildingIds: ids,
	})
	if err != nil {
		return model.Building{}, pgerr.Translate(r.pool, "get building", err)
	}
	return buildingFromRow(row)
}

// List implements store.BuildingRepository.List.
func (r *BuildingRepository) List(ctx context.Context, s store.Scope, f store.BuildingFilter) ([]model.Building, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	ids, all := s.BuildingFilter()
	rows, err := r.q.BuildingList(ctx, sqlcgen.BuildingListParams{
		CompanyID:         s.CompanyID,
		AllBuildings:      all,
		BuildingIds:       ids,
		FilterIds:         uuidsOrEmpty(f.IDs),
		IncludeDeleted:    f.IncludeDeleted,
		NameContains:      f.NameContains,
		Sector:            f.Sector,
		ResponsibleUserID: f.ResponsibleUserID,
		PageOffset:        f.Page.Offset,
		PageLimit:         pageLimit(f.Page, buildingPageDefaultLimit, buildingPageMaxLimit),
	})
	if err != nil {
		return nil, pgerr.Translate(r.pool, "list buildings", err)
	}
	out := make([]model.Building, 0, len(rows))
	for _, row := range rows {
		b, err := buildingFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, nil
}

// responsibleUserVisible reports whether userID is nil or belongs to
// s.CompanyID, the shared foreign-key check Create and Update both need for
// ResponsibleUserID.
func (r *BuildingRepository) responsibleUserVisible(ctx context.Context, s store.Scope, userID *uuid.UUID) (bool, error) {
	if userID == nil {
		return true, nil
	}
	return r.q.BuildingResponsibleUserVisible(ctx, sqlcgen.BuildingResponsibleUserVisibleParams{
		ResponsibleUserID: *userID, CompanyID: s.CompanyID,
	})
}

// Create implements store.BuildingRepository.Create.
//
// CONTROLLER RULING (fix round 1): creating a building requires an
// AllBuildings Scope. A narrow Scope names a fixed, pre-granted set of
// existing buildings (Scope.BuildingIDs) — it has no way to grant itself a
// NEW building id, so a narrow principal creating one would either name an
// id nobody granted it, or create a building that its own Scope can never
// then see (BuildingGetScoped requires id = any(building_ids), and building
// creation can't retroactively add to that list).
func (r *BuildingRepository) Create(ctx context.Context, s store.Scope, b model.Building) (model.Building, error) {
	if !s.Valid() {
		return model.Building{}, store.ErrInvalidScope
	}
	if b.CompanyID != s.CompanyID {
		return model.Building{}, store.ErrNotFound
	}
	if _, all := s.BuildingFilter(); !all {
		return model.Building{}, store.ErrNotFound
	}
	visible, err := r.responsibleUserVisible(ctx, s, b.ResponsibleUserID)
	if err != nil {
		return model.Building{}, pgerr.Translate(r.pool, "check responsible user visibility", err)
	}
	if !visible {
		return model.Building{}, store.ErrNotFound
	}
	id := b.ID
	if id == uuid.Nil {
		id = uuid.New()
	}
	row, err := r.q.BuildingCreate(ctx, sqlcgen.BuildingCreateParams{
		ID:                id,
		CompanyID:         s.CompanyID,
		Name:              b.Name,
		Address:           b.Address,
		Latitude:          decimalPtrToNumeric(b.Latitude),
		Longitude:         decimalPtrToNumeric(b.Longitude),
		Floors:            b.Floors,
		PersonnelCount:    b.PersonnelCount,
		TotalAreaM2:       decimalPtrToNumeric(b.TotalAreaM2),
		Sector:            b.Sector,
		ResponsibleUserID: b.ResponsibleUserID,
		BillCutoffDay:     b.BillCutoffDay,
		At:                tsOrNow(b.CreatedAt),
	})
	if err != nil {
		return model.Building{}, pgerr.Translate(r.pool, "create building", err)
	}
	return buildingFromRow(row)
}

// Update implements store.BuildingRepository.Update.
func (r *BuildingRepository) Update(ctx context.Context, s store.Scope, b model.Building) (model.Building, error) {
	if !s.Valid() {
		return model.Building{}, store.ErrInvalidScope
	}
	if b.CompanyID != s.CompanyID {
		return model.Building{}, store.ErrNotFound
	}
	visible, err := r.responsibleUserVisible(ctx, s, b.ResponsibleUserID)
	if err != nil {
		return model.Building{}, pgerr.Translate(r.pool, "check responsible user visibility", err)
	}
	if !visible {
		return model.Building{}, store.ErrNotFound
	}
	ids, all := s.BuildingFilter()
	row, err := r.q.BuildingUpdate(ctx, sqlcgen.BuildingUpdateParams{
		Name:              b.Name,
		Address:           b.Address,
		Latitude:          decimalPtrToNumeric(b.Latitude),
		Longitude:         decimalPtrToNumeric(b.Longitude),
		Floors:            b.Floors,
		PersonnelCount:    b.PersonnelCount,
		TotalAreaM2:       decimalPtrToNumeric(b.TotalAreaM2),
		Sector:            b.Sector,
		ResponsibleUserID: b.ResponsibleUserID,
		BillCutoffDay:     b.BillCutoffDay,
		UpdatedAt:         ts(b.UpdatedAt),
		ID:                b.ID,
		CompanyID:         s.CompanyID,
		AllBuildings:      all,
		BuildingIds:       ids,
	})
	if err != nil {
		return model.Building{}, pgerr.Translate(r.pool, "update building", err)
	}
	return buildingFromRow(row)
}

// SoftDelete implements store.BuildingRepository.SoftDelete.
func (r *BuildingRepository) SoftDelete(ctx context.Context, s store.Scope, id uuid.UUID, at time.Time) error {
	if !s.Valid() {
		return store.ErrInvalidScope
	}
	ids, all := s.BuildingFilter()
	n, err := r.q.BuildingSoftDelete(ctx, sqlcgen.BuildingSoftDeleteParams{
		DeletedAt: ts(at), ID: id, CompanyID: s.CompanyID, AllBuildings: all, BuildingIds: ids,
	})
	if err != nil {
		return pgerr.Translate(r.pool, "soft delete building", err)
	}
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

// Contacts — Isolation: building_contacts has no company_id and is reached
// ONLY by joining to buildings IN THE SAME QUERY: see BuildingContactsList in
// queries/buildings.sql (a LEFT JOIN, not a Go-level pre-check run
// separately on the pool before this). Another tenant's buildingID
// contributes zero rows and is reported as ErrNotFound; a visible building
// with no contacts yet returns an empty slice.
func (r *BuildingRepository) Contacts(ctx context.Context, s store.Scope, buildingID uuid.UUID) ([]model.BuildingContact, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	ids, all := s.BuildingFilter()
	rows, err := r.q.BuildingContactsList(ctx, sqlcgen.BuildingContactsListParams{
		BuildingID: buildingID, CompanyID: s.CompanyID, AllBuildings: all, BuildingIds: ids,
	})
	if err != nil {
		return nil, pgerr.Translate(r.pool, "list building contacts", err)
	}
	if len(rows) == 0 {
		return nil, store.ErrNotFound
	}
	out := make([]model.BuildingContact, 0, len(rows))
	for _, row := range rows {
		if row.ID == nil {
			continue // visible building, zero contacts: the LEFT JOIN sentinel row
		}
		out = append(out, model.BuildingContact{
			ID: *row.ID, BuildingID: *row.BuildingID, Name: row.Name, Phone: row.Phone, SortOrder: *row.SortOrder,
		})
	}
	return out, nil
}

// ReplaceContacts swaps the whole contact list in one transaction. Isolation:
// BuildingGetScopedForShare locks and verifies the parent INSIDE this
// transaction (zero rows -> ErrNotFound, nothing replaced);
// BuildingContactsDelete and BuildingContactInsert are each independently
// scoped via a join/exists to buildings in their own statement, so a
// cross-tenant buildingID is refused even without the lock above (see the
// fix-round-1 mutation proofs in the task report). The contacts' own
// BuildingID fields are ignored in favour of buildingID.
func (r *BuildingRepository) ReplaceContacts(ctx context.Context, s store.Scope, buildingID uuid.UUID, contacts []model.BuildingContact) ([]model.BuildingContact, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	ids, all := s.BuildingFilter()
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, pgerr.Translate(r.pool, "begin replace building contacts", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := r.q.WithTx(tx)
	if _, err := qtx.BuildingGetScopedForShare(ctx, sqlcgen.BuildingGetScopedForShareParams{
		ID: buildingID, CompanyID: s.CompanyID, AllBuildings: all, BuildingIds: ids,
	}); err != nil {
		return nil, pgerr.Translate(r.pool, "lock building for replace contacts", err)
	}
	if err := qtx.BuildingContactsDelete(ctx, sqlcgen.BuildingContactsDeleteParams{
		BuildingID: buildingID, CompanyID: s.CompanyID, AllBuildings: all, BuildingIds: ids,
	}); err != nil {
		return nil, pgerr.Translate(r.pool, "delete building contacts", err)
	}
	out := make([]model.BuildingContact, 0, len(contacts))
	for _, c := range contacts {
		row, err := qtx.BuildingContactInsert(ctx, sqlcgen.BuildingContactInsertParams{
			ID: uuid.New(), BuildingID: buildingID, Name: c.Name, Phone: c.Phone, SortOrder: c.SortOrder,
			CompanyID: s.CompanyID, AllBuildings: all, BuildingIds: ids,
		})
		if err != nil {
			return nil, pgerr.Translate(r.pool, "insert building contact", err)
		}
		out = append(out, model.BuildingContact{
			ID: row.ID, BuildingID: row.BuildingID, Name: row.Name, Phone: row.Phone, SortOrder: row.SortOrder,
		})
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, pgerr.Translate(r.pool, "commit replace building contacts", err)
	}
	return out, nil
}
