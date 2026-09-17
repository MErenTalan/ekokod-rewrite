package assets

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// Activity statuses (R163).
const (
	StatusActive  = "active"
	StatusPassive = "passive"
)

// BuildingView is a building with the optional list includes.
type BuildingView struct {
	Building            model.Building
	AnalyzerCount       *int
	ActiveAnalyzerCount *int
	ActivityStatus      *string
}

// BuildingListInput narrows GET /buildings.
type BuildingListInput struct {
	Q, Sector                    string
	IncludeCounts, IncludeStatus bool
	Page                         store.Page
}

// ListBuildings lists buildings in scope, with counts and status when asked (R163).
func (s *Service) ListBuildings(ctx context.Context, sc store.Scope, in BuildingListInput) ([]BuildingView, error) {
	f := store.BuildingFilter{NameContains: in.Q, Page: in.Page}
	if in.Sector != "" {
		f.Sector = &in.Sector
	}
	buildings, err := s.d.Buildings.List(ctx, sc, f)
	if err != nil {
		return nil, err
	}
	out := make([]BuildingView, len(buildings))
	for i, b := range buildings {
		out[i] = BuildingView{Building: b}
	}
	if !in.IncludeCounts && !in.IncludeStatus {
		return out, nil
	}
	analyzers, err := listAll(func(p store.Page) ([]model.Analyzer, error) {
		return s.d.Analyzers.List(ctx, sc, store.AnalyzerFilter{Page: p})
	})
	if err != nil {
		return nil, err
	}
	total, active := map[uuid.UUID]int{}, map[uuid.UUID]int{}
	now := s.d.Clock.Now()
	for _, a := range analyzers {
		if a.BuildingID == nil {
			continue
		}
		total[*a.BuildingID]++
		if Active(a, now) {
			active[*a.BuildingID]++
		}
	}
	for i := range out {
		id := out[i].Building.ID
		if in.IncludeCounts {
			t, a := total[id], active[id]
			out[i].AnalyzerCount, out[i].ActiveAnalyzerCount = &t, &a
		}
		if in.IncludeStatus {
			status := StatusPassive
			if active[id] > 0 {
				status = StatusActive
			}
			out[i].ActivityStatus = &status
		}
	}
	return out, nil
}

// BuildingDetail is GET /buildings/{id}.
type BuildingDetail struct {
	Building      model.Building
	Contacts      []model.BuildingContact
	TariffHistory []model.Tariff
}

// GetBuilding returns a building with contacts and its tariff history.
func (s *Service) GetBuilding(ctx context.Context, sc store.Scope, id uuid.UUID) (BuildingDetail, error) {
	b, err := s.d.Buildings.Get(ctx, sc, id)
	if err != nil {
		return BuildingDetail{}, err
	}
	contacts, err := s.d.Buildings.Contacts(ctx, sc, id)
	if err != nil {
		return BuildingDetail{}, err
	}
	tariffs, err := listAll(func(p store.Page) ([]model.Tariff, error) {
		return s.d.Tariffs.List(ctx, sc, store.TariffFilter{BuildingID: &id, Page: p})
	})
	if err != nil {
		return BuildingDetail{}, err
	}
	return BuildingDetail{Building: b, Contacts: contacts, TariffHistory: tariffs}, nil
}

// Contact is one building contact.
type Contact struct{ Name, Phone *string }

// BuildingInput is a building create or partial update; nil keeps a field, "" clears.
type BuildingInput struct {
	Name, Address, Sector *string
	Latitude, Longitude   *decimal.Decimal
	TotalAreaM2           *decimal.Decimal
	Floors                *int32
	PersonnelCount        *int32
	ResponsibleUserID     *uuid.UUID
	ClearResponsible      bool
	BillCutoffDay         *int16
	Contacts              *[]Contact // nil keeps; empty clears
}

