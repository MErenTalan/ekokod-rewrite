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
			"anomaly.check", "bills.compute", "bills.read", "calendar.edit", "integrations.credentials",
			"jobs.runs.read", "jobs.trigger", "messages.read", "nav.core", "nav.financial", "nav.solar_plants",
			"settings.analyzers", "settings.analyzers.edit", "settings.buildings", "settings.company",
			"settings.company.edit", "settings.integrations", "settings.plants", "settings.smtp",
			"settings.users", "solar_tariffs.read", "tariffs.bulk.read", "tariffs.defaults", "tariffs.edit",
			"tariffs.icmal", "tariffs.read", "tariffs.templates.read", "write",
		},
		model.UserRoleCompanyAdmin: {
			"alarms.edit", "alarms.evaluate", "alarms.read", "analyzers.refresh", "anomaly.check",
			"bills.compute", "bills.read", "calendar.edit", "integrations.credentials", "jobs.runs.read",
			"messages.read", "nav.core", "nav.financial", "nav.solar_plants", "settings.analyzers",
			"settings.analyzers.edit", "settings.buildings", "settings.company", "settings.company.edit",
			"settings.plants", "settings.users", "solar_tariffs.read", "tariffs.bulk.read", "tariffs.edit",
			"tariffs.icmal", "tariffs.read", "tariffs.templates.read", "write",
		},
		model.UserRoleCompanyReadonlyAdmin: {
			"alarms.read", "bills.read", "messages.read", "nav.core", "nav.financial", "nav.solar_plants",
			"settings.analyzers", "settings.buildings", "settings.company", "settings.plants",
			"settings.users", "solar_tariffs.read", "tariffs.bulk.read", "tariffs.read",
			"tariffs.templates.read",
		},
		model.UserRoleBuildingAdmin: {
			"alarms.edit", "alarms.read", "analyzers.refresh", "anomaly.check", "bills.compute", "bills.read",
			"messages.read", "nav.core", "settings.analyzers", "tariffs.read", "write",
		},
		model.UserRoleBuildingReadonlyAdmin: {"alarms.read", "bills.read", "messages.read", "nav.core",
			"settings.analyzers", "tariffs.read"},
		model.UserRoleDemo: {"alarms.read", "bills.read", "messages.read", "nav.core", "tariffs.read"},
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
