package tenancy

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/MErenTalan/ekokod-rewrite/internal/auth"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	perr "github.com/MErenTalan/ekokod-rewrite/internal/platform/errors"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// Actor is who performs a user-management action.
type Actor struct {
	UserID uuid.UUID
	Role   model.UserRole
}

var assignable = map[model.UserRole]auth.RoleSet{
	model.UserRoleAdmin: auth.Roles(model.UserRoleAdmin, model.UserRoleCompanyAdmin, model.UserRoleCompanyReadonlyAdmin,
		model.UserRoleBuildingAdmin, model.UserRoleBuildingReadonlyAdmin),
	model.UserRoleCompanyAdmin: auth.Roles(model.UserRoleCompanyAdmin, model.UserRoleCompanyReadonlyAdmin,
		model.UserRoleBuildingAdmin, model.UserRoleBuildingReadonlyAdmin),
}

// AssignableRoles lists the roles actor may give (R174).
func AssignableRoles(actor model.UserRole) []model.UserRole { return assignable[actor].List() }

// UserInput is a user create or partial update; nil keeps a field.
type UserInput struct {
	Name, Email, Phone *string
	Role               *model.UserRole
	IsActive           *bool
	Password           *string // create only
	// BuildingIDs, when set, is the complete list of buildings a building
	// admin is responsible for; buildings not in it stop pointing at them.
	BuildingIDs *[]uuid.UUID
}

// buildingRole is a role whose scope is its assigned buildings.
func buildingRole(r model.UserRole) bool {
	return r == model.UserRoleBuildingAdmin || r == model.UserRoleBuildingReadonlyAdmin
}

const buildingPage = 500

// companyBuildings lists every building of the scope company.
func (s *Service) companyBuildings(ctx context.Context, sc store.Scope) ([]model.Building, error) {
	var all []model.Building
	for offset := int32(0); ; offset += buildingPage {
		page, err := s.d.Buildings.List(ctx, sc, store.BuildingFilter{Page: store.Page{Limit: buildingPage, Offset: offset}})
		if err != nil {
			return nil, err
		}
		all = append(all, page...)
		if len(page) < buildingPage {
			return all, nil
		}
	}
}

// checkBuildings refuses ids that are not buildings of the scope company, or
// any ids for a role that is not building-scoped.
func (s *Service) checkBuildings(ctx context.Context, sc store.Scope, role model.UserRole, ids []uuid.UUID) ([]model.Building, error) {
	if len(ids) > 0 && !buildingRole(role) {
		return nil, validation("building_ids", "role_not_building_scoped")
	}
	all, err := s.companyBuildings(ctx, sc)
	if err != nil {
		return nil, err
	}
	known := make(map[uuid.UUID]bool, len(all))
	for _, b := range all {
		known[b.ID] = true
	}
	for _, id := range ids {
		if !known[id] {
			return nil, validation("building_ids", "invalid")
		}
	}
	return all, nil
}

// assignBuildings makes userID responsible for exactly ids among all.
func (s *Service) assignBuildings(ctx context.Context, sc store.Scope, userID uuid.UUID, all []model.Building, ids []uuid.UUID) error {
	want := make(map[uuid.UUID]bool, len(ids))
	for _, id := range ids {
		want[id] = true
	}
	for _, b := range all {
		mine := b.ResponsibleUserID != nil && *b.ResponsibleUserID == userID
		switch {
		case want[b.ID] && !mine:
			uid := userID
			b.ResponsibleUserID = &uid
		case !want[b.ID] && mine:
			b.ResponsibleUserID = nil
		default:
			continue
		}
		if _, err := s.d.Buildings.Update(ctx, sc, b); err != nil {
			return err
		}
	}
	return nil
}

// ResponsibleBuildings maps each user of the scope company to the buildings
// they are responsible for.
func (s *Service) ResponsibleBuildings(ctx context.Context, sc store.Scope) (map[uuid.UUID][]uuid.UUID, error) {
	all, err := s.companyBuildings(ctx, sc)
	if err != nil {
		return nil, err
	}
	out := map[uuid.UUID][]uuid.UUID{}
	for _, b := range all {
		if b.ResponsibleUserID != nil {
			out[*b.ResponsibleUserID] = append(out[*b.ResponsibleUserID], b.ID)
		}
	}
	return out, nil
}

// ListUsers lists the scope company's users the actor may see: platform
// admins only to an admin, and to a building-scoped caller only itself and
// the users responsible for one of its buildings.
func (s *Service) ListUsers(ctx context.Context, actor Actor, sc store.Scope, f store.UserFilter) ([]model.User, error) {
	if actor.Role != model.UserRoleAdmin {
		f.Roles = withoutAdmin(f.Roles)
		if len(f.Roles) == 0 {
			return nil, nil
		}
	}
	list, err := s.d.Users.List(ctx, sc, f)
	if err != nil || sc.AllBuildings {
		return list, err
	}
	assigned, err := s.ResponsibleBuildings(ctx, sc)
	if err != nil {
		return nil, err
	}
	out := list[:0]
	for _, u := range list {
		if u.ID == actor.UserID || slices.ContainsFunc(assigned[u.ID], sc.AllowsBuilding) {
			out = append(out, u)
		}
	}
	return out, nil
}

// withoutAdmin narrows a role filter (empty = every role) to non-admin roles.
func withoutAdmin(roles []model.UserRole) []model.UserRole {
	if len(roles) == 0 {
		return []model.UserRole{model.UserRoleCompanyAdmin, model.UserRoleCompanyReadonlyAdmin,
			model.UserRoleBuildingAdmin, model.UserRoleBuildingReadonlyAdmin, model.UserRoleDemo}
	}
	var out []model.UserRole
	for _, r := range roles {
		if r != model.UserRoleAdmin {
			out = append(out, r)
		}
	}
	return out
}

