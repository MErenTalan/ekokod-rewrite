package tariff

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// RecordBulkAssignment writes what actually happened, after the assignment
// (R241): a call that failed halfway records the buildings that got a version,
// never the ones that were asked for. A recording failure is not allowed to
// undo an assignment that already succeeded, so the caller logs and continues.
func (s *Service) RecordBulkAssignment(ctx context.Context, sc store.Scope, a model.TariffBulkAssignment) error {
	if s.deps.BulkAssignments == nil || len(a.BuildingIDs) == 0 {
		return nil
	}
	a.CompanyID = sc.CompanyID
	_, err := s.deps.BulkAssignments.Create(ctx, sc, a)
	return err
}

// BulkHistory lists the company's bulk assignments, newest first.
func (s *Service) BulkHistory(ctx context.Context, sc store.Scope, p store.Page) ([]model.TariffBulkAssignment, error) {
	if s.deps.BulkAssignments == nil {
		return nil, nil
	}
	return s.deps.BulkAssignments.List(ctx, sc, p)
}

// recordAssignment builds the record from what was written.
func (s *Service) recordAssignment(ctx context.Context, sc store.Scope, defs []Definition, templateID *uuid.UUID, by uuid.UUID, effectiveFrom time.Time) {
	ids := make([]uuid.UUID, 0, len(defs))
	var name *string
	for _, def := range defs {
		if def.Tariff.BuildingID != nil {
			ids = append(ids, *def.Tariff.BuildingID)
		}
		if name == nil && def.Tariff.Name != nil {
			name = def.Tariff.Name
		}
	}
	var actor *uuid.UUID
	if by != uuid.Nil {
		actor = &by
	}
	if err := s.RecordBulkAssignment(ctx, sc, model.TariffBulkAssignment{
		TemplateID: templateID, TariffName: name, EffectiveFrom: effectiveFrom, BuildingIDs: ids, CreatedBy: actor,
	}); err != nil {
		s.deps.Log.WarnContext(ctx, "bulk assignment not recorded", "error", err, "buildings", len(ids))
	}
}
