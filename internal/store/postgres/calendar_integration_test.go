//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

func calendarEventFixture(companyID uuid.UUID, title string, startsAt time.Time) model.CalendarEvent {
	return model.CalendarEvent{
		CompanyID: companyID, Title: title,
		StartsAt: startsAt, EndsAt: startsAt.Add(time.Hour),
	}
}

func calendarVacationFixture(companyID uuid.UUID, start, end time.Time) model.CompanyVacation {
	return model.CompanyVacation{CompanyID: companyID, StartDate: start, EndDate: end}
}

// --- calendar_events ---------------------------------------------------------

func TestCalendarEventCreateGetListUpdateDelete(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenantA := testfixtures.NewTenant(t, ctx, pool, 8001)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 8002)
	repo := postgres.NewCalendarRepository(pool)

	start := time.Date(2026, time.March, 10, 9, 0, 0, 0, time.UTC)
	created, err := repo.CreateEvent(ctx, tenantA.Scope, calendarEventFixture(tenantA.Company.ID, "Kickoff", start))
	require.NoError(t, err)
	require.Equal(t, "Kickoff", created.Title)

	got, err := repo.Event(ctx, tenantA.Scope, created.ID)
	require.NoError(t, err)
	require.Equal(t, created.ID, got.ID)

	// Another company cannot read, list, update or delete it: it is
	// indistinguishable from a missing event.
	_, err = repo.Event(ctx, tenantB.Scope, created.ID)
	require.ErrorIs(t, err, store.ErrNotFound)

	listB, err := repo.ListEvents(ctx, tenantB.Scope, store.CalendarFilter{})
	require.NoError(t, err)
	require.Empty(t, listB)

	err = repo.DeleteEvent(ctx, tenantB.Scope, created.ID)
	require.ErrorIs(t, err, store.ErrNotFound)

	listA, err := repo.ListEvents(ctx, tenantA.Scope, store.CalendarFilter{})
	require.NoError(t, err)
	require.Len(t, listA, 1)

	updated := got
	updated.Title = "Kickoff (rescheduled)"
	updated.StartsAt = start.Add(24 * time.Hour)
	updated.EndsAt = updated.StartsAt.Add(2 * time.Hour)
	updated.AllDay = true
	colour := "#ff0000"
	updated.Colour = &colour

	saved, err := repo.UpdateEvent(ctx, tenantA.Scope, updated)
	require.NoError(t, err)
	require.Equal(t, "Kickoff (rescheduled)", saved.Title)
	require.True(t, saved.StartsAt.Equal(updated.StartsAt))
	require.True(t, saved.AllDay)
	require.Equal(t, colour, *saved.Colour)

	require.NoError(t, repo.DeleteEvent(ctx, tenantA.Scope, created.ID))

	_, err = repo.Event(ctx, tenantA.Scope, created.ID)
	require.ErrorIs(t, err, store.ErrNotFound)

	afterDelete, err := repo.ListEvents(ctx, tenantA.Scope, store.CalendarFilter{})
	require.NoError(t, err)
	require.Empty(t, afterDelete)
}

func TestCalendarEventCreateRefusesMismatchedCompanyID(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenantA := testfixtures.NewTenant(t, ctx, pool, 8010)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 8011)
	repo := postgres.NewCalendarRepository(pool)

	start := time.Date(2026, time.March, 10, 9, 0, 0, 0, time.UTC)
	_, err := repo.CreateEvent(ctx, tenantA.Scope, calendarEventFixture(tenantB.Company.ID, "x", start))
	require.ErrorIs(t, err, store.ErrNotFound)

	list, err := repo.ListEvents(ctx, tenantA.Scope, store.CalendarFilter{})
	require.NoError(t, err)
	require.Empty(t, list, "the refused create must not have written anything")
}

