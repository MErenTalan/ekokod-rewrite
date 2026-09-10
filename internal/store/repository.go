package store

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
)

// The repository contract. One interface per aggregate, and every method takes
// ctx first and a Scope second.
//
// The Scope parameter is second on EVERY method, including the ones where it
// cannot narrow anything (market prices and the national tariff schedule are
// platform-wide tables with no company_id). That uniformity is the point:
// internal/arch's TestEveryStoreMethodIsScoped requires a Scope on every
// exported, context-taking method under internal/store/postgres, and an
// interface that omitted it "because this table has no tenant" would be the
// one place a reviewer has to think rather than check. Those methods still
// validate the Scope and still return ErrInvalidScope for a zero one, because
// a caller with no valid tenant has no business reading anything.
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
//
// Errors are the sentinels in errors.go, matched with errors.Is: ErrNotFound,
// ErrConflict, ErrInvalidScope. A row belonging to another tenant returns
// ErrNotFound and never ErrInvalidScope — the two must stay indistinguishable
// from outside, or the error itself becomes an oracle for what exists in
// another company.

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
type UserRepository interface {
	Get(ctx context.Context, s Scope, id uuid.UUID) (model.User, error)

	// GetByEmail resolves a login. The email column is citext, so the match
	// is case-insensitive, and the lookup is still scoped: authentication
	// resolves the company first and then the user within it.
	GetByEmail(ctx context.Context, s Scope, email string) (model.User, error)

	List(ctx context.Context, s Scope, f UserFilter) ([]model.User, error)
	Create(ctx context.Context, s Scope, u model.User) (model.User, error)
	Update(ctx context.Context, s Scope, u model.User) (model.User, error)
	SoftDelete(ctx context.Context, s Scope, id uuid.UUID, at time.Time) error

	// SetPassword writes the new hash and appends the OLD one to the
	// history, in one transaction. Splitting the two would let a crash
	// between them lose the record that blocks reuse.
	SetPassword(ctx context.Context, s Scope, userID uuid.UUID, hash string, at time.Time) error

	// PasswordHistory returns the most recent entries first, newest bounded
	// by limit, so a caller can refuse a reused password.
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
type SessionRepository interface {
	Get(ctx context.Context, s Scope, id uuid.UUID) (model.Session, error)

	// GetByRefreshTokenHash looks a session up by the HASH of the presented
	// token. The plaintext token never reaches this layer.
	GetByRefreshTokenHash(ctx context.Context, s Scope, hash string) (model.Session, error)

	List(ctx context.Context, s Scope, f SessionFilter) ([]model.Session, error)
	Create(ctx context.Context, s Scope, sess model.Session) (model.Session, error)

	// Revoke stamps revoked_at. Sessions are never deleted on logout, so that
	// a replayed token is provably a replay rather than an unknown token.
	Revoke(ctx context.Context, s Scope, id uuid.UUID, at time.Time) error

	// RevokeAllForUser is what a password change and a role change both call.
	RevokeAllForUser(ctx context.Context, s Scope, userID uuid.UUID, at time.Time) (int64, error)

	// DeleteExpired is housekeeping: it removes sessions that expired before
	// before, and returns how many rows went.
	DeleteExpired(ctx context.Context, s Scope, before time.Time) (int64, error)
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

// AuditRepository appends to and reads the audit trail.
//
// There is no Update and no Delete, by design: an audit row that can be edited
// is not an audit row.
type AuditRepository interface {
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
	Create(ctx context.Context, s Scope, b model.Building) (model.Building, error)
	Update(ctx context.Context, s Scope, b model.Building) (model.Building, error)
	SoftDelete(ctx context.Context, s Scope, id uuid.UUID, at time.Time) error

	// Contacts returns a building's contacts in sort_order.
	Contacts(ctx context.Context, s Scope, buildingID uuid.UUID) ([]model.BuildingContact, error)

	// ReplaceContacts swaps the whole contact list in one transaction. The
	// UI edits them as a list, so a per-row API would make a partial write
	// the normal case.
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
// pretend otherwise.
type PlantRepository interface {
	Get(ctx context.Context, s Scope, id uuid.UUID) (model.PowerPlant, error)
	List(ctx context.Context, s Scope, f PlantFilter) ([]model.PowerPlant, error)
	Create(ctx context.Context, s Scope, p model.PowerPlant) (model.PowerPlant, error)
	Update(ctx context.Context, s Scope, p model.PowerPlant) (model.PowerPlant, error)
	SoftDelete(ctx context.Context, s Scope, id uuid.UUID, at time.Time) error

	MonthlyTargets(ctx context.Context, s Scope, plantID uuid.UUID) ([]model.PlantMonthlyTarget, error)
	ReplaceMonthlyTargets(ctx context.Context, s Scope, plantID uuid.UUID, targets []model.PlantMonthlyTarget) ([]model.PlantMonthlyTarget, error)

	Devices(ctx context.Context, s Scope, plantID uuid.UUID) ([]model.PlantDevice, error)

	// UpsertDevice is keyed on (plant_id, device_sn): the provider's device
	// list is re-fetched on a schedule and must converge rather than
	// accumulate duplicates.
	UpsertDevice(ctx context.Context, s Scope, d model.PlantDevice) (model.PlantDevice, error)

	AlarmRecipients(ctx context.Context, s Scope, plantID uuid.UUID) ([]model.PlantAlarmRecipient, error)
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
type ReadingRepository interface {
	// BulkInsert writes rows idempotently: COPY into a staging table, then
	// one `insert … on conflict (analyzer_id, ts, kind) do update`
	// (04-data-model.md §14). Re-ingesting a window must update, never
	// duplicate, because F2's ingestion is resumable and will re-ingest.
	//
	// The counts are real, not estimated: the job model records processed
	// and skipped separately and guessing them is not acceptable.
	BulkInsert(ctx context.Context, s Scope, rows []model.MeterReading) (inserted, updated int, err error)

	// Range returns readings for one analyzer over the half-open window.
	// There is deliberately no unbounded variant: 04-data-model.md §14 says
	// a query with no time bound is a bug, and this signature makes one
	// impossible to express.
	Range(ctx context.Context, s Scope, analyzerID uuid.UUID, r TimeRange, kind model.ReadingKind) ([]model.MeterReading, error)

	// BoundaryReadings returns the last reading at or before each of start
	// and end — the exact operation 02-domain-rules.md §3.1 specifies for
	// billing.
	//
	// Either return may be nil, and nil is NOT a zero reading. §3.1: "If
	// either reading is missing, or if reading_start and reading_end are the
	// same reading, the period yields no row — it is not emitted as zero." A
	// zero here becomes a wrong invoice, which is why these are pointers.
	BoundaryReadings(ctx context.Context, s Scope, analyzerID uuid.UUID, start, end time.Time) (startReading, endReading *model.MeterReading, err error)

	// Latest returns the most recent reading of a kind, or nil if there is
	// none. Nil rather than ErrNotFound: an analyzer with no readings yet is
	// an ordinary state, not a lookup failure.
	Latest(ctx context.Context, s Scope, analyzerID uuid.UUID, kind model.ReadingKind) (*model.MeterReading, error)
}

// CursorRepository tracks per-analyzer ingestion high-water marks.
type CursorRepository interface {
	Get(ctx context.Context, s Scope, analyzerID uuid.UUID, kind model.ReadingKind) (model.IngestionCursor, error)
	List(ctx context.Context, s Scope, analyzerIDs []uuid.UUID) ([]model.IngestionCursor, error)

	// RecordSuccess advances last_ts and last_success_at and RESETS
	// consecutive_failures. Clearing the counter is part of recording a
	// success, not a separate call a caller can forget.
	RecordSuccess(ctx context.Context, s Scope, analyzerID uuid.UUID, kind model.ReadingKind, lastTs, at time.Time) error

	// RecordFailure increments consecutive_failures and stores the message.
	// The message reaches an operator, so the caller passes already-scrubbed
	// text: nothing carrying a DSN or credential may be stored here.
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
type AnomalyRepository interface {
	Get(ctx context.Context, s Scope, id uuid.UUID) (model.ConsumptionAnomaly, error)
	List(ctx context.Context, s Scope, f AnomalyFilter) ([]model.ConsumptionAnomaly, error)
	Create(ctx context.Context, s Scope, a model.ConsumptionAnomaly) (model.ConsumptionAnomaly, error)

	// Resolve stamps resolved_at, resolved_by and the resolution, and stores
	// any manual override values. An anomaly is never deleted: the record
	// that a period was once suspect is what explains a restated bill.
	Resolve(ctx context.Context, s Scope, id uuid.UUID, resolvedBy uuid.UUID, resolution string, overrides []byte, at time.Time) (model.ConsumptionAnomaly, error)
}

// ProductionRepository reads and writes plant_production.
type ProductionRepository interface {
	// BulkInsert is idempotent on (plant_id, ts, device_id), like
	// ReadingRepository.BulkInsert and for the same reason.
	BulkInsert(ctx context.Context, s Scope, rows []model.PlantProduction) (inserted, updated int, err error)

	// Range reads the hypertable directly, over a bounded window.
	Range(ctx context.Context, s Scope, plantID uuid.UUID, r TimeRange) ([]model.PlantProduction, error)

	// Latest is the most recent sample for a plant, or nil if there is none.
	Latest(ctx context.Context, s Scope, plantID uuid.UUID) (*model.PlantProduction, error)
}

// AnalyticsRepository reads the six continuous aggregates.
//
// Everything here is MATERIALISED and therefore may lag the hypertable by up
// to the aggregate's refresh lag. It is correct for dashboards and reports and
// WRONG for billing, which must go through ReadingRepository.BoundaryReadings.
// The names are chosen so that no call site can mistake one for the other.
type AnalyticsRepository interface {
	ConsumptionHourly(ctx context.Context, s Scope, analyzerIDs []uuid.UUID, r TimeRange) ([]model.ConsumptionBucket, error)
	ConsumptionDaily(ctx context.Context, s Scope, analyzerIDs []uuid.UUID, r TimeRange) ([]model.ConsumptionBucket, error)
	ConsumptionMonthly(ctx context.Context, s Scope, analyzerIDs []uuid.UUID, r TimeRange) ([]model.ConsumptionBucket, error)
	ConsumptionYearly(ctx context.Context, s Scope, analyzerIDs []uuid.UUID, r TimeRange) ([]model.ConsumptionBucket, error)

	ProductionDaily(ctx context.Context, s Scope, plantIDs []uuid.UUID, r TimeRange) ([]model.PlantProductionBucket, error)
	ProductionMonthly(ctx context.Context, s Scope, plantIDs []uuid.UUID, r TimeRange) ([]model.PlantProductionBucket, error)
}

// PriceRepository reads and writes the market price series and the YEKDEM
// table.
//
// Both tables are PLATFORM-WIDE: neither has a company_id, so the Scope
// narrows nothing. It is still required and still validated — see this file's
// header — and a repository here must not pretend to filter by it.
type PriceRepository interface {
	UpsertHourly(ctx context.Context, s Scope, prices []model.MarketPrice) (int64, error)
	HourlyRange(ctx context.Context, s Scope, r TimeRange) ([]model.MarketPrice, error)

	UpsertYekdem(ctx context.Context, s Scope, values []model.YekdemMonthly) (int64, error)
	Yekdem(ctx context.Context, s Scope, year, month int16) (model.YekdemMonthly, error)
}

// ForecastRepository stores and reads forecast runs.
type ForecastRepository interface {
	// BulkInsert is idempotent on (analyzer_id, ts, generated_at). Because
	// generated_at is part of the key, a new run ACCUMULATES rather than
	// overwriting the previous one, which is what makes a forecast scorable
	// against what actually happened.
	BulkInsert(ctx context.Context, s Scope, rows []model.Forecast) (inserted, updated int, err error)

	Range(ctx context.Context, s Scope, analyzerID uuid.UUID, r TimeRange) ([]model.Forecast, error)

	// LatestRun returns the forecasts from the most recent run covering the
	// window.
	LatestRun(ctx context.Context, s Scope, analyzerID uuid.UUID, r TimeRange) ([]model.Forecast, error)

	RecordGaps(ctx context.Context, s Scope, gaps []model.ForecastGap) error
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

	Taxes(ctx context.Context, s Scope, tariffID uuid.UUID) ([]model.TariffTax, error)
	ReplaceTaxes(ctx context.Context, s Scope, tariffID uuid.UUID, taxes []model.TariffTax) ([]model.TariffTax, error)

	ManualYekdem(ctx context.Context, s Scope, tariffID uuid.UUID) ([]model.TariffManualYekdem, error)
	ReplaceManualYekdem(ctx context.Context, s Scope, tariffID uuid.UUID, values []model.TariffManualYekdem) ([]model.TariffManualYekdem, error)
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

// NationalTariffRepository reads and writes the published national tariff
// schedule that backs the public bill calculator.
//
// The table is PLATFORM-WIDE — no company_id — so the Scope narrows nothing
// here either.
type NationalTariffRepository interface {
	List(ctx context.Context, s Scope, f NationalTariffFilter) ([]model.NationalTariffScheduleEntry, error)
	Effective(ctx context.Context, s Scope, group model.DistributionUserGroup, level model.VoltageLevel, term model.TariffTerm, on time.Time) (model.NationalTariffScheduleEntry, error)
	Upsert(ctx context.Context, s Scope, entries []model.NationalTariffScheduleEntry) (int64, error)
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

	InsertRows(ctx context.Context, s Scope, importID uuid.UUID, rows []model.IcmalRow) (int64, error)
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
	Create(ctx context.Context, s Scope, b model.Bill, lines []model.BillLine, members []uuid.UUID) (model.Bill, error)

	// Supersede is the recomputation path from 04-data-model.md §14: in ONE
	// transaction it marks the existing live bill superseded and inserts the
	// replacement. The old bill and its lines stay readable, which is what
	// makes a disputed invoice explainable months later.
	Supersede(ctx context.Context, s Scope, replacing uuid.UUID, b model.Bill, lines []model.BillLine, members []uuid.UUID, at time.Time) (model.Bill, error)

	// UpdateStatus moves a bill between draft, issued and flagged. It cannot
	// set superseded — that transition belongs to Supersede, which also
	// inserts the replacement, and allowing it here would let a period end up
	// with no live bill at all.
	UpdateStatus(ctx context.Context, s Scope, id uuid.UUID, status model.BillStatus, flagReason *string, at time.Time) (model.Bill, error)

	// SetPDFPath records where the rendered invoice was stored.
	SetPDFPath(ctx context.Context, s Scope, id uuid.UUID, path string) error

	Lines(ctx context.Context, s Scope, billID uuid.UUID) ([]model.BillLine, error)
	Members(ctx context.Context, s Scope, billID uuid.UUID) ([]model.BillMember, error)

	HourlyDetail(ctx context.Context, s Scope, billID uuid.UUID) ([]model.BillHourlyDetail, error)
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
	Range       *TimeRange
	Page        Page
}

// AlarmRepository reads and writes alarm rules, their attachments and their
// firing history.
type AlarmRepository interface {
	Get(ctx context.Context, s Scope, id uuid.UUID) (model.Alarm, error)
	List(ctx context.Context, s Scope, f AlarmFilter) ([]model.Alarm, error)
	Create(ctx context.Context, s Scope, a model.Alarm) (model.Alarm, error)
	Update(ctx context.Context, s Scope, a model.Alarm) (model.Alarm, error)
	SoftDelete(ctx context.Context, s Scope, id uuid.UUID, at time.Time) error

	Analyzers(ctx context.Context, s Scope, alarmID uuid.UUID) ([]model.AlarmAnalyzer, error)
	ReplaceAnalyzers(ctx context.Context, s Scope, alarmID uuid.UUID, analyzerIDs []uuid.UUID) error

	Channels(ctx context.Context, s Scope, alarmID uuid.UUID) ([]model.AlarmChannel, error)
	ReplaceChannels(ctx context.Context, s Scope, alarmID uuid.UUID, channels []model.AlarmChannel) error

	CreateEvent(ctx context.Context, s Scope, e model.AlarmEvent) (model.AlarmEvent, error)
	ListEvents(ctx context.Context, s Scope, f AlarmEventFilter) ([]model.AlarmEvent, error)

	// MarkNotified records delivery. A non-nil notificationError with a nil
	// notified_at is a real state: the alarm fired and nobody was told.
	MarkNotified(ctx context.Context, s Scope, eventID uuid.UUID, at time.Time, notificationError *string) error

	// MarkBillFired claims a (alarm, bill) pair. It returns false if the pair
	// already existed, which is how an invoice is stopped from firing the
	// same alarm twice after a recomputation. Claiming and checking is ONE
	// call precisely so that two workers cannot both pass a check and both
	// fire.
	MarkBillFired(ctx context.Context, s Scope, alarmID, billID uuid.UUID) (claimed bool, err error)

	// MarkIsolarForwarded is the same claim-once idiom for iSolar alarm
	// forwarding, keyed on (plant_id, alarm_ref).
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
type CarbonRepository interface {
	Factor(ctx context.Context, s Scope, id uuid.UUID) (model.EmissionFactor, error)
	ListFactors(ctx context.Context, s Scope, f EmissionFactorFilter) ([]model.EmissionFactor, error)

	// UpsertFactor is keyed on (coalesce(company_id, zero uuid), key), the
	// table's unique index: a company may shadow a platform key with its own
	// factor but cannot have two of its own.
	UpsertFactor(ctx context.Context, s Scope, f model.EmissionFactor) (model.EmissionFactor, error)

	Conversions(ctx context.Context, s Scope, factorID uuid.UUID) ([]model.EmissionFactorConversion, error)
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

	ClauseDates(ctx context.Context, s Scope, projectID uuid.UUID) ([]model.ISO50001ClauseDate, error)
	ReplaceClauseDates(ctx context.Context, s Scope, projectID uuid.UUID, dates []model.ISO50001ClauseDate) error

	Notes(ctx context.Context, s Scope, projectID uuid.UUID, clauseID *string) ([]model.ISO50001Note, error)
	CreateNote(ctx context.Context, s Scope, n model.ISO50001Note) (model.ISO50001Note, error)
	UpdateNote(ctx context.Context, s Scope, n model.ISO50001Note) (model.ISO50001Note, error)
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
// which decrypts on the spot with internal/platform/crypto. Task 11's
// TestIntegrationCredentialsAreNeverReturnedInPlaintext pins that the ordinary
// read never carries it.
type IntegrationRepository interface {
	// Definitions is the platform catalogue: no company_id, so the Scope
	// narrows nothing.
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
	Range       *TimeRange
	Page        Page
}

// OpsRepository backs the Messages screen and operator diagnostics.
//
// Nothing written through here may contain a DSN, a password or any other
// secret: both job_runs.error and operational_messages.detail are shown to
// operators and exported. The store layer's scrubbing helpers run before a
// message reaches these methods, not inside them.
type OpsRepository interface {
	// StartRun inserts a job_runs row in the 'running' state and returns it,
	// so the caller holds the id it will finish.
	StartRun(ctx context.Context, s Scope, run model.JobRun) (model.JobRun, error)

	// FinishRun stamps finished_at, the final status and the three counts.
	// The counts are explicit rather than derived: the spec's job model
	// requires processed/skipped/failed and a job that cannot say how many
	// rows it skipped cannot be trusted to have skipped them deliberately.
	FinishRun(ctx context.Context, s Scope, id uuid.UUID, status string, processed, skipped, failed int32, errText *string, detail []byte, at time.Time) (model.JobRun, error)

	GetRun(ctx context.Context, s Scope, id uuid.UUID) (model.JobRun, error)
	ListRuns(ctx context.Context, s Scope, f JobRunFilter) ([]model.JobRun, error)

	AppendMessage(ctx context.Context, s Scope, m model.OperationalMessage) (model.OperationalMessage, error)
	ListMessages(ctx context.Context, s Scope, f MessageFilter) ([]model.OperationalMessage, error)
}
