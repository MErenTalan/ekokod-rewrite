package tariff

import (
	"context"
	"errors"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/tariff"
	perr "github.com/MErenTalan/ekokod-rewrite/internal/platform/errors"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// PublicBill estimates a bill for the public calculator (01 §7.19, F12a
// R351) from the national tariff schedule in force on the period's start.
func (s *Service) PublicBill(ctx context.Context, in tariff.PublicInput) (tariff.PublicResult, error) {
	if s.deps.Catalogue == nil {
		return tariff.PublicResult{}, ErrInvalidRequest
	}
	rows, err := s.deps.Catalogue.ListNationalTariffSchedule(ctx, store.NationalTariffFilter{Page: store.Page{Limit: 1000}})
	if err != nil {
		return tariff.PublicResult{}, err
	}
	res, err := tariff.PublicBill(in, tariff.SchedulePicker(rows, in.Start))
	var pe *tariff.PublicError
	if errors.As(err, &pe) {
		return tariff.PublicResult{}, perr.Validation.WithParams(map[string]any{pe.Field: []string{pe.Code}})
	}
	return res, err
}