// TestCalendarEventUpdateRefusesMismatchedCompanyID pins the "foreign
// CompanyID on Update" ruling: an update whose model carries another
// company's id returns ErrNotFound before any database call, and the
// original row is untouched.
func TestCalendarEventUpdateRefusesMismatchedCompanyID(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenantA := testfixtures.NewTenant(t, ctx, pool, 8020)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 8021)
	repo := postgres.NewCalendarRepository(pool)

	start := time.Date(2026, time.March, 10, 9, 0, 0, 0, time.UTC)
	created, err := repo.CreateEvent(ctx, tenantA.Scope, calendarEventFixture(tenantA.Company.ID, "Kickoff", start))
	require.NoError(t, err)

	forged := created
	forged.CompanyID = tenantB.Company.ID
	forged.Title = "hijacked"
	_, err = repo.UpdateEvent(ctx, tenantA.Scope, forged)
	require.ErrorIs(t, err, store.ErrNotFound)

	unchanged, err := repo.Event(ctx, tenantA.Scope, created.ID)
	require.NoError(t, err)
	require.Equal(t, "Kickoff", unchanged.Title, "the refused update must not have written anything")
}

// TestCalendarEventCreateAndUpdateRefuseAnotherTenantsCreatedBy proves
// created_by, when set, must name a user of the caller's own company — even
// under AdminScope, so that Scope.AllowsBuilding-style reasoning (AllBuildings
// makes everything look permitted) is never what authorises a stored foreign
// key (R1).
func TestCalendarEventCreateAndUpdateRefuseAnotherTenantsCreatedBy(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenantA := testfixtures.NewTenant(t, ctx, pool, 8030)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 8031)
	repo := postgres.NewCalendarRepository(pool)

	start := time.Date(2026, time.March, 10, 9, 0, 0, 0, time.UTC)
	e := calendarEventFixture(tenantA.Company.ID, "Kickoff", start)
	foreignUser := tenantB.Users[model.UserRoleCompanyAdmin].ID
	e.CreatedBy = &foreignUser

	_, err := repo.CreateEvent(ctx, tenantA.AdminScope, e)
	require.ErrorIs(t, err, store.ErrNotFound)

	list, err := repo.ListEvents(ctx, tenantA.Scope, store.CalendarFilter{})
	require.NoError(t, err)
	require.Empty(t, list, "the refused create must not have written anything")

	ownUser := tenantA.Users[model.UserRoleCompanyAdmin].ID
	e.CreatedBy = &ownUser
	created, err := repo.CreateEvent(ctx, tenantA.Scope, e)
	require.NoError(t, err)
	require.Equal(t, ownUser, *created.CreatedBy)

	updated := created
	updated.CreatedBy = &foreignUser
	_, err = repo.UpdateEvent(ctx, tenantA.Scope, updated)
	require.ErrorIs(t, err, store.ErrNotFound)

	unchanged, err := repo.Event(ctx, tenantA.Scope, created.ID)
	require.NoError(t, err)
	require.Equal(t, ownUser, *unchanged.CreatedBy, "the refused update must not have written anything")
}

// TestCalendarListEventsFiltersByRange proves ListEvents narrows by
// starts_at and rejects an invalid Range before any database call.
func TestCalendarListEventsFiltersByRange(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenantA := testfixtures.NewTenant(t, ctx, pool, 8040)
	repo := postgres.NewCalendarRepository(pool)

	jan := time.Date(2026, time.January, 5, 9, 0, 0, 0, time.UTC)
	mar := time.Date(2026, time.March, 5, 9, 0, 0, 0, time.UTC)
	_, err := repo.CreateEvent(ctx, tenantA.Scope, calendarEventFixture(tenantA.Company.ID, "January", jan))
	require.NoError(t, err)
	_, err = repo.CreateEvent(ctx, tenantA.Scope, calendarEventFixture(tenantA.Company.ID, "March", mar))
	require.NoError(t, err)

	feb := store.TimeRange{
		From: time.Date(2026, time.February, 1, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC),
	}
	list, err := repo.ListEvents(ctx, tenantA.Scope, store.CalendarFilter{Range: &feb})
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, "March", list[0].Title)

	invalid := store.TimeRange{From: feb.To, To: feb.From}
	_, err = repo.ListEvents(ctx, tenantA.Scope, store.CalendarFilter{Range: &invalid})
	require.ErrorIs(t, err, store.ErrInvalidRange)
}

// --- company_weekend_days -----------------------------------------------------

