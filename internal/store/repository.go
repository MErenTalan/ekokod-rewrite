package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
)

// The repository contract. One interface per aggregate.
//
// TWO KINDS OF INTERFACE live in this file, and the line between them is the
// tenancy model of the whole system. 03-target-architecture.md §2.5 is
// binding: "The repository layer takes an explicit Scope value; there is no
// 'unscoped' query method outside the admin package."
//
//   - SCOPED interfaces — every section but the last. Every method takes ctx
//     first and a Scope second, with no exception. They are implemented in
//     package internal/store/postgres, where internal/arch's
//     TestEveryStoreMethodIsScoped enforces the signature, and they are the
//     only store surface product request handling uses.
//   - UNSCOPED ADMIN interfaces — the final section, every one named Admin….
//     No method takes a Scope. They are implemented ONLY by package
//     internal/store/postgres/admin, the one package that guard exempts, and
//     each method says why its caller cannot hold a Scope. The list is closed.
//
// PLATFORM DATA: READS ARE SCOPED, WRITES ARE ADMIN. market_prices_hourly,
// yekdem_monthly, national_tariff_schedule and integration_definitions have no
// company_id at all, and emission_factors / emission_factor_conversions hold
// platform rows (company_id NULL) beside company-owned ones.
//
//   - A TENANT READ of platform data is scoped. The Scope narrows nothing for a
//     platform row, but it is still required and still validated: a caller
//     with no valid tenant has no business reading anything, and a scoped
//     surface with no exceptions is one a reviewer checks rather than argues.
//   - A WRITE to platform data is never on a scoped interface. Any valid Scope
//     would authorise it, so tenant A's credentials could change the prices on
//     tenant B's invoices. Those writes are on the Admin interfaces.
//
// job_runs, operational_messages and audit_log also allow company_id NULL, for
// platform work. Their scoped methods store s.CompanyID and see only rows whose
// company_id = s.CompanyID — never the NULL rows. Platform rows are written
// through AdminAuditRepository and AdminJournalRepository, so a platform job
// never has to invent a company to satisfy a Scope.
//
// WRITES STORE THE SCOPE'S COMPANY. A scoped write stores s.CompanyID as the
// row's company_id. A model value whose CompanyID names another company — or is
// nil where the column is nullable and nil means a platform row — is refused
// with ErrNotFound and nothing is written.
//
// ROWS WITHOUT company_id. These relations carry no company_id and are
// reachable only through a parent that does (the global constraints' known
// exception to "every tenant-scoped table carries company_id directly"):
//
//	user_password_history, sessions                      -> users
//	building_contacts                                    -> buildings
//	power_plant_monthly_targets, power_plant_devices,
//	power_plant_alarm_recipients, plant_production,
//	plant_production_daily/_monthly,
//	isolar_forwarded_alarms                              -> power_plants
//	meter_readings, ingestion_cursors,
//	consumption_anomalies, forecasts, forecast_gaps,
//	consumption_hourly/_daily/_monthly/_yearly           -> analyzers
//	tariff_taxes, tariff_manual_yekdem                   -> tariffs
//	icmal_rows                                           -> icmal_imports
//	emission_factor_conversions                          -> emission_factors
//	iso50001_clause_dates, iso50001_notes                -> iso50001_projects
//	bill_lines, bill_members, bill_hourly_detail         -> bills
//	alarm_analyzers, alarm_channels, alarm_events,
//	alarm_fired_bills                                    -> alarms
//
// Every method touching one of them carries an "Isolation:" line naming the
// parent its query MUST join through. Joining through the parent means applying
// the parent's whole Scope predicate — company_id = s.CompanyID and, where the
// parent has a building_id, the Scope's building branch — exactly as the parent
// repository's own Get applies it, so a child is visible exactly when its
// parent is. `where id = $1` on the child alone is the bug: a surrogate id is a
// guessable handle into another tenant. The rules every such method follows:
//
//   - A single parent id or child id not visible to the Scope — missing,
//     another tenant's, or outside the Scope's buildings — returns ErrNotFound,
//     for reads and writes alike, and a write touches nothing.
//   - A visible parent with no children returns an empty result, not
//     ErrNotFound.
//   - A batch write (BulkInsert, RecordGaps, InsertRows, a Replace… taking ids)
//     in which ANY row names a parent not visible to the Scope is refused WHOLE
//     with ErrNotFound and writes nothing. A partial write would make the result
//     an oracle for which ids exist elsewhere.
//   - A read taking a LIST of parent ids, or a parent id as a filter field,
//     narrows: ids not visible to the Scope contribute no rows, which is
//     indistinguishable from ids with no data.
//   - Every other id a write stores as a foreign key (created_by, resolved_by,
//     a member analyzer, a device) must itself be visible to the Scope — for a
//     user, belong to s.CompanyID — or the write is refused with ErrNotFound. A
//     foreign key to another tenant's row is a cross-tenant link even when the
//     row being written is your own. (audit_log is the exception: it has no
//     foreign keys by design, and records ids as given.)
//
// Tasks 9-11 pin each of these with an isolation test: tenant A's Scope, tenant
// B's ids, ErrNotFound or nothing back.
//
// Method-name conventions, so that Tasks 9-11 never have to invent one:
//
//	Get       — one row by id, ErrNotFound if it is absent OR out of scope.
//	List      — many rows, filtered; an empty result is not an error.
//	Create    — insert, returning the stored row with its generated columns.
//	Update    — full replace by id, returning the stored row.
//	SoftDelete— set deleted_at, for tables that have the column.
//	Delete    — real DELETE, only for tables with no deleted_at.
//	Upsert    — insert or update, for tables keyed by something natural.
//	Replace…  — swap the whole child collection of one parent in one
//	            transaction (tariff taxes, bill lines, alarm channels …),
//	            because those collections are always written as a set.
//	…Platform…— on an Admin interface, the unscoped sibling of a scoped
//	            method that writes the same table for one tenant.
//
// Errors are the sentinels in errors.go, matched with errors.Is: ErrNotFound,
// ErrConflict, ErrInvalidScope, ErrInvalidRange. ErrInvalidScope and
// ErrInvalidRange are returned before any database round trip, the Scope
// checked first. A row belonging to another tenant returns ErrNotFound and
// never ErrInvalidScope — the two must stay indistinguishable from outside, or
// the error itself becomes an oracle for what exists in another company.

// Page is the pagination every List filter embeds.
//
// A zero Limit means "the repository's default", NOT "no limit": an unbounded
// list over meter_readings would try to materialise millions of rows. Each
// repository documents its own default and its own cap.
type Page struct {
	Limit  int32
	Offset int32
}

// TimeRange is a half-open window, [From, To).
//
// Half-open is not a detail: adjacent ranges tile without overlapping, so a
// reading exactly on a period boundary belongs to exactly one period. Both
// ends are UTC instants; a caller working in local months converts from
// Europe/Istanbul before calling.
type TimeRange struct {
	From time.Time
	To   time.Time
}

// Valid reports whether r can be used: both ends set, and From strictly before
// To. Ends are compared as instants, so the two may carry different
// time.Locations.
//
// Every repository method that takes a TimeRange — directly, or through a
// non-nil *TimeRange in a filter — returns ErrInvalidRange for an invalid one
// BEFORE any database round trip. A zero end is never read as "unbounded" and
// an empty or inverted window is never read as "nothing": both are caller bugs
// and are reported as one. A nil *TimeRange in a filter means the filter does
// not narrow by time, and is only offered on tables that are not hypertables.
// There is deliberately no maximum span.
func (r TimeRange) Valid() bool {
	return !r.From.IsZero() && !r.To.IsZero() && r.From.Before(r.To)
}

// ---------------------------------------------------------------------------
// Tenancy, identity and access (migration 00002)
// ---------------------------------------------------------------------------

// CompanyFilter narrows a company listing.
type CompanyFilter struct {
	// NameContains is a case-insensitive substring match on the name.
	NameContains string
	Sector       *string
	// IncludeDeleted lifts the `deleted_at is null` predicate. It exists for
	// operator tooling and audit views; ordinary product code leaves it false.
	IncludeDeleted bool
	Page           Page
}

// CompanyRepository reads and writes companies.
//
// Every method is still scoped even though a company IS the tenant: a Scope
// may only ever reach its own CompanyID, so Get with another company's id
// returns ErrNotFound. Cross-company listing belongs in the admin package.
type CompanyRepository interface {
	Get(ctx context.Context, s Scope, id uuid.UUID) (model.Company, error)
	List(ctx context.Context, s Scope, f CompanyFilter) ([]model.Company, error)
	Create(ctx context.Context, s Scope, c model.Company) (model.Company, error)
	Update(ctx context.Context, s Scope, c model.Company) (model.Company, error)
	SoftDelete(ctx context.Context, s Scope, id uuid.UUID, at time.Time) error
}

// UserFilter narrows a user listing.
type UserFilter struct {
	Roles          []model.UserRole
	IsActive       *bool
	EmailContains  string
	IncludeDeleted bool
	Page           Page
}

// UserRepository reads and writes users, their password history and their
// login bookkeeping.
//
// There is no lookup by email here. A login has no Scope to offer — the Scope
// is what a login produces — so resolving one is
// AdminAuthRepository.UserByEmail.
type UserRepository interface {
	Get(ctx context.Context, s Scope, id uuid.UUID) (model.User, error)
	List(ctx context.Context, s Scope, f UserFilter) ([]model.User, error)
	Create(ctx context.Context, s Scope, u model.User) (model.User, error)
	Update(ctx context.Context, s Scope, u model.User) (model.User, error)
	SoftDelete(ctx context.Context, s Scope, id uuid.UUID, at time.Time) error

	// SetPassword writes the new hash and appends the OLD one to the
	// history, in one transaction. Splitting the two would let a crash
	// between them lose the record that blocks reuse.
	//
	// Isolation: user_password_history has no company_id — join through
	// users (company_id = s.CompanyID). Another tenant's userID returns
	// ErrNotFound and writes neither the hash nor the history row.
	SetPassword(ctx context.Context, s Scope, userID uuid.UUID, hash string, at time.Time) error

	// PasswordHistory returns the most recent entries first, newest bounded
	// by limit, so a caller can refuse a reused password.
	//
	// Isolation: join user_password_history through users (company_id =
	// s.CompanyID). Another tenant's userID returns ErrNotFound.
	PasswordHistory(ctx context.Context, s Scope, userID uuid.UUID, limit int32) ([]model.PasswordHistoryEntry, error)

	// RecordLogin stamps last_login_at. It is separate from Update so that a
	// login cannot accidentally rewrite a user's role.
	RecordLogin(ctx context.Context, s Scope, userID uuid.UUID, at time.Time) error
}

// SessionFilter narrows a session listing.
type SessionFilter struct {
	UserID *uuid.UUID
	// ActiveAt, when set, keeps only sessions not revoked and not expired at
	// that instant.
	ActiveAt *time.Time
	Page     Page
}

