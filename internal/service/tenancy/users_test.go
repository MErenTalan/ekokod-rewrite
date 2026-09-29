package tenancy

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/auth"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	perr "github.com/MErenTalan/ekokod-rewrite/internal/platform/errors"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

type fakeBuildings struct {
	store.BuildingRepository
	rows map[uuid.UUID]model.Building
}

func (f *fakeBuildings) List(_ context.Context, _ store.Scope, _ store.BuildingFilter) ([]model.Building, error) {
	var out []model.Building
	for _, b := range f.rows {
		out = append(out, b)
	}
	return out, nil
}

func (f *fakeBuildings) Update(_ context.Context, _ store.Scope, b model.Building) (model.Building, error) {
	f.rows[b.ID] = b
	return b, nil
}

func (f *fakeBuildings) owner(id uuid.UUID) *uuid.UUID { return f.rows[id].ResponsibleUserID }

type fakeUsers struct {
	store.UserRepository
	rows map[uuid.UUID]model.User
}

func (f *fakeUsers) Create(_ context.Context, _ store.Scope, u model.User) (model.User, error) {
	u.ID = uuid.New()
	f.rows[u.ID] = u
	return u, nil
}

func (f *fakeUsers) Get(_ context.Context, _ store.Scope, id uuid.UUID) (model.User, error) {
	if u, ok := f.rows[id]; ok {
		return u, nil
	}
	return model.User{}, store.ErrNotFound
}

func (f *fakeUsers) Update(_ context.Context, _ store.Scope, u model.User) (model.User, error) {
	f.rows[u.ID] = u
	return u, nil
}

type fakeCompanies struct{ store.CompanyRepository }

func (fakeCompanies) Get(_ context.Context, _ store.Scope, id uuid.UUID) (model.Company, error) {
	return model.Company{ID: id, Name: "Acme"}, nil
}

type fakeSessions struct{ store.SessionRepository }

func (fakeSessions) RevokeAllForUserWithReason(context.Context, store.Scope, uuid.UUID, string, *uuid.UUID, time.Time) (int64, error) {
	return 0, nil
}

func usersService(b *fakeBuildings, u *fakeUsers) *Service {
	return &Service{d: Deps{
		Companies: fakeCompanies{}, Users: u, Sessions: fakeSessions{}, Buildings: b,
		Hasher: auth.Hasher{Pepper: []byte("pepper-pepper-pepper-pepper-1234"), Cost: 4}, Clock: clock.NewFake(time.Unix(0, 0)),
	}}
}

// The admin user form had no way to say which buildings a building admin
// manages; a building admin's scope is exactly buildings.responsible_user_id.
func TestBuildingAdminAssignment(t *testing.T) {
	sc := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	b1, b2, b3 := uuid.New(), uuid.New(), uuid.New()
	other := uuid.New()
	bs := &fakeBuildings{rows: map[uuid.UUID]model.Building{
		b1: {ID: b1}, b2: {ID: b2}, b3: {ID: b3, ResponsibleUserID: &other},
	}}
	us := &fakeUsers{rows: map[uuid.UUID]model.User{}}
	svc := usersService(bs, us)
	admin := Actor{UserID: uuid.New(), Role: model.UserRoleAdmin}
	role := model.UserRoleBuildingAdmin
	name, email, pw := "Bina Yöneticisi", "ba@example.com", "Uzun-Guclu-Parola-2026!x"

	ids := []uuid.UUID{b1, b2}
	u, err := svc.CreateUser(t.Context(), admin, sc, UserInput{Name: &name, Email: &email, Role: &role, Password: &pw, BuildingIDs: &ids})
	require.NoError(t, err)
	require.Equal(t, u.ID, *bs.owner(b1))
	require.Equal(t, u.ID, *bs.owner(b2))

	ids = []uuid.UUID{b2, b3}
	_, err = svc.UpdateUser(t.Context(), admin, sc, u.ID, UserInput{BuildingIDs: &ids})
	require.NoError(t, err)
	require.Nil(t, bs.owner(b1), "a building left out is released")
	require.Equal(t, u.ID, *bs.owner(b3), "a building moves from its previous responsible user")

	got, err := svc.ResponsibleBuildings(t.Context(), sc)
	require.NoError(t, err)
	require.ElementsMatch(t, []uuid.UUID{b2, b3}, got[u.ID])

	unknown := []uuid.UUID{uuid.New()}
	_, err = svc.UpdateUser(t.Context(), admin, sc, u.ID, UserInput{BuildingIDs: &unknown})
	var pe *perr.Error
	require.ErrorAs(t, err, &pe)
	require.Equal(t, []string{"invalid"}, pe.Params["building_ids"])

	ca := model.UserRoleCompanyAdmin
	_, err = svc.UpdateUser(t.Context(), admin, sc, u.ID, UserInput{Role: &ca, BuildingIDs: &ids})
	require.ErrorAs(t, err, &pe)
	require.Equal(t, []string{"role_not_building_scoped"}, pe.Params["building_ids"])
}

