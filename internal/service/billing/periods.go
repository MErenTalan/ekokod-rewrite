package billing

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	domain "github.com/MErenTalan/ekokod-rewrite/internal/domain/billing"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// PreviewRequest prices one analyzer or building over an explicit window (R559).
type PreviewRequest struct {
	Scope     model.BillScope // analyzer or building
	SubjectID uuid.UUID
	From, To  time.Time
}

// PreviewResult is an unsaved bill: ids are zero, nothing was written.
type PreviewResult struct {
	Bill  model.Bill
	Lines []model.BillLine
	// Saved is the building's saved window for Bill.PeriodKey, if any.
	Saved *model.BillPeriod
}

// Preview computes a bill for req's window without persisting anything.
func (s *Service) Preview(ctx context.Context, sc store.Scope, req PreviewRequest) (PreviewResult, error) {
	w := energy.Window{From: req.From.In(s.loc), To: req.To.In(s.loc)}
	if !sc.Valid() || req.SubjectID == uuid.Nil || domain.ValidateSavedPeriod(w) != nil {
		return PreviewResult{}, ErrInvalidRequest
	}
	building, members, err := s.subjectMembers(ctx, sc, req.Scope, req.SubjectID)
	if err != nil {
		return PreviewResult{}, err
	}
	key := domain.KeyOf(w.From, s.loc)
	subj, err := s.computeSubject(ctx, sc, building, members, key, req.Scope, req.SubjectID, subjectOpts{window: &w, preview: true})
	if err != nil {
		return PreviewResult{}, err
	}
	bill, lines, _ := domain.ToModel(subj.invoice, req.Scope)
	bill.CompanyID, bill.BuildingID, bill.MissingDays = sc.CompanyID, &building.ID, subj.missingDays
	if req.Scope == model.BillScopeAnalyzer {
		bill.AnalyzerID = &members[0].ID
	}
	out := PreviewResult{Bill: bill, Lines: lines}
	saved, err := s.deps.Bills.BillPeriods(ctx, sc, building.ID, []string{key})
	if err != nil {
		return PreviewResult{}, err
	}
	if len(saved) == 1 {
		out.Saved = &saved[0]
	}
	return out, nil
}

// subjectMembers is the building and analyzers an analyzer or building bill covers.
func (s *Service) subjectMembers(ctx context.Context, sc store.Scope, scope model.BillScope, id uuid.UUID) (model.Building, []model.Analyzer, error) {
	switch scope {
	case model.BillScopeAnalyzer:
		a, err := s.deps.Analyzers.Get(ctx, sc, id)
		if err != nil {
			return model.Building{}, nil, err
		}
		if a.BuildingID == nil {
			return model.Building{}, nil, fmt.Errorf("%w: analyzer has no building", ErrInvalidRequest)
		}
		b, err := s.deps.Buildings.Get(ctx, sc, *a.BuildingID)
		return b, []model.Analyzer{a}, err
	case model.BillScopeBuilding:
		b, err := s.deps.Buildings.Get(ctx, sc, id)
		if err != nil {
			return model.Building{}, nil, err
		}
		members, err := s.buildingAnalyzers(ctx, sc, b.ID)
		if err != nil {
			return model.Building{}, nil, err
		}
		if len(members) == 0 {
			return model.Building{}, nil, computeErr(CodeNoConsumptionData, "building_id", b.ID.String())
		}
		return b, members, nil
	default:
		return model.Building{}, nil, ErrInvalidRequest
	}
}

// SavePeriodRequest stores key's window for a building (R557).
type SavePeriodRequest struct {
	BuildingID uuid.UUID
	PeriodKey  string
	From, To   time.Time
	UserID     *uuid.UUID
}

// Regeneration names one bill a period change must recompute.
type Regeneration struct {
	Scope     model.BillScope
	SubjectID uuid.UUID
	PeriodKey string
}

// SavePeriod upserts the window and returns the bills to regenerate.
func (s *Service) SavePeriod(ctx context.Context, sc store.Scope, req SavePeriodRequest) (model.BillPeriod, []Regeneration, error) {
	w := energy.Window{From: req.From.In(s.loc), To: req.To.In(s.loc)}
	if !sc.Valid() || req.BuildingID == uuid.Nil || domain.ValidateSavedPeriod(w) != nil {
		return model.BillPeriod{}, nil, ErrInvalidRequest
	}
	if _, err := domain.Period(req.PeriodKey, 1, s.loc); err != nil {
		return model.BillPeriod{}, nil, ErrInvalidRequest
	}
	building, err := s.deps.Buildings.Get(ctx, sc, req.BuildingID)
	if err != nil {
		return model.BillPeriod{}, nil, err
	}
	before, err := s.neighbourWindows(ctx, sc, building, req.PeriodKey, nil)
	if err != nil {
		return model.BillPeriod{}, nil, err
	}
	after, err := s.neighbourWindows(ctx, sc, building, req.PeriodKey, &w)
	if err != nil {
		return model.BillPeriod{}, nil, err
	}
	saved, err := s.deps.Bills.UpsertBillPeriod(ctx, sc, model.BillPeriod{
		BuildingID: building.ID, PeriodKey: req.PeriodKey, PeriodStart: w.From.UTC(), PeriodEnd: w.To.UTC(), CreatedBy: req.UserID,
	})
	if err != nil {
		return model.BillPeriod{}, nil, err
	}
	regen, err := s.regenerations(ctx, sc, building, req.PeriodKey, before, after)
	return saved, regen, err
}