// SessionRepository manages refresh-token sessions.
//
// Isolation, for EVERY method: sessions has no company_id — join through users
// (company_id = s.CompanyID). A session id or user id of another tenant
// returns ErrNotFound, and a write touches nothing.
//
// There is no lookup by refresh-token hash here. A refresh request carries only
// the token, so it has no Scope to offer; resolving one is
// AdminAuthRepository.SessionByRefreshTokenHash.
type SessionRepository interface {
	// Get — Isolation: join sessions through users.
	Get(ctx context.Context, s Scope, id uuid.UUID) (model.Session, error)

	// List — Isolation: join sessions through users; only sessions of users
	// of s.CompanyID are listed, and an f.UserID of another tenant yields an
	// empty list.
	List(ctx context.Context, s Scope, f SessionFilter) ([]model.Session, error)

	// Create — Isolation: sess.UserID must be a user of s.CompanyID, or the
	// insert is refused with ErrNotFound.
	Create(ctx context.Context, s Scope, sess model.Session) (model.Session, error)

	// Revoke stamps revoked_at. Sessions are never deleted on logout, so that
	// a replayed token is provably a replay rather than an unknown token.
	//
	// Isolation: join sessions through users.
	Revoke(ctx context.Context, s Scope, id uuid.UUID, at time.Time) error

	// RevokeAllForUser is what a password change and a role change both call.
	//
	// Isolation: join sessions through users; another tenant's userID
	// returns ErrNotFound.
	RevokeAllForUser(ctx context.Context, s Scope, userID uuid.UUID, at time.Time) (int64, error)

	// Rotate replaces a refresh session in one transaction (R141): it locks
	// oldID, revokes it with reason 'rotated' and inserts next with
	// rotated_from = oldID and next.UserID forced to the old session's user.
	// An old session already revoked returns ErrConflict and inserts nothing.
	//
	// Isolation: join sessions through users; another tenant's oldID returns
	// ErrNotFound.
	Rotate(ctx context.Context, s Scope, oldID uuid.UUID, next model.Session, at time.Time) (model.Session, error)

	// RevokeWithReason revokes a live session and records why; a visible but
	// already revoked session keeps its first reason and returns nil.
	//
	// Isolation: join sessions through users.
	RevokeWithReason(ctx context.Context, s Scope, id uuid.UUID, reason string, at time.Time) error

	// RevokeAllForUserWithReason revokes every live session of the user except
	// `except` (nil = none) and returns how many it revoked.
	//
	// Isolation: join sessions through users; another tenant's userID returns
	// ErrNotFound.
	RevokeAllForUserWithReason(ctx context.Context, s Scope, userID uuid.UUID, reason string, except *uuid.UUID, at time.Time) (int64, error)

	// Touch stamps last_used_at = at when it is unset or older than five
	// minutes, so authenticated requests do not write on every call.
	//
	// Isolation: join sessions through users.
	Touch(ctx context.Context, s Scope, id uuid.UUID, at time.Time) error

	// DeleteExpired is housekeeping: it removes sessions that expired before
	// before, and returns how many rows went.
	//
	// Isolation: join sessions through users; it deletes only the expired
	// sessions of s.CompanyID's users, so a sweep runs once per tenant.
	DeleteExpired(ctx context.Context, s Scope, before time.Time) (int64, error)
}

// PasswordResetRepository writes single-use password reset tokens (R148).
//
// Isolation, for EVERY method: password_reset_tokens has no company_id — join
// through users. There is no lookup by token hash here: a reset request
// carries only the token, so resolving it is
// AdminAuthRepository.PasswordResetByTokenHash.
type PasswordResetRepository interface {
	// Create invalidates the user's unused tokens and inserts p, in one
	// transaction. A user not visible to the Scope returns ErrNotFound.
	Create(ctx context.Context, s Scope, p model.PasswordReset) (model.PasswordReset, error)

	// MarkUsed stamps used_at; an already used token returns ErrConflict.
	MarkUsed(ctx context.Context, s Scope, id uuid.UUID, at time.Time) error
}

// AuditFilter narrows an audit listing.
type AuditFilter struct {
	UserID     *uuid.UUID
	EntityType *string
	EntityID   *uuid.UUID
	Action     *string
	Range      *TimeRange
	Page       Page
}

// AuditRepository appends to and reads a tenant's audit trail.
//
// There is no Update and no Delete, by design: an audit row that can be edited
// is not an audit row.
//
// audit_log.company_id is nullable, for platform actions. These methods store
// and see only company_id = s.CompanyID, never a NULL row; a platform action is
// appended through AdminAuditRepository.AppendPlatform.
type AuditRepository interface {
	// Append stores s.CompanyID as company_id. An entry whose CompanyID is nil
	// (a platform row) or names another company is refused with ErrNotFound.
	// UserID and EntityID are recorded as given: audit_log has no foreign keys
	// by design, so that a deleted subject does not erase its history.
	Append(ctx context.Context, s Scope, e model.AuditEntry) (model.AuditEntry, error)
	List(ctx context.Context, s Scope, f AuditFilter) ([]model.AuditEntry, error)
}

// ---------------------------------------------------------------------------
// Buildings, analyzers and plants (migration 00003)
// ---------------------------------------------------------------------------

// BuildingFilter narrows a building listing.
//
// Note the deliberate near-collision with Scope's BuildingFilter METHOD, which
// returns the (ids, all) pair a query builds its predicate from. They are
// different things at different layers — this one is what the CALLER asks for,
// that one is what the SCOPE permits — and the query applies both. Go keeps
// the names apart (a method name and a package-level type never collide), and
// renaming either would mean disagreeing with a signature Tasks 9-13 are
// already written against.
type BuildingFilter struct {
	// IDs further narrows within the scope. It never widens: a building not
	// permitted by the Scope stays invisible however this is set.
	IDs               []uuid.UUID
	NameContains      string
	Sector            *string
	ResponsibleUserID *uuid.UUID
	IncludeDeleted    bool
	Page              Page
}

// BuildingRepository reads and writes buildings.
type BuildingRepository interface {
	Get(ctx context.Context, s Scope, id uuid.UUID) (model.Building, error)
	List(ctx context.Context, s Scope, f BuildingFilter) ([]model.Building, error)

	// Create requires an AllBuildings Scope: a narrow Scope names a fixed
	// set of already-granted buildings and has no way to grant itself a new
	// one, so it is refused with ErrNotFound rather than either naming an
	// ungranted id or creating a building its own Scope could never see
	// again.
	Create(ctx context.Context, s Scope, b model.Building) (model.Building, error)
	Update(ctx context.Context, s Scope, b model.Building) (model.Building, error)
	SoftDelete(ctx context.Context, s Scope, id uuid.UUID, at time.Time) error

	// Contacts returns a building's contacts in sort_order.
	//
	// Isolation: building_contacts has no company_id — join through
	// buildings (company_id and the Scope's building branch). A building
	// not visible to the Scope returns ErrNotFound.
	Contacts(ctx context.Context, s Scope, buildingID uuid.UUID) ([]model.BuildingContact, error)

	// ReplaceContacts swaps the whole contact list in one transaction. The
	// UI edits them as a list, so a per-row API would make a partial write
	// the normal case.
	//
	// Isolation: join building_contacts through buildings. A building not
	// visible to the Scope returns ErrNotFound and nothing is replaced; the
	// contacts' own BuildingID fields are ignored in favour of buildingID.
	ReplaceContacts(ctx context.Context, s Scope, buildingID uuid.UUID, contacts []model.BuildingContact) ([]model.BuildingContact, error)
}

// AnalyzerFilter narrows an analyzer listing.
type AnalyzerFilter struct {
	IDs        []uuid.UUID
	BuildingID *uuid.UUID
	// Unassigned, when true, keeps only analyzers with a NULL building_id.
	// It is a separate flag rather than a nil BuildingID because nil already
	// means "any building", and conflating the two is the fail-open shape
	// Scope exists to prevent.
	//
	// An unassigned analyzer is reachable ONLY by a Scope with AllBuildings:
	// it belongs to no building, so a scope listing building ids cannot
	// possibly permit it.
	Unassigned bool

	Providers      []model.IntegrationProvider
	IsActive       *bool
	IncludeDeleted bool
	Page           Page
}

// AnalyzerRepository reads and writes analyzers.
type AnalyzerRepository interface {
	Get(ctx context.Context, s Scope, id uuid.UUID) (model.Analyzer, error)

	// GetByInstallation resolves the natural key the providers use, which is
	// the unique index (provider, provider_subtype, installation_number).
	GetByInstallation(ctx context.Context, s Scope, provider model.IntegrationProvider, subtype, installationNumber string) (model.Analyzer, error)

	List(ctx context.Context, s Scope, f AnalyzerFilter) ([]model.Analyzer, error)
	Create(ctx context.Context, s Scope, a model.Analyzer) (model.Analyzer, error)
	Update(ctx context.Context, s Scope, a model.Analyzer) (model.Analyzer, error)
	SoftDelete(ctx context.Context, s Scope, id uuid.UUID, at time.Time) error

	// TouchLastReading advances last_reading_at, never retreats it: an
	// out-of-order backfill must not make an analyzer look staler than it is.
	TouchLastReading(ctx context.Context, s Scope, id uuid.UUID, at time.Time) error
}

// PlantFilter narrows a power plant listing.
type PlantFilter struct {
	IDs []uuid.UUID
	// PlantKind is 'rooftop' or 'grid'.
	PlantKind *string
	// IsolarLinked, when set, keeps plants with (true) or without (false) an
	// isolar_ps_id.
	IsolarLinked   *bool
	IncludeDeleted bool
	Page           Page
}

// PlantRepository reads and writes power plants and everything hanging off
// them.
//
// power_plants has NO building_id, so a Scope narrows plants to the company
// and no further. A repository must not invent a join through analyzers to
// pretend otherwise. "Visible to the Scope" for a plant therefore means
// company_id = s.CompanyID and deleted_at is null.
type PlantRepository interface {
	Get(ctx context.Context, s Scope, id uuid.UUID) (model.PowerPlant, error)
	List(ctx context.Context, s Scope, f PlantFilter) ([]model.PowerPlant, error)
	Create(ctx context.Context, s Scope, p model.PowerPlant) (model.PowerPlant, error)
	Update(ctx context.Context, s Scope, p model.PowerPlant) (model.PowerPlant, error)
	SoftDelete(ctx context.Context, s Scope, id uuid.UUID, at time.Time) error

	// MonthlyTargets — Isolation: power_plant_monthly_targets has no
	// company_id — join through power_plants. Another tenant's plantID
	// returns ErrNotFound.
	MonthlyTargets(ctx context.Context, s Scope, plantID uuid.UUID) ([]model.PlantMonthlyTarget, error)

	// ReplaceMonthlyTargets — Isolation: join power_plant_monthly_targets
	// through power_plants. Another tenant's plantID returns ErrNotFound and
	// nothing is replaced.
	ReplaceMonthlyTargets(ctx context.Context, s Scope, plantID uuid.UUID, targets []model.PlantMonthlyTarget) ([]model.PlantMonthlyTarget, error)

	// Devices — Isolation: power_plant_devices has no company_id — join
	// through power_plants. Another tenant's plantID returns ErrNotFound.
	Devices(ctx context.Context, s Scope, plantID uuid.UUID) ([]model.PlantDevice, error)

	// UpsertDevice is keyed on (plant_id, device_sn): the provider's device
	// list is re-fetched on a schedule and must converge rather than
	// accumulate duplicates.
	//
	// Isolation: join power_plant_devices through power_plants on d.PlantID.
	// A d.PlantID of another tenant is refused with ErrNotFound. The update
	// branch never changes plant_id, so a device can be neither moved to nor
	// taken from another plant.
	UpsertDevice(ctx context.Context, s Scope, d model.PlantDevice) (model.PlantDevice, error)

	// AlarmRecipients — Isolation: power_plant_alarm_recipients has no
	// company_id — join through power_plants. Another tenant's plantID
	// returns ErrNotFound.
	AlarmRecipients(ctx context.Context, s Scope, plantID uuid.UUID) ([]model.PlantAlarmRecipient, error)

	// ReplaceAlarmRecipients — Isolation: join power_plant_alarm_recipients
	// through power_plants. Another tenant's plantID returns ErrNotFound and
	// nothing is replaced.
	ReplaceAlarmRecipients(ctx context.Context, s Scope, plantID uuid.UUID, emails []string) ([]model.PlantAlarmRecipient, error)
}

