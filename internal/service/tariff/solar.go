package tariff

import (
	"context"

	"github.com/google/uuid"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// Solar tariffs price a plant's generation (01 §7.11, tab 2). They are plant
// scoped: a plant the caller cannot see answers exactly like an unknown one
// (R244), which is what keeps a plant id from being an oracle.

// ListSolarTariffs returns a plant's tariff history, newest first.
func (s *Service) ListSolarTariffs(ctx context.Context, sc store.Scope, plantID uuid.UUID, p store.Page) ([]model.SolarTariff, error) {
	if err := s.plantVisible(ctx, sc, plantID); err != nil {
		return nil, err
	}
	return s.deps.SolarTariffs.List(ctx, sc, store.SolarTariffFilter{PlantID: &plantID, Page: p})
}

// CreateSolarTariff validates and stores one feed-in price.
func (s *Service) CreateSolarTariff(ctx context.Context, sc store.Scope, t model.SolarTariff) (model.SolarTariff, error) {
	if err := s.plantVisible(ctx, sc, t.PlantID); err != nil {
		return model.SolarTariff{}, err
	}
	if t.EffectiveFrom.IsZero() {
		return model.SolarTariff{}, ErrInvalidRequest
	}
	if t.FeedInTariff.IsNegative() {
		return model.SolarTariff{}, ErrInvalidRequest
	}
	if t.PurchasePrice != nil && t.PurchasePrice.IsNegative() {
		return model.SolarTariff{}, ErrInvalidRequest
	}
	if t.Currency == "" {
		t.Currency = model.CurrencyTRY
	}
	if !t.Currency.Valid() {
		return model.SolarTariff{}, ErrInvalidRequest
	}
	t.CompanyID = sc.CompanyID
	t.CreatedAt = s.deps.Clock.Now().UTC()
	return s.deps.SolarTariffs.Create(ctx, sc, t)
}

// DeleteSolarTariff soft-deletes one price; the history keeps it out of sight
// but an invoice priced with it is unaffected.
func (s *Service) DeleteSolarTariff(ctx context.Context, sc store.Scope, id uuid.UUID) error {
	return s.deps.SolarTariffs.SoftDelete(ctx, sc, id, s.deps.Clock.Now().UTC())
}

func (s *Service) plantVisible(ctx context.Context, sc store.Scope, plantID uuid.UUID) error {
	if s.deps.Plants == nil || plantID == uuid.Nil {
		return store.ErrNotFound
	}
	_, err := s.deps.Plants.Get(ctx, sc, plantID)
	return err
}

// ListDefaults reads the platform default-tariff catalogue (R243).
func (s *Service) ListDefaults(ctx context.Context, f store.NationalTariffFilter) ([]model.NationalTariffScheduleEntry, error) {
	if s.deps.Catalogue == nil {
		return nil, ErrInvalidRequest
	}
	return s.deps.Catalogue.ListNationalTariffSchedule(ctx, f)
}

// UpsertDefault writes one catalogue row, keyed by (effective_from,
// user_group, voltage_level, term) — the unique key the table already
// declares, which is why editing a row is a re-POST rather than a PATCH.
func (s *Service) UpsertDefault(ctx context.Context, e model.NationalTariffScheduleEntry) error {
	if s.deps.Catalogue == nil {
		return ErrInvalidRequest
	}
	if e.EffectiveFrom.IsZero() || !e.UserGroup.Valid() || !e.VoltageLevel.Valid() || !e.Term.Valid() {
		return ErrInvalidRequest
	}
	_, err := s.deps.Catalogue.UpsertNationalTariffSchedule(ctx, []model.NationalTariffScheduleEntry{e})
	return err
}

// DeleteDefault removes one catalogue row.
func (s *Service) DeleteDefault(ctx context.Context, id uuid.UUID) error {
	if s.deps.Catalogue == nil {
		return ErrInvalidRequest
	}
	return s.deps.Catalogue.DeleteNationalTariffScheduleEntry(ctx, id)
}
