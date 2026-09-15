package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/internal/pgerr"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/sqlcgen"
)

// ISO50001Repository implements store.ISO50001Repository.
type ISO50001Repository struct {
	q    *sqlcgen.Queries
	pool *pgxpool.Pool
}

// NewISO50001Repository builds an ISO50001Repository on pool.
func NewISO50001Repository(pool *pgxpool.Pool) *ISO50001Repository {
	return &ISO50001Repository{q: sqlcgen.New(pool), pool: pool}
}

var _ store.ISO50001Repository = (*ISO50001Repository)(nil)

func iso50001ProjectFromRow(row sqlcgen.Iso50001Project) model.ISO50001Project {
	return model.ISO50001Project{
		ID: row.ID, CompanyID: row.CompanyID, BuildingID: row.BuildingID,
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
}

// Project returns a building's project, or ErrNotFound.
func (r *ISO50001Repository) Project(ctx context.Context, s store.Scope, buildingID uuid.UUID) (model.ISO50001Project, error) {
	if !s.Valid() {
		return model.ISO50001Project{}, store.ErrInvalidScope
	}
	buildingIDs, allBuildings := s.BuildingFilter()
	row, err := r.q.ISO50001GetProject(ctx, sqlcgen.ISO50001GetProjectParams{
		BuildingID: buildingID, CompanyID: s.CompanyID, AllBuildings: allBuildings, BuildingIds: buildingIDs,
	})
	if err != nil {
		return model.ISO50001Project{}, pgerr.Translate(r.pool, "get iso50001 project", err)
	}
	return iso50001ProjectFromRow(row), nil
}

// EnsureProject returns the building's project, creating it if it does not
// exist.
func (r *ISO50001Repository) EnsureProject(ctx context.Context, s store.Scope, buildingID uuid.UUID) (model.ISO50001Project, error) {
	if !s.Valid() {
		return model.ISO50001Project{}, store.ErrInvalidScope
	}
	buildingIDs, allBuildings := s.BuildingFilter()
	row, err := r.q.ISO50001EnsureProject(ctx, sqlcgen.ISO50001EnsureProjectParams{
		CompanyID: s.CompanyID, BuildingID: buildingID, AllBuildings: allBuildings, BuildingIds: buildingIDs,
	})
	if err != nil {
		return model.ISO50001Project{}, pgerr.Translate(r.pool, "ensure iso50001 project", err)
	}
	return iso50001ProjectFromRow(row), nil
}

// --- iso50001_clause_dates ------------------------------------------------

// ClauseDates — Isolation: iso50001_clause_dates has no company_id, so
// visibility is proven by ISO50001ProjectVisible (its own query carries the
// scope predicate) before listing: a project id that does not exist, or is
// not visible, returns ErrNotFound instead of an ambiguous empty list.
func (r *ISO50001Repository) ClauseDates(ctx context.Context, s store.Scope, projectID uuid.UUID) ([]model.ISO50001ClauseDate, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	buildingIDs, allBuildings := s.BuildingFilter()
	if _, err := r.q.ISO50001ProjectVisible(ctx, sqlcgen.ISO50001ProjectVisibleParams{
		ID: projectID, CompanyID: s.CompanyID, AllBuildings: allBuildings, BuildingIds: buildingIDs,
	}); err != nil {
		return nil, pgerr.Translate(r.pool, "check iso50001 project visibility", err)
	}
	rows, err := r.q.ISO50001ListClauseDates(ctx, sqlcgen.ISO50001ListClauseDatesParams{
		ProjectID: projectID, CompanyID: s.CompanyID, AllBuildings: allBuildings, BuildingIds: buildingIDs,
	})
	if err != nil {
		return nil, pgerr.Translate(r.pool, "list iso50001 clause dates", err)
	}
	out := make([]model.ISO50001ClauseDate, 0, len(rows))
	for _, row := range rows {
		out = append(out, model.ISO50001ClauseDate{
			ProjectID: row.ProjectID, ClauseID: row.ClauseID,
			StartDate: iso50001OptionalDate(row.StartDate), EndDate: iso50001OptionalDate(row.EndDate),
		})
	}
	return out, nil
}

type iso50001ClauseDateRecord struct {
	ClauseID  string  `json:"clause_id"`
	StartDate *string `json:"start_date"`
	EndDate   *string `json:"end_date"`
}

func iso50001ClauseDateRecordsJSON(dates []model.ISO50001ClauseDate) ([]byte, error) {
	records := make([]iso50001ClauseDateRecord, len(dates))
	for i, d := range dates {
		records[i] = iso50001ClauseDateRecord{ClauseID: d.ClauseID, StartDate: iso50001DateString(d.StartDate), EndDate: iso50001DateString(d.EndDate)}
	}
	return json.Marshal(records)
}

// iso50001DateString formats a *time.Time as the "YYYY-MM-DD" text Postgres' date
// input accepts, or nil for a nil pointer (jsonb_to_recordset then reads it
// as SQL NULL).
func iso50001DateString(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format("2006-01-02")
	return &s
}

// ReplaceClauseDates — Isolation: the parent project is locked FOR SHARE
// inside this transaction (ISO50001ProjectVisibleForShare) before the
// delete+insert: a project not visible to the Scope returns ErrNotFound and
// nothing is replaced.
func (r *ISO50001Repository) ReplaceClauseDates(ctx context.Context, s store.Scope, projectID uuid.UUID, dates []model.ISO50001ClauseDate) error {
	if !s.Valid() {
		return store.ErrInvalidScope
	}
	buildingIDs, allBuildings := s.BuildingFilter()
	payload, err := iso50001ClauseDateRecordsJSON(dates)
	if err != nil {
		return err
	}
	return r.inTx(ctx, func(qtx *sqlcgen.Queries) error {
		if _, err := qtx.ISO50001ProjectVisibleForShare(ctx, sqlcgen.ISO50001ProjectVisibleForShareParams{
			ID: projectID, CompanyID: s.CompanyID, AllBuildings: allBuildings, BuildingIds: buildingIDs,
		}); err != nil {
			return pgerr.Translate(r.pool, "lock iso50001 project", err)
		}
		if err := qtx.ISO50001DeleteClauseDates(ctx, projectID); err != nil {
			return pgerr.Translate(r.pool, "delete iso50001 clause dates", err)
		}
		if len(dates) == 0 {
			return nil
		}
		if err := qtx.ISO50001InsertClauseDates(ctx, sqlcgen.ISO50001InsertClauseDatesParams{
			ProjectID: projectID, Dates: payload,
		}); err != nil {
			return pgerr.Translate(r.pool, "insert iso50001 clause dates", err)
		}
		return nil
	})
}

// --- iso50001_notes --------------------------------------------------------

func iso50001NoteFromRow(row sqlcgen.Iso50001Note) model.ISO50001Note {
	return model.ISO50001Note{
		ID: row.ID, ProjectID: row.ProjectID, ClauseID: row.ClauseID,
		Title: row.Title, Body: row.Body, CreatedBy: row.CreatedBy,
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
}

// Notes — Isolation: iso50001_notes has no company_id, so visibility is
// proven by ISO50001ProjectVisible before listing.
func (r *ISO50001Repository) Notes(ctx context.Context, s store.Scope, projectID uuid.UUID, clauseID *string) ([]model.ISO50001Note, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	buildingIDs, allBuildings := s.BuildingFilter()
	if _, err := r.q.ISO50001ProjectVisible(ctx, sqlcgen.ISO50001ProjectVisibleParams{
		ID: projectID, CompanyID: s.CompanyID, AllBuildings: allBuildings, BuildingIds: buildingIDs,
	}); err != nil {
		return nil, pgerr.Translate(r.pool, "check iso50001 project visibility", err)
	}
	rows, err := r.q.ISO50001ListNotes(ctx, sqlcgen.ISO50001ListNotesParams{
		ProjectID: projectID, CompanyID: s.CompanyID, AllBuildings: allBuildings, BuildingIds: buildingIDs, ClauseID: clauseID,
	})
	if err != nil {
		return nil, pgerr.Translate(r.pool, "list iso50001 notes", err)
	}
	out := make([]model.ISO50001Note, 0, len(rows))
	for _, row := range rows {
		out = append(out, iso50001NoteFromRow(row))
	}
	return out, nil
}

// CreateNote — Isolation: join through iso50001_projects on n.ProjectID, and
// created_by (when set) must be a user of s.CompanyID. Both checks are
// embedded in the insert's own `where exists (...)` clauses (iso50001.sql).
func (r *ISO50001Repository) CreateNote(ctx context.Context, s store.Scope, n model.ISO50001Note) (model.ISO50001Note, error) {
	if !s.Valid() {
		return model.ISO50001Note{}, store.ErrInvalidScope
	}
	buildingIDs, allBuildings := s.BuildingFilter()
	row, err := r.q.ISO50001CreateNote(ctx, sqlcgen.ISO50001CreateNoteParams{
		ID: n.ID, ProjectID: n.ProjectID, ClauseID: n.ClauseID, Title: n.Title, Body: n.Body,
		CreatedBy: n.CreatedBy, CompanyID: s.CompanyID, AllBuildings: allBuildings, BuildingIds: buildingIDs,
	})
	if err != nil {
		return model.ISO50001Note{}, pgerr.Translate(r.pool, "create iso50001 note", err)
	}
	return iso50001NoteFromRow(row), nil
}

// UpdateNote — Isolation: join iso50001_notes through iso50001_projects by
// n.ID in the same UPDATE statement. project_id is never updated: a note
// stays under the project it was created in.
func (r *ISO50001Repository) UpdateNote(ctx context.Context, s store.Scope, n model.ISO50001Note) (model.ISO50001Note, error) {
	if !s.Valid() {
		return model.ISO50001Note{}, store.ErrInvalidScope
	}
	buildingIDs, allBuildings := s.BuildingFilter()
	row, err := r.q.ISO50001UpdateNote(ctx, sqlcgen.ISO50001UpdateNoteParams{
		ClauseID: n.ClauseID, Title: n.Title, Body: n.Body,
		ID: n.ID, CompanyID: s.CompanyID, AllBuildings: allBuildings, BuildingIds: buildingIDs,
	})
	if err != nil {
		return model.ISO50001Note{}, pgerr.Translate(r.pool, "update iso50001 note", err)
	}
	return iso50001NoteFromRow(row), nil
}

// DeleteNote — Isolation: join iso50001_notes through iso50001_projects by
// id, in the same DELETE statement (`using`).
func (r *ISO50001Repository) DeleteNote(ctx context.Context, s store.Scope, id uuid.UUID) error {
	if !s.Valid() {
		return store.ErrInvalidScope
	}
	buildingIDs, allBuildings := s.BuildingFilter()
	n, err := r.q.ISO50001DeleteNote(ctx, sqlcgen.ISO50001DeleteNoteParams{
		ID: id, CompanyID: s.CompanyID, AllBuildings: allBuildings, BuildingIds: buildingIDs,
	})
	if err != nil {
		return pgerr.Translate(r.pool, "delete iso50001 note", err)
	}
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

func iso50001OptionalDate(d pgtype.Date) *time.Time {
	if !d.Valid {
		return nil
	}
	t := d.Time
	return &t
}

// inTx runs fn inside one transaction on r.pool, committing on a nil error
// and rolling back otherwise. It is a METHOD, not a package-level function,
// specifically so its name is namespaced to *ISO50001Repository and cannot
// collide with a sibling task's identically-named helper on its own
// repository type in the same package (see CarbonRepository.inTx, which is
// the same pattern under a different receiver).
func (r *ISO50001Repository) inTx(ctx context.Context, fn func(qtx *sqlcgen.Queries) error) error {
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