// ---------------------------------------------------------------------------
// Time series (migrations 00004 and 00005)
// ---------------------------------------------------------------------------

// ReadingRepository reads and writes meter_readings.
//
// This is the billing-grade surface: it reads the hypertable directly and
// NEVER a continuous aggregate. 04-data-model.md §4.3 requires that
// distinction to be explicit in the code, which is why the aggregate reads
// live on AnalyticsRepository under names that cannot be confused with these.
//
// Isolation, for EVERY method: meter_readings has no company_id — join through
// analyzers (company_id and the Scope's building branch; an analyzer with no
// building only under AllBuildings, as AnalyzerFilter.Unassigned documents).
type ReadingRepository interface {
	// BulkInsert writes rows idempotently: COPY into a staging table, then
	// one `insert … on conflict (analyzer_id, ts, kind) do update`
	// (04-data-model.md §14). Re-ingesting a window must update, never
	// duplicate, because F2's ingestion is resumable and will re-ingest.
	//
	// The counts are real, not estimated: the job model records processed
	// and skipped separately and guessing them is not acceptable.
	//
	// Isolation: the staging-to-table insert joins through analyzers. If ANY
	// row's analyzer is not visible to the Scope the whole batch is refused
	// with ErrNotFound and nothing is written.
	//
	// A duplicate (analyzer_id, ts, kind) WITHIN rows itself is refused
	// loudly: it is detected before any database round trip and returns an
	// error for which errors.Is(err, ErrConflict) is true, naming the first
	// duplicate key (analyzer id, ts, kind — never a register value); nothing
	// is written. Silently letting `on conflict … do update` arbitrate
	// between two batched rows with different register values is not
	// acceptable for a billing-grade register. Upstream ingestion (F2) is
	// expected to deduplicate before calling this.
	BulkInsert(ctx context.Context, s Scope, rows []model.MeterReading) (inserted, updated int, err error)

	// Range returns readings for one analyzer over the half-open window.
	// There is deliberately no unbounded variant: 04-data-model.md §14 says
	// a query with no time bound is a bug, and this signature makes one
	// impossible to express. An invalid r returns ErrInvalidRange.
	//
	// Isolation: join through analyzers. An analyzer not visible to the
	// Scope returns ErrNotFound; a visible one with no readings in r returns
	// an empty slice.
	Range(ctx context.Context, s Scope, analyzerID uuid.UUID, r TimeRange, kind model.ReadingKind) ([]model.MeterReading, error)

	// BoundaryReadings returns the last reading at or before each of start
	// and end — the exact operation 02-domain-rules.md §3.1 specifies for
	// billing.
	//
	// It takes a kind because the primary key (analyzer_id, ts, kind) allows
	// two kinds to carry different register values at the very same instant
	// (02-domain-rules.md §2.3, §3.4); which kind billing reads is an F4
	// decision this signature leaves open rather than guesses at.
	//
	// Either return may be nil, and nil is NOT a zero reading. §3.1: "If
	// either reading is missing, or if reading_start and reading_end are the
	// same reading, the period yields no row — it is not emitted as zero." A
	// zero here becomes a wrong invoice, which is why these are pointers.
	//
	// This is the one hypertable read bounded on one side only, and
	// deliberately: §3.1 wants the last reading however long ago it was
	// taken. The upper bound (ts <= start, ts <= end) still excludes every
	// later chunk, and the query is a descending index probe with limit 1.
	//
	// Isolation: join through analyzers. An analyzer not visible to the
	// Scope returns ErrNotFound — never two nils, which would read as "no
	// data" for a meter that is simply someone else's.
	BoundaryReadings(ctx context.Context, s Scope, analyzerID uuid.UUID, kind model.ReadingKind, start, end time.Time) (startReading, endReading *model.MeterReading, err error)

	// Latest returns the most recent reading of a kind WITHIN r, or nil if
	// there is none in r. Nil rather than ErrNotFound: an analyzer with no
	// recent readings is an ordinary state, not a lookup failure.
	//
	// It takes a TimeRange because §14 has no exception for "the newest
	// row": an unbounded `order by ts desc limit 1` excludes no chunk, and
	// for an analyzer that has never reported it can probe every chunk of
	// the hypertable before returning nil. A caller that only wants to know WHEN
	// an analyzer last reported reads Analyzer.LastReadingAt, which costs no
	// hypertable access at all. An invalid r returns ErrInvalidRange.
	//
	// Isolation: join through analyzers. An analyzer not visible to the
	// Scope returns ErrNotFound, not nil.
	Latest(ctx context.Context, s Scope, analyzerID uuid.UUID, r TimeRange, kind model.ReadingKind) (*model.MeterReading, error)
}

// CursorRepository tracks per-analyzer ingestion high-water marks.
//
// Isolation, for EVERY method: ingestion_cursors has no company_id — join
// through analyzers.
type CursorRepository interface {
	// Get — Isolation: join through analyzers. An analyzer not visible to the
	// Scope, or one with no cursor yet, returns ErrNotFound.
	Get(ctx context.Context, s Scope, analyzerID uuid.UUID, kind model.ReadingKind) (model.IngestionCursor, error)

	// List — analyzerIDs is a REQUIRED POSITIONAL parameter: EMPTY or nil
	// means NO ROWS, fail-closed, identical to Scope.BuildingIDs — there is
	// no "every analyzer visible to scope" form. A non-empty list narrows
	// further; ids not visible to the Scope contribute no rows.
	List(ctx context.Context, s Scope, analyzerIDs []uuid.UUID) ([]model.IngestionCursor, error)

	// RecordSuccess advances last_ts and last_success_at and RESETS
	// consecutive_failures. Clearing the counter is part of recording a
	// success, not a separate call a caller can forget.
	//
	// last_ts is a HIGH-WATER MARK: an out-of-order call (lastTs behind what
	// is already stored) never moves it backwards — the stored value is
	// greatest(current, lastTs), never a plain overwrite.
	//
	// Isolation: join through analyzers. An analyzer not visible to the
	// Scope returns ErrNotFound and no cursor is created or changed.
	RecordSuccess(ctx context.Context, s Scope, analyzerID uuid.UUID, kind model.ReadingKind, lastTs, at time.Time) error

	// RecordFailure increments consecutive_failures and stores the message.
	// The message reaches an operator, so the caller passes already-scrubbed
	// text: nothing carrying a DSN or credential may be stored here.
	//
	// Isolation: join through analyzers. An analyzer not visible to the
	// Scope returns ErrNotFound and no cursor is created or changed.
	RecordFailure(ctx context.Context, s Scope, analyzerID uuid.UUID, kind model.ReadingKind, message string, at time.Time) error
}

// AnomalyFilter narrows an anomaly listing.
type AnomalyFilter struct {
	AnalyzerIDs []uuid.UUID
	// Unresolved, when true, keeps only anomalies with a NULL resolved_at.
	Unresolved bool
	Reason     *string
	Range      *TimeRange
	Page       Page
}

// AnomalyRepository records and resolves suspect consumption periods.
//
// Isolation, for EVERY method: consumption_anomalies has no company_id — join
// through analyzers.
type AnomalyRepository interface {
	// Get — Isolation: join consumption_anomalies through analyzers. An
	// anomaly whose analyzer is not visible to the Scope returns ErrNotFound.
	Get(ctx context.Context, s Scope, id uuid.UUID) (model.ConsumptionAnomaly, error)

	// List — Isolation: join through analyzers; f.AnalyzerIDs not visible to
	// the Scope contribute no rows.
	List(ctx context.Context, s Scope, f AnomalyFilter) ([]model.ConsumptionAnomaly, error)

	// Create — Isolation: a.AnalyzerID not visible to the Scope is refused
	// with ErrNotFound.
	Create(ctx context.Context, s Scope, a model.ConsumptionAnomaly) (model.ConsumptionAnomaly, error)

	// Resolve stamps resolved_at, resolved_by and the resolution, and stores
	// any manual override values. An anomaly is never deleted: the record
	// that a period was once suspect is what explains a restated bill.
	//
	// Isolation: join consumption_anomalies through analyzers; an anomaly id
	// not visible to the Scope returns ErrNotFound. resolvedBy must be a user
	// of s.CompanyID, or the call is refused with ErrNotFound.
	Resolve(ctx context.Context, s Scope, id uuid.UUID, resolvedBy uuid.UUID, resolution string, overrides []byte, at time.Time) (model.ConsumptionAnomaly, error)
}

// ProductionRepository reads and writes plant_production.
//
// Isolation, for EVERY method: plant_production has no company_id — join
// through power_plants.
type ProductionRepository interface {
	// BulkInsert is idempotent on (plant_id, ts, device_id), like
	// ReadingRepository.BulkInsert and for the same reason.
	//
	// Isolation: join through power_plants. If ANY row's plant is not
	// visible to the Scope, or ANY row's device_id is not a device of that
	// same row's plant, the whole batch is refused with ErrNotFound and
	// nothing is written.
	//
	// A duplicate (plant_id, ts, device_id) WITHIN rows itself is refused
	// loudly, the same way and for the same reason as
	// ReadingRepository.BulkInsert's duplicate-key doc: an ErrConflict naming
	// the first duplicate key, before any database round trip, nothing
	// written.
	BulkInsert(ctx context.Context, s Scope, rows []model.PlantProduction) (inserted, updated int, err error)

	// Range reads the hypertable directly, over a bounded window. An invalid
	// r returns ErrInvalidRange.
	//
	// Isolation: join through power_plants. A plant not visible to the Scope
	// returns ErrNotFound.
	Range(ctx context.Context, s Scope, plantID uuid.UUID, r TimeRange) ([]model.PlantProduction, error)

	// Latest is the most recent sample for a plant WITHIN r, or nil if there
	// is none in r. It is bounded for the reason ReadingRepository.Latest is.
	// An invalid r returns ErrInvalidRange.
	//
	// Isolation: join through power_plants. A plant not visible to the Scope
	// returns ErrNotFound, not nil.
	Latest(ctx context.Context, s Scope, plantID uuid.UUID, r TimeRange) (*model.PlantProduction, error)
}

