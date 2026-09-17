package tariff

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
	_ "time/tzdata" // Europe/Istanbul without host tzdata

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/tariff"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// ErrInvalidRequest is a malformed request.
var ErrInvalidRequest = errors.New("tariff: invalid request")

// Input is a tariff definition to write.
type Input struct {
	Tariff       model.Tariff
	VatRate      *decimal.Decimal
	Taxes        []model.TariffTax
	ExtraCharges []model.TariffExtraCharge
	ManualYekdem []model.TariffManualYekdem
}

// Definition is a stored tariff with its child collections.
type Definition struct {
	Tariff       model.Tariff
	Taxes        []model.TariffTax
	ExtraCharges []model.TariffExtraCharge
	ManualYekdem []model.TariffManualYekdem
}

// Deps are the service's collaborators.
type Deps struct {
	Tariffs   store.TariffRepository
	Templates store.TariffTemplateRepository
	Buildings store.BuildingRepository
	Analyzers store.AnalyzerRepository
	Icmal     store.IcmalRepository
	Prices    store.PriceRepository
	Params    store.BillingParameterRepository
	Clock     clock.Clock
	Log       *slog.Logger
}

// Service is the tariff service.
type Service struct {
	deps Deps
	loc  *time.Location
}

// New validates d.
func New(d Deps) (*Service, error) {
	for name, missing := range map[string]bool{
		"Tariffs": d.Tariffs == nil, "Templates": d.Templates == nil, "Buildings": d.Buildings == nil,
		"Analyzers": d.Analyzers == nil, "Icmal": d.Icmal == nil, "Prices": d.Prices == nil, "Params": d.Params == nil,
		"Clock": d.Clock == nil, "Log": d.Log == nil,
	} {
		if missing {
			return nil, fmt.Errorf("tariff: Deps.%s is required", name)
		}
	}
	loc, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		return nil, err
	}
	return &Service{deps: d, loc: loc}, nil
}

func validate(in Input) (model.Tariff, error) {
	return tariff.Validate(tariff.Draft{Tariff: in.Tariff, VatRate: in.VatRate, Taxes: in.Taxes, ExtraCharges: in.ExtraCharges, ManualYekdem: in.ManualYekdem})
}

// Create validates in before any I/O (R128) and writes a new tariff version.
func (s *Service) Create(ctx context.Context, sc store.Scope, in Input) (Definition, error) {
	t, err := validate(in)
	if err != nil {
		return Definition{}, err
	}
	return s.write(ctx, sc, t, in, false)
}

// Update validates in and replaces tariff id with it.
func (s *Service) Update(ctx context.Context, sc store.Scope, id uuid.UUID, in Input) (Definition, error) {
	t, err := validate(in)
	if err != nil {
		return Definition{}, err
	}
	t.ID = id
	return s.write(ctx, sc, t, in, true)
}

func (s *Service) write(ctx context.Context, sc store.Scope, t model.Tariff, in Input, update bool) (Definition, error) {
	now := s.deps.Clock.Now().UTC()
	t.CompanyID, t.UpdatedAt = sc.CompanyID, now
	var saved model.Tariff
	var err error
	if update {
		saved, err = s.deps.Tariffs.Update(ctx, sc, t)
	} else {
		t.CreatedAt = now
		saved, err = s.deps.Tariffs.Create(ctx, sc, t)
	}
	if err != nil {
		return Definition{}, err
	}
	def := Definition{Tariff: saved}
	if def.Taxes, err = s.deps.Tariffs.ReplaceTaxes(ctx, sc, saved.ID, in.Taxes); err != nil {
		return Definition{}, err
	}
	if def.ExtraCharges, err = s.deps.Tariffs.ReplaceExtraCharges(ctx, sc, saved.ID, in.ExtraCharges); err != nil {
		return Definition{}, err
	}
	if def.ManualYekdem, err = s.deps.Tariffs.ReplaceManualYekdem(ctx, sc, saved.ID, in.ManualYekdem); err != nil {
		return Definition{}, err
	}
	return def, nil
}

