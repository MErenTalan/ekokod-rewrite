// Package calendar manages company events, weekend days and vacation periods
// (01 §7.18, 05 §14). Only weekend days and vacations change the load-profile
// split; events never do (R137).
package calendar

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	perr "github.com/MErenTalan/ekokod-rewrite/internal/platform/errors"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// Service implements the calendar.
type Service struct {
	repo  store.CalendarRepository
	clock clock.Clock
}

// New builds a Service.
func New(repo store.CalendarRepository, clk clock.Clock) (*Service, error) {
	if repo == nil || clk == nil {
		return nil, errors.New("calendar: missing dependency")
	}
	return &Service{repo: repo, clock: clk}, nil
}

func validation(field string, codes ...string) error {
	return perr.Validation.WithParams(map[string]any{field: codes})
}

// EventInput is an event create or full replace.
type EventInput struct {
	Title            string
	StartsAt, EndsAt time.Time
	AllDay           bool
	Colour           *string
}

func (in EventInput) validate() error {
	if strings.TrimSpace(in.Title) == "" {
		return validation("title", "required")
	}
	if !in.EndsAt.After(in.StartsAt) {
		return validation("ends_at", "gtfield")
	}
	return nil
}

// ListEvents returns events overlapping [from, to).
func (s *Service) ListEvents(ctx context.Context, sc store.Scope, from, to time.Time) ([]model.CalendarEvent, error) {
	r := store.TimeRange{From: from, To: to}
	if !r.Valid() {
		return nil, validation("to", "gtfield")
	}
	return s.repo.ListEvents(ctx, sc, store.CalendarFilter{Range: &r, Page: store.Page{Limit: 1000}})
}

// CreateEvent stores an event.
func (s *Service) CreateEvent(ctx context.Context, sc store.Scope, by uuid.UUID, in EventInput) (model.CalendarEvent, error) {
	if err := in.validate(); err != nil {
		return model.CalendarEvent{}, err
	}
	return s.repo.CreateEvent(ctx, sc, model.CalendarEvent{
		CompanyID: sc.CompanyID, Title: strings.TrimSpace(in.Title), StartsAt: in.StartsAt, EndsAt: in.EndsAt, AllDay: in.AllDay,
		Colour: in.Colour, CreatedBy: &by, CreatedAt: s.clock.Now(),
	})
}

// UpdateEvent replaces an event.
func (s *Service) UpdateEvent(ctx context.Context, sc store.Scope, id uuid.UUID, in EventInput) (model.CalendarEvent, error) {
	current, err := s.repo.Event(ctx, sc, id)
	if err != nil {
		return model.CalendarEvent{}, err
	}
	if err := in.validate(); err != nil {
		return model.CalendarEvent{}, err
	}
	current.Title, current.StartsAt, current.EndsAt = strings.TrimSpace(in.Title), in.StartsAt, in.EndsAt
	current.AllDay, current.Colour = in.AllDay, in.Colour
	return s.repo.UpdateEvent(ctx, sc, current)
}

// DeleteEvent removes an event.
func (s *Service) DeleteEvent(ctx context.Context, sc store.Scope, id uuid.UUID) error {
	return s.repo.DeleteEvent(ctx, sc, id)
}

// Period is one vacation period, inclusive dates.
type Period struct {
	ID          uuid.UUID
	Start, End  time.Time
	Description *string
}

// Vacations is the weekend configuration and vacation periods.
type Vacations struct {
	WeekendDays   []int
	WeekendSource string // "company" or "default" (R69)
	Periods       []Period
}

// defaultWeekend mirrors loadprofile.DefaultWeekendDays: Sunday and Saturday.
var defaultWeekend = []int{0, 6}

// GetVacations returns the configuration the load profile uses.
func (s *Service) GetVacations(ctx context.Context, sc store.Scope) (Vacations, error) {
	days, err := s.repo.WeekendDays(ctx, sc)
	if err != nil {
		return Vacations{}, err
	}
	out := Vacations{WeekendSource: "company", Periods: []Period{}}
	for _, d := range days {
		out.WeekendDays = append(out.WeekendDays, int(d.DayOfWeek))
	}
	sort.Ints(out.WeekendDays)
	if len(out.WeekendDays) == 0 {
		out.WeekendDays, out.WeekendSource = append([]int(nil), defaultWeekend...), "default"
	}
	periods, err := s.repo.Vacations(ctx, sc, nil)
	if err != nil {
		return Vacations{}, err
	}
	for _, p := range periods {
		out.Periods = append(out.Periods, Period{ID: p.ID, Start: p.StartDate, End: p.EndDate, Description: p.Description})
	}
	sort.Slice(out.Periods, func(i, j int) bool { return out.Periods[i].Start.Before(out.Periods[j].Start) })
	return out, nil
}

// PutVacations replaces the weekend days and every vacation period.
func (s *Service) PutVacations(ctx context.Context, sc store.Scope, weekend []int, periods []Period) (Vacations, error) {
	seen := map[int]bool{}
	days := make([]int16, 0, len(weekend))
	for _, d := range weekend {
		if d < 0 || d > 6 || seen[d] {
			return Vacations{}, validation("weekend_days", "invalid")
		}
		seen[d] = true
		days = append(days, int16(d))
	}
	for _, p := range periods {
		if p.End.Before(p.Start) {
			return Vacations{}, validation("periods", "end_before_start")
		}
	}
	if err := s.repo.ReplaceWeekendDays(ctx, sc, days); err != nil {
		return Vacations{}, err
	}
	existing, err := s.repo.Vacations(ctx, sc, nil)
	if err != nil {
		return Vacations{}, err
	}
	for _, p := range existing {
		if err := s.repo.DeleteVacation(ctx, sc, p.ID); err != nil {
			return Vacations{}, err
		}
	}
	for _, p := range periods {
		if _, err := s.repo.CreateVacation(ctx, sc, model.CompanyVacation{
			CompanyID: sc.CompanyID, StartDate: p.Start, EndDate: p.End, Description: p.Description,
		}); err != nil {
			return Vacations{}, err
		}
	}
	return s.GetVacations(ctx, sc)
}