// AnalyticsRepository reads the six continuous aggregates.
//
// Everything here is MATERIALISED and therefore may lag the hypertable by up
// to the aggregate's refresh lag. It is correct for dashboards and reports and
// WRONG for billing, which must go through ReadingRepository.BoundaryReadings.
// The names are chosen so that no call site can mistake one for the other.
//
// Isolation, for EVERY method: the aggregates have no company_id — the
// consumption_* aggregates join through analyzers and the production_*
// aggregates through power_plants. analyzerIDs/plantIDs are REQUIRED
// POSITIONAL parameters: EMPTY or nil means NO ROWS, fail-closed, identical
// to Scope.BuildingIDs — there is no "every id visible to scope" form. A
// non-empty list narrows further; ids not visible to the Scope contribute no
// rows. Every method returns ErrInvalidRange for an invalid r.
//
// Bucket boundaries for every view coarser than hourly (Daily, Monthly,
// Yearly) are Europe/Istanbul-LOCAL instants, not UTC: a caller building a
// TimeRange from UTC calendar-day boundaries will see the window's own edges
// land mid-bucket. consumption_hourly is timezone-free (an hour is an hour
// everywhere).
type AnalyticsRepository interface {
	ConsumptionHourly(ctx context.Context, s Scope, analyzerIDs []uuid.UUID, r TimeRange) ([]model.ConsumptionBucket, error)
	ConsumptionDaily(ctx context.Context, s Scope, analyzerIDs []uuid.UUID, r TimeRange) ([]model.ConsumptionBucket, error)
	ConsumptionMonthly(ctx context.Context, s Scope, analyzerIDs []uuid.UUID, r TimeRange) ([]model.ConsumptionBucket, error)
	ConsumptionYearly(ctx context.Context, s Scope, analyzerIDs []uuid.UUID, r TimeRange) ([]model.ConsumptionBucket, error)

	ProductionDaily(ctx context.Context, s Scope, plantIDs []uuid.UUID, r TimeRange) ([]model.PlantProductionBucket, error)
	ProductionMonthly(ctx context.Context, s Scope, plantIDs []uuid.UUID, r TimeRange) ([]model.PlantProductionBucket, error)
}

// AggregateView names one continuous aggregate built by migration 00005. It
// is a closed set: a caller can never pass an arbitrary view name, so the
// refresh statement AdminAggregateRepository issues is never assembled from
// caller input (R72).
type AggregateView string

// The four consumption aggregates migration 00005 built. plant_production_*
// is out of scope for F3's refresh seam (R71/R72/R73 name only the
// consumption path F2's ingestion pipeline backfills).
const (
	ViewConsumptionHourly  AggregateView = "consumption_hourly"
	ViewConsumptionDaily   AggregateView = "consumption_daily"
	ViewConsumptionMonthly AggregateView = "consumption_monthly"
	ViewConsumptionYearly  AggregateView = "consumption_yearly"
)

// ConsumptionViews returns the four consumption aggregates, finest first. A
// refresh walks them in this order so that a coarser view is never expected
// to reflect a finer one that has not been refreshed yet (R71) — even though,
// per migration 00005's operator note, each view is in fact defined directly
// over meter_readings and refreshing one never depends on another having run.
func ConsumptionViews() []AggregateView {
	return []AggregateView{
		ViewConsumptionHourly,
		ViewConsumptionDaily,
		ViewConsumptionMonthly,
		ViewConsumptionYearly,
	}
}

// PriceRepository is a tenant's READ access to the market price series and the
// YEKDEM table.
//
// Both tables are PLATFORM-WIDE: neither has a company_id, so the Scope
// narrows nothing. It is still required and still validated — see this file's
// header — and a repository here must not pretend to filter by it. The WRITES
// are AdminMarketDataRepository's: a scoped price write would let any tenant
// change every tenant's invoices.
type PriceRepository interface {
	// HourlyRange returns ErrInvalidRange for an invalid r.
	HourlyRange(ctx context.Context, s Scope, r TimeRange) ([]model.MarketPrice, error)
	Yekdem(ctx context.Context, s Scope, year, month int16) (model.YekdemMonthly, error)
}

// ForecastRepository stores and reads forecast runs.
//
// Isolation, for EVERY method: forecasts and forecast_gaps have no company_id —
// join through analyzers.
type ForecastRepository interface {
	// BulkInsert is idempotent on (analyzer_id, ts, generated_at). Because
	// generated_at is part of the key, a new run ACCUMULATES rather than
	// overwriting the previous one, which is what makes a forecast scorable
	// against what actually happened.
	//
	// Isolation: join through analyzers. If ANY row's analyzer is not
	// visible to the Scope the whole batch is refused with ErrNotFound and
	// nothing is written.
	//
	// A duplicate (analyzer_id, ts, generated_at) WITHIN rows itself is
	// refused loudly, the same way and for the same reason as
	// ReadingRepository.BulkInsert's duplicate-key doc: an ErrConflict naming
	// the first duplicate key, before any database round trip, nothing
	// written.
	BulkInsert(ctx context.Context, s Scope, rows []model.Forecast) (inserted, updated int, err error)

	// Range — Isolation: join through analyzers. An analyzer not visible to
	// the Scope returns ErrNotFound. An invalid r returns ErrInvalidRange.
	Range(ctx context.Context, s Scope, analyzerID uuid.UUID, r TimeRange) ([]model.Forecast, error)

	// LatestRun returns the forecasts from the most recent run covering the
	// window. An invalid r returns ErrInvalidRange.
	//
	// Isolation: join through analyzers. An analyzer not visible to the
	// Scope returns ErrNotFound.
	LatestRun(ctx context.Context, s Scope, analyzerID uuid.UUID, r TimeRange) ([]model.Forecast, error)

	// RecordGaps — Isolation: join forecast_gaps through analyzers on each
	// gap's AnalyzerID. If ANY gap's analyzer is not visible to the Scope the
	// whole call is refused with ErrNotFound and nothing is written.
	RecordGaps(ctx context.Context, s Scope, gaps []model.ForecastGap) error

	// Gaps — Isolation: join forecast_gaps through analyzers. An analyzer not
	// visible to the Scope returns ErrNotFound.
	Gaps(ctx context.Context, s Scope, analyzerID uuid.UUID, generatedAt time.Time) ([]model.ForecastGap, error)
}

// ---------------------------------------------------------------------------
// Tariffs (migration 00006)
// ---------------------------------------------------------------------------

// TariffFilter narrows a tariff listing.
type TariffFilter struct {
	IDs        []uuid.UUID
	BuildingID *uuid.UUID
	// CompanyWide, when true, keeps only tariffs with a NULL building_id.
	// Separate from BuildingID for the same reason AnalyzerFilter.Unassigned
	// is separate: nil already means "any".
	CompanyWide    bool
	EffectiveOn    *time.Time
	IncludeDeleted bool
	Page           Page
}

// TariffRepository reads and writes tariffs and their child collections.
type TariffRepository interface {
	Get(ctx context.Context, s Scope, id uuid.UUID) (model.Tariff, error)
	List(ctx context.Context, s Scope, f TariffFilter) ([]model.Tariff, error)
	Create(ctx context.Context, s Scope, t model.Tariff) (model.Tariff, error)
	Update(ctx context.Context, s Scope, t model.Tariff) (model.Tariff, error)
	SoftDelete(ctx context.Context, s Scope, id uuid.UUID, at time.Time) error

	// Effective resolves the tariff in force for a building on a date: the
	// live row with the GREATEST effective_from NOT AFTER on. Picking the
	// wrong row misprices every invoice in the period, so this is one method
	// rather than a pattern each caller reimplements.
	//
	// A building with no building-specific tariff falls back to the
	// company-wide one (building_id is null); if neither exists the result is
	// ErrNotFound.
	Effective(ctx context.Context, s Scope, buildingID uuid.UUID, on time.Time) (model.Tariff, error)

	// Taxes — Isolation: tariff_taxes has no company_id — join through
	// tariffs. A tariff not visible to the Scope returns ErrNotFound.
	Taxes(ctx context.Context, s Scope, tariffID uuid.UUID) ([]model.TariffTax, error)

	// ReplaceTaxes — Isolation: join tariff_taxes through tariffs. A tariff
	// not visible to the Scope returns ErrNotFound and nothing is replaced.
	ReplaceTaxes(ctx context.Context, s Scope, tariffID uuid.UUID, taxes []model.TariffTax) ([]model.TariffTax, error)

	// ManualYekdem — Isolation: tariff_manual_yekdem has no company_id — join
	// through tariffs. A tariff not visible to the Scope returns ErrNotFound.
	ManualYekdem(ctx context.Context, s Scope, tariffID uuid.UUID) ([]model.TariffManualYekdem, error)

	// ReplaceManualYekdem — Isolation: join tariff_manual_yekdem through
	// tariffs. A tariff not visible to the Scope returns ErrNotFound and
	// nothing is replaced.
	ReplaceManualYekdem(ctx context.Context, s Scope, tariffID uuid.UUID, values []model.TariffManualYekdem) ([]model.TariffManualYekdem, error)

	// ExtraCharges — Isolation: tariff_extra_charges has no company_id — join
	// through tariffs. A tariff not visible to the Scope returns ErrNotFound.
	ExtraCharges(ctx context.Context, s Scope, tariffID uuid.UUID) ([]model.TariffExtraCharge, error)

	// ReplaceExtraCharges — Isolation: join through tariffs. A tariff not
	// visible to the Scope returns ErrNotFound and nothing is replaced.
	ReplaceExtraCharges(ctx context.Context, s Scope, tariffID uuid.UUID, charges []model.TariffExtraCharge) ([]model.TariffExtraCharge, error)
}

// BillingParameterRepository reads the dated, platform-wide billing
// parameters (R106). The Scope is validated and narrows nothing; writes are
// AdminCatalogueRepository.UpsertBillingParameters.
type BillingParameterRepository interface {
	// Effective returns the row with the greatest effective_from not after
	// on's Europe/Istanbul calendar date, or ErrNotFound.
	Effective(ctx context.Context, s Scope, on time.Time) (model.BillingParameters, error)
	List(ctx context.Context, s Scope) ([]model.BillingParameters, error)
}

// TariffTemplateFilter narrows a template listing.
type TariffTemplateFilter struct {
	NameContains string
	IsDefault    *bool
	Page         Page
}

// TariffTemplateRepository reads and writes saved tariff definitions.
//
// tariff_templates has no deleted_at, so removal is a real Delete.
type TariffTemplateRepository interface {
	Get(ctx context.Context, s Scope, id uuid.UUID) (model.TariffTemplate, error)
	List(ctx context.Context, s Scope, f TariffTemplateFilter) ([]model.TariffTemplate, error)
	Create(ctx context.Context, s Scope, t model.TariffTemplate) (model.TariffTemplate, error)
	Update(ctx context.Context, s Scope, t model.TariffTemplate) (model.TariffTemplate, error)
	Delete(ctx context.Context, s Scope, id uuid.UUID) error
}

// SolarTariffFilter narrows a solar tariff listing.
type SolarTariffFilter struct {
	PlantID        *uuid.UUID
	EffectiveOn    *time.Time
	IncludeDeleted bool
	Page           Page
}

// SolarTariffRepository reads and writes plant feed-in tariffs.
type SolarTariffRepository interface {
	Get(ctx context.Context, s Scope, id uuid.UUID) (model.SolarTariff, error)
	List(ctx context.Context, s Scope, f SolarTariffFilter) ([]model.SolarTariff, error)
	Create(ctx context.Context, s Scope, t model.SolarTariff) (model.SolarTariff, error)
	SoftDelete(ctx context.Context, s Scope, id uuid.UUID, at time.Time) error

	// Effective resolves by effective_from exactly as TariffRepository.Effective does.
	Effective(ctx context.Context, s Scope, plantID uuid.UUID, on time.Time) (model.SolarTariff, error)
}

