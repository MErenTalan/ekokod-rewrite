package iso50001

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"

	domain "github.com/MErenTalan/ekokod-rewrite/internal/domain/iso50001"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

const (
	maxTitle = 200
	maxBody  = 10_000
)

func noteFields(clause string, title *string, body string) (*string, string, error) {
	if _, ok := domain.SubByID(clause); !ok {
		return nil, "", validation("clause", "unknown")
	}
	body = strings.TrimSpace(body)
	switch {
	case body == "":
		return nil, "", validation("body", "required")
	case len([]rune(body)) > maxBody:
		return nil, "", validation("body", "too_long")
	}
	if title != nil {
		t := strings.TrimSpace(*title)
		if len([]rune(t)) > maxTitle {
			return nil, "", validation("title", "too_long")
		}
		if t == "" {
			title = nil
		} else {
			title = &t
		}
	}
	return title, body, nil
}

// Notes lists one sub-clause's notes, newest first (R335).
func (s *Service) Notes(ctx context.Context, sc store.Scope, building uuid.UUID, clause string) ([]model.ISO50001Note, error) {
	if _, ok := domain.SubByID(clause); !ok {
		return nil, validation("clause", "unknown")
	}
	if _, err := s.d.Buildings.Get(ctx, sc, building); err != nil {
		return nil, err
	}
	p, err := s.d.ISO.Project(ctx, sc, building)
	if errors.Is(err, store.ErrNotFound) {
		return []model.ISO50001Note{}, nil
	}
	if err != nil {
		return nil, err
	}
	return s.d.ISO.Notes(ctx, sc, p.ID, &clause)
}

// CreateNote is R335; the project is created on the first write.
func (s *Service) CreateNote(ctx context.Context, sc store.Scope, userID, building uuid.UUID, clause string, title *string, body string) (model.ISO50001Note, error) {
	title, body, err := noteFields(clause, title, body)
	if err != nil {
		return model.ISO50001Note{}, err
	}
	if _, err := s.d.Buildings.Get(ctx, sc, building); err != nil {
		return model.ISO50001Note{}, err
	}
	p, err := s.d.ISO.EnsureProject(ctx, sc, building)
	if err != nil {
		return model.ISO50001Note{}, err
	}
	return s.d.ISO.CreateNote(ctx, sc, model.ISO50001Note{ProjectID: p.ID, ClauseID: clause, Title: title, Body: body, CreatedBy: &userID})
}

// UpdateNote edits a note in place; the scoped SQL keeps it in its project.
func (s *Service) UpdateNote(ctx context.Context, sc store.Scope, id uuid.UUID, clause string, title *string, body string) (model.ISO50001Note, error) {
	title, body, err := noteFields(clause, title, body)
	if err != nil {
		return model.ISO50001Note{}, err
	}
	return s.d.ISO.UpdateNote(ctx, sc, model.ISO50001Note{ID: id, ClauseID: clause, Title: title, Body: body})
}

// DeleteNote removes a note through the scoped SQL.
func (s *Service) DeleteNote(ctx context.Context, sc store.Scope, id uuid.UUID) error {
	return s.d.ISO.DeleteNote(ctx, sc, id)
}
