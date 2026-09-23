package carbon

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	domain "github.com/MErenTalan/ekokod-rewrite/internal/domain/carbon"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	perr "github.com/MErenTalan/ekokod-rewrite/internal/platform/errors"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// ErrAutomated refuses editing or deleting an accrued record (R306, Q-F7).
var ErrAutomated = perr.New("carbon_activity_automated", 409, "errors.carbon.automated")

var istanbul = func() *time.Location {
	l, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		panic(err)
	}
	return l
}()

// ActivityInput is R305's body.
type ActivityInput struct {
	BuildingID             uuid.UUID
	SubCategory, FactorKey string
	Unit                   string
	Quantity               decimal.Decimal
	PeriodStart, PeriodEnd time.Time
	Description            *string
	Details                map[string]string
}

// ActivityQuery is R319's filter.
type ActivityQuery struct {
	BuildingID *uuid.UUID
	From, To   *time.Time
	Scope      *model.CarbonScope
	Status     *model.CarbonStatus
	Type       *string
	Automated  *bool
	Page       store.Page
}

const (
	maxSpanDays  = 366
	maxDetails   = 10
	maxDetailLen = 100
	maxDescLen   = 500
	snapSource   = "factor_source"
	snapSourceYr = "factor_source_year"
)

var maxQuantity = decimal.New(1, 12)

