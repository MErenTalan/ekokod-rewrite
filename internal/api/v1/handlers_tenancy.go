package v1

import (
	"net/http"

	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/dto"
	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/kit"
	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/mw"
	"github.com/MErenTalan/ekokod-rewrite/internal/auth"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/tenancy"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

const pageCap = 500

func tenancyRoutes() []Route {
	rolesA := auth.Roles(roleA)
	rolesACA := auth.Roles(roleA, roleCA)
	rolesACACR := auth.Roles(roleA, roleCA, roleCR)
	return []Route{
		{Method: http.MethodGet, Pattern: "/companies", OperationID: "companies.list", Tag: "companies", Access: RoleGated, Roles: rolesA,
			Summary: "Every company (platform operator).", Request: dto.CompanyListRequest{}, Response: dto.Page[dto.Company]{},
			Status: http.StatusOK, Handler: (*Handlers).listCompanies},
		{Method: http.MethodPost, Pattern: "/companies", OperationID: "companies.create", Tag: "companies", Access: RoleGated, Roles: rolesA,
			Entity: "company", PlatformAudit: true, Summary: "Create a company.", Request: dto.CompanyCreateRequest{},
			Response: dto.Company{}, Status: http.StatusCreated, Handler: (*Handlers).createCompany},
		{Method: http.MethodGet, Pattern: "/companies/{id}", OperationID: "companies.get", Tag: "companies", Access: RoleGated, Roles: rolesACACR,
			Summary: "A company with analyzer counts by provider.", Request: dto.IDPath{}, Response: dto.CompanyDetail{},
			Status: http.StatusOK, Handler: (*Handlers).getCompany},
		{Method: http.MethodPatch, Pattern: "/companies/{id}", OperationID: "companies.update", Tag: "companies", Access: RoleGated, Roles: rolesACA,
			Entity: "company", Summary: "Update a company.", Request: dto.CompanyUpdateRequest{}, Response: dto.Company{},
			Status: http.StatusOK, Handler: (*Handlers).updateCompany},
		{Method: http.MethodDelete, Pattern: "/companies/{id}", OperationID: "companies.delete", Tag: "companies", Access: RoleGated, Roles: rolesA,
			Entity: "company", PlatformAudit: true, Summary: "Soft-delete a company.", Request: dto.IDPath{},
			Status: http.StatusNoContent, Handler: (*Handlers).deleteCompany},
		{Method: http.MethodGet, Pattern: "/users", OperationID: "users.list", Tag: "users", Access: RoleGated, Roles: rolesACACR,
			Summary: "Users of the company in scope.", Request: dto.UserListRequest{}, Response: dto.Page[dto.User]{},
			Status: http.StatusOK, Handler: (*Handlers).listUsers},
		{Method: http.MethodPost, Pattern: "/users", OperationID: "users.create", Tag: "users", Access: RoleGated, Roles: rolesACA,
			Entity: "user", Summary: "Create a user; role options depend on the caller's role.", Request: dto.UserCreateRequest{},
			Response: dto.User{}, Status: http.StatusCreated, Handler: (*Handlers).createUser},
		{Method: http.MethodPatch, Pattern: "/users/{id}", OperationID: "users.update", Tag: "users", Access: RoleGated, Roles: rolesACA,
			Entity: "user", Summary: "Update a user; role, status and e-mail changes end their sessions.",
			Request: dto.UserUpdateRequest{}, Response: dto.User{}, Status: http.StatusOK, Handler: (*Handlers).updateUser},
		{Method: http.MethodDelete, Pattern: "/users/{id}", OperationID: "users.delete", Tag: "users", Access: RoleGated, Roles: rolesACA,
			Entity: "user", Summary: "Soft-delete a user.", Request: dto.IDPath{}, Status: http.StatusNoContent, Handler: (*Handlers).deleteUser},
		{Method: http.MethodGet, Pattern: "/smtp-settings", OperationID: "smtp.get", Tag: "smtp", Access: RoleGated, Roles: rolesA,
			Summary: "SMTP settings of the company in scope; the password is never returned.", Response: dto.SMTPSettings{},
			Status: http.StatusOK, Handler: (*Handlers).getSMTP},
		{Method: http.MethodPut, Pattern: "/smtp-settings", OperationID: "smtp.upsert", Tag: "smtp", Access: RoleGated, Roles: rolesA,
			Entity: "smtp_settings", Summary: "Create or replace SMTP settings.", Request: dto.SMTPPutRequest{},
			Response: dto.SMTPSettings{}, Status: http.StatusOK, Handler: (*Handlers).putSMTP},
		{Method: http.MethodPost, Pattern: "/smtp-settings/test", OperationID: "smtp.test", Tag: "smtp", Access: RoleGated, Roles: rolesA,
			Entity: "smtp_settings", NoIdempotency: true, Summary: "Send a test message with the stored settings.",
			Request: dto.SMTPTestRequest{}, Status: http.StatusNoContent, Handler: (*Handlers).testSMTP},
	}
}

func companyDTO(c model.Company) dto.Company {
	return dto.Company{
		ID: c.ID, Name: c.Name, Address: c.Address, TotalAreaM2: dto.DP(c.TotalAreaM2), PersonnelCount: c.PersonnelCount,
		ContactName: c.ContactName, ContactPhone: c.ContactPhone, Sector: c.Sector,
		CreatedAt: dto.T(c.CreatedAt), UpdatedAt: dto.T(c.UpdatedAt),
	}
}

func companyInput(name *string, f dto.CompanyFields) tenancy.CompanyInput {
	in := tenancy.CompanyInput{
		Name: name, Address: f.Address, ContactName: f.ContactName, ContactPhone: f.ContactPhone,
		Sector: f.Sector, PersonnelCount: f.PersonnelCount,
	}
	if f.TotalAreaM2 != nil {
		in.TotalAreaM2 = &f.TotalAreaM2.Decimal
	}
	return in
}

func (h *Handlers) listCompanies(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.CompanyListRequest) (any, error) {
		page, limit, err := kit.ResolvePage(req.PageRequest, pageCap)
		if err != nil {
			return nil, err
		}
		list, err := h.Tenancy.ListCompanies(r.Context(), store.CompanyFilter{NameContains: req.Q, Sector: req.Sector, Page: page})
		if err != nil {
			return nil, err
		}
		items := make([]dto.Company, len(list))
		for i, c := range list {
			items[i] = companyDTO(c)
		}
		return kit.PageOf(items, page, limit), nil
	})
}