// Get returns a tariff with its child collections.
func (s *Service) Get(ctx context.Context, sc store.Scope, id uuid.UUID) (Definition, error) {
	t, err := s.deps.Tariffs.Get(ctx, sc, id)
	if err != nil {
		return Definition{}, err
	}
	return s.children(ctx, sc, t)
}

func (s *Service) children(ctx context.Context, sc store.Scope, t model.Tariff) (Definition, error) {
	def := Definition{Tariff: t}
	var err error
	if def.Taxes, err = s.deps.Tariffs.Taxes(ctx, sc, t.ID); err != nil {
		return Definition{}, err
	}
	if def.ExtraCharges, err = s.deps.Tariffs.ExtraCharges(ctx, sc, t.ID); err != nil {
		return Definition{}, err
	}
	if def.ManualYekdem, err = s.deps.Tariffs.ManualYekdem(ctx, sc, t.ID); err != nil {
		return Definition{}, err
	}
	return def, nil
}

// List returns the Scope's tariffs matching f.
func (s *Service) List(ctx context.Context, sc store.Scope, f store.TariffFilter) ([]model.Tariff, error) {
	return s.deps.Tariffs.List(ctx, sc, f)
}

// Delete soft-deletes a tariff version.
func (s *Service) Delete(ctx context.Context, sc store.Scope, id uuid.UUID) error {
	return s.deps.Tariffs.SoftDelete(ctx, sc, id, s.deps.Clock.Now().UTC())
}

// Applicable is the tariff in force for a building on a date: building rows
// first, then company-wide (Tariffs.Effective, I-15).
func (s *Service) Applicable(ctx context.Context, sc store.Scope, buildingID uuid.UUID, on time.Time) (Definition, error) {
	t, err := s.deps.Tariffs.Effective(ctx, sc, buildingID, on)
	if err != nil {
		return Definition{}, err
	}
	return s.children(ctx, sc, t)
}

// CurrentForBuildings maps every visible building to its applicable tariff, or nil.
func (s *Service) CurrentForBuildings(ctx context.Context, sc store.Scope, on time.Time) (map[uuid.UUID]*Definition, error) {
	buildings, err := listAll(func(p store.Page) ([]model.Building, error) {
		return s.deps.Buildings.List(ctx, sc, store.BuildingFilter{Page: p})
	})
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]*Definition, len(buildings))
	for _, b := range buildings {
		def, err := s.Applicable(ctx, sc, b.ID, on)
		switch {
		case errors.Is(err, store.ErrNotFound):
			out[b.ID] = nil
		case err != nil:
			return nil, err
		default:
			out[b.ID] = &def
		}
	}
	return out, nil
}

// BulkAssign creates one version of in per building, all or nothing: every
// building must be visible before anything is written.
func (s *Service) BulkAssign(ctx context.Context, sc store.Scope, in Input, buildingIDs []uuid.UUID) ([]Definition, error) {
	if len(buildingIDs) == 0 {
		return nil, ErrInvalidRequest
	}
	if _, err := validate(in); err != nil {
		return nil, err
	}
	for _, id := range buildingIDs {
		if _, err := s.deps.Buildings.Get(ctx, sc, id); err != nil {
			return nil, err
		}
	}
	out := make([]Definition, 0, len(buildingIDs))
	for _, id := range buildingIDs {
		one := in
		building := id
		one.Tariff.BuildingID = &building
		def, err := s.Create(ctx, sc, one)
		if err != nil {
			return nil, err
		}
		out = append(out, def)
	}
	return out, nil
}

const pageSize = 500

func listAll[T any](list func(store.Page) ([]T, error)) ([]T, error) {
	var out []T
	for offset := int32(0); ; offset += pageSize {
		page, err := list(store.Page{Limit: pageSize, Offset: offset})
		if err != nil {
			return nil, err
		}
		out = append(out, page...)
		if len(page) < pageSize {
			return out, nil
		}
	}
}