func applyBuilding(b model.Building, in BuildingInput) model.Building {
	if in.Name != nil {
		b.Name = *in.Name
	}
	b.Address = keepOrClear(b.Address, in.Address)
	b.Sector = keepOrClear(b.Sector, in.Sector)
	if in.Latitude != nil {
		b.Latitude = in.Latitude
	}
	if in.Longitude != nil {
		b.Longitude = in.Longitude
	}
	if in.TotalAreaM2 != nil {
		b.TotalAreaM2 = in.TotalAreaM2
	}
	if in.Floors != nil {
		b.Floors = in.Floors
	}
	if in.PersonnelCount != nil {
		b.PersonnelCount = in.PersonnelCount
	}
	if in.ResponsibleUserID != nil {
		b.ResponsibleUserID = in.ResponsibleUserID
	}
	if in.ClearResponsible {
		b.ResponsibleUserID = nil
	}
	if in.BillCutoffDay != nil {
		b.BillCutoffDay = *in.BillCutoffDay
	}
	return b
}

// CreateBuilding creates a building and its contacts (R175).
func (s *Service) CreateBuilding(ctx context.Context, sc store.Scope, in BuildingInput) (BuildingDetail, error) {
	if in.Name == nil || *in.Name == "" {
		return BuildingDetail{}, validation("name", "required")
	}
	now := s.d.Clock.Now()
	b := applyBuilding(model.Building{ID: uuid.New(), CompanyID: sc.CompanyID, BillCutoffDay: 1, CreatedAt: now, UpdatedAt: now}, in)
	created, err := s.d.Buildings.Create(ctx, sc, b)
	if errors.Is(err, store.ErrNotFound) && b.ResponsibleUserID != nil {
		return BuildingDetail{}, validation("responsible_user_id", "invalid")
	}
	if err != nil {
		return BuildingDetail{}, err
	}
	if err := s.replaceContacts(ctx, sc, created.ID, in.Contacts); err != nil {
		return BuildingDetail{}, err
	}
	return s.GetBuilding(ctx, sc, created.ID)
}

// UpdateBuilding applies a partial update.
func (s *Service) UpdateBuilding(ctx context.Context, sc store.Scope, id uuid.UUID, in BuildingInput) (BuildingDetail, error) {
	b, err := s.d.Buildings.Get(ctx, sc, id)
	if err != nil {
		return BuildingDetail{}, err
	}
	if in.Name != nil && *in.Name == "" {
		return BuildingDetail{}, validation("name", "required")
	}
	b = applyBuilding(b, in)
	b.UpdatedAt = s.d.Clock.Now()
	if _, err := s.d.Buildings.Update(ctx, sc, b); errors.Is(err, store.ErrNotFound) {
		return BuildingDetail{}, validation("responsible_user_id", "invalid")
	} else if err != nil {
		return BuildingDetail{}, err
	}
	if err := s.replaceContacts(ctx, sc, id, in.Contacts); err != nil {
		return BuildingDetail{}, err
	}
	return s.GetBuilding(ctx, sc, id)
}

func (s *Service) replaceContacts(ctx context.Context, sc store.Scope, id uuid.UUID, contacts *[]Contact) error {
	if contacts == nil {
		return nil
	}
	rows := make([]model.BuildingContact, len(*contacts))
	for i, c := range *contacts {
		rows[i] = model.BuildingContact{BuildingID: id, Name: c.Name, Phone: c.Phone, SortOrder: int16(i)}
	}
	_, err := s.d.Buildings.ReplaceContacts(ctx, sc, id, rows)
	return err
}

// DeleteBuilding soft-deletes a building with no live analyzers (R175).
func (s *Service) DeleteBuilding(ctx context.Context, sc store.Scope, id uuid.UUID) error {
	if _, err := s.d.Buildings.Get(ctx, sc, id); err != nil {
		return err
	}
	attached, err := s.d.Analyzers.List(ctx, sc, store.AnalyzerFilter{BuildingID: &id, Page: store.Page{Limit: 1}})
	if err != nil {
		return err
	}
	if len(attached) > 0 {
		return ErrBuildingHasAnalyzers
	}
	return s.d.Buildings.SoftDelete(ctx, sc, id, s.d.Clock.Now())
}