// NationalTariffFilter narrows a national schedule listing.
type NationalTariffFilter struct {
	UserGroup    *model.DistributionUserGroup
	VoltageLevel *model.VoltageLevel
	Term         *model.TariffTerm
	EffectiveOn  *time.Time
	Page         Page
}

// NationalTariffRepository is a tenant's READ access to the published national
// tariff schedule that backs the public bill calculator.
//
// The table is PLATFORM-WIDE — no company_id — so the Scope narrows nothing
// here either. The schedule is written only through
// AdminCatalogueRepository.UpsertNationalTariffSchedule.
type NationalTariffRepository interface {
	List(ctx context.Context, s Scope, f NationalTariffFilter) ([]model.NationalTariffScheduleEntry, error)
	Effective(ctx context.Context, s Scope, group model.DistributionUserGroup, level model.VoltageLevel, term model.TariffTerm, on time.Time) (model.NationalTariffScheduleEntry, error)
}

// IcmalFilter narrows an icmal import listing.
type IcmalFilter struct {
	Status *string
	Range  *TimeRange
	Page   Page
}

// IcmalRepository reads and writes distributor summary imports.
type IcmalRepository interface {
	CreateImport(ctx context.Context, s Scope, imp model.IcmalImport) (model.IcmalImport, error)
	GetImport(ctx context.Context, s Scope, id uuid.UUID) (model.IcmalImport, error)
	ListImports(ctx context.Context, s Scope, f IcmalFilter) ([]model.IcmalImport, error)

	// UpdateImportResult moves an import through pending -> analysed ->
	// applied|rejected and stores the derived coefficients and warnings.
	UpdateImportResult(ctx context.Context, s Scope, id uuid.UUID, status string, result []byte) (model.IcmalImport, error)

	// InsertRows — Isolation: icmal_rows has no company_id — join through
	// icmal_imports. An import not visible to the Scope returns ErrNotFound.
	// A row's BuildingID, when set, must be a building visible to the Scope;
	// if ANY is not, the whole call is refused with ErrNotFound and nothing
	// is written.
	InsertRows(ctx context.Context, s Scope, importID uuid.UUID, rows []model.IcmalRow) (int64, error)

	// ListRows — Isolation: join icmal_rows through icmal_imports. An import
	// not visible to the Scope returns ErrNotFound.
	ListRows(ctx context.Context, s Scope, importID uuid.UUID, p Page) ([]model.IcmalRow, error)
}

// ---------------------------------------------------------------------------
// Bills (migration 00009)
// ---------------------------------------------------------------------------

// BillFilter narrows a bill listing.
type BillFilter struct {
	IDs        []uuid.UUID
	BuildingID *uuid.UUID
	AnalyzerID *uuid.UUID
	BillScope  *model.BillScope
	PeriodKey  *string
	Statuses   []model.BillStatus
	// IncludeSuperseded lifts the default `status <> 'superseded'`
	// predicate. Superseded bills are hidden by default because the product
	// shows the current bill for a period; audit views set this.
	IncludeSuperseded bool
	Page              Page
}

// BillRepository reads and writes bills and their child collections.
type BillRepository interface {
	Get(ctx context.Context, s Scope, id uuid.UUID) (model.Bill, error)
	List(ctx context.Context, s Scope, f BillFilter) ([]model.Bill, error)

	// Current returns the one non-superseded bill for a scope and period, or
	// ErrNotFound. It is the read the partial unique index exists to make
	// unambiguous.
	Current(ctx context.Context, s Scope, billScope model.BillScope, subjectID uuid.UUID, periodKey string) (model.Bill, error)

	// Create inserts a bill. It returns ErrConflict if a non-superseded bill
	// already exists for the same (scope, subject, period): recomputation
	// goes through Supersede, never through a delete-and-insert.
	//
	// Isolation: bill_lines and bill_members have no company_id and are
	// written under the new bill, which carries s.CompanyID. b's building
	// and analyzer, and every id in members, must be visible to the Scope;
	// if ANY is not, the whole call is refused with ErrNotFound and nothing
	// is written.
	Create(ctx context.Context, s Scope, b model.Bill, lines []model.BillLine, members []uuid.UUID) (model.Bill, error)

	// Supersede is the recomputation path from 04-data-model.md §14: in ONE
	// transaction it marks the existing live bill superseded and inserts the
	// replacement. The old bill and its lines stay readable, which is what
	// makes a disputed invoice explainable months later.
	//
	// Isolation: replacing must be a bill visible to the Scope, or the call
	// returns ErrNotFound and changes nothing; the replacement's lines and
	// members follow Create's rule.
	Supersede(ctx context.Context, s Scope, replacing uuid.UUID, b model.Bill, lines []model.BillLine, members []uuid.UUID, at time.Time) (model.Bill, error)

	// UpdateStatus moves a bill between draft, issued and flagged. It cannot
	// set superseded — that transition belongs to Supersede, which also
	// inserts the replacement, and allowing it here would let a period end up
	// with no live bill at all.
	UpdateStatus(ctx context.Context, s Scope, id uuid.UUID, status model.BillStatus, flagReason *string, at time.Time) (model.Bill, error)

	// SetPDFPath records where the rendered invoice was stored.
	SetPDFPath(ctx context.Context, s Scope, id uuid.UUID, path string) error

	// Lines — Isolation: bill_lines has no company_id — join through bills.
	// A bill not visible to the Scope returns ErrNotFound.
	Lines(ctx context.Context, s Scope, billID uuid.UUID) ([]model.BillLine, error)

	// Members — Isolation: bill_members has no company_id — join through
	// bills. A bill not visible to the Scope returns ErrNotFound.
	Members(ctx context.Context, s Scope, billID uuid.UUID) ([]model.BillMember, error)

	// HourlyDetail — Isolation: bill_hourly_detail has no company_id — join
	// through bills. A bill not visible to the Scope returns ErrNotFound.
	HourlyDetail(ctx context.Context, s Scope, billID uuid.UUID) ([]model.BillHourlyDetail, error)

	// ReplaceHourlyDetail — Isolation: join bill_hourly_detail through bills.
	// A bill not visible to the Scope returns ErrNotFound and nothing is
	// replaced.
	ReplaceHourlyDetail(ctx context.Context, s Scope, billID uuid.UUID, rows []model.BillHourlyDetail) (int64, error)
}

// ---------------------------------------------------------------------------
// Reports (migration 00010)
// ---------------------------------------------------------------------------

// ReportFilter narrows a report listing.
type ReportFilter struct {
	BuildingID *uuid.UUID
	Type       *model.ReportType
	Period     *string
	Statuses   []model.ReportStatus
	Page       Page
}

// ReportRepository reads and writes generated building reports.
type ReportRepository interface {
	Get(ctx context.Context, s Scope, id uuid.UUID) (model.Report, error)
	List(ctx context.Context, s Scope, f ReportFilter) ([]model.Report, error)

	// Upsert is keyed on (building_id, type, period), which is unique:
	// regenerating a period REPLACES its report rather than accumulating a
	// second one. Reports are a rendering, not a financial record — bills are
	// the thing that supersedes rather than replaces.
	Upsert(ctx context.Context, s Scope, r model.Report) (model.Report, error)

	// UpdateStatus records completion or failure. errorMessage is
	// operator-facing text and must already be scrubbed.
	UpdateStatus(ctx context.Context, s Scope, id uuid.UUID, status model.ReportStatus, errorMessage *string, at time.Time) (model.Report, error)
}

// ---------------------------------------------------------------------------
// Alarms (migration 00011)
// ---------------------------------------------------------------------------

// AlarmFilter narrows an alarm listing.
type AlarmFilter struct {
	IDs   []uuid.UUID
	Types []model.AlarmType
	// AnalyzerID keeps only alarms attached to that analyzer, through
	// alarm_analyzers.
	AnalyzerID     *uuid.UUID
	IsEnabled      *bool
	IncludeDeleted bool
	Page           Page
}

// AlarmEventFilter narrows an alarm event listing.
type AlarmEventFilter struct {
	AlarmID    *uuid.UUID
	AnalyzerID *uuid.UUID
	// Undelivered, when true, keeps only events with a NULL notified_at.
	Undelivered bool
	// Notified, when non-nil, keeps only events whose notified_at is set
	// (true) or NULL (false). Undelivered stays for the NULL-only case it
	// already serves; Notified answers the opposite question F7's frequency
	// suppression asks (R218): was anyone actually told, and when?
	Notified *bool
	Range    *TimeRange
	Page     Page
}

// AlarmRepository reads and writes alarm rules, their attachments and their
// firing history.
type AlarmRepository interface {
	Get(ctx context.Context, s Scope, id uuid.UUID) (model.Alarm, error)
	List(ctx context.Context, s Scope, f AlarmFilter) ([]model.Alarm, error)
	Create(ctx context.Context, s Scope, a model.Alarm) (model.Alarm, error)
	Update(ctx context.Context, s Scope, a model.Alarm) (model.Alarm, error)
	SoftDelete(ctx context.Context, s Scope, id uuid.UUID, at time.Time) error

	// Analyzers — Isolation: alarm_analyzers has no company_id — join through
	// alarms. An alarm not visible to the Scope returns ErrNotFound.
	Analyzers(ctx context.Context, s Scope, alarmID uuid.UUID) ([]model.AlarmAnalyzer, error)

	// ReplaceAnalyzers — Isolation: join alarm_analyzers through alarms. An
	// alarm not visible to the Scope returns ErrNotFound; if ANY of
	// analyzerIDs is not visible to the Scope the whole call is refused with
	// ErrNotFound. Either way nothing is replaced.
	ReplaceAnalyzers(ctx context.Context, s Scope, alarmID uuid.UUID, analyzerIDs []uuid.UUID) error

	// Channels — Isolation: alarm_channels has no company_id — join through
	// alarms. An alarm not visible to the Scope returns ErrNotFound.
	Channels(ctx context.Context, s Scope, alarmID uuid.UUID) ([]model.AlarmChannel, error)

	// ReplaceChannels — Isolation: join alarm_channels through alarms. An
	// alarm not visible to the Scope returns ErrNotFound and nothing is
	// replaced.
	ReplaceChannels(ctx context.Context, s Scope, alarmID uuid.UUID, channels []model.AlarmChannel) error

	// CreateEvent — Isolation: alarm_events has no company_id — join through
	// alarms on e.AlarmID. An alarm not visible to the Scope, or an
	// e.AnalyzerID not visible to it, is refused with ErrNotFound.
	CreateEvent(ctx context.Context, s Scope, e model.AlarmEvent) (model.AlarmEvent, error)

	// ListEvents — Isolation: join alarm_events through alarms; events of
	// alarms not visible to the Scope are never listed, whatever f names.
	ListEvents(ctx context.Context, s Scope, f AlarmEventFilter) ([]model.AlarmEvent, error)

	// MarkNotified records delivery. A non-nil notificationError with a nil
	// notified_at is a real state: the alarm fired and nobody was told.
	//
	// Isolation: join alarm_events through alarms by eventID — never
	// `where id = $1` on alarm_events alone. An event whose alarm is not
	// visible to the Scope returns ErrNotFound and nothing is written.
	MarkNotified(ctx context.Context, s Scope, eventID uuid.UUID, at time.Time, notificationError *string) error

	// MarkBillFired claims a (alarm, bill) pair. It returns false if the pair
	// already existed, which is how an invoice is stopped from firing the
	// same alarm twice after a recomputation. Claiming and checking is ONE
	// call precisely so that two workers cannot both pass a check and both
	// fire.
	//
	// Isolation: alarm_fired_bills has no company_id — join through BOTH
	// alarms and bills. If either is not visible to the Scope the call
	// returns ErrNotFound and claims nothing.
	MarkBillFired(ctx context.Context, s Scope, alarmID, billID uuid.UUID) (claimed bool, err error)

	// MarkIsolarForwarded is the same claim-once idiom for iSolar alarm
	// forwarding, keyed on (plant_id, alarm_ref).
	//
	// Isolation: isolar_forwarded_alarms has no company_id — join through
	// power_plants. A plant not visible to the Scope returns ErrNotFound and
	// claims nothing.
	MarkIsolarForwarded(ctx context.Context, s Scope, plantID uuid.UUID, alarmRef string, at time.Time) (claimed bool, err error)
}

