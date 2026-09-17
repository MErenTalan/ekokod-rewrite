package dto

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// CalendarEventsRequest is GET /calendar/events.
type CalendarEventsRequest struct {
	From Date `query:"from" json:"-" validate:"required" required:"true"`
	To   Date `query:"to" json:"-" validate:"required" required:"true"`
}

// CalendarEventFields is an event create or replace.
type CalendarEventFields struct {
	Title    string    `json:"title" validate:"required,max=200" required:"true"`
	StartsAt time.Time `json:"starts_at" validate:"required" required:"true"`
	EndsAt   time.Time `json:"ends_at" validate:"required" required:"true"`
	AllDay   bool      `json:"all_day"`
	Colour   *string   `json:"colour,omitempty" validate:"omitempty,hexcolor,len=7" pattern:"^#[0-9a-fA-F]{6}$"`
}

// CalendarEventUpdateRequest is PATCH /calendar/events/{id} (full replace).
type CalendarEventUpdateRequest struct {
	ID uuid.UUID `path:"id" json:"-"`
	CalendarEventFields
}

// CalendarEvent is a stored event.
type CalendarEvent struct {
	ID       uuid.UUID `json:"id" required:"true"`
	Title    string    `json:"title" required:"true"`
	StartsAt time.Time `json:"starts_at" required:"true"`
	EndsAt   time.Time `json:"ends_at" required:"true"`
	AllDay   bool      `json:"all_day" required:"true"`
	Colour   *string   `json:"colour"`
}

// CalendarEvents is the event list.
type CalendarEvents struct {
	Items []CalendarEvent `json:"items" required:"true"`
}

// VacationPeriod is an inclusive date range.
type VacationPeriod struct {
	ID          *uuid.UUID `json:"id,omitempty"`
	StartDate   Date       `json:"start_date" validate:"required" required:"true"`
	EndDate     Date       `json:"end_date" validate:"required" required:"true"`
	Description *string    `json:"description,omitempty" validate:"omitempty,max=200"`
}

// Vacations is GET/PUT /calendar/vacations.
type Vacations struct {
	WeekendDays   []int            `json:"weekend_days" required:"true"`
	WeekendSource string           `json:"weekend_source" required:"true" enum:"company,default"`
	Periods       []VacationPeriod `json:"periods" required:"true"`
}

// VacationsPutRequest replaces the configuration.
type VacationsPutRequest struct {
	WeekendDays []int            `json:"weekend_days" validate:"max=7,dive,min=0,max=6"`
	Periods     []VacationPeriod `json:"periods" validate:"max=366,dive"`
}

// IntegrationDefinition is a provider catalogue row.
type IntegrationDefinition struct {
	ID        uuid.UUID       `json:"id" required:"true"`
	Provider  string          `json:"provider" required:"true" enum:"osos,gridbox,aril,pm5340,isolar"`
	Subtype   string          `json:"subtype" required:"true"`
	Endpoints json.RawMessage `json:"endpoints" required:"true"`
	UpdatedAt time.Time       `json:"updated_at" required:"true"`
}

// IntegrationDefinitions is the catalogue list.
type IntegrationDefinitions struct {
	Items []IntegrationDefinition `json:"items" required:"true"`
}

// IntegrationDefinitionCreateRequest is POST /integration-definitions.
type IntegrationDefinitionCreateRequest struct {
	Provider  string          `json:"provider" validate:"required,oneof=osos gridbox aril pm5340 isolar" required:"true" enum:"osos,gridbox,aril,pm5340,isolar"`
	Subtype   string          `json:"subtype" validate:"required,max=100" required:"true"`
	Endpoints json.RawMessage `json:"endpoints" validate:"required" required:"true"`
}

// IntegrationDefinitionUpdateRequest is PATCH /integration-definitions/{id}.
type IntegrationDefinitionUpdateRequest struct {
	ID        uuid.UUID       `path:"id" json:"-"`
	Subtype   string          `json:"subtype" validate:"required,max=100" required:"true"`
	Endpoints json.RawMessage `json:"endpoints" validate:"required" required:"true"`
}

// IntegrationCredential is a configured integration; never a secret (R177).
type IntegrationCredential struct {
	ID             uuid.UUID       `json:"id" required:"true"`
	DefinitionID   uuid.UUID       `json:"definition_id" required:"true"`
	Provider       string          `json:"provider" required:"true"`
	Subtype        string          `json:"subtype" required:"true"`
	Username       *string         `json:"username"`
	HasSecret      bool            `json:"has_secret" required:"true"`
	ExtraKeys      []string        `json:"extra_keys" required:"true"`
	Settings       json.RawMessage `json:"settings"`
	PM5340URL      *string         `json:"pm5340_url"`
	IsolarRegion   *string         `json:"isolar_region"`
	IsActive       bool            `json:"is_active" required:"true"`
	TokenExpiresAt *time.Time      `json:"token_expires_at"`
	LastVerifiedAt *time.Time      `json:"last_verified_at"`
	UpdatedAt      time.Time       `json:"updated_at" required:"true"`
}

// IntegrationCredentials is the credential list.
type IntegrationCredentials struct {
	Items []IntegrationCredential `json:"items" required:"true"`
}

// IntegrationCredentialFields are write-only secrets plus settings.
type IntegrationCredentialFields struct {
	Username           *string           `json:"username,omitempty" validate:"omitempty,max=200"`
	Secret             *string           `json:"secret,omitempty" validate:"omitempty,max=1024"`
	Extra              map[string]string `json:"extra,omitempty" validate:"omitempty,max=20"`
	Settings           json.RawMessage   `json:"settings,omitempty"`
	PM5340URL          *string           `json:"pm5340_url,omitempty" validate:"omitempty,max=2048"`
	InstallationNumber *string           `json:"installation_number,omitempty" validate:"omitempty,max=100"`
	IsolarRegion       *string           `json:"isolar_region,omitempty" validate:"omitempty,max=10"`
	IsActive           *bool             `json:"is_active,omitempty"`
}

// IntegrationCredentialCreateRequest is POST /integration-credentials.
type IntegrationCredentialCreateRequest struct {
	Provider string `json:"provider" validate:"required,oneof=osos gridbox aril pm5340 isolar" required:"true" enum:"osos,gridbox,aril,pm5340,isolar"`
	Subtype  string `json:"subtype" validate:"required,max=100" required:"true"`
	IntegrationCredentialFields
}

// IntegrationCredentialUpdateRequest is PATCH /integration-credentials/{id}.
type IntegrationCredentialUpdateRequest struct {
	ID uuid.UUID `path:"id" json:"-"`
	IntegrationCredentialFields
}

// BackfillRequest is POST /integration-credentials/{id}/backfill.
type BackfillRequest struct {
	ID          uuid.UUID   `path:"id" json:"-"`
	AnalyzerIDs []uuid.UUID `json:"analyzer_ids,omitempty" validate:"max=500"`
	Kinds       []string    `json:"kinds,omitempty" validate:"omitempty,dive,oneof=load_profile daily billing reset current_index"`
	From        time.Time   `json:"from" validate:"required" required:"true"`
	To          time.Time   `json:"to" validate:"required" required:"true"`
	Force       bool        `json:"force"`
}

// AuthorizeURLRequest is GET /integrations/isolar/authorize-url.
type AuthorizeURLRequest struct {
	CredentialID uuid.UUID `query:"credential_id" json:"-" validate:"required" required:"true"`
}

// AuthorizeURL is the provider URL to open.
type AuthorizeURL struct {
	URL string `json:"url" required:"true"`
}
