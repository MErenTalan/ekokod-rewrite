package seed

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MErenTalan/ekokod-rewrite/internal/auth"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/admin"
)

// UserInput is one operator-created user (R182).
type UserInput struct {
	ID          uuid.UUID // zero: generated
	Email, Name string
	Role        model.UserRole
	CompanyID   uuid.UUID // either this or CompanyName
	CompanyName string    // created when no live company has this name
	Password    string
}

// ErrWeakPassword wraps the R146 codes a rejected password violated.
var ErrWeakPassword = errors.New("seed: password violates the policy")

// CreateUser creates a user (and, by name, its company) with a policy-checked
// password. Demo users are refused: only `seed demo` creates them (R174).
func CreateUser(ctx context.Context, pool *pgxpool.Pool, hasher auth.Hasher, in UserInput, now time.Time) (model.User, error) {
	if !in.Role.Valid() {
		return model.User{}, fmt.Errorf("seed: unknown role %q", in.Role)
	}
	if in.Role == model.UserRoleDemo {
		return model.User{}, errors.New("seed: demo users are created by `seed demo` only")
	}
	companies := postgres.NewCompanyRepository(pool)
	company, err := resolveCompany(ctx, pool, companies, in, now)
	if err != nil {
		return model.User{}, err
	}
	if codes := auth.CheckPolicy(hasher, auth.PolicyInput{
		Password: in.Password, Name: in.Name, Email: in.Email, CompanyName: company.Name,
	}); len(codes) > 0 {
		return model.User{}, fmt.Errorf("%w: %s", ErrWeakPassword, strings.Join(codes, ", "))
	}
	hash, err := hasher.Hash(in.Password)
	if err != nil {
		return model.User{}, err
	}
	return postgres.NewUserRepository(pool).Create(ctx, store.SystemScope(company.ID), model.User{
		ID: in.ID, CompanyID: company.ID, Name: in.Name, Email: strings.TrimSpace(in.Email), PasswordHash: hash,
		Role: in.Role, IsActive: true, Locale: "tr", CreatedAt: now, UpdatedAt: now,
	})
}

func resolveCompany(ctx context.Context, pool *pgxpool.Pool, companies *postgres.CompanyRepository, in UserInput, now time.Time) (model.Company, error) {
	if in.CompanyID != uuid.Nil {
		return companies.Get(ctx, store.SystemScope(in.CompanyID), in.CompanyID)
	}
	name := strings.TrimSpace(in.CompanyName)
	if name == "" {
		return model.Company{}, errors.New("seed: a company id or name is required")
	}
	matches, err := admin.NewTenantRepository(pool).ListCompanies(ctx, store.CompanyFilter{NameContains: name})
	if err != nil {
		return model.Company{}, err
	}
	for _, c := range matches {
		if strings.EqualFold(c.Name, name) {
			return c, nil
		}
	}
	id := uuid.New()
	return companies.Create(ctx, store.SystemScope(id), model.Company{ID: id, Name: name, CreatedAt: now, UpdatedAt: now})
}