// ---------------------------------------------------------------------------
// Carbon and ISO 50001 (migration 00007)
// ---------------------------------------------------------------------------

// EmissionFactorFilter narrows a factor listing.
type EmissionFactorFilter struct {
	Keys         []string
	MainCategory *string
	Scope        *model.CarbonScope
	// IncludePlatform includes the master catalogue rows (company_id NULL)
	// alongside the company's own. It defaults to false so that a caller
	// listing "our factors" gets exactly those.
	IncludePlatform bool
	Page            Page
}

// CarbonActivityFilter narrows an activity listing.
type CarbonActivityFilter struct {
	BuildingIDs  []uuid.UUID
	Scopes       []model.CarbonScope
	Statuses     []model.CarbonStatus
	ActivityType *string
	// Period bounds on the period_start/period_end DATE columns.
	PeriodFrom  *time.Time
	PeriodTo    *time.Time
	IsAutomated *bool
	Page        Page
}

// CarbonRepository reads and writes the emission factor catalogue and the
// recorded activities computed from it.
//
// A factor is VISIBLE to a Scope when it is a platform factor (company_id NULL)
// or belongs to s.CompanyID; it is WRITABLE only when it belongs to
// s.CompanyID. Platform factors and their conversions are written through
// AdminCatalogueRepository.
type CarbonRepository interface {
	// Factor returns a platform factor or one of s.CompanyID's own. Another
	// company's factor returns ErrNotFound.
	Factor(ctx context.Context, s Scope, id uuid.UUID) (model.EmissionFactor, error)

	// ListFactors returns s.CompanyID's own factors, and the platform
	// catalogue too when f.IncludePlatform is set. Another company's factors
	// are never listed.
	ListFactors(ctx context.Context, s Scope, f EmissionFactorFilter) ([]model.EmissionFactor, error)

	// UpsertFactor writes a COMPANY-OWNED factor, keyed on
	// (coalesce(company_id, zero uuid), key), the table's unique index: a
	// company may shadow a platform key with its own factor but cannot have
	// two of its own.
	//
	// The stored company_id is s.CompanyID. A factor whose CompanyID is nil
	// (a platform factor) or names another company is refused with
	// ErrNotFound, and so is an f.ID naming an existing factor that is not
	// s.CompanyID's own. Upserting a key that exists in the platform
	// catalogue creates the company's shadowing row and never modifies the
	// platform row.
	UpsertFactor(ctx context.Context, s Scope, f model.EmissionFactor) (model.EmissionFactor, error)

	// Conversions — Isolation: emission_factor_conversions has no company_id
	// — join through emission_factors. Readable for a platform factor or one
	// of s.CompanyID's own; another company's factorID returns ErrNotFound.
	Conversions(ctx context.Context, s Scope, factorID uuid.UUID) ([]model.EmissionFactorConversion, error)

	// ReplaceConversions — Isolation: join emission_factor_conversions
	// through emission_factors, which must belong to s.CompanyID. A platform
	// factor's id or another company's is refused with ErrNotFound and
	// nothing is replaced; platform conversions are
	// AdminCatalogueRepository.ReplacePlatformConversions.
	ReplaceConversions(ctx context.Context, s Scope, factorID uuid.UUID, conversions []model.EmissionFactorConversion) error

	SelectedActivities(ctx context.Context, s Scope, buildingID uuid.UUID) ([]model.CarbonSelectedActivity, error)
	ReplaceSelectedActivities(ctx context.Context, s Scope, buildingID uuid.UUID, keys []string) error

	Activity(ctx context.Context, s Scope, id uuid.UUID) (model.CarbonActivity, error)
	ListActivities(ctx context.Context, s Scope, f CarbonActivityFilter) ([]model.CarbonActivity, error)
	CreateActivity(ctx context.Context, s Scope, a model.CarbonActivity) (model.CarbonActivity, error)
	UpdateActivity(ctx context.Context, s Scope, a model.CarbonActivity) (model.CarbonActivity, error)
	DeleteActivity(ctx context.Context, s Scope, id uuid.UUID) error

	// UpsertAutomatedActivity is the derived-from-meter-data path, keyed on
	// the partial unique index (building_id, activity_type, period_start)
	// where is_automated. A daily recomputation must converge on one row.
	UpsertAutomatedActivity(ctx context.Context, s Scope, a model.CarbonActivity) (model.CarbonActivity, error)

	SetActivityStatus(ctx context.Context, s Scope, id uuid.UUID, status model.CarbonStatus, at time.Time) (model.CarbonActivity, error)

	CreateReport(ctx context.Context, s Scope, r model.CarbonReport) (model.CarbonReport, error)
	ListReports(ctx context.Context, s Scope, buildingID *uuid.UUID, p Page) ([]model.CarbonReport, error)
}

// ISO50001Repository reads and writes a building's ISO 50001 programme.
type ISO50001Repository interface {
	// Project returns a building's project, or ErrNotFound.
	Project(ctx context.Context, s Scope, buildingID uuid.UUID) (model.ISO50001Project, error)

	// EnsureProject returns the building's project, creating it if it does
	// not exist. iso50001_projects has a unique index on building_id, so this
	// is an upsert rather than a check-then-insert that could race.
	EnsureProject(ctx context.Context, s Scope, buildingID uuid.UUID) (model.ISO50001Project, error)

	// ClauseDates — Isolation: iso50001_clause_dates has no company_id — join
	// through iso50001_projects. A project not visible to the Scope returns
	// ErrNotFound.
	ClauseDates(ctx context.Context, s Scope, projectID uuid.UUID) ([]model.ISO50001ClauseDate, error)

	// ReplaceClauseDates — Isolation: join iso50001_clause_dates through
	// iso50001_projects. A project not visible to the Scope returns
	// ErrNotFound and nothing is replaced.
	ReplaceClauseDates(ctx context.Context, s Scope, projectID uuid.UUID, dates []model.ISO50001ClauseDate) error

	// Notes — Isolation: iso50001_notes has no company_id — join through
	// iso50001_projects. A project not visible to the Scope returns
	// ErrNotFound.
	Notes(ctx context.Context, s Scope, projectID uuid.UUID, clauseID *string) ([]model.ISO50001Note, error)

	// CreateNote — Isolation: join through iso50001_projects on n.ProjectID.
	// A project not visible to the Scope, or an n.CreatedBy that is not a
	// user of s.CompanyID, is refused with ErrNotFound.
	CreateNote(ctx context.Context, s Scope, n model.ISO50001Note) (model.ISO50001Note, error)

	// UpdateNote — Isolation: join iso50001_notes through iso50001_projects
	// by n.ID — never `where id = $1` on the note alone. A note not visible
	// to the Scope returns ErrNotFound and nothing is written. project_id is
	// not updated: a note stays under the project it was created in, so an
	// update cannot move it into another tenant's project.
	UpdateNote(ctx context.Context, s Scope, n model.ISO50001Note) (model.ISO50001Note, error)

	// DeleteNote — Isolation: join iso50001_notes through iso50001_projects
	// by id. A note not visible to the Scope returns ErrNotFound and nothing
	// is deleted.
	DeleteNote(ctx context.Context, s Scope, id uuid.UUID) error
}

// ---------------------------------------------------------------------------
// Files, integrations, calendar and operations (migration 00008)
// ---------------------------------------------------------------------------

// FileFilter narrows a stored file listing.
type FileFilter struct {
	OwnerType      *string
	OwnerID        *uuid.UUID
	ClauseID       *string
	IncludeDeleted bool
	Page           Page
}

// FileRepository reads and writes stored file METADATA. The bytes live in
// object storage; nothing here reads or writes them.
type FileRepository interface {
	Get(ctx context.Context, s Scope, id uuid.UUID) (model.StoredFile, error)
	List(ctx context.Context, s Scope, f FileFilter) ([]model.StoredFile, error)
	Create(ctx context.Context, s Scope, f model.StoredFile) (model.StoredFile, error)

	// SoftDelete marks the metadata deleted. Reclaiming the bytes is a
	// separate, later job, so that a mistaken delete is recoverable.
	SoftDelete(ctx context.Context, s Scope, id uuid.UUID, at time.Time) error
}

// IntegrationRepository reads the provider catalogue and reads and writes each
// company's credentials for it.
//
// The plaintext/ciphertext split is the whole design here. Get and List return
// model.IntegrationCredential, which holds CIPHERTEXT and has no plaintext
// field at all; a caller that genuinely needs the secret asks OpenSecret,
// which decrypts on the spot with internal/platform/crypto.
// TestIntegrationCredentialsAreNeverReturnedInPlaintext pins that the ordinary
// read never carries it.
type IntegrationRepository interface {
	// Definitions is a tenant's read of the platform catalogue: no
	// company_id, so the Scope narrows nothing. The catalogue is written only
	// through AdminCatalogueRepository.UpsertIntegrationDefinitions.
	Definitions(ctx context.Context, s Scope) ([]model.IntegrationDefinition, error)
	Definition(ctx context.Context, s Scope, provider model.IntegrationProvider, subtype string) (model.IntegrationDefinition, error)

	Credential(ctx context.Context, s Scope, definitionID uuid.UUID) (model.IntegrationCredential, error)
	ListCredentials(ctx context.Context, s Scope) ([]model.IntegrationCredential, error)

	// UpsertCredential seals the supplied plaintext secrets and stores the
	// ciphertext. The plaintext is a parameter and never a struct field, so
	// it cannot be accidentally retained, logged or returned.
	UpsertCredential(ctx context.Context, s Scope, c model.IntegrationCredential, secret, extra []byte) (model.IntegrationCredential, error)

	// OpenSecret decrypts one credential's secrets. It is the ONLY method
	// that returns plaintext, and its name says so at every call site.
	OpenSecret(ctx context.Context, s Scope, credentialID uuid.UUID) (secret, extra []byte, err error)

	DeleteCredential(ctx context.Context, s Scope, credentialID uuid.UUID) error

	// RecordVerification stamps last_verified_at and token_expires_at after a
	// successful provider handshake.
	RecordVerification(ctx context.Context, s Scope, credentialID uuid.UUID, verifiedAt time.Time, tokenExpiresAt *time.Time) error
}