func (h *Handlers) createCompany(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusCreated, func(req dto.CompanyCreateRequest) (any, error) {
		c, err := h.Tenancy.CreateCompany(r.Context(), companyInput(&req.Name, req.CompanyFields))
		return companyDTO(c), err
	})
}

func (h *Handlers) getCompany(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.IDPath) (any, error) {
		d, err := h.Tenancy.GetCompany(r.Context(), principal(r).User.Role, mw.ScopeFrom(r), req.ID)
		if err != nil {
			return nil, err
		}
		out := dto.CompanyDetail{Company: companyDTO(d.Company), AnalyzerCounts: make([]dto.AnalyzerCount, len(d.AnalyzerCounts))}
		for i, c := range d.AnalyzerCounts {
			out.AnalyzerCounts[i] = dto.AnalyzerCount{Provider: string(c.Provider), Subtype: c.Subtype, Count: c.Count}
		}
		return out, nil
	})
}

func (h *Handlers) updateCompany(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.CompanyUpdateRequest) (any, error) {
		c, err := h.Tenancy.UpdateCompany(r.Context(), principal(r).User.Role, mw.ScopeFrom(r), req.ID, companyInput(req.Name, req.CompanyFields))
		return companyDTO(c), err
	})
}

func (h *Handlers) deleteCompany(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusNoContent, func(req dto.IDPath) (any, error) {
		return nil, h.Tenancy.DeleteCompany(r.Context(), principal(r).User.CompanyID, req.ID)
	})
}

