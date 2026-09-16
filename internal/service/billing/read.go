package billing

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// Get returns a bill with its lines and members (05 §7).
func (s *Service) Get(ctx context.Context, sc store.Scope, id uuid.UUID) (model.Bill, []model.BillLine, []model.BillMember, error) {
	b, err := s.deps.Bills.Get(ctx, sc, id)
	if err != nil {
		return model.Bill{}, nil, nil, err
	}
	lines, err := s.deps.Bills.Lines(ctx, sc, id)
	if err != nil {
		return model.Bill{}, nil, nil, err
	}
	members, err := s.deps.Bills.Members(ctx, sc, id)
	if err != nil {
		return model.Bill{}, nil, nil, err
	}
	return b, lines, members, nil
}

// List returns the Scope's bills matching f.
func (s *Service) List(ctx context.Context, sc store.Scope, f store.BillFilter) ([]model.Bill, error) {
	return s.deps.Bills.List(ctx, sc, f)
}

// HourlyDetail returns a PTF bill's priced hours.
func (s *Service) HourlyDetail(ctx context.Context, sc store.Scope, id uuid.UUID) ([]model.BillHourlyDetail, error) {
	return s.deps.Bills.HourlyDetail(ctx, sc, id)
}

// ErrUnknownScope is returned by Latest for an invalid bill scope.
var ErrUnknownScope = errors.New("billing: unknown bill scope")

// Latest returns the newest live bill of a subject.
func (s *Service) Latest(ctx context.Context, sc store.Scope, scope model.BillScope, subjectID uuid.UUID) (model.Bill, error) {
	f := store.BillFilter{BillScope: &scope, Page: store.Page{Limit: 1}}
	switch scope {
	case model.BillScopeAnalyzer:
		f.AnalyzerID = &subjectID
	case model.BillScopeBuilding:
		f.BuildingID = &subjectID
	case model.BillScopeCompany:
		if subjectID != sc.CompanyID {
			return model.Bill{}, store.ErrNotFound
		}
	default:
		return model.Bill{}, ErrUnknownScope
	}
	bills, err := s.deps.Bills.List(ctx, sc, f)
	if err != nil {
		return model.Bill{}, err
	}
	if len(bills) == 0 {
		return model.Bill{}, store.ErrNotFound
	}
	return bills[0], nil
}