func civil(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// today is the Istanbul calendar date as a civil date.
func (s *Service) today() time.Time { return civil(s.d.Clock.Now().In(istanbul)) }

func (s *Service) validateShape(in ActivityInput) error {
	if _, ok := domain.SubByKey(in.SubCategory); !ok {
		return validation("sub_category", "unknown")
	}
	if !in.Quantity.IsPositive() || in.Quantity.GreaterThan(maxQuantity) {
		return validation("quantity", "range")
	}
	start, end := civil(in.PeriodStart), civil(in.PeriodEnd)
	switch {
	case end.Before(start):
		return validation("period_end", "invalid_range")
	case int(end.Sub(start).Hours()/24)+1 > maxSpanDays:
		return validation("period_end", "range_too_long")
	case end.After(s.today()):
		return validation("period_end", "future")
	}
	if in.Description != nil && len([]rune(*in.Description)) > maxDescLen {
		return validation("description", "too_long")
	}
	if len(in.Details) > maxDetails {
		return validation("details", "invalid")
	}
	for k, v := range in.Details {
		if len([]rune(k)) > 40 || len([]rune(v)) > maxDetailLen {
			return validation("details", "invalid")
		}
	}
	return nil
}

// compute resolves the factor and unit and fills every derived field (R305).
func (s *Service) compute(ctx context.Context, sc store.Scope, in ActivityInput, a *model.CarbonActivity) error {
	f, err := s.Effective(ctx, sc, in.FactorKey)
	if errors.Is(err, store.ErrNotFound) {
		return validation("factor_key", "not_found")
	}
	if err != nil {
		return err
	}
	if !f.Usable() {
		return validation("factor_key", "archived")
	}
	if !contains(f.SubCategories, in.SubCategory) {
		return validation("factor_key", "not_applicable")
	}
	multiplier, ok := decimal.Zero, false
	if in.Unit == f.BaseUnit {
		multiplier, ok = decimal.NewFromInt(1), true
	}
	for _, c := range f.Conversions {
		if !ok && c.Unit == in.Unit {
			multiplier, ok = c.Multiplier, true
		}
	}
	if !ok {
		return validation("unit", "not_supported")
	}
	sub, _ := domain.SubByKey(in.SubCategory)
	details := map[string]string{}
	for k, v := range in.Details {
		details[k] = v
	}
	delete(details, snapSource)
	delete(details, snapSourceYr)
	if f.Source != nil {
		details[snapSource] = *f.Source
	}
	if f.SourceYear != nil {
		details[snapSourceYr] = strconv.Itoa(int(*f.SourceYear))
	}
	raw, err := json.Marshal(details)
	if err != nil {
		return err
	}
	factorID, key, value := f.ID, f.Key, f.BaseFactor
	a.MainCategory, a.SubCategory, a.ActivityType = sub.Main, sub.Key, sub.Key
	a.PeriodStart, a.PeriodEnd = civil(in.PeriodStart), civil(in.PeriodEnd)
	a.Quantity, a.Unit = in.Quantity, in.Unit
	a.FactorID, a.FactorKey, a.FactorValue, a.ConversionMultiplier = &factorID, &key, &value, multiplier
	a.EmissionKgco2e = domain.Emission(in.Quantity, multiplier, f.BaseFactor)
	a.Scope, a.IsoCategory = sub.Scope, sub.ISO
	a.Description, a.Details = in.Description, raw
	a.Status = model.CarbonStatusPending
	return nil
}

func (s *Service) requireSelected(ctx context.Context, sc store.Scope, buildingID uuid.UUID, sub string) error {
	selected, err := s.Selected(ctx, sc, buildingID)
	if err != nil {
		return err
	}
	if !contains(selected, sub) {
		return validation("sub_category", "not_selected")
	}
	return nil
}

// CreateActivity is R305: the emission is computed here, never taken from the client.
func (s *Service) CreateActivity(ctx context.Context, sc store.Scope, userID uuid.UUID, in ActivityInput) (model.CarbonActivity, error) {
	if err := s.validateShape(in); err != nil {
		return model.CarbonActivity{}, err
	}
	if err := s.requireSelected(ctx, sc, in.BuildingID, in.SubCategory); err != nil {
		return model.CarbonActivity{}, err
	}
	a := model.CarbonActivity{CompanyID: sc.CompanyID, BuildingID: in.BuildingID, CreatedBy: &userID}
	if err := s.compute(ctx, sc, in, &a); err != nil {
		return model.CarbonActivity{}, err
	}
	return s.d.Carbon.CreateActivity(ctx, sc, a)
}

// UpdateActivity is R306: recomputed with today's factor, back to pending.
func (s *Service) UpdateActivity(ctx context.Context, sc store.Scope, id uuid.UUID, in ActivityInput) (model.CarbonActivity, error) {
	old, err := s.d.Carbon.Activity(ctx, sc, id)
	if err != nil {
		return model.CarbonActivity{}, err
	}
	if old.IsAutomated {
		return model.CarbonActivity{}, ErrAutomated
	}
	in.BuildingID = old.BuildingID
	if err := s.validateShape(in); err != nil {
		return model.CarbonActivity{}, err
	}
	// A record keeps its sub-category even if the building later dropped it.
	if in.SubCategory != old.SubCategory {
		if err := s.requireSelected(ctx, sc, old.BuildingID, in.SubCategory); err != nil {
			return model.CarbonActivity{}, err
		}
	}
	next := old
	if err := s.compute(ctx, sc, in, &next); err != nil {
		return model.CarbonActivity{}, err
	}
	return s.d.Carbon.UpdateActivity(ctx, sc, next)
}

// DeleteActivity removes a manual record.
func (s *Service) DeleteActivity(ctx context.Context, sc store.Scope, id uuid.UUID) error {
	old, err := s.d.Carbon.Activity(ctx, sc, id)
	if err != nil {
		return err
	}
	if old.IsAutomated {
		return ErrAutomated
	}
	return s.d.Carbon.DeleteActivity(ctx, sc, id)
}

// SetStatus is R307: approve or reject, idempotent.
func (s *Service) SetStatus(ctx context.Context, sc store.Scope, id uuid.UUID, st model.CarbonStatus) (model.CarbonActivity, error) {
	if st != model.CarbonStatusApproved && st != model.CarbonStatusRejected {
		return model.CarbonActivity{}, validation("status", "invalid")
	}
	return s.d.Carbon.SetActivityStatus(ctx, sc, id, st, s.d.Clock.Now())
}

// Activities is R319's list. A named building outside the scope is 404.
func (s *Service) Activities(ctx context.Context, sc store.Scope, q ActivityQuery) ([]model.CarbonActivity, error) {
	f := store.CarbonActivityFilter{OverlapFrom: q.From, OverlapTo: q.To, ActivityType: q.Type, IsAutomated: q.Automated, Page: q.Page}
	if q.BuildingID != nil {
		if _, err := s.d.Buildings.Get(ctx, sc, *q.BuildingID); err != nil {
			return nil, err
		}
		f.BuildingIDs = []uuid.UUID{*q.BuildingID}
	}
	if q.Scope != nil {
		f.Scopes = []model.CarbonScope{*q.Scope}
	}
	if q.Status != nil {
		f.Statuses = []model.CarbonStatus{*q.Status}
	}
	return s.d.Carbon.ListActivities(ctx, sc, f)
}

// allActivities pages through a building's activities overlapping [from, to].
func (s *Service) allActivities(ctx context.Context, sc store.Scope, buildingID uuid.UUID, from, to time.Time, automated *bool) ([]model.CarbonActivity, error) {
	var out []model.CarbonActivity
	for offset := int32(0); ; offset += pageSize {
		page, err := s.d.Carbon.ListActivities(ctx, sc, store.CarbonActivityFilter{BuildingIDs: []uuid.UUID{buildingID},
			OverlapFrom: &from, OverlapTo: &to, IsAutomated: automated, Page: store.Page{Limit: pageSize, Offset: offset}})
		if err != nil {
			return nil, err
		}
		out = append(out, page...)
		if len(page) < pageSize {
			return out, nil
		}
	}
}

func record(a model.CarbonActivity) domain.Record {
	return domain.Record{Sub: a.SubCategory, Start: a.PeriodStart, End: a.PeriodEnd, KgCO2e: a.EmissionKgco2e,
		Quantity: a.Quantity, Status: a.Status, Automated: a.IsAutomated}
}