func userDTO(u model.User) dto.User {
	return dto.User{
		ID: u.ID, Name: u.Name, Email: u.Email, Phone: u.Phone, Role: dto.Role(u.Role), IsActive: u.IsActive,
		Locale: dto.Locale(u.Locale), LastLoginAt: dto.TP(u.LastLoginAt), CreatedAt: dto.T(u.CreatedAt),
	}
}

func actor(r *http.Request) tenancy.Actor {
	p := principal(r)
	return tenancy.Actor{UserID: p.User.ID, Role: p.User.Role}
}

func (h *Handlers) listUsers(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.UserListRequest) (any, error) {
		page, limit, err := kit.ResolvePage(req.PageRequest, pageCap)
		if err != nil {
			return nil, err
		}
		roles := make([]model.UserRole, len(req.Role))
		for i, role := range req.Role {
			roles[i] = model.UserRole(role)
			if !roles[i].Valid() {
				return nil, kit.ErrInvalidParameters.WithParams(map[string]any{"role": []string{"invalid"}})
			}
		}
		list, err := h.Tenancy.ListUsers(r.Context(), mw.ScopeFrom(r), store.UserFilter{Roles: roles, IsActive: req.IsActive, EmailContains: req.Q, Page: page})
		if err != nil {
			return nil, err
		}
		items := make([]dto.User, len(list))
		for i, u := range list {
			items[i] = userDTO(u)
		}
		return kit.PageOf(items, page, limit), nil
	})
}

func (h *Handlers) createUser(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusCreated, func(req dto.UserCreateRequest) (any, error) {
		role := model.UserRole(req.Role)
		u, err := h.Tenancy.CreateUser(r.Context(), actor(r), mw.ScopeFrom(r), tenancy.UserInput{
			Name: &req.Name, Email: &req.Email, Phone: req.Phone, Role: &role, IsActive: req.IsActive, Password: &req.Password,
		})
		return userDTO(u), err
	})
}

func (h *Handlers) updateUser(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.UserUpdateRequest) (any, error) {
		in := tenancy.UserInput{Name: req.Name, Email: req.Email, Phone: req.Phone, IsActive: req.IsActive}
		if req.Role != nil {
			role := model.UserRole(*req.Role)
			in.Role = &role
		}
		u, err := h.Tenancy.UpdateUser(r.Context(), actor(r), mw.ScopeFrom(r), req.ID, in)
		return userDTO(u), err
	})
}

func (h *Handlers) deleteUser(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusNoContent, func(req dto.IDPath) (any, error) {
		return nil, h.Tenancy.DeleteUser(r.Context(), actor(r), mw.ScopeFrom(r), req.ID)
	})
}

func smtpDTO(s model.SMTPSettings) dto.SMTPSettings {
	return dto.SMTPSettings{
		Host: s.Host, Port: s.Port, Secure: s.Secure, Username: s.Username, FromAddress: s.FromAddress,
		HasPassword: len(s.PasswordEnc) > 0, UpdatedAt: dto.T(s.UpdatedAt),
	}
}

func (h *Handlers) getSMTP(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(struct{}) (any, error) {
		s, err := h.Tenancy.GetSMTP(r.Context(), mw.ScopeFrom(r))
		return smtpDTO(s), err
	})
}

func (h *Handlers) putSMTP(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusOK, func(req dto.SMTPPutRequest) (any, error) {
		s, err := h.Tenancy.PutSMTP(r.Context(), mw.ScopeFrom(r), tenancy.SMTPInput{
			Host: req.Host, Port: req.Port, Secure: req.Secure, Username: req.Username, FromAddress: req.FromAddress, Password: req.Password,
		})
		return smtpDTO(s), err
	})
}

func (h *Handlers) testSMTP(w http.ResponseWriter, r *http.Request) {
	serve(w, r, http.StatusNoContent, func(req dto.SMTPTestRequest) (any, error) {
		return nil, h.Tenancy.TestSMTP(r.Context(), mw.ScopeFrom(r), req.To, kit.Locale(r))
	})
}