// SMTPRepository reads and writes a company's outbound mail settings.
//
// smtp_settings' primary key IS company_id, so there is one row per company
// and no id parameter anywhere here.
type SMTPRepository interface {
	Get(ctx context.Context, s Scope) (model.SMTPSettings, error)

	// Upsert seals the supplied plaintext password. As with
	// IntegrationRepository.UpsertCredential, the plaintext is a parameter
	// rather than a field of the struct.
	//
	// A nil or empty password on an UPDATE of an existing row leaves the
	// stored password unchanged -- every other field still updates. A nil or
	// empty password on the FIRST Upsert for a company (no existing row) is
	// refused with an error and writes no row: smtp_settings.password_enc is
	// NOT NULL, so an SMTP configuration without a password is not storable.
	Upsert(ctx context.Context, s Scope, settings model.SMTPSettings, password []byte) (model.SMTPSettings, error)

	// OpenPassword decrypts the stored password. The only method that returns
	// it.
	OpenPassword(ctx context.Context, s Scope) ([]byte, error)

	Delete(ctx context.Context, s Scope) error
}

// CalendarFilter narrows a calendar event listing.
type CalendarFilter struct {
	Range *TimeRange
	Page  Page
}

// CalendarRepository reads and writes a company's calendar, weekend days and
// vacations — the three things that decide which days count as working days.
type CalendarRepository interface {
	Event(ctx context.Context, s Scope, id uuid.UUID) (model.CalendarEvent, error)
	ListEvents(ctx context.Context, s Scope, f CalendarFilter) ([]model.CalendarEvent, error)
	CreateEvent(ctx context.Context, s Scope, e model.CalendarEvent) (model.CalendarEvent, error)
	UpdateEvent(ctx context.Context, s Scope, e model.CalendarEvent) (model.CalendarEvent, error)
	DeleteEvent(ctx context.Context, s Scope, id uuid.UUID) error

	WeekendDays(ctx context.Context, s Scope) ([]model.CompanyWeekendDay, error)
	// ReplaceWeekendDays takes days 0..6 with 0 = Sunday, matching both the
	// schema's check constraint and time.Weekday.
	ReplaceWeekendDays(ctx context.Context, s Scope, days []int16) error

	// Vacations returns every vacation when r is nil, and ErrInvalidRange
	// for a non-nil invalid one.
	Vacations(ctx context.Context, s Scope, r *TimeRange) ([]model.CompanyVacation, error)
	CreateVacation(ctx context.Context, s Scope, v model.CompanyVacation) (model.CompanyVacation, error)
	DeleteVacation(ctx context.Context, s Scope, id uuid.UUID) error
}

// JobRunFilter narrows a job run listing.
type JobRunFilter struct {
	JobType  *string
	Statuses []string
	// Running, when true, keeps only runs with a NULL finished_at.
	Running bool
	Range   *TimeRange
	Page    Page
}

// MessageFilter narrows an operational message listing.
type MessageFilter struct {
	Kinds       []string
	Categories  []string
	Statuses    []string
	RelatedType *string
	RelatedID   *uuid.UUID
	// Q is a case-insensitive SUBSTRING matched against message and detail
	// (R226). Not full-text: neither column is indexed for it, and the screen's
	// window is one tenant's recent rows. The query escapes `\`, `%` and `_`,
	// so a metacharacter searches for itself rather than matching everything.
	// An empty Q filters nothing. The service caps the length at 200.
	Q     string
	Range *TimeRange
	Page  Page
}

// OpsRepository backs a tenant's Messages screen and job diagnostics.
//
// Nothing written through here may contain a DSN, a password or any other
// secret: both job_runs.error and operational_messages.detail are shown to
// operators and exported. The store layer's scrubbing helpers run before a
// message reaches these methods, not inside them.
//
// job_runs.company_id and operational_messages.company_id are nullable, for
// platform work. Every method here stores and sees only company_id =
// s.CompanyID, never a NULL row. A platform job's run and messages are written
// through AdminJournalRepository.
type OpsRepository interface {
	// StartRun inserts a job_runs row in the 'running' state and returns it,
	// so the caller holds the id it will finish. The stored company_id is
	// s.CompanyID; a run whose CompanyID is nil (a platform run) or names
	// another company is refused with ErrNotFound.
	StartRun(ctx context.Context, s Scope, run model.JobRun) (model.JobRun, error)

	// FinishRun stamps finished_at, the final status and the three counts.
	// The counts are explicit rather than derived: the spec's job model
	// requires processed/skipped/failed and a job that cannot say how many
	// rows it skipped cannot be trusted to have skipped them deliberately.
	//
	// Only a run with company_id = s.CompanyID can be finished here; a
	// platform run's id or another company's returns ErrNotFound.
	FinishRun(ctx context.Context, s Scope, id uuid.UUID, status string, processed, skipped, failed int32, errText *string, detail []byte, at time.Time) (model.JobRun, error)

	GetRun(ctx context.Context, s Scope, id uuid.UUID) (model.JobRun, error)
	ListRuns(ctx context.Context, s Scope, f JobRunFilter) ([]model.JobRun, error)

	// AppendMessage stores s.CompanyID as company_id; a message whose
	// CompanyID is nil (a platform message) or names another company is
	// refused with ErrNotFound.
	AppendMessage(ctx context.Context, s Scope, m model.OperationalMessage) (model.OperationalMessage, error)
	ListMessages(ctx context.Context, s Scope, f MessageFilter) ([]model.OperationalMessage, error)
}

// ---------------------------------------------------------------------------
// F2 integration layer (migrations 00012 and 00013)
// ---------------------------------------------------------------------------

// GenerationRepository reads and writes generation_anchors — the last known
// cumulative export register value for one analyzer, which PM5340's interval
// energy (MeterReading.IntervalGenerationKwh) is reconciled against.
//
// Isolation: generation_anchors has no company_id; every method joins
// through analyzers, per repository.go's "ROWS WITHOUT company_id" header.
type GenerationRepository interface {
	// Anchor returns ErrNotFound when the analyzer has no anchor yet, OR is
	// not visible to s — the two are indistinguishable from outside the
	// company, by design.
	Anchor(ctx context.Context, s Scope, analyzerID uuid.UUID) (model.GenerationAnchor, error)

	// SetAnchor upserts on analyzer_id, as a single atomic statement. The
	// analyzer's visibility is validated IN THE WRITE STATEMENT ITSELF (the
	// F1 ba2fb97 pattern: `insert … select … from analyzers where … and
	// company_id = $n and deleted_at is null and (all_buildings or
	// building_id = any(building_ids))`), never by a Go-side
	// Scope.AllowsBuilding pre-check against the stored analyzer_id — that
	// would be using AllowsBuilding to validate a stored foreign key, which
	// it cannot do (see Scope.AllowsBuilding's own doc). Not visible →
	// ErrNotFound, nothing written.
	SetAnchor(ctx context.Context, s Scope, a model.GenerationAnchor) error
}

// ProviderSeriesRepository reads and writes provider_hourly_values — OSOS's
// own labelled hourly cross-check series (06 §2, removed-behaviour 23).
// NEVER read by consumption or billing; it exists only so an operator can
// compare it against meter_readings' own figures.
//
// Isolation: provider_hourly_values has no company_id; every method joins
// through analyzers.
type ProviderSeriesRepository interface {
	// UpsertHourly writes rows idempotently, keyed on (analyzer_id, ts): the
	// same COPY-into-staging-then-upsert shape ReadingRepository.BulkInsert
	// uses, for the same reason (a per-call temporary table cannot appear in
	// sqlc's schema catalogue).
	//
	// Isolation: the batch's distinct analyzer ids are locked FOR SHARE and
	// checked against s in one transaction, exactly once per batch — this
	// IS the tenant predicate for this write, not a predicate embedded in
	// the upsert SQL itself (unlike SetAnchor, which writes exactly one
	// analyzer's row and so embeds its own check). Fewer visible ids than
	// distinct input ids refuses the WHOLE batch with ErrNotFound and writes
	// nothing.
	//
	// A duplicate (analyzer_id, ts) key WITHIN the caller's own batch is
	// refused before any database round trip, with an error for which
	// errors.Is(err, ErrConflict) is true, naming the key; nothing is
	// written — the same defence ReadingRepository.BulkInsert uses against
	// `on conflict … do update` silently arbitrating between two batched
	// rows.
	UpsertHourly(ctx context.Context, s Scope, rows []model.ProviderHourlyValue) (inserted, updated int, err error)

	// HourlyRange returns one analyzer's rows over the half-open window r.
	// An invalid r returns ErrInvalidRange before any database call. Join
	// through analyzers, like every other method here: an analyzer not
	// visible to s returns ErrNotFound; a visible one with nothing in r
	// returns an empty slice.
	HourlyRange(ctx context.Context, s Scope, analyzerID uuid.UUID, r TimeRange) ([]model.ProviderHourlyValue, error)
}

// ===========================================================================
// UNSCOPED ADMIN INTERFACES
// Implemented ONLY by package internal/store/postgres/admin.
// ===========================================================================
//
// Nothing below takes a Scope. That is not an omission; it is the reason these
// interfaces are separate from everything above. Each method exists because its
// caller provably has no tenant to offer — a login that has not yet produced a
// Scope, or platform work that belongs to no company — and each says so.
//
// They are implemented only by internal/store/postgres/admin, the single
// package internal/arch's TestEveryStoreMethodIsScoped exempts. That guard
// walks internal/store/postgres/... and fails on any exported context-taking
// method without a Scope, so an implementation placed anywhere else in that
// tree is red. It does NOT see code outside internal/store/postgres, so
// keeping every implementation confined to internal/store/postgres/admin is
// a closed-list discipline this package's own structure must maintain.
// Product request handling never depends on these interfaces except where a
// method names its caller.
//
// Platform writes store company_id NULL. A model value whose CompanyID is
// non-nil names a tenant row, which this surface does not write: it is refused
// with ErrNotFound and nothing is written. Likewise a platform method handed
// the id of a tenant's row (a tenant's job run, a company-owned factor) returns
// ErrNotFound: from here, tenant rows do not exist.
//
// The interfaces are grouped by the task that implements them, one interface
// per group, so that parallel work never edits the same declaration. Keep each
// group in its own file under internal/store/postgres/admin (auth.go, audit.go,
// marketdata.go, catalogue.go, journal.go, ingestion.go, aggregates.go).
//
// THE LIST IS CLOSED. A method is added here only when its caller cannot hold
// a Scope, never because holding one is inconvenient; every method added is a
// permanent unscoped path into the database.