func TestCalendarReplaceWeekendDaysAndIsolation(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenantA := testfixtures.NewTenant(t, ctx, pool, 8050)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 8051)
	repo := postgres.NewCalendarRepository(pool)

	// 0 = Sunday, 6 = Saturday, matching time.Weekday.
	require.NoError(t, repo.ReplaceWeekendDays(ctx, tenantA.Scope, []int16{0, 6}))

	days, err := repo.WeekendDays(ctx, tenantA.Scope)
	require.NoError(t, err)
	require.Len(t, days, 2)

	// Tenant B sees nothing of tenant A's set.
	daysB, err := repo.WeekendDays(ctx, tenantB.Scope)
	require.NoError(t, err)
	require.Empty(t, daysB)

	// A duplicate entry in the input is absorbed, not rejected.
	require.NoError(t, repo.ReplaceWeekendDays(ctx, tenantA.Scope, []int16{0, 0, 5}))
	replaced, err := repo.WeekendDays(ctx, tenantA.Scope)
	require.NoError(t, err)
	require.Len(t, replaced, 2)
	got := []int16{replaced[0].DayOfWeek, replaced[1].DayOfWeek}
	require.ElementsMatch(t, []int16{0, 5}, got)

	// Replacing with an empty set clears it.
	require.NoError(t, repo.ReplaceWeekendDays(ctx, tenantA.Scope, nil))
	cleared, err := repo.WeekendDays(ctx, tenantA.Scope)
	require.NoError(t, err)
	require.Empty(t, cleared)
}

// --- company_vacations ---------------------------------------------------------

func TestCalendarVacationCreateListDeleteAndIsolation(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenantA := testfixtures.NewTenant(t, ctx, pool, 8060)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 8061)
	repo := postgres.NewCalendarRepository(pool)

	start := time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, time.July, 10, 0, 0, 0, 0, time.UTC)
	created, err := repo.CreateVacation(ctx, tenantA.Scope, calendarVacationFixture(tenantA.Company.ID, start, end))
	require.NoError(t, err)
	require.True(t, created.StartDate.Equal(start))
	require.True(t, created.EndDate.Equal(end))

	all, err := repo.Vacations(ctx, tenantA.Scope, nil)
	require.NoError(t, err)
	require.Len(t, all, 1)

	// Cross-tenant: tenant B sees none of tenant A's vacations, and cannot
	// delete tenant A's row.
	listB, err := repo.Vacations(ctx, tenantB.Scope, nil)
	require.NoError(t, err)
	require.Empty(t, listB)

	err = repo.DeleteVacation(ctx, tenantB.Scope, created.ID)
	require.ErrorIs(t, err, store.ErrNotFound)

	_, err = repo.CreateVacation(ctx, tenantA.Scope, calendarVacationFixture(tenantB.Company.ID, start, end))
	require.ErrorIs(t, err, store.ErrNotFound)

	require.NoError(t, repo.DeleteVacation(ctx, tenantA.Scope, created.ID))
	afterDelete, err := repo.Vacations(ctx, tenantA.Scope, nil)
	require.NoError(t, err)
	require.Empty(t, afterDelete)
}

// TestCalendarVacationsRangeOverlapAndInvalidRange proves Vacations narrows
// by overlap with the given range and rejects an invalid one before any
// database call.
func TestCalendarVacationsRangeOverlapAndInvalidRange(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenantA := testfixtures.NewTenant(t, ctx, pool, 8070)
	repo := postgres.NewCalendarRepository(pool)

	july := calendarVacationFixture(tenantA.Company.ID,
		time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, time.July, 10, 0, 0, 0, 0, time.UTC))
	december := calendarVacationFixture(tenantA.Company.ID,
		time.Date(2026, time.December, 20, 0, 0, 0, 0, time.UTC),
		time.Date(2026, time.December, 31, 0, 0, 0, 0, time.UTC))
	_, err := repo.CreateVacation(ctx, tenantA.Scope, july)
	require.NoError(t, err)
	_, err = repo.CreateVacation(ctx, tenantA.Scope, december)
	require.NoError(t, err)

	window := store.TimeRange{
		From: time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC),
	}
	inWindow, err := repo.Vacations(ctx, tenantA.Scope, &window)
	require.NoError(t, err)
	require.Len(t, inWindow, 1)
	require.True(t, inWindow[0].StartDate.Equal(july.StartDate))

	invalid := store.TimeRange{From: window.To, To: window.From}
	_, err = repo.Vacations(ctx, tenantA.Scope, &invalid)
	require.ErrorIs(t, err, store.ErrInvalidRange)
}

