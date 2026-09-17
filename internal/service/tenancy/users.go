package tenancy

import (
	"context"
	"errors"
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
}

// ListUsers lists the scope company's users.
func (s *Service) ListUsers(ctx context.Context, sc store.Scope, f store.UserFilter) ([]model.User, error) {
	return s.d.Users.List(ctx, sc, f)
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
	return created, err
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