// AdminAuthRepository resolves the two credentials a request presents before it
// has a tenant.
type AdminAuthRepository interface {
	// UserByEmail resolves a login. It cannot take a Scope because the Scope
	// is what a successful login PRODUCES: the request carries an email and a
	// password and nothing else. users.email is unique across the platform
	// (the unique index on users(email) where deleted_at is null), so the
	// email alone identifies the user and, through them, the tenant.
	//
	// The match is case-insensitive (email is citext). A soft-deleted user,
	// or a user whose company is soft-deleted, returns ErrNotFound. An
	// inactive user IS returned: IsActive is the caller's to refuse, and the
	// caller must refuse it with the same response as a wrong password.
	UserByEmail(ctx context.Context, email string) (model.User, error)

	// SessionByRefreshTokenHash resolves a refresh. It cannot take a Scope
	// for the same reason: a refresh request carries only the token.
	// sessions.refresh_token_hash is unique across the platform. The
	// plaintext token never reaches this layer; hash is the hash.
	//
	// It returns the owning user with the session because sessions has no
	// company_id: without the user's CompanyID the caller could not build
	// the Scope every following call needs, and would need a second unscoped
	// lookup to get it.
	//
	// Revoked and expired sessions ARE returned. Rejecting them is the
	// caller's job, and only a returned revoked session lets it tell a
	// replayed token from an unknown one (see SessionRepository.Revoke). A
	// session whose user or whose user's company is soft-deleted returns
	// ErrNotFound.
	SessionByRefreshTokenHash(ctx context.Context, hash string) (model.Session, model.User, error)

	// PasswordResetByTokenHash resolves a password reset, which carries only
	// the token (R148). Used and expired tokens ARE returned; a token whose
	// user or company is soft-deleted returns ErrNotFound.
	PasswordResetByTokenHash(ctx context.Context, hash string) (model.PasswordReset, model.User, error)
}

// AdminTenantRepository is the platform operator's view across tenants.
type AdminTenantRepository interface {
	// ListCompanies lists every company for the admin role's company list and
	// switcher (05 §3 GET /companies). It cannot take a Scope: its whole
	// purpose is to show companies other than the caller's own.
	ListCompanies(ctx context.Context, f CompanyFilter) ([]model.Company, error)
}

// SectorFigures is one sector peer's inputs to the comparison (R162): no name
// and no company, only figures.
type SectorFigures struct {
	BuildingID     uuid.UUID
	PersonnelCount *int32
	TotalAreaM2    *decimal.Decimal
	// DailySum is consumption over the daily window across DaysWithData days;
	// nil when the building has no readings there.
	DailySum     *decimal.Decimal
	DaysWithData int32
	// MonthlySum is consumption over the month window; nil without readings.
	MonthlySum *decimal.Decimal
}

// AdminSectorRepository reads sector peers across tenants for the sectoral
// comparison (02 §10.4). It cannot take a Scope: the peers ARE other tenants'
// buildings. It returns figures only; the service must never expose a peer's
// id, name or company.
type AdminSectorRepository interface {
	// SectorFigures returns every live building of a live company whose
	// trimmed, case-folded sector equals sector, with consumption over daily
	// and month. Invalid ranges return ErrInvalidRange before any I/O.
	SectorFigures(ctx context.Context, sector string, daily, month TimeRange) ([]SectorFigures, error)
}

// AdminAuditRepository appends platform audit rows.
type AdminAuditRepository interface {
	// AppendPlatform appends an audit row with company_id NULL: an action
	// by the platform or an operator that belongs to no tenant (a catalogue
	// update, a market-data import). It cannot take a Scope because a
	// Scope's CompanyID is never Nil — there is no Scope that means "no
	// company" — and borrowing a tenant's would attribute a platform action
	// to that tenant. A tenant's action is AuditRepository.Append.
	//
	// An entry whose CompanyID is non-nil is refused with ErrNotFound.
	AppendPlatform(ctx context.Context, e model.AuditEntry) (model.AuditEntry, error)
}

// AdminMarketDataRepository writes the platform-wide market series that every
// tenant's bills are priced from.
//
// None of it can take a Scope: the prices are published for the whole market,
// the ingestion job that fetches them acts for no tenant, and any Scope that
// authorised the write would let one tenant's credentials reprice every
// tenant's invoices. Tenants read them through PriceRepository.
type AdminMarketDataRepository interface {
	// UpsertHourlyPrices writes market_prices_hourly, keyed on its ts primary
	// key, and returns the number of rows written. Re-importing a day
	// converges rather than duplicating.
	//
	// Refused, before any database round trip, with an error for which
	// errors.Is(err, ErrConflict) is true: prices containing a zero Ts, or
	// two entries sharing the same Ts. A malformed import must not silently
	// pick a winner between two conflicting prices for the same hour.
	UpsertHourlyPrices(ctx context.Context, prices []model.MarketPrice) (int64, error)

	// UpsertYekdem writes yekdem_monthly, keyed on its (year, month) primary
	// key, and returns the number of rows written.
	//
	// Refused, before any database round trip, with an error for which
	// errors.Is(err, ErrConflict) is true: values containing a Month outside
	// 1..12, or two entries sharing the same (Year, Month).
	UpsertYekdem(ctx context.Context, values []model.YekdemMonthly) (int64, error)
}

// AdminCatalogueRepository writes the platform reference catalogues. The
// seed loader is its main caller.
//
// None of it can take a Scope: the catalogues are shared by every tenant and
// maintained by the platform, so a Scope would identify no owner — and any
// Scope that authorised the write would let one tenant rewrite what every
// tenant reads. Tenants read them through NationalTariffRepository,
// CarbonRepository and IntegrationRepository.
type AdminCatalogueRepository interface {
	// UpsertNationalTariffSchedule writes national_tariff_schedule, keyed on
	// its unique (effective_from, user_group, voltage_level, term), and
	// returns the number of rows written.
	UpsertNationalTariffSchedule(ctx context.Context, entries []model.NationalTariffScheduleEntry) (int64, error)

	// UpsertPlatformFactor writes a PLATFORM emission factor (company_id
	// NULL), keyed on (coalesce(company_id, zero uuid), key). It never
	// touches a company-owned factor, including one that shadows the same
	// key: seeding must not overwrite a tenant's override. A factor whose
	// CompanyID is non-nil is refused with ErrNotFound. The scoped sibling
	// is CarbonRepository.UpsertFactor.
	UpsertPlatformFactor(ctx context.Context, f model.EmissionFactor) (model.EmissionFactor, error)

	// ReplacePlatformConversions swaps the conversions of a PLATFORM factor
	// in one transaction. A company-owned factor's id returns ErrNotFound and
	// nothing is replaced. The scoped sibling is
	// CarbonRepository.ReplaceConversions.
	ReplacePlatformConversions(ctx context.Context, factorID uuid.UUID, conversions []model.EmissionFactorConversion) error

	// UpsertIntegrationDefinitions writes integration_definitions, keyed on
	// its unique (provider, subtype), and returns the number of rows written.
	UpsertIntegrationDefinitions(ctx context.Context, defs []model.IntegrationDefinition) (int64, error)

	// CreateIntegrationDefinition, UpdateIntegrationDefinition and
	// DeleteIntegrationDefinition are the admin role's catalogue editing
	// (05 §15). A duplicate (provider, subtype) is ErrConflict; deleting a
	// definition a credential still references is ErrConflict.
	CreateIntegrationDefinition(ctx context.Context, d model.IntegrationDefinition) (model.IntegrationDefinition, error)
	UpdateIntegrationDefinition(ctx context.Context, d model.IntegrationDefinition) (model.IntegrationDefinition, error)
	DeleteIntegrationDefinition(ctx context.Context, id uuid.UUID) error

	// UpsertBillingParameters writes one dated billing_parameters row keyed on
	// effective_from (R106).
	UpsertBillingParameters(ctx context.Context, p model.BillingParameters) (model.BillingParameters, error)
}

// BillableBuilding is one building the billing dispatcher invoices.
type BillableBuilding struct {
	CompanyID, BuildingID uuid.UUID
	CutoffDay             int
}

// AdminBillingRepository is the billing dispatcher's cross-tenant discovery.
type AdminBillingRepository interface {
	// BillableBuildings is every non-deleted building of a non-deleted company
	// with at least one non-deleted analyzer.
	BillableBuildings(ctx context.Context) ([]BillableBuilding, error)
}

// AdminAlarmRepository finds the tenants an alarm tick has work for. Like
// every other Admin* interface it cannot take a Scope: the dispatcher acts for
// no single tenant, and asking each company in turn would write an empty
// job_run and a summary message per company per hour (R219's noise, at scale).
type AdminAlarmRepository interface {
	// CompaniesWithEnabledAlarms returns the id of every live company that has
	// at least one enabled, non-deleted alarm rule.
	CompaniesWithEnabledAlarms(ctx context.Context) ([]uuid.UUID, error)
}

// AdminJournalRepository records platform jobs — work that runs for no tenant,
// such as the market price import — in the same job_runs and
// operational_messages tables tenants' jobs use, with company_id NULL.
//
// None of it can take a Scope: the schema says platform work is company_id
// NULL, a Scope's CompanyID is never Nil, and a platform job made to borrow a
// tenant's Scope would file its failures on that tenant's Messages screen. The
// scoped siblings are OpsRepository.StartRun, FinishRun and AppendMessage.
type AdminJournalRepository interface {
	// StartPlatformRun inserts a job_runs row with company_id NULL in the
	// 'running' state. A run whose CompanyID is non-nil is refused with
	// ErrNotFound.
	StartPlatformRun(ctx context.Context, run model.JobRun) (model.JobRun, error)

	// FinishPlatformRun stamps a PLATFORM run exactly as
	// OpsRepository.FinishRun stamps a tenant's. A tenant run's id returns
	// ErrNotFound and nothing is written, so this path cannot rewrite a
	// tenant's journal.
	FinishPlatformRun(ctx context.Context, id uuid.UUID, status string, processed, skipped, failed int32, errText *string, detail []byte, at time.Time) (model.JobRun, error)

	// AppendPlatformMessage appends an operational message with company_id
	// NULL. A message whose CompanyID is non-nil is refused with ErrNotFound.
	// As with OpsRepository, the text must already be scrubbed.
	AppendPlatformMessage(ctx context.Context, m model.OperationalMessage) (model.OperationalMessage, error)
}

// AdminIngestionRepository lists the credentials the scheduled dispatcher
// must fan out to. It cannot take a Scope: the dispatcher acts for no
// tenant and must see every tenant's active credentials in one pass, the
// same reason AdminMarketDataRepository and AdminCatalogueRepository cannot
// take one.
type AdminIngestionRepository interface {
	// ActiveCredentials returns every is_active credential of a non-deleted
	// company, ordered by (company_id, credential id). No secret column
	// (secret_enc, extra_enc) is selected — model.CredentialRef has no field
	// to carry one.
	ActiveCredentials(ctx context.Context) ([]model.CredentialRef, error)
}

// AdminAggregateRepository refreshes continuous aggregates.
//
// This is platform work, not tenant work: refresh_continuous_aggregate takes
// a time range and nothing else, so the call necessarily covers every
// tenant's buckets in that range. A Scope parameter would be a lie, so this
// lives here rather than on AnalyticsRepository (R72).
type AdminAggregateRepository interface {
	// Refresh materialises [r.From, r.To) of view. view is validated against
	// the closed AggregateView set and r against Valid, both BEFORE any
	// database round trip: an unknown view returns ErrUnknownView, an invalid
	// r returns ErrInvalidRange.
	//
	// It must not run inside a transaction: refresh_continuous_aggregate
	// commits its own work (SQLSTATE 25001 otherwise).
	Refresh(ctx context.Context, view AggregateView, r TimeRange) error
}
