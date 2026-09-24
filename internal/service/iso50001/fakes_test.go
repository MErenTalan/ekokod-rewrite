package iso50001_test

import (
	"context"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// fakeISO keeps one project per building with the store's scope rules.
type fakeISO struct {
	store.ISO50001Repository
	owner    map[uuid.UUID]uuid.UUID // building → company
	projects map[uuid.UUID]model.ISO50001Project
	dates    map[uuid.UUID][]model.ISO50001ClauseDate
	notes    map[uuid.UUID]model.ISO50001Note
	seq      int
}

func newFakeISO() *fakeISO {
	return &fakeISO{owner: map[uuid.UUID]uuid.UUID{}, projects: map[uuid.UUID]model.ISO50001Project{},
		dates: map[uuid.UUID][]model.ISO50001ClauseDate{}, notes: map[uuid.UUID]model.ISO50001Note{}}
}

func (f *fakeISO) visible(sc store.Scope, building uuid.UUID) bool {
	return sc.Valid() && f.owner[building] == sc.CompanyID && sc.AllowsBuilding(building)
}

func (f *fakeISO) projectBuilding(id uuid.UUID) (uuid.UUID, bool) {
	for b, p := range f.projects {
		if p.ID == id {
			return b, true
		}
	}
	return uuid.Nil, false
}

func (f *fakeISO) Project(_ context.Context, sc store.Scope, b uuid.UUID) (model.ISO50001Project, error) {
	p, ok := f.projects[b]
	if !ok || !f.visible(sc, b) {
		return model.ISO50001Project{}, store.ErrNotFound
	}
	return p, nil
}

func (f *fakeISO) EnsureProject(_ context.Context, sc store.Scope, b uuid.UUID) (model.ISO50001Project, error) {
	if !f.visible(sc, b) {
		return model.ISO50001Project{}, store.ErrNotFound
	}
	if p, ok := f.projects[b]; ok {
		return p, nil
	}
	p := model.ISO50001Project{ID: uuid.New(), CompanyID: sc.CompanyID, BuildingID: b}
	f.projects[b] = p
	return p, nil
}

func (f *fakeISO) ClauseDates(_ context.Context, sc store.Scope, id uuid.UUID) ([]model.ISO50001ClauseDate, error) {
	b, ok := f.projectBuilding(id)
	if !ok || !f.visible(sc, b) {
		return nil, store.ErrNotFound
	}
	return f.dates[id], nil
}

func (f *fakeISO) ReplaceClauseDates(_ context.Context, sc store.Scope, id uuid.UUID, d []model.ISO50001ClauseDate) error {
	b, ok := f.projectBuilding(id)
	if !ok || !f.visible(sc, b) {
		return store.ErrNotFound
	}
	f.dates[id] = d
	return nil
}

func (f *fakeISO) Notes(_ context.Context, sc store.Scope, id uuid.UUID, clause *string) ([]model.ISO50001Note, error) {
	b, ok := f.projectBuilding(id)
	if !ok || !f.visible(sc, b) {
		return nil, store.ErrNotFound
	}
	var out []model.ISO50001Note
	for _, n := range f.notes {
		if n.ProjectID == id && (clause == nil || n.ClauseID == *clause) {
			out = append(out, n)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (f *fakeISO) CreateNote(_ context.Context, sc store.Scope, n model.ISO50001Note) (model.ISO50001Note, error) {
	b, ok := f.projectBuilding(n.ProjectID)
	if !ok || !f.visible(sc, b) {
		return model.ISO50001Note{}, store.ErrNotFound
	}
	f.seq++
	n.ID, n.CreatedAt = uuid.New(), time.Date(2026, 6, 1, 9, f.seq, 0, 0, time.UTC)
	n.UpdatedAt = n.CreatedAt
	f.notes[n.ID] = n
	return n, nil
}

func (f *fakeISO) UpdateNote(_ context.Context, sc store.Scope, n model.ISO50001Note) (model.ISO50001Note, error) {
	old, ok := f.notes[n.ID]
	if !ok {
		return model.ISO50001Note{}, store.ErrNotFound
	}
	b, _ := f.projectBuilding(old.ProjectID)
	if !f.visible(sc, b) {
		return model.ISO50001Note{}, store.ErrNotFound
	}
	old.Title, old.Body, old.ClauseID = n.Title, n.Body, n.ClauseID
	f.notes[n.ID] = old
	return old, nil
}

func (f *fakeISO) DeleteNote(_ context.Context, sc store.Scope, id uuid.UUID) error {
	old, ok := f.notes[id]
	if !ok {
		return store.ErrNotFound
	}
	b, _ := f.projectBuilding(old.ProjectID)
	if !f.visible(sc, b) {
		return store.ErrNotFound
	}
	delete(f.notes, id)
	return nil
}

// fakeFiles scopes by company only, like the real FileRepository.
type fakeFiles struct {
	store.FileRepository
	rows map[uuid.UUID]model.StoredFile
}

func (f *fakeFiles) Get(_ context.Context, sc store.Scope, id uuid.UUID) (model.StoredFile, error) {
	r, ok := f.rows[id]
	if !ok || r.CompanyID != sc.CompanyID || r.DeletedAt != nil {
		return model.StoredFile{}, store.ErrNotFound
	}
	return r, nil
}

func (f *fakeFiles) List(_ context.Context, sc store.Scope, fl store.FileFilter) ([]model.StoredFile, error) {
	var out []model.StoredFile
	for _, r := range f.rows {
		if r.CompanyID != sc.CompanyID || r.DeletedAt != nil || fl.Page.Offset > 0 {
			continue
		}
		if fl.OwnerType != nil && r.OwnerType != *fl.OwnerType || fl.OwnerID != nil && (r.OwnerID == nil || *r.OwnerID != *fl.OwnerID) {
			continue
		}
		if fl.ClauseID != nil && (r.ClauseID == nil || *r.ClauseID != *fl.ClauseID) {
			continue
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].OriginalName < out[j].OriginalName })
	return out, nil
}

func (f *fakeFiles) Create(_ context.Context, sc store.Scope, r model.StoredFile) (model.StoredFile, error) {
	if r.CompanyID != sc.CompanyID {
		return model.StoredFile{}, store.ErrNotFound
	}
	r.ID = uuid.New()
	f.rows[r.ID] = r
	return r, nil
}

func (f *fakeFiles) SoftDelete(_ context.Context, sc store.Scope, id uuid.UUID, at time.Time) error {
	r, ok := f.rows[id]
	if !ok || r.CompanyID != sc.CompanyID {
		return store.ErrNotFound
	}
	r.DeletedAt = &at
	f.rows[id] = r
	return nil
}

type fakeBuildings struct {
	store.BuildingRepository
	byID map[uuid.UUID]model.Building
}

func (f *fakeBuildings) Get(_ context.Context, sc store.Scope, id uuid.UUID) (model.Building, error) {
	b, ok := f.byID[id]
	if !ok || b.CompanyID != sc.CompanyID || !sc.AllowsBuilding(id) {
		return model.Building{}, store.ErrNotFound
	}
	return b, nil
}
