package tenancy

import (
	"context"
	"errors"
	"sort"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	perr "github.com/MErenTalan/ekokod-rewrite/internal/platform/errors"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// CompanyInput is a company create or partial update; nil keeps a field.
type CompanyInput struct {
	Name, Address, ContactName, ContactPhone, Sector *string
	TotalAreaM2                                      *decimal.Decimal
	PersonnelCount                                   *int32
}

// AnalyzerCount is one provider/subtype count on a company.
type AnalyzerCount struct {
	Provider model.IntegrationProvider
	Subtype  string
	Count    int
}

// CompanyDetail is a company with its analyzer counts (05 §3).
type CompanyDetail struct {
	Company        model.Company
	AnalyzerCounts []AnalyzerCount
}

// ListCompanies lists every tenant; the route allows the admin role only.
func (s *Service) ListCompanies(ctx context.Context, f store.CompanyFilter) ([]model.Company, error) {
	return s.d.AdminTenant.ListCompanies(ctx, f)
}

// CreateCompany creates a tenant.
func (s *Service) CreateCompany(ctx context.Context, in CompanyInput) (model.Company, error) {
	if in.Name == nil || *in.Name == "" {
		return model.Company{}, validation("name", "required")
	}
	id := uuid.New()
	now := s.d.Clock.Now()
	c := apply(model.Company{ID: id, CreatedAt: now, UpdatedAt: now}, in)
	created, err := s.d.Companies.Create(ctx, store.Scope{CompanyID: id, AllBuildings: true}, c)
	if errors.Is(err, store.ErrConflict) {
		return model.Company{}, ErrCompanyNameTaken
	}
	return created, err
}

// companyScope is the scope a company route acts in: an admin addresses any
// company by path id, everyone else only their resolved scope's company (R173).
func companyScope(role model.UserRole, sc store.Scope, id uuid.UUID) (store.Scope, error) {
	if role == model.UserRoleAdmin {
		return store.Scope{CompanyID: id, AllBuildings: true}, nil
	}
	if id != sc.CompanyID {
		return store.Scope{}, perr.NotFound
	}
	return sc, nil
}

// GetCompany returns a company with analyzer counts by provider and subtype.
func (s *Service) GetCompany(ctx context.Context, role model.UserRole, sc store.Scope, id uuid.UUID) (CompanyDetail, error) {
	target, err := companyScope(role, sc, id)
	if err != nil {
		return CompanyDetail{}, err
	}
	company, err := s.d.Companies.Get(ctx, target, id)
	if err != nil {
		return CompanyDetail{}, err
	}
	counts := map[[2]string]int{}
	const page = 500
	for offset := int32(0); ; offset += page {
		list, err := s.d.Analyzers.List(ctx, target, store.AnalyzerFilter{Page: store.Page{Limit: page, Offset: offset}})
		if err != nil {
			return CompanyDetail{}, err
		}
		for _, a := range list {
			counts[[2]string{string(a.Provider), a.ProviderSubtype}]++
		}
		if len(list) < page {
			break
		}
	}
	out := CompanyDetail{Company: company, AnalyzerCounts: []AnalyzerCount{}}
	for k, n := range counts {
		out.AnalyzerCounts = append(out.AnalyzerCounts, AnalyzerCount{Provider: model.IntegrationProvider(k[0]), Subtype: k[1], Count: n})
	}
	sort.Slice(out.AnalyzerCounts, func(i, j int) bool {
		a, b := out.AnalyzerCounts[i], out.AnalyzerCounts[j]
		return a.Provider < b.Provider || (a.Provider == b.Provider && a.Subtype < b.Subtype)
	})
	return out, nil
}

// UpdateCompany applies a partial update.
func (s *Service) UpdateCompany(ctx context.Context, role model.UserRole, sc store.Scope, id uuid.UUID, in CompanyInput) (model.Company, error) {
	target, err := companyScope(role, sc, id)
	if err != nil {
		return model.Company{}, err
	}
	company, err := s.d.Companies.Get(ctx, target, id)
	if err != nil {
		return model.Company{}, err
	}
	if in.Name != nil && *in.Name == "" {
		return model.Company{}, validation("name", "required")
	}
	company = apply(company, in)
	company.UpdatedAt = s.d.Clock.Now()
	updated, err := s.d.Companies.Update(ctx, target, company)
	if errors.Is(err, store.ErrConflict) {
		return model.Company{}, ErrCompanyNameTaken
	}
	return updated, err
}

// DeleteCompany soft-deletes a tenant; the admin's own company is refused.
func (s *Service) DeleteCompany(ctx context.Context, actorCompany, id uuid.UUID) error {
	if id == actorCompany {
		return ErrCannotDeleteOwn
	}
	return s.d.Companies.SoftDelete(ctx, store.Scope{CompanyID: id, AllBuildings: true}, id, s.d.Clock.Now())
}

func apply(c model.Company, in CompanyInput) model.Company {
	if in.Name != nil {
		c.Name = *in.Name
	}
	c.Address = keepOrClear(c.Address, in.Address)
	c.ContactName = keepOrClear(c.ContactName, in.ContactName)
	c.ContactPhone = keepOrClear(c.ContactPhone, in.ContactPhone)
	c.Sector = keepOrClear(c.Sector, in.Sector)
	if in.TotalAreaM2 != nil {
		c.TotalAreaM2 = in.TotalAreaM2
	}
	if in.PersonnelCount != nil {
		c.PersonnelCount = in.PersonnelCount
	}
	return c
}

// keepOrClear keeps current for nil, clears for "", else sets.
func keepOrClear(current, in *string) *string {
	switch {
	case in == nil:
		return current
	case *in == "":
		return nil
	default:
		v := *in
		return &v
	}
}
