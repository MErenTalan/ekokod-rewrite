package dto

import (
	"time"

	"github.com/google/uuid"
)

// TariffTemplateFields is a named, reusable tariff definition (01 §7.11).
type TariffTemplateFields struct {
	Name        string  `json:"name" validate:"required,max=200" required:"true"`
	Description *string `json:"description,omitempty" validate:"omitempty,max=2000"`
	IsDefault   bool    `json:"is_default"`
	// Tariff is the definition the template applies, validated exactly as a
	// tariff version is (R240: no second validation path).
	Tariff TariffFields `json:"tariff" validate:"required" required:"true"`
}

// TariffTemplate is a stored template.
type TariffTemplate struct {
	ID uuid.UUID `json:"id" required:"true"`
	TariffTemplateFields
	CreatedAt time.Time `json:"created_at" required:"true"`
	UpdatedAt time.Time `json:"updated_at" required:"true"`
}

// TariffTemplateUpdateRequest is PATCH /tariff-templates/{id}; it replaces.
type TariffTemplateUpdateRequest struct {
	ID uuid.UUID `path:"id" json:"-"`
	TariffTemplateFields
}

// TariffTemplateListRequest is GET /tariff-templates.
type TariffTemplateListRequest struct {
	PageRequest
}

// TariffTemplateApplyRequest is POST /tariff-templates/{id}/apply.
type TariffTemplateApplyRequest struct {
	ID            uuid.UUID   `path:"id" json:"-"`
	BuildingIDs   []uuid.UUID `json:"building_ids" validate:"required,min=1,max=500" required:"true"`
	EffectiveFrom Date        `json:"effective_from" validate:"required" required:"true"`
}

// BulkTariffRequest is POST /buildings/bulk-tariff.
type BulkTariffRequest struct {
	BuildingIDs []uuid.UUID  `json:"building_ids" validate:"required,min=1,max=500" required:"true"`
	Tariff      TariffFields `json:"tariff" validate:"required" required:"true"`
}

// BulkTariffResult names every building that received a version. A call that
// writes nothing returns an empty list rather than a count that hides which
// buildings were touched (R241).
type BulkTariffResult struct {
	BuildingIDs []uuid.UUID `json:"building_ids" required:"true"`
	TariffIDs   []uuid.UUID `json:"tariff_ids" required:"true"`
}

// BuildingTariffState is one row of the current-state table: a building with
// no tariff appears with a null id, because "which buildings have no tariff"
// is the question the screen asks.
type BuildingTariffState struct {
	BuildingID    uuid.UUID  `json:"building_id" required:"true"`
	BuildingName  string     `json:"building_name" required:"true"`
	TariffID      *uuid.UUID `json:"tariff_id"`
	TariffName    *string    `json:"tariff_name"`
	EffectiveFrom *Date      `json:"effective_from"`
	UsePtfYekdem  bool       `json:"use_ptf_yekdem" required:"true"`
}

// BulkTariffAssignment is one history row.
type BulkTariffAssignment struct {
	ID            uuid.UUID   `json:"id" required:"true"`
	TemplateID    *uuid.UUID  `json:"template_id"`
	TariffName    *string     `json:"tariff_name"`
	EffectiveFrom Date        `json:"effective_from" required:"true"`
	BuildingIDs   []uuid.UUID `json:"building_ids" required:"true"`
	CreatedBy     *uuid.UUID  `json:"created_by"`
	CreatedAt     time.Time   `json:"created_at" required:"true"`
}
