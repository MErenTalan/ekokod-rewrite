package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Report is a generated monthly or yearly building report. Mirrors table
// `reports` (migration 00010); (building_id, type, period) is unique, so
// regenerating a period replaces rather than accumulates.
//
// Payload is a rendered snapshot of every computed figure, as jsonb. Migration
// 00010 is explicit that it is never queried by field: any figure needed for
// filtering or listing is duplicated into a column instead.
type Report struct {
	ID         uuid.UUID
	CompanyID  uuid.UUID
	BuildingID uuid.UUID

	Type ReportType
	// Period is 'YYYY-MM' for a monthly report and 'YYYY' for a yearly one.
	Period         string
	PlantSelection PlantSelection

	Payload   json.RawMessage
	PdfPath   *string
	ExcelPath *string

	EmailSubject *string
	EmailBody    *string

	Status ReportStatus
	// ErrorMessage is set when Status is ReportStatusError. It is written by
	// the generating job, so it must never carry a DSN or credential: the
	// store layer scrubs errors before they can reach it.
	ErrorMessage *string
	ProcessedAt  *time.Time

	CreatedAt time.Time
	UpdatedAt time.Time
}