// CreateUser creates a user with an assignable role and a policy-valid password.
func (s *Service) CreateUser(ctx context.Context, actor Actor, sc store.Scope, in UserInput) (model.User, error) {
	if in.Role == nil || !assignable[actor.Role].Has(*in.Role) {
		return model.User{}, validation("role", errRoleNotAssignable)
	}
	if in.Name == nil || in.Email == nil || in.Password == nil {
		return model.User{}, validation("password", errPasswordRequiredKey)
	}
	company, err := s.d.Companies.Get(ctx, sc, sc.CompanyID)
	if err != nil {
		return model.User{}, err
	}
	var buildings []model.Building
	if in.BuildingIDs != nil {
		if buildings, err = s.checkBuildings(ctx, sc, *in.Role, *in.BuildingIDs); err != nil {
			return model.User{}, err
		}
	}
	if codes := auth.CheckPolicy(s.d.Hasher, auth.PolicyInput{
		Password: *in.Password, Name: *in.Name, Email: *in.Email, CompanyName: company.Name,
	}); len(codes) > 0 {
		return model.User{}, perr.Validation.WithParams(map[string]any{"password": codes})
	}
	hash, err := s.d.Hasher.Hash(*in.Password)
	if err != nil {
		return model.User{}, err
	}
	now := s.d.Clock.Now()
	active := true
	if in.IsActive != nil {
		active = *in.IsActive
	}
	created, err := s.d.Users.Create(ctx, sc, model.User{
		CompanyID: sc.CompanyID, Name: strings.TrimSpace(*in.Name), Email: strings.TrimSpace(*in.Email),
		Phone: keepOrClear(nil, in.Phone), PasswordHash: hash, Role: *in.Role, IsActive: active, Locale: "tr",
		CreatedAt: now, UpdatedAt: now,
	})
	if errors.Is(err, store.ErrConflict) {
		return model.User{}, ErrEmailTaken
	}
	if err != nil {
		return model.User{}, err
	}
	if in.BuildingIDs != nil {
		if err := s.assignBuildings(ctx, sc, created.ID, buildings, *in.BuildingIDs); err != nil {
			return model.User{}, err
		}
	}
	return created, nil
}

// target loads a user the actor may manage: a company admin never manages an admin.
func (s *Service) target(ctx context.Context, actor Actor, sc store.Scope, id uuid.UUID) (model.User, error) {
	user, err := s.d.Users.Get(ctx, sc, id)
	if err != nil {
		return model.User{}, err
	}
	if !assignable[actor.Role].Has(user.Role) {
		return model.User{}, perr.Forbidden
	}
	return user, nil
}

// UpdateUser applies a partial update; role, activity and e-mail changes end
// the user's sessions (R174).
func (s *Service) UpdateUser(ctx context.Context, actor Actor, sc store.Scope, id uuid.UUID, in UserInput) (model.User, error) {
	user, err := s.target(ctx, actor, sc, id)
	if err != nil {
		return model.User{}, err
	}
	roleChanged := in.Role != nil && *in.Role != user.Role
	activeChanged := in.IsActive != nil && *in.IsActive != user.IsActive
	emailChanged := in.Email != nil && !strings.EqualFold(strings.TrimSpace(*in.Email), user.Email)
	if user.ID == actor.UserID && (roleChanged || activeChanged) {
		return model.User{}, ErrCannotModifySelf
	}
	if roleChanged && !assignable[actor.Role].Has(*in.Role) {
		return model.User{}, validation("role", errRoleNotAssignable)
	}
	role := user.Role
	if in.Role != nil {
		role = *in.Role
	}
	var buildings []model.Building
	if in.BuildingIDs != nil {
		if buildings, err = s.checkBuildings(ctx, sc, role, *in.BuildingIDs); err != nil {
			return model.User{}, err
		}
	}
	if in.Name != nil {
		if strings.TrimSpace(*in.Name) == "" {
			return model.User{}, validation("name", "required")
		}
		user.Name = strings.TrimSpace(*in.Name)
	}
	if in.Email != nil {
		user.Email = strings.TrimSpace(*in.Email)
	}
	user.Phone = keepOrClear(user.Phone, in.Phone)
	if in.Role != nil {
		user.Role = *in.Role
	}
	if in.IsActive != nil {
		user.IsActive = *in.IsActive
	}
	now := s.d.Clock.Now()
	user.UpdatedAt = now
	updated, err := s.d.Users.Update(ctx, sc, user)
	if errors.Is(err, store.ErrConflict) {
		return model.User{}, ErrEmailTaken
	}
	if err != nil {
		return model.User{}, err
	}
	if in.BuildingIDs != nil {
		if err := s.assignBuildings(ctx, sc, user.ID, buildings, *in.BuildingIDs); err != nil {
			return model.User{}, err
		}
	}
	if roleChanged || activeChanged || emailChanged {
		if _, err := s.d.Sessions.RevokeAllForUserWithReason(ctx, sc, user.ID, "admin_change", nil, now); err != nil {
			return model.User{}, err
		}
	}
	return updated, nil
}

// DeleteUser soft-deletes a user and ends their sessions.
func (s *Service) DeleteUser(ctx context.Context, actor Actor, sc store.Scope, id uuid.UUID) error {
	if id == actor.UserID {
		return ErrCannotModifySelf
	}
	user, err := s.target(ctx, actor, sc, id)
	if err != nil {
		return err
	}
	now := s.d.Clock.Now()
	if _, err := s.d.Sessions.RevokeAllForUserWithReason(ctx, sc, user.ID, "admin_change", nil, now); err != nil {
		return err
	}
	return s.d.Users.SoftDelete(ctx, sc, user.ID, now)
}
