package postgres

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/internal/pgerr"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/sqlcgen"
)

// CalendarRepository implements store.CalendarRepository.
//
// calendar_events, company_weekend_days and company_vacations all carry
// company_id directly and have no building_id (like stored_files), so Scope
// narrows every method to company_id only -- there is no building set to
// intersect against.
type CalendarRepository struct {
	q    *sqlcgen.Queries
	pool *pgxpool.Pool
}

// NewCalendarRepository builds a CalendarRepository on pool.
func NewCalendarRepository(pool *pgxpool.Pool) *CalendarRepository {
	return &CalendarRepository{q: sqlcgen.New(pool), pool: pool}
}

var _ store.CalendarRepository = (*CalendarRepository)(nil)

const (
	calendarListDefaultLimit = 100
	calendarListMaxLimit     = 1000
)

func calendarPageBounds(p store.Page) (limit, offset int32) {
	limit = p.Limit
	if limit <= 0 {
		limit = calendarListDefaultLimit
	}
	if limit > calendarListMaxLimit {
		limit = calendarListMaxLimit
	}
	offset = p.Offset
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

// inTx runs fn inside one transaction on r.pool, committing on a nil error
// and rolling back otherwise. It is a METHOD, not a package-level function,
// namespaced to *CalendarRepository so its name cannot collide with a
// sibling repository's identically-named helper in the same package (see
// CarbonRepository.inTx / ISO50001Repository.inTx, the same pattern under a
// different receiver).
func (r *CalendarRepository) inTx(ctx context.Context, fn func(qtx *sqlcgen.Queries) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return pgerr.Translate(r.pool, "begin transaction", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := sqlcgen.New(tx)
	if err := fn(qtx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return pgerr.Translate(r.pool, "commit transaction", err)
	}
	return nil
}

func calendarEventFromRow(row sqlcgen.CalendarEvent) model.CalendarEvent {
	return model.CalendarEvent{
		ID: row.ID, CompanyID: row.CompanyID, Title: row.Title,
		StartsAt: row.StartsAt.Time, EndsAt: row.EndsAt.Time, AllDay: row.AllDay,
		Colour: row.Colour, CreatedBy: row.CreatedBy, CreatedAt: row.CreatedAt.Time,
	}
}

func calendarVacationFromRow(row sqlcgen.CompanyVacation) model.CompanyVacation {
	return model.CompanyVacation{
		ID: row.ID, CompanyID: row.CompanyID,
		StartDate: row.StartDate.Time, EndDate: row.EndDate.Time, Description: row.Description,
	}
}

// --- calendar_events --------------------------------------------------------

// Event returns one calendar event by id, scoped to s.
func (r *CalendarRepository) Event(ctx context.Context, s store.Scope, id uuid.UUID) (model.CalendarEvent, error) {
	if !s.Valid() {
		return model.CalendarEvent{}, store.ErrInvalidScope
	}
	row, err := r.q.CalendarGetEvent(ctx, sqlcgen.CalendarGetEventParams{ID: id, CompanyID: s.CompanyID})
	if err != nil {
		return model.CalendarEvent{}, pgerr.Translate(r.pool, "get calendar event", err)
	}
	return calendarEventFromRow(row), nil
}

// ListEvents returns a company's calendar events matching f, narrowed by
// f.Range.From <= starts_at < f.Range.To when set.
func (r *CalendarRepository) ListEvents(ctx context.Context, s store.Scope, f store.CalendarFilter) ([]model.CalendarEvent, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	if f.Range != nil && !f.Range.Valid() {
		return nil, store.ErrInvalidRange
	}
	limit, offset := calendarPageBounds(f.Page)
	var from, to pgtype.Timestamptz
	if f.Range != nil {
		from = pgtype.Timestamptz{Time: f.Range.From, Valid: true}
		to = pgtype.Timestamptz{Time: f.Range.To, Valid: true}
	}
	rows, err := r.q.CalendarListEvents(ctx, sqlcgen.CalendarListEventsParams{
		CompanyID: s.CompanyID, RangeFrom: from, RangeTo: to, OffsetVal: offset, LimitVal: limit,
	})
	if err != nil {
		return nil, pgerr.Translate(r.pool, "list calendar events", err)
	}
	out := make([]model.CalendarEvent, 0, len(rows))
	for _, row := range rows {
		out = append(out, calendarEventFromRow(row))
	}
	return out, nil
}

// CreateEvent inserts a calendar event. e.CompanyID must equal s.CompanyID,
// checked before any database call; e.CreatedBy, when set, must name a user
// of s.CompanyID, checked atomically in the same insert statement (R1).
func (r *CalendarRepository) CreateEvent(ctx context.Context, s store.Scope, e model.CalendarEvent) (model.CalendarEvent, error) {
	if !s.Valid() {
		return model.CalendarEvent{}, store.ErrInvalidScope
	}
	if e.CompanyID != s.CompanyID {
		return model.CalendarEvent{}, store.ErrNotFound
	}
	row, err := r.q.CalendarCreateEvent(ctx, sqlcgen.CalendarCreateEventParams{
		ID: e.ID, CompanyID: s.CompanyID, Title: e.Title,
		StartsAt: pgtype.Timestamptz{Time: e.StartsAt, Valid: true},
		EndsAt:   pgtype.Timestamptz{Time: e.EndsAt, Valid: true},
		AllDay:   e.AllDay, Colour: e.Colour, CreatedBy: e.CreatedBy,
	})
	if err != nil {
		return model.CalendarEvent{}, pgerr.Translate(r.pool, "create calendar event", err)
	}
	return calendarEventFromRow(row), nil
}

// UpdateEvent replaces one event's mutable content by id. e.CompanyID naming
// another company is refused with ErrNotFound before any database call (the
// contract's "foreign CompanyID on Update" ruling); e.CreatedBy is
// re-validated exactly as CreateEvent validates it.
func (r *CalendarRepository) UpdateEvent(ctx context.Context, s store.Scope, e model.CalendarEvent) (model.CalendarEvent, error) {
	if !s.Valid() {
		return model.CalendarEvent{}, store.ErrInvalidScope
	}
	if e.CompanyID != s.CompanyID {
		return model.CalendarEvent{}, store.ErrNotFound
	}
	row, err := r.q.CalendarUpdateEvent(ctx, sqlcgen.CalendarUpdateEventParams{
		ID: e.ID, CompanyID: s.CompanyID, Title: e.Title,
		StartsAt: pgtype.Timestamptz{Time: e.StartsAt, Valid: true},
		EndsAt:   pgtype.Timestamptz{Time: e.EndsAt, Valid: true},
		AllDay:   e.AllDay, Colour: e.Colour, CreatedBy: e.CreatedBy,
	})
	if err != nil {
		return model.CalendarEvent{}, pgerr.Translate(r.pool, "update calendar event", err)
	}
	return calendarEventFromRow(row), nil
}

// DeleteEvent removes one event by id, scoped to s. calendar_events has no
// deleted_at, so this is a real DELETE.
func (r *CalendarRepository) DeleteEvent(ctx context.Context, s store.Scope, id uuid.UUID) error {
	if !s.Valid() {
		return store.ErrInvalidScope
	}
	n, err := r.q.CalendarDeleteEvent(ctx, sqlcgen.CalendarDeleteEventParams{ID: id, CompanyID: s.CompanyID})
	if err != nil {
		return pgerr.Translate(r.pool, "delete calendar event", err)
	}
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

// --- company_weekend_days ----------------------------------------------------

// WeekendDays returns s.CompanyID's weekend days, ordered by day_of_week.
func (r *CalendarRepository) WeekendDays(ctx context.Context, s store.Scope) ([]model.CompanyWeekendDay, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	rows, err := r.q.CalendarListWeekendDays(ctx, s.CompanyID)
	if err != nil {
		return nil, pgerr.Translate(r.pool, "list company weekend days", err)
	}
	out := make([]model.CompanyWeekendDay, 0, len(rows))
	for _, row := range rows {
		out = append(out, model.CompanyWeekendDay{CompanyID: row.CompanyID, DayOfWeek: row.DayOfWeek})
	}
	return out, nil
}

// ReplaceWeekendDays swaps s.CompanyID's whole weekend-day set in one
// transaction (R5): delete every current row, then insert days. days is 0..6
// with 0 = Sunday, matching both the schema's check constraint and
// time.Weekday; a value outside that range surfaces as a scrubbed,
// non-sentinel error from the check-constraint violation (deliberately not
// pre-validated in Go -- pgerr.Translate's documented policy for
// check_violation).
func (r *CalendarRepository) ReplaceWeekendDays(ctx context.Context, s store.Scope, days []int16) error {
	if !s.Valid() {
		return store.ErrInvalidScope
	}
	return r.inTx(ctx, func(qtx *sqlcgen.Queries) error {
		if err := qtx.CalendarDeleteWeekendDays(ctx, s.CompanyID); err != nil {
			return pgerr.Translate(r.pool, "delete company weekend days", err)
		}
		if len(days) == 0 {
			return nil
		}
		if err := qtx.CalendarInsertWeekendDays(ctx, sqlcgen.CalendarInsertWeekendDaysParams{
			CompanyID: s.CompanyID, Days: days,
		}); err != nil {
			return pgerr.Translate(r.pool, "insert company weekend days", err)
		}
		return nil
	})
}

// --- company_vacations --------------------------------------------------------

// Vacations returns every vacation of s.CompanyID when r is nil, or those
// overlapping r when set. ErrInvalidRange for a non-nil invalid r, before any
// database call.
func (r *CalendarRepository) Vacations(ctx context.Context, s store.Scope, timeRange *store.TimeRange) ([]model.CompanyVacation, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	if timeRange != nil && !timeRange.Valid() {
		return nil, store.ErrInvalidRange
	}
	var from, to pgtype.Date
	if timeRange != nil {
		from = pgtype.Date{Time: timeRange.From.UTC(), Valid: true}
		to = pgtype.Date{Time: timeRange.To.UTC(), Valid: true}
	}
	rows, err := r.q.CalendarListVacations(ctx, sqlcgen.CalendarListVacationsParams{
		CompanyID: s.CompanyID, FromDate: from, ToDate: to,
	})
	if err != nil {
		return nil, pgerr.Translate(r.pool, "list company vacations", err)
	}
	out := make([]model.CompanyVacation, 0, len(rows))
	for _, row := range rows {
		out = append(out, calendarVacationFromRow(row))
	}
	return out, nil
}

// CreateVacation inserts one vacation range. v.CompanyID must equal
// s.CompanyID, checked before any database call. The schema's valid_range
// check (start_date <= end_date) is enforced by the database, not
// pre-validated here, for the same reason as ReplaceWeekendDays' day values.
func (r *CalendarRepository) CreateVacation(ctx context.Context, s store.Scope, v model.CompanyVacation) (model.CompanyVacation, error) {
	if !s.Valid() {
		return model.CompanyVacation{}, store.ErrInvalidScope
	}
	if v.CompanyID != s.CompanyID {
		return model.CompanyVacation{}, store.ErrNotFound
	}
	row, err := r.q.CalendarCreateVacation(ctx, sqlcgen.CalendarCreateVacationParams{
		ID: v.ID, CompanyID: s.CompanyID,
		StartDate:   pgtype.Date{Time: v.StartDate, Valid: true},
		EndDate:     pgtype.Date{Time: v.EndDate, Valid: true},
		Description: v.Description,
	})
	if err != nil {
		return model.CompanyVacation{}, pgerr.Translate(r.pool, "create company vacation", err)
	}
	return calendarVacationFromRow(row), nil
}

// DeleteVacation removes one vacation by id, scoped to s. company_vacations
// has no deleted_at, so this is a real DELETE.
func (r *CalendarRepository) DeleteVacation(ctx context.Context, s store.Scope, id uuid.UUID) error {
	if !s.Valid() {
		return store.ErrInvalidScope
	}
	n, err := r.q.CalendarDeleteVacation(ctx, sqlcgen.CalendarDeleteVacationParams{ID: id, CompanyID: s.CompanyID})
	if err != nil {
		return pgerr.Translate(r.pool, "delete company vacation", err)
	}
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}
