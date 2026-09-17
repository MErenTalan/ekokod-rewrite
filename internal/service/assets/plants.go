package assets

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// PlantDetail is a plant with its devices, monthly targets and alarm recipients.
type PlantDetail struct {
	Plant           model.PowerPlant
	MonthlyTargets  []model.PlantMonthlyTarget
	Devices         []model.PlantDevice
	AlarmRecipients []string
}

// PlantInput is a plant create or partial update; nil keeps, "" clears strings.
type PlantInput struct {
	Name, InstallationNumber, PlantKind, PvBrandModel, Address *string
	PanelPowerW, PanelEfficiencyPct, TiltAngleDeg              *decimal.Decimal
	TotalCapacityKw, YearlyTargetKwh                           *decimal.Decimal
	Latitude, Longitude                                        *decimal.Decimal
	PanelCount, StringCount                                    *int32
	Orientation                                                *model.PanelOrientation
	InstallationDate                                           *time.Time
	MonthlyTargets                                             *[]decimal.Decimal // exactly 12 when set
	AlarmRecipients                                            *[]string
}

// ListPlants lists the scope company's plants.
func (s *Service) ListPlants(ctx context.Context, sc store.Scope, page store.Page) ([]model.PowerPlant, error) {
	return s.d.Plants.List(ctx, sc, store.PlantFilter{Page: page})
}

// GetPlant returns a plant with its children.
func (s *Service) GetPlant(ctx context.Context, sc store.Scope, id uuid.UUID) (PlantDetail, error) {
	p, err := s.d.Plants.Get(ctx, sc, id)
	if err != nil {
		return PlantDetail{}, err
	}
	targets, err := s.d.Plants.MonthlyTargets(ctx, sc, id)
	if err != nil {
		return PlantDetail{}, err
	}
	devices, err := s.d.Plants.Devices(ctx, sc, id)
	if err != nil {
		return PlantDetail{}, err
	}
	recipients, err := s.d.Plants.AlarmRecipients(ctx, sc, id)
	if err != nil {
		return PlantDetail{}, err
	}
	emails := make([]string, len(recipients))
	for i, r := range recipients {
		emails[i] = r.Email
	}
	return PlantDetail{Plant: p, MonthlyTargets: targets, Devices: devices, AlarmRecipients: emails}, nil
}

func applyPlant(p model.PowerPlant, in PlantInput) model.PowerPlant {
	if in.Name != nil {
		p.Name = *in.Name
	}
	if in.PlantKind != nil {
		p.PlantKind = *in.PlantKind
	}
	p.InstallationNumber = keepOrClear(p.InstallationNumber, in.InstallationNumber)
	p.PvBrandModel = keepOrClear(p.PvBrandModel, in.PvBrandModel)
	p.Address = keepOrClear(p.Address, in.Address)
	for _, pair := range []struct {
		dst **decimal.Decimal
		src *decimal.Decimal
	}{
		{&p.PanelPowerW, in.PanelPowerW}, {&p.PanelEfficiencyPct, in.PanelEfficiencyPct}, {&p.TiltAngleDeg, in.TiltAngleDeg},
		{&p.TotalCapacityKw, in.TotalCapacityKw}, {&p.YearlyTargetKwh, in.YearlyTargetKwh},
		{&p.Latitude, in.Latitude}, {&p.Longitude, in.Longitude},
	} {
		if pair.src != nil {
			*pair.dst = pair.src
		}
	}
	if in.PanelCount != nil {
		p.PanelCount = in.PanelCount
	}
	if in.StringCount != nil {
		p.StringCount = in.StringCount
	}
	if in.Orientation != nil {
		p.Orientation = in.Orientation
	}
	if in.InstallationDate != nil {
		p.InstallationDate = in.InstallationDate
	}
	return p
}

func (s *Service) writeChildren(ctx context.Context, sc store.Scope, id uuid.UUID, in PlantInput) error {
	if in.MonthlyTargets != nil {
		if len(*in.MonthlyTargets) != 12 {
			return validation("monthly_targets", "len")
		}
		targets := make([]model.PlantMonthlyTarget, 12)
		for i, v := range *in.MonthlyTargets {
			targets[i] = model.PlantMonthlyTarget{PlantID: id, Month: int16(i + 1), TargetKwh: v}
		}
		if _, err := s.d.Plants.ReplaceMonthlyTargets(ctx, sc, id, targets); err != nil {
			return err
		}
	}
	if in.AlarmRecipients != nil {
		emails := make([]string, 0, len(*in.AlarmRecipients))
		for _, e := range *in.AlarmRecipients {
			emails = append(emails, strings.ToLower(strings.TrimSpace(e)))
		}
		if _, err := s.d.Plants.ReplaceAlarmRecipients(ctx, sc, id, emails); err != nil {
			return err
		}
	}
	return nil
}

// CreatePlant creates a plant, its monthly targets and alarm recipients.
func (s *Service) CreatePlant(ctx context.Context, sc store.Scope, in PlantInput) (PlantDetail, error) {
	if in.Name == nil || *in.Name == "" {
		return PlantDetail{}, validation("name", "required")
	}
	if in.MonthlyTargets != nil && len(*in.MonthlyTargets) != 12 {
		return PlantDetail{}, validation("monthly_targets", "len")
	}
	now := s.d.Clock.Now()
	p := applyPlant(model.PowerPlant{ID: uuid.New(), CompanyID: sc.CompanyID, PlantKind: "rooftop", CreatedAt: now, UpdatedAt: now}, in)
	created, err := s.d.Plants.Create(ctx, sc, p)
	if err != nil {
		return PlantDetail{}, err
	}
	if err := s.writeChildren(ctx, sc, created.ID, in); err != nil {
		return PlantDetail{}, err
	}
	return s.GetPlant(ctx, sc, created.ID)
}

// UpdatePlant applies a partial update.
func (s *Service) UpdatePlant(ctx context.Context, sc store.Scope, id uuid.UUID, in PlantInput) (PlantDetail, error) {
	p, err := s.d.Plants.Get(ctx, sc, id)
	if err != nil {
		return PlantDetail{}, err
	}
	if in.MonthlyTargets != nil && len(*in.MonthlyTargets) != 12 {
		return PlantDetail{}, validation("monthly_targets", "len")
	}
	p = applyPlant(p, in)
	p.UpdatedAt = s.d.Clock.Now()
	if _, err := s.d.Plants.Update(ctx, sc, p); err != nil {
		return PlantDetail{}, err
	}
	if err := s.writeChildren(ctx, sc, id, in); err != nil {
		return PlantDetail{}, err
	}
	return s.GetPlant(ctx, sc, id)
}

// DeletePlant soft-deletes a plant.
func (s *Service) DeletePlant(ctx context.Context, sc store.Scope, id uuid.UUID) error {
	return s.d.Plants.SoftDelete(ctx, sc, id, s.d.Clock.Now())
}