// DeletePeriod drops the saved window; key falls back to the cut-off rule.
func (s *Service) DeletePeriod(ctx context.Context, sc store.Scope, buildingID uuid.UUID, key string) ([]Regeneration, error) {
	if !sc.Valid() || buildingID == uuid.Nil {
		return nil, ErrInvalidRequest
	}
	if _, err := domain.Period(key, 1, s.loc); err != nil {
		return nil, ErrInvalidRequest
	}
	building, err := s.deps.Buildings.Get(ctx, sc, buildingID)
	if err != nil {
		return nil, err
	}
	before, err := s.neighbourWindows(ctx, sc, building, key, nil)
	if err != nil {
		return nil, err
	}
	if err := s.deps.Bills.DeleteBillPeriod(ctx, sc, building.ID, key); err != nil {
		return nil, err
	}
	after, err := s.neighbourWindows(ctx, sc, building, key, nil)
	if err != nil {
		return nil, err
	}
	return s.regenerations(ctx, sc, building, key, before, after)
}

// neighbourWindows resolves key−1, key and key+1, with key's window replaced
// by override when given; an overlap or empty window is period_conflict (R557).
func (s *Service) neighbourWindows(ctx context.Context, sc store.Scope, building model.Building, key string, override *energy.Window) (map[string]energy.Window, error) {
	keys := make([]string, 5)
	for i := range keys {
		keys[i] = domain.ShiftKey(key, i-2)
	}
	periods, err := s.deps.Bills.BillPeriods(ctx, sc, building.ID, keys)
	if err != nil {
		return nil, err
	}
	saved := make(map[string]*energy.Window, len(periods)+1)
	for _, p := range periods {
		saved[p.PeriodKey] = &energy.Window{From: p.PeriodStart.In(s.loc), To: p.PeriodEnd.In(s.loc)}
	}
	if override != nil {
		prev, next := saved[keys[1]], saved[keys[3]]
		if (prev != nil && prev.To.After(override.From)) || (next != nil && next.From.Before(override.To)) {
			return nil, computeErr(CodePeriodConflict, "period_key", key)
		}
		saved[key] = override
	}
	out := make(map[string]energy.Window, 3)
	for i := 1; i <= 3; i++ {
		w, err := domain.ResolvePeriod(keys[i], int(building.BillCutoffDay), saved[keys[i-1]], saved[keys[i]], saved[keys[i+1]], s.loc)
		if errors.Is(err, domain.ErrPeriodConflict) {
			return nil, computeErr(CodePeriodConflict, "period_key", keys[i])
		}
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrInvalidRequest, err)
		}
		out[keys[i]] = w
	}
	return out, nil
}

// regenerations: key's building and analyzer bills always; a neighbour's live
// bills when its window moved (whatever their status: computed bills are
// issued by default); the company bill of each such key when the scope
// covers every building (R557).
func (s *Service) regenerations(ctx context.Context, sc store.Scope, building model.Building, key string, before, after map[string]energy.Window) ([]Regeneration, error) {
	members, err := s.buildingAnalyzers(ctx, sc, building.ID)
	if err != nil {
		return nil, err
	}
	var out []Regeneration
	for _, k := range []string{domain.ShiftKey(key, -1), key, domain.ShiftKey(key, 1)} {
		if k != key && before[k].From.Equal(after[k].From) && before[k].To.Equal(after[k].To) {
			continue
		}
		subjects := []Regeneration{{Scope: model.BillScopeBuilding, SubjectID: building.ID, PeriodKey: k}}
		for _, a := range members {
			subjects = append(subjects, Regeneration{Scope: model.BillScopeAnalyzer, SubjectID: a.ID, PeriodKey: k})
		}
		if _, all := sc.BuildingFilter(); all {
			subjects = append(subjects, Regeneration{Scope: model.BillScopeCompany, SubjectID: sc.CompanyID, PeriodKey: k})
		}
		for _, r := range subjects {
			_, err := s.deps.Bills.Current(ctx, sc, r.Scope, r.SubjectID, k)
			switch {
			case errors.Is(err, store.ErrNotFound):
				if k == key && r.Scope != model.BillScopeCompany {
					out = append(out, r) // the saved period's own bills are made now
				}
				continue
			case err != nil:
				return nil, err
			}
			out = append(out, r)
		}
	}
	return out, nil
}

// Building is a visible building (the bill-periods routes' 404 check).
func (s *Service) Building(ctx context.Context, sc store.Scope, id uuid.UUID) (model.Building, error) {
	return s.deps.Buildings.Get(ctx, sc, id)
}

// BillPeriods lists a building's saved windows.
func (s *Service) BillPeriods(ctx context.Context, sc store.Scope, buildingID uuid.UUID) ([]model.BillPeriod, error) {
	return s.deps.Bills.BillPeriods(ctx, sc, buildingID, nil)
}
