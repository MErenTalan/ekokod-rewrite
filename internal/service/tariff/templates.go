package tariff

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// payload validates in and encodes it as a template payload (LBR E.2 #33).
func payload(in Input) ([]byte, error) {
	if _, err := validate(in); err != nil {
		return nil, err
	}
	in.Tariff.ID, in.Tariff.BuildingID, in.Tariff.CompanyID = uuid.Nil, nil, uuid.Nil
	return json.Marshal(in)
}

// CreateTemplate stores a validated definition.
func (s *Service) CreateTemplate(ctx context.Context, sc store.Scope, name string, description *string, in Input, isDefault bool) (model.TariffTemplate, error) {
	raw, err := payload(in)
	if err != nil {
		return model.TariffTemplate{}, err
	}
	now := s.deps.Clock.Now().UTC()
	return s.deps.Templates.Create(ctx, sc, model.TariffTemplate{CompanyID: sc.CompanyID, Name: name, Description: description,
		IsDefault: isDefault, Payload: raw, CreatedAt: now, UpdatedAt: now})
}

// UpdateTemplate replaces a template with a validated definition.
func (s *Service) UpdateTemplate(ctx context.Context, sc store.Scope, id uuid.UUID, name string, description *string, in Input, isDefault bool) (model.TariffTemplate, error) {
	raw, err := payload(in)
	if err != nil {
		return model.TariffTemplate{}, err
	}
	return s.deps.Templates.Update(ctx, sc, model.TariffTemplate{ID: id, CompanyID: sc.CompanyID, Name: name, Description: description,
		IsDefault: isDefault, Payload: raw, UpdatedAt: s.deps.Clock.Now().UTC()})
}

// DeleteTemplate removes a template.
func (s *Service) DeleteTemplate(ctx context.Context, sc store.Scope, id uuid.UUID) error {
	return s.deps.Templates.Delete(ctx, sc, id)
}

// ListTemplates lists the Scope's templates.
func (s *Service) ListTemplates(ctx context.Context, sc store.Scope, f store.TariffTemplateFilter) ([]model.TariffTemplate, error) {
	return s.deps.Templates.List(ctx, sc, f)
}

// ApplyTemplate creates a version of the template for every building,
// effective from effectiveFrom, carrying its power fields, taxes and extras.
func (s *Service) ApplyTemplate(ctx context.Context, sc store.Scope, templateID uuid.UUID, buildingIDs []uuid.UUID, effectiveFrom time.Time, by uuid.UUID) ([]Definition, error) {
	tpl, err := s.deps.Templates.Get(ctx, sc, templateID)
	if err != nil {
		return nil, err
	}
	var in Input
	if err := json.Unmarshal(tpl.Payload, &in); err != nil {
		return nil, fmt.Errorf("%w: template payload: %w", ErrInvalidRequest, err)
	}
	in.Tariff.EffectiveFrom = effectiveFrom
	// The unrecorded path: this call records once, with the template that
	// produced the versions — which is what the history is asked for.
	defs, err := s.bulkAssign(ctx, sc, in, buildingIDs)
	if err != nil {
		return nil, err
	}
	s.recordAssignment(ctx, sc, defs, &templateID, by, effectiveFrom)
	return defs, nil
}
