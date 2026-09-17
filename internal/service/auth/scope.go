package auth

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	perr "github.com/MErenTalan/ekokod-rewrite/internal/platform/errors"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

const buildingPageSize = 500

// ResolveScope turns the caller and an optional company_id parameter into the
// request's Scope (R139, R158). Any company the caller may not use is not found.
func (s *Service) ResolveScope(ctx context.Context, p Principal, companyParam *uuid.UUID) (store.Scope, error) {
	own := p.User.CompanyID
	if p.User.Role == model.UserRoleAdmin {
		if companyParam == nil || *companyParam == own {
			return store.Scope{CompanyID: own, AllBuildings: true}, nil
		}
		target := store.Scope{CompanyID: *companyParam, AllBuildings: true}
		if _, err := s.d.Companies.Get(ctx, target, *companyParam); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return store.Scope{}, perr.NotFound
			}
			return store.Scope{}, err
		}
		return target, nil
	}
	if companyParam != nil && *companyParam != own {
		return store.Scope{}, perr.NotFound
	}
	switch p.User.Role {
	case model.UserRoleCompanyAdmin, model.UserRoleCompanyReadonlyAdmin, model.UserRoleDemo:
		return store.Scope{CompanyID: own, AllBuildings: true}, nil
	case model.UserRoleBuildingAdmin, model.UserRoleBuildingReadonlyAdmin:
		ids := []uuid.UUID{}
		for offset := int32(0); ; offset += buildingPageSize {
			page, err := s.d.Buildings.List(ctx, store.SystemScope(own), store.BuildingFilter{
				ResponsibleUserID: &p.User.ID, Page: store.Page{Limit: buildingPageSize, Offset: offset},
			})
			if err != nil {
				return store.Scope{}, err
			}
			for _, b := range page {
				ids = append(ids, b.ID)
			}
			if len(page) < buildingPageSize {
				return store.Scope{CompanyID: own, BuildingIDs: ids}, nil
			}
		}
	default:
		return store.Scope{}, perr.Forbidden
	}
}
