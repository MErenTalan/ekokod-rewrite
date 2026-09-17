package auth

import (
	"sort"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
)

// RoleSet is a set of user roles; the zero value allows nobody.
type RoleSet uint8

func roleBit(r model.UserRole) RoleSet {
	for i, known := range model.UserRoles() {
		if known == r {
			return 1 << i
		}
	}
	return 0
}

// Roles builds a set.
func Roles(rs ...model.UserRole) RoleSet {
	var s RoleSet
	for _, r := range rs {
		s |= roleBit(r)
	}
	return s
}

// Has reports membership; an unknown role is never a member.
func (s RoleSet) Has(r model.UserRole) bool {
	bit := roleBit(r)
	return bit != 0 && s&bit != 0
}

// List returns the members in declaration order.
func (s RoleSet) List() []model.UserRole {
	var out []model.UserRole
	for _, r := range model.UserRoles() {
		if s.Has(r) {
			out = append(out, r)
		}
	}
	return out
}

var (
	// AllRoles is 05's "scope" column: A CA CR BA BR D.
	AllRoles = Roles(model.UserRoles()...)
	// WriteRoles are the roles that may mutate anything: A CA BA.
	WriteRoles = Roles(model.UserRoleAdmin, model.UserRoleCompanyAdmin, model.UserRoleBuildingAdmin)
)

// IsReadOnly reports the roles every mutation is refused for (01 §2).
func IsReadOnly(r model.UserRole) bool {
	return r == model.UserRoleCompanyReadonlyAdmin || r == model.UserRoleBuildingReadonlyAdmin || r == model.UserRoleDemo
}

var (
	roleA  = model.UserRoleAdmin
	roleCA = model.UserRoleCompanyAdmin
	roleCR = model.UserRoleCompanyReadonlyAdmin
	roleBA = model.UserRoleBuildingAdmin
	roleBR = model.UserRoleBuildingReadonlyAdmin
)

// permissionTable is R159: permission → roles holding it.
var permissionTable = map[string]RoleSet{
	"write":                    WriteRoles,
	"nav.core":                 AllRoles,
	"nav.solar_plants":         Roles(roleA, roleCA, roleCR),
	"nav.financial":            Roles(roleA, roleCA, roleCR),
	"admin.companies":          Roles(roleA),
	"settings.integrations":    Roles(roleA),
	"settings.company":         Roles(roleA, roleCA, roleCR),
	"settings.company.edit":    Roles(roleA, roleCA),
	"settings.buildings":       Roles(roleA, roleCA, roleCR),
	"settings.plants":          Roles(roleA, roleCA, roleCR),
	"settings.users":           Roles(roleA, roleCA, roleCR),
	"settings.analyzers":       Roles(roleA, roleCA, roleCR, roleBA, roleBR),
	"settings.smtp":            Roles(roleA),
	"analyzers.refresh":        Roles(roleA, roleCA, roleBA),
	"bills.compute":            Roles(roleA, roleCA, roleBA),
	"calendar.edit":            Roles(roleA, roleCA),
	"integrations.credentials": Roles(roleA, roleCA),
}

// PermissionsFor returns the sorted permissions of a role, nil for an unknown role.
func PermissionsFor(r model.UserRole) []string {
	if !r.Valid() {
		return nil
	}
	var out []string
	for p, roles := range permissionTable {
		if roles.Has(r) {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

// AllPermissions returns every permission name, sorted.
func AllPermissions() []string {
	out := make([]string, 0, len(permissionTable))
	for p := range permissionTable {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}