func (f *fakeUsers) List(_ context.Context, _ store.Scope, filter store.UserFilter) ([]model.User, error) {
	var out []model.User
	for _, u := range f.rows {
		if len(filter.Roles) == 0 || slices.Contains(filter.Roles, u.Role) {
			out = append(out, u)
		}
	}
	return out, nil
}

// A company admin saw the platform admin in its user list, and a building
// scope must see only itself and the users of its own buildings.
func TestListUsersHidesAdminsAndUnrelatedUsers(t *testing.T) {
	company := uuid.New()
	mk := func(role model.UserRole) model.User { return model.User{ID: uuid.New(), CompanyID: company, Role: role} }
	admin, ca, ba1, ba2 := mk(model.UserRoleAdmin), mk(model.UserRoleCompanyAdmin), mk(model.UserRoleBuildingAdmin), mk(model.UserRoleBuildingAdmin)
	b1, b2 := uuid.New(), uuid.New()
	u := &fakeUsers{rows: map[uuid.UUID]model.User{admin.ID: admin, ca.ID: ca, ba1.ID: ba1, ba2.ID: ba2}}
	b := &fakeBuildings{rows: map[uuid.UUID]model.Building{
		b1: {ID: b1, CompanyID: company, ResponsibleUserID: &ba1.ID},
		b2: {ID: b2, CompanyID: company, ResponsibleUserID: &ba2.ID},
	}}
	svc := usersService(b, u)
	ids := func(list []model.User) []uuid.UUID {
		out := make([]uuid.UUID, 0, len(list))
		for _, x := range list {
			out = append(out, x.ID)
		}
		slices.SortFunc(out, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
		return out
	}
	sorted := func(v ...uuid.UUID) []uuid.UUID {
		slices.SortFunc(v, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
		return v
	}
	all := store.Scope{CompanyID: company, AllBuildings: true}
	ctx := context.Background()

	got, err := svc.ListUsers(ctx, Actor{UserID: admin.ID, Role: model.UserRoleAdmin}, all, store.UserFilter{})
	require.NoError(t, err)
	require.Equal(t, sorted(admin.ID, ca.ID, ba1.ID, ba2.ID), ids(got))

	got, err = svc.ListUsers(ctx, Actor{UserID: ca.ID, Role: model.UserRoleCompanyAdmin}, all, store.UserFilter{})
	require.NoError(t, err)
	require.Equal(t, sorted(ca.ID, ba1.ID, ba2.ID), ids(got), "no platform admin")

	got, err = svc.ListUsers(ctx, Actor{UserID: ca.ID, Role: model.UserRoleCompanyAdmin}, all, store.UserFilter{Roles: []model.UserRole{model.UserRoleAdmin}})
	require.NoError(t, err)
	require.Empty(t, got, "asking for admins by role yields nothing")

	got, err = svc.ListUsers(ctx, Actor{UserID: ba1.ID, Role: model.UserRoleBuildingAdmin},
		store.Scope{CompanyID: company, BuildingIDs: []uuid.UUID{b1}}, store.UserFilter{})
	require.NoError(t, err)
	require.Equal(t, []uuid.UUID{ba1.ID}, ids(got), "only itself and users of its buildings")
}