// TestCalendarVacationsIstanbulDayBoundary is F1 final review pass A,
// Important Finding 2's proof, reproduced as a test: Vacations converted a
// TimeRange's instants to `date` bounds via .UTC() instead of Europe/
// Istanbul, so a query for the Istanbul calendar day 2026-03-01 — the
// half-open instant window [2026-02-28T21:00Z, 2026-03-01T21:00Z), since
// Istanbul is a fixed UTC+3 with no DST — returned the WRONG single-day
// vacation (2026-02-28's) and excluded the RIGHT one (2026-03-01's).
//
// Pre-fix this test fails: got contains feb28's id (wrongly included) and
// is missing mar1's id (wrongly excluded) — exactly the report's proof.
func TestCalendarVacationsIstanbulDayBoundary(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenantA := testfixtures.NewTenant(t, ctx, pool, 8072)
	repo := postgres.NewCalendarRepository(pool)

	feb28, err := repo.CreateVacation(ctx, tenantA.Scope, calendarVacationFixture(tenantA.Company.ID,
		time.Date(2026, time.February, 28, 0, 0, 0, 0, time.UTC),
		time.Date(2026, time.February, 28, 0, 0, 0, 0, time.UTC)))
	require.NoError(t, err)
	mar1, err := repo.CreateVacation(ctx, tenantA.Scope, calendarVacationFixture(tenantA.Company.ID,
		time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC)))
	require.NoError(t, err)

	istanbul, err := time.LoadLocation("Europe/Istanbul")
	require.NoError(t, err)

	// The Istanbul calendar day 2026-03-01, expressed as its own half-open
	// midnight-to-midnight instant window in Europe/Istanbul.
	window := store.TimeRange{
		From: time.Date(2026, time.March, 1, 0, 0, 0, 0, istanbul),
		To:   time.Date(2026, time.March, 2, 0, 0, 0, 0, istanbul),
	}
	got, err := repo.Vacations(ctx, tenantA.Scope, &window)
	require.NoError(t, err)
	ids := make([]uuid.UUID, 0, len(got))
	for _, v := range got {
		ids = append(ids, v.ID)
	}
	require.Contains(t, ids, mar1.ID, "the 2026-03-01 vacation must be included in the 2026-03-01 Istanbul-day window")
	require.NotContains(t, ids, feb28.ID, "the 2026-02-28 vacation must NOT be included in the 2026-03-01 Istanbul-day window")
}

func TestCalendarRepositoryRejectsInvalidScope(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	repo := postgres.NewCalendarRepository(pool)
	var invalid store.Scope

	_, err := repo.Event(ctx, invalid, uuid.New())
	require.ErrorIs(t, err, store.ErrInvalidScope)
	_, err = repo.ListEvents(ctx, invalid, store.CalendarFilter{})
	require.ErrorIs(t, err, store.ErrInvalidScope)
	_, err = repo.CreateEvent(ctx, invalid, model.CalendarEvent{})
	require.ErrorIs(t, err, store.ErrInvalidScope)
	_, err = repo.UpdateEvent(ctx, invalid, model.CalendarEvent{})
	require.ErrorIs(t, err, store.ErrInvalidScope)
	err = repo.DeleteEvent(ctx, invalid, uuid.New())
	require.ErrorIs(t, err, store.ErrInvalidScope)

	_, err = repo.WeekendDays(ctx, invalid)
	require.ErrorIs(t, err, store.ErrInvalidScope)
	err = repo.ReplaceWeekendDays(ctx, invalid, []int16{0})
	require.ErrorIs(t, err, store.ErrInvalidScope)

	_, err = repo.Vacations(ctx, invalid, nil)
	require.ErrorIs(t, err, store.ErrInvalidScope)
	_, err = repo.CreateVacation(ctx, invalid, model.CompanyVacation{})
	require.ErrorIs(t, err, store.ErrInvalidScope)
	err = repo.DeleteVacation(ctx, invalid, uuid.New())
	require.ErrorIs(t, err, store.ErrInvalidScope)
}
