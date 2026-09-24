package iso50001

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	domain "github.com/MErenTalan/ekokod-rewrite/internal/domain/iso50001"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// Counts is what one sub-clause holds.
type Counts struct{ Notes, Files int }

// State is R331–R334's project read model.
type State struct {
	Dates                    map[string]domain.DateRange
	Counts                   map[string]Counts
	Progress                 int
	Statuses                 map[string]string
	GanttAvailable           bool
	ProjectStart, ProjectEnd *time.Time
}

// Project is the building's state; a building without a project reads empty (R331).
func (s *Service) Project(ctx context.Context, sc store.Scope, building uuid.UUID) (State, error) {
	if _, err := s.d.Buildings.Get(ctx, sc, building); err != nil {
		return State{}, err
	}
	st := State{Dates: map[string]domain.DateRange{}, Counts: map[string]Counts{}}
	for _, id := range domain.SubIDs() {
		st.Counts[id] = Counts{}
	}
	p, err := s.d.ISO.Project(ctx, sc, building)
	switch {
	case errors.Is(err, store.ErrNotFound):
		return s.finish(st), nil
	case err != nil:
		return State{}, err
	}
	dates, err := s.d.ISO.ClauseDates(ctx, sc, p.ID)
	if err != nil {
		return State{}, err
	}
	for _, d := range dates {
		if d.StartDate != nil && d.EndDate != nil {
			st.Dates[d.ClauseID] = domain.DateRange{Start: *d.StartDate, End: *d.EndDate}
		}
	}
	notes, err := s.d.ISO.Notes(ctx, sc, p.ID, nil)
	if err != nil {
		return State{}, err
	}
	for _, n := range notes {
		c := st.Counts[n.ClauseID]
		c.Notes++
		st.Counts[n.ClauseID] = c
	}
	files, err := s.buildingFiles(ctx, sc, building, nil)
	if err != nil {
		return State{}, err
	}
	for _, f := range files {
		if f.ClauseID != nil {
			c := st.Counts[*f.ClauseID]
			c.Files++
			st.Counts[*f.ClauseID] = c
		}
	}
	return s.finish(st), nil
}

func (s *Service) finish(st State) State {
	done := map[string]bool{}
	for id, c := range st.Counts {
		done[id] = c.Notes > 0 || c.Files > 0
	}
	st.Progress = domain.Progress(done)
	st.Statuses, st.GanttAvailable, st.ProjectStart, st.ProjectEnd = domain.Gantt(st.Dates, done, s.d.Clock.Now().In(istanbul))
	return st
}

// SetDates is R332; the project is created on the first write (R331).
func (s *Service) SetDates(ctx context.Context, sc store.Scope, building uuid.UUID, rows []domain.ClauseDates) (State, error) {
	for field, code := range domain.ValidateDates(rows) {
		return State{}, validation(field, code)
	}
	if _, err := s.d.Buildings.Get(ctx, sc, building); err != nil {
		return State{}, err
	}
	p, err := s.d.ISO.EnsureProject(ctx, sc, building)
	if err != nil {
		return State{}, err
	}
	out := make([]model.ISO50001ClauseDate, len(rows))
	for i, r := range rows {
		start, end := civil(r.Start), civil(r.End)
		out[i] = model.ISO50001ClauseDate{ProjectID: p.ID, ClauseID: r.ClauseID, StartDate: &start, EndDate: &end}
	}
	if err := s.d.ISO.ReplaceClauseDates(ctx, sc, p.ID, out); err != nil {
		return State{}, err
	}
	return s.Project(ctx, sc, building)
}

func civil(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// buildingFiles pages through the building's live evidence files.
func (s *Service) buildingFiles(ctx context.Context, sc store.Scope, building uuid.UUID, clause *string) ([]model.StoredFile, error) {
	owner := OwnerType
	var out []model.StoredFile
	for offset := int32(0); ; offset += pageSize {
		page, err := s.d.Files.List(ctx, sc, store.FileFilter{OwnerType: &owner, OwnerID: &building, ClauseID: clause,
			Page: store.Page{Limit: pageSize, Offset: offset}})
		if err != nil {
			return nil, err
		}
		out = append(out, page...)
		if len(page) < pageSize {
			return out, nil
		}
	}
}
