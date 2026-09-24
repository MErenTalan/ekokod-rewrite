package report

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/google/uuid"

	domain "github.com/MErenTalan/ekokod-rewrite/internal/domain/report"
	perr "github.com/MErenTalan/ekokod-rewrite/internal/platform/errors"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// R273's limits.
const (
	MaxBuildings = 50
	MaxPlants    = 100
)

// Request is one preview or generation.
type Request struct {
	Type        string
	Period      string
	Selection   string
	BuildingIDs []uuid.UUID
	PlantIDs    []uuid.UUID
}

// period is a parsed Request.Period; Month is 0 for a yearly report.
type period struct{ Year, Month int }

func (p period) key() string {
	if p.Month == 0 {
		return strconv.Itoa(p.Year)
	}
	return fmt.Sprintf("%04d-%02d", p.Year, p.Month)
}

// first and end are the period's Istanbul bounds.
func (p period) first() time.Time {
	m := p.Month
	if m == 0 {
		m = 1
	}
	return time.Date(p.Year, time.Month(m), 1, 0, 0, 0, 0, istanbul)
}

func (p period) end() time.Time {
	if p.Month == 0 {
		return p.first().AddDate(1, 0, 0)
	}
	return p.first().AddDate(0, 1, 0)
}

func invalid(field, code string) error {
	return perr.Validation.WithParams(map[string]any{field: []string{code}})
}

// Validate is R273. It answers 422 with the field that failed.
func (s *Service) Validate(r Request) (period, error) {
	var p period
	switch r.Type {
	case domain.TypeMonthly:
		t, err := time.Parse("2006-01", r.Period)
		if err != nil {
			return p, invalid("period", "invalid")
		}
		p = period{Year: t.Year(), Month: int(t.Month())}
	case domain.TypeYearly:
		y, err := strconv.Atoi(r.Period)
		if err != nil || len(r.Period) != 4 {
			return p, invalid("period", "invalid")
		}
		p = period{Year: y}
	default:
		return p, invalid("type", "oneof")
	}
	if p.Year < 2000 || p.Year > s.d.Clock.Now().In(istanbul).Year()+1 {
		return p, invalid("period", "range")
	}
	switch r.Selection {
	case domain.SelectionAll, domain.SelectionGrid, domain.SelectionRooftop:
	default:
		return p, invalid("plant_selection", "oneof")
	}
	if len(r.BuildingIDs) == 0 {
		return p, invalid("building_ids", "required")
	}
	if len(r.BuildingIDs) > MaxBuildings {
		return p, invalid("building_ids", "max")
	}
	if hasDuplicate(r.BuildingIDs) {
		return p, invalid("building_ids", "unique")
	}
	if len(r.PlantIDs) > MaxPlants {
		return p, invalid("plant_ids", "max")
	}
	if hasDuplicate(r.PlantIDs) {
		return p, invalid("plant_ids", "unique")
	}
	return p, nil
}

func hasDuplicate(ids []uuid.UUID) bool {
	sorted := slices.Clone(ids)
	slices.SortFunc(sorted, func(a, b uuid.UUID) int { return slices.Compare(a[:], b[:]) })
	return len(slices.Compact(sorted)) != len(ids)
}

// Preview computes a report without persisting it (05 §11).
func (s *Service) Preview(ctx context.Context, sc store.Scope, r Request) (domain.Payload, error) {
	if _, err := s.Validate(r); err != nil {
		return domain.Payload{}, err
	}
	return s.Build(ctx, sc, r)
}

// Build loads under sc and builds; the worker calls it with SystemScope.
func (s *Service) Build(ctx context.Context, sc store.Scope, r Request) (domain.Payload, error) {
	p, err := s.Validate(r)
	if err != nil {
		return domain.Payload{}, err
	}
	in, err := s.load(ctx, sc, r, p)
	if err != nil {
		return domain.Payload{}, err
	}
	out := domain.Payload{Version: domain.PayloadVersion, Type: r.Type, Period: p.key()}
	if r.Type == domain.TypeMonthly {
		m := domain.BuildMonthly(domain.MonthlyInput{Year: p.Year, Month: p.Month, Selection: r.Selection,
			Buildings: in.buildings, Plants: in.plants})
		out.Monthly = &m
		return out, nil
	}
	y := domain.BuildYearly(domain.YearlyInput{Year: p.Year, Selection: r.Selection,
		Buildings: in.buildings, Plants: in.plants, Factor: in.factor})
	out.Yearly = &y
	return out, nil
}
