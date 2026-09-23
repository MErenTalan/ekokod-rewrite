package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// StoredFile is one uploaded file's metadata; the bytes live in object
// storage at StoredPath. Mirrors table `stored_files` (migration 00008).
type StoredFile struct {
	ID        uuid.UUID
	CompanyID uuid.UUID

	// OwnerType and OwnerID form a polymorphic reference — 'iso50001',
	// 'carbon', 'report', 'bill' or 'plant' — which is why there is no
	// foreign key and why the repository must scope by CompanyID rather than
	// relying on one.
	OwnerType string
	OwnerID   *uuid.UUID
	ClauseID  *string

	OriginalName string
	StoredPath   string
	ContentType  string
	SizeBytes    int64
	Checksum     string

	UploadedBy *uuid.UUID
	CreatedAt  time.Time
	DeletedAt  *time.Time
}

// IntegrationDefinition is the platform catalogue entry for one provider and
// subtype: which endpoints it exposes. Mirrors table
// `integration_definitions` (migration 00008); (provider, subtype) is unique.
//
// It is admin-managed and platform-wide — no company_id — so it is read
// unscoped by design.
type IntegrationDefinition struct {
	ID       uuid.UUID
	Provider IntegrationProvider
	Subtype  string

	Endpoints json.RawMessage

	CreatedAt time.Time
	UpdatedAt time.Time
}

// IntegrationCredential is one company's credentials for one integration
// definition. Mirrors table `integration_credentials` (migration 00008);
// (company_id, definition_id) is unique.
//
// SecretEnc and ExtraEnc are AES-256-GCM CIPHERTEXT and stay ciphertext in
// this struct. There is deliberately no plaintext field: the ordinary Get must
// not carry a decrypted secret (Task 11's
// TestIntegrationCredentialsAreNeverReturnedInPlaintext pins that), and a
// caller that genuinely needs the plaintext asks for it through an explicit
// method that decrypts on the spot.
//
// Settings holds the NON-secret options only.
type IntegrationCredential struct {
	ID           uuid.UUID
	CompanyID    uuid.UUID
	DefinitionID uuid.UUID

	Username  *string
	SecretEnc []byte
	ExtraEnc  []byte
	Settings  json.RawMessage

	Pm5340URL    *string
	IsolarRegion *string

	TokenExpiresAt *time.Time
	LastVerifiedAt *time.Time
	IsActive       bool

	CreatedAt time.Time
	UpdatedAt time.Time
}

// CredentialRef identifies an active credential for platform dispatch. It
// carries NO ciphertext — not SecretEnc, ExtraEnc, nor anything else that
// would let the scheduled dispatcher (which fans work out to every tenant's
// adapter without ever holding a Scope) leak a secret through a diagnostic
// log or an operational message. A caller that needs the plaintext to
// actually call a provider asks for it separately, through the credential
// package's own decrypting lookup.
type CredentialRef struct {
	CredentialID uuid.UUID
	CompanyID    uuid.UUID
	DefinitionID uuid.UUID
	Provider     IntegrationProvider
	Subtype      string
}

// SMTPSettings is one company's outbound mail configuration. Mirrors table
// `smtp_settings` (migration 00008), whose primary key IS company_id: a
// company has at most one.
//
// PasswordEnc is ciphertext, for the same reason as
// IntegrationCredential.SecretEnc.
type SMTPSettings struct {
	CompanyID uuid.UUID

	Host   string
	Port   int32
	Secure bool

	Username    string
	PasswordEnc []byte
	// FromAddress is citext.
	FromAddress string

	UpdatedAt time.Time
}

// CalendarEvent is one entry on a company's calendar. Mirrors table
// `calendar_events` (migration 00008).
type CalendarEvent struct {
	ID        uuid.UUID
	CompanyID uuid.UUID

	Title string
	// StartsAt and EndsAt are timestamptz stored UTC. When AllDay is set the
	// instants still bound the day in Europe/Istanbul, not in UTC.
	StartsAt time.Time
	EndsAt   time.Time
	AllDay   bool
	Colour   *string

	CreatedBy *uuid.UUID
	CreatedAt time.Time
}

// CompanyWeekendDay records one day of the week a company treats as a weekend.
// Mirrors table `company_weekend_days` (migration 00008), primary key
// (company_id, day_of_week).
type CompanyWeekendDay struct {
	CompanyID uuid.UUID
	// DayOfWeek is 0..6 with 0 = Sunday, checked by the schema. Note that
	// this matches time.Weekday's numbering.
	DayOfWeek int16
}

// CompanyVacation is one non-working range for a company. Mirrors table
// `company_vacations` (migration 00008); the schema requires
// StartDate <= EndDate.
type CompanyVacation struct {
	ID        uuid.UUID
	CompanyID uuid.UUID

	// StartDate and EndDate are `date` columns and the range is INCLUSIVE of
	// both ends.
	StartDate   time.Time
	EndDate     time.Time
	Description *string
}

// JobRun is one execution of a background job, backing the Messages screen and
// operator diagnostics. Mirrors table `job_runs` (migration 00008).
//
// Processed, Skipped and Failed are explicit counts rather than a summary
// string, because the spec's job model requires them and because a job that
// cannot say how many rows it skipped cannot be trusted to have skipped them
// deliberately.
type JobRun struct {
	ID uuid.UUID
	// CompanyID is nullable: a platform-wide job belongs to no tenant.
	CompanyID *uuid.UUID

	JobType string
	// TaskID is the queue's own id for the task this run executed, when the
	// run came from one (migration 00016). It is how a screen watching a job
	// finds the run that explains it.
	TaskID *string
	// Scope describes what the run covered, as jsonb. It is NOT
	// store.Scope — internal/domain imports nothing from the project, and
	// the two are different things: this is a record of what a job did, not
	// an authorisation boundary.
	Scope json.RawMessage

	StartedAt  time.Time
	FinishedAt *time.Time
	// Status is 'running', 'success', 'partial' or 'failed'. A plain text
	// column with a default, not a SQL enum.
	Status string

	Processed int32
	Skipped   int32
	Failed    int32

	// Error is operator-facing text. Nothing that reaches it may contain a
	// DSN or credential.
	Error  *string
	Detail json.RawMessage
}

// OperationalMessage is one row of the Messages screen. Mirrors table
// `operational_messages` (migration 00008).
type OperationalMessage struct {
	// ID is bigserial: this table is high-volume and append-only.
	ID int64
	// CompanyID is nullable for platform-wide messages.
	CompanyID *uuid.UUID

	// Kind is 'alarm', 'job' or 'system'; Category names the producer
	// ('bill-generation', 'analyzer-refresh', …); Status is 'success',
	// 'error', 'warning' or 'info'. All three are plain text columns.
	Kind     string
	Category string
	Status   string

	Message string
	Detail  *string

	RelatedType *string
	RelatedID   *uuid.UUID
	Metadata    json.RawMessage

	CreatedAt time.Time
}
