package auth_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/auth"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
)

// Literal copy of the R159 table; a change to PermissionsFor must change this test too.
func TestPermissionsMatrix(t *testing.T) {
	want := map[model.UserRole][]string{
		model.UserRoleAdmin: {
			"admin.companies", "alarms.edit", "alarms.evaluate", "alarms.read", "analyzers.refresh",
			"anomaly.check", "bills.compute", "calendar.edit", "integrations.credentials", "jobs.runs.read",
			"jobs.trigger", "messages.read", "nav.core", "nav.financial", "nav.solar_plants",
			"settings.analyzers", "settings.analyzers.edit", "settings.buildings", "settings.company",
			"settings.company.edit", "settings.integrations", "settings.plants", "settings.smtp",
			"settings.users", "write",
		},
		model.UserRoleCompanyAdmin: {
			"alarms.edit", "alarms.evaluate", "alarms.read", "analyzers.refresh", "anomaly.check",
			"bills.compute", "calendar.edit", "integrations.credentials", "jobs.runs.read", "messages.read",
			"nav.core", "nav.financial", "nav.solar_plants", "settings.analyzers", "settings.analyzers.edit",
			"settings.buildings", "settings.company", "settings.company.edit", "settings.plants",
			"settings.users", "write",
		},
		model.UserRoleCompanyReadonlyAdmin: {
			"alarms.read", "messages.read", "nav.core", "nav.financial", "nav.solar_plants",
			"settings.analyzers", "settings.buildings", "settings.company", "settings.plants",
			"settings.users",
		},
		model.UserRoleBuildingAdmin: {
			"alarms.edit", "alarms.read", "analyzers.refresh", "anomaly.check", "bills.compute",
			"messages.read", "nav.core", "settings.analyzers", "write",
		},
		model.UserRoleBuildingReadonlyAdmin: {"alarms.read", "messages.read", "nav.core", "settings.analyzers"},
		model.UserRoleDemo:                  {"alarms.read", "messages.read", "nav.core"},
	}
	for _, role := range model.UserRoles() {
		require.Equal(t, want[role], auth.PermissionsFor(role), role)
	}
	require.Nil(t, auth.PermissionsFor("nobody"))
}

func TestRoleSets(t *testing.T) {
	s := auth.Roles(model.UserRoleAdmin, model.UserRoleCompanyAdmin)
	require.True(t, s.Has(model.UserRoleAdmin))
	require.False(t, s.Has(model.UserRoleDemo))
	require.False(t, auth.Roles().Has(model.UserRoleAdmin), "an empty set allows nobody")
	for _, r := range model.UserRoles() {
		require.True(t, auth.AllRoles.Has(r))
	}
	require.True(t, auth.WriteRoles.Has(model.UserRoleBuildingAdmin))
	require.False(t, auth.WriteRoles.Has(model.UserRoleCompanyReadonlyAdmin))
	require.True(t, auth.IsReadOnly(model.UserRoleDemo))
	require.True(t, auth.IsReadOnly(model.UserRoleBuildingReadonlyAdmin))
	require.False(t, auth.IsReadOnly(model.UserRoleCompanyAdmin))
	require.Equal(t, []model.UserRole{model.UserRoleAdmin, model.UserRoleCompanyAdmin}, s.List())
}
