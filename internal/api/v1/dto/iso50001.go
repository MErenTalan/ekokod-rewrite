package dto

import (
	"time"

	"github.com/google/uuid"
)

// 05 §13, F11a R330–R341.

// ISOSubClause is one sub-clause in the request locale.
type ISOSubClause struct {
	ID          string  `json:"id" required:"true"`
	Title       string  `json:"title" required:"true"`
	Description string  `json:"description" required:"true"`
	Template    *string `json:"template,omitempty"`
}

// ISOClause is one main clause (5–9).
type ISOClause struct {
	ID    string         `json:"id" required:"true"`
	Title string         `json:"title" required:"true"`
	Subs  []ISOSubClause `json:"subs" required:"true"`
}

// ISOClauses is GET /iso50001/clauses (Q-G2).
type ISOClauses struct {
	Items []ISOClause `json:"items" required:"true"`
}

// ISOBuildingPath names the project's building.
type ISOBuildingPath struct {
	BuildingID uuid.UUID `path:"building_id" json:"-"`
}

// ISOClausePath names a building and a sub-clause.
type ISOClausePath struct {
	BuildingID uuid.UUID `path:"building_id" json:"-"`
	Clause     string    `path:"clause" json:"-"`
}

// ISOClauseState is one main clause's plan and Gantt status (R334).
type ISOClauseState struct {
	ClauseID string  `json:"clause_id" required:"true"`
	Start    *Date   `json:"start,omitempty"`
	End      *Date   `json:"end,omitempty"`
	Status   *string `json:"status,omitempty" enum:"not_started,in_progress,completed,expired"`
}

// ISOCount is what one sub-clause holds.
type ISOCount struct {
	ClauseID string `json:"clause_id" required:"true"`
	Notes    int    `json:"notes" required:"true"`
	Files    int    `json:"files" required:"true"`
}

// ISOProject is GET /iso50001/{building_id} (R331–R334).
type ISOProject struct {
	Progress       int              `json:"progress" required:"true"`
	GanttAvailable bool             `json:"gantt_available" required:"true"`
	ProjectStart   *Date            `json:"project_start,omitempty"`
	ProjectEnd     *Date            `json:"project_end,omitempty"`
	Clauses        []ISOClauseState `json:"clauses" required:"true"`
	Counts         []ISOCount       `json:"counts" required:"true"`
}

// ISODateRow is one calendar row; missing dates are reported, not assumed (R332).
type ISODateRow struct {
	ClauseID string `json:"clause_id" validate:"required,max=2" required:"true"`
	Start    *Date  `json:"start,omitempty"`
	End      *Date  `json:"end,omitempty"`
}

// ISODatesRequest is PUT /iso50001/{building_id}/dates.
type ISODatesRequest struct {
	BuildingID uuid.UUID    `path:"building_id" json:"-"`
	Clauses    []ISODateRow `json:"clauses" validate:"required,max=10" required:"true"`
}

// ISONote is one note.
type ISONote struct {
	ID        uuid.UUID  `json:"id" required:"true"`
	ClauseID  string     `json:"clause_id" required:"true"`
	Title     *string    `json:"title,omitempty"`
	Body      string     `json:"body" required:"true"`
	CreatedBy *uuid.UUID `json:"created_by,omitempty"`
	CreatedAt time.Time  `json:"created_at" required:"true"`
	UpdatedAt time.Time  `json:"updated_at" required:"true"`
}

// ISONotes is a sub-clause's notes.
type ISONotes struct {
	Items []ISONote `json:"items" required:"true"`
}

// ISONoteCreateRequest is POST …/clauses/{clause}/notes (R335).
type ISONoteCreateRequest struct {
	BuildingID uuid.UUID `path:"building_id" json:"-"`
	Clause     string    `path:"clause" json:"-"`
	Title      *string   `json:"title,omitempty" validate:"omitempty,max=200"`
	Body       string    `json:"body" validate:"required,max=10000" required:"true"`
}

// ISONoteUpdateRequest is PATCH /iso50001/notes/{id}; the clause travels
// with it because the scoped update sets it.
type ISONoteUpdateRequest struct {
	ID       uuid.UUID `path:"id" json:"-"`
	ClauseID string    `json:"clause_id" validate:"required,max=8" required:"true"`
	Title    *string   `json:"title,omitempty" validate:"omitempty,max=200"`
	Body     string    `json:"body" validate:"required,max=10000" required:"true"`
}

// ISOFile is one evidence file's metadata; the bytes come only from the
// authorising download (R338).
type ISOFile struct {
	ID          uuid.UUID `json:"id" required:"true"`
	ClauseID    string    `json:"clause_id" required:"true"`
	Name        string    `json:"name" required:"true"`
	ContentType string    `json:"content_type" required:"true"`
	SizeBytes   int64     `json:"size_bytes" required:"true"`
	CreatedAt   time.Time `json:"created_at" required:"true"`
}

// ISOFiles is a sub-clause's files.
type ISOFiles struct {
	Items []ISOFile `json:"items" required:"true"`
}

// ISOTemplate is one downloadable template (R340).
type ISOTemplate struct {
	ID          string   `json:"id" required:"true"`
	FileName    string   `json:"file_name" required:"true"`
	Clauses     []string `json:"clauses" required:"true"`
	Description string   `json:"description" required:"true"`
}

// ISOTemplates is GET /iso50001/templates.
type ISOTemplates struct {
	Items []ISOTemplate `json:"items" required:"true"`
}

// ISOTemplatePath names a template.
type ISOTemplatePath struct {
	ID string `path:"id" json:"-"`
}
