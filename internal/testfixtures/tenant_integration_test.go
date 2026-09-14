//go:build integration

package testfixtures_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

// The fixture factory is itself load-bearing: Tasks 9-13 prove tenant
// isolation with it, so a fixture that quietly produced one building, or two
// tenants that collided, would make those proofs vacuous. These tests pin the
// properties the factory promises.

// TestTwoTenantsCoexistInOneDatabase is the property every repository test
// depends on: `mine := NewTenant(…, 1)` and `theirs := NewTenant(…, 2)` in the
// same database. It fails on the schema's unique indexes —
// companies(lower(name)), users(email),
// analyzers(provider, provider_subtype, installation_number) and
// power_plants(isolar_ps_id) — if any fixture value stops carrying the seed.
func TestTwoTenantsCoexistInOneDatabase(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewMigratedPool(t)

	mine := testfixtures.NewTenant(t, ctx, pool, 1)
	theirs := testfixtures.NewTenant(t, ctx, pool, 2)

	require.NotEqual(t, mine.Company.ID, theirs.Company.ID)
	require.NotEqual(t, mine.Company.Name, theirs.Company.Name)

	var companies int
	require.NoError(t, pool.QueryRow(ctx, `select count(*) from companies`).Scan(&companies))
	require.Equal(t, 2, companies)

	var analyzers int
	require.NoError(t, pool.QueryRow(ctx, `select count(*) from analyzers`).Scan(&analyzers))
	require.Equal(t, len(mine.Analyzers)+len(theirs.Analyzers), analyzers)
}

// TestNewTenantIsDeterministic pins that the same seed produces the same ids
// and values, in a DIFFERENT database. Reproducing a failure means re-running
// with the seed it printed, and that only works if the seed decides everything.
func TestNewTenantIsDeterministic(t *testing.T) {
	ctx := context.Background()

	first := testfixtures.NewTenant(t, ctx, testfixtures.NewMigratedPool(t), 7)
	second := testfixtures.NewTenant(t, ctx, testfixtures.NewMigratedPool(t), 7)

	require.Equal(t, first.Company, second.Company)
	require.Equal(t, first.Users, second.Users)
	require.Equal(t, first.Buildings, second.Buildings)
	require.Equal(t, first.Analyzers, second.Analyzers)
	require.Equal(t, first.Plants, second.Plants)
	require.Equal(t, first.Tariffs, second.Tariffs)
	require.Equal(t, first.Scope, second.Scope)
	require.Equal(t, first.AdminScope, second.AdminScope)
}

// TestTenantShapeMakesScopeLeaksObservable is the reason the fixture looks the
// way it does. Every assertion here is something Task 13's isolation test
// silently relies on.
func TestTenantShapeMakesScopeLeaksObservable(t *testing.T) {
	ctx := context.Background()
	tenant := testfixtures.NewTenant(t, ctx, testfixtures.NewMigratedPool(t), 3)

	require.GreaterOrEqual(t, len(tenant.Buildings), 2,
		"with one building an unscoped repository is indistinguishable from a scoped one")
	require.GreaterOrEqual(t, len(tenant.Analyzers), 2*len(tenant.Buildings))
	require.NotEmpty(t, tenant.Plants)
	require.NotEmpty(t, tenant.Tariffs)

	for _, role := range model.UserRoles() {
		u, ok := tenant.Users[role]
		require.Truef(t, ok, "no fixture user for role %s", role)
		require.Equal(t, role, u.Role)
		require.Equal(t, tenant.Company.ID, u.CompanyID)
	}

	require.True(t, tenant.Scope.AllowsBuilding(tenant.Buildings[0].ID))
	require.False(t, tenant.Scope.AllowsBuilding(tenant.Buildings[1].ID))
	require.True(t, tenant.AdminScope.AllowsBuilding(tenant.Buildings[1].ID))

	ids, all := tenant.Scope.BuildingFilter()
	require.False(t, all, "the narrow scope must not report AllBuildings")
	require.Len(t, ids, 1)
	require.Equal(t, tenant.Buildings[0].ID, ids[0])
}

// TestTenantRowsAreActuallyInTheDatabase guards against a factory that builds
// the structs and forgets an insert: every struct it returns must correspond
// to a real row, or a repository test would fail on a missing row rather than
// on the behaviour it meant to test.
func TestTenantRowsAreActuallyInTheDatabase(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewMigratedPool(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 5)

	count := func(sql string, args ...any) int {
		t.Helper()
		var n int
		require.NoError(t, pool.QueryRow(ctx, sql, args...).Scan(&n))
		return n
	}

	require.Equal(t, 1, count(`select count(*) from companies where id = $1`, tenant.Company.ID))
	require.Equal(t, len(tenant.Users), count(`select count(*) from users where company_id = $1`, tenant.Company.ID))
	require.Equal(t, len(tenant.Buildings), count(`select count(*) from buildings where company_id = $1`, tenant.Company.ID))
	require.Equal(t, len(tenant.Analyzers), count(`select count(*) from analyzers where company_id = $1`, tenant.Company.ID))
	require.Equal(t, len(tenant.Plants), count(`select count(*) from power_plants where company_id = $1`, tenant.Company.ID))
	require.Equal(t, len(tenant.Tariffs), count(`select count(*) from tariffs where company_id = $1`, tenant.Company.ID))

	// The decimal values survive the round trip through numeric exactly. The
	// meter multiplier is the one that matters most: it multiplies every
	// register value at ingestion.
	var multiplier string
	require.NoError(t, pool.QueryRow(ctx,
		`select meter_multiplier::text from analyzers where id = $1`,
		tenant.Analyzers[0].ID).Scan(&multiplier))
	require.Equal(t, "40.000000", multiplier)

	var vat, distribution string
	require.NoError(t, pool.QueryRow(ctx,
		`select vat_rate::text, distribution_cost::text from tariffs where id = $1`,
		tenant.Tariffs[0].ID).Scan(&vat, &distribution))
	require.Equal(t, "20.000", vat)
	require.Equal(t, "0.987654", distribution)

	// EVERY decimal the fixture writes, compared as text at the fixture
	// value's own scale. A numeric(p,s) column rounds any extra fractional
	// digits on write without an error, so a fixture literal with more
	// places than its column holds a value the database does not — and a
	// repository test comparing the fixture to what it reads back would
	// then fail for a reason that has nothing to do with the repository.
	// Comparing at the fixture's own scale catches both a changed value and
	// a scale the column does not have.
	type column struct {
		table, column string
		id            uuid.UUID
		want          *decimal.Decimal
	}
	columns := []column{
		{"companies", "total_area_m2", tenant.Company.ID, tenant.Company.TotalAreaM2},
	}
	for _, b := range tenant.Buildings {
		columns = append(columns,
			column{"buildings", "latitude", b.ID, b.Latitude},
			column{"buildings", "longitude", b.ID, b.Longitude},
			column{"buildings", "total_area_m2", b.ID, b.TotalAreaM2},
		)
	}
	for _, a := range tenant.Analyzers {
		multiplier := a.MeterMultiplier
		columns = append(columns,
			column{"analyzers", "installed_power_kw", a.ID, a.InstalledPowerKw},
			column{"analyzers", "meter_multiplier", a.ID, &multiplier},
			column{"analyzers", "latitude", a.ID, a.Latitude},
			column{"analyzers", "longitude", a.ID, a.Longitude},
		)
	}
	for _, p := range tenant.Plants {
		columns = append(columns,
			column{"power_plants", "panel_power_w", p.ID, p.PanelPowerW},
			column{"power_plants", "panel_efficiency_pct", p.ID, p.PanelEfficiencyPct},
			column{"power_plants", "tilt_angle_deg", p.ID, p.TiltAngleDeg},
			column{"power_plants", "total_capacity_kw", p.ID, p.TotalCapacityKw},
			column{"power_plants", "yearly_target_kwh", p.ID, p.YearlyTargetKwh},
			column{"power_plants", "isolar_installed_kw", p.ID, p.IsolarInstalledKw},
			column{"power_plants", "latitude", p.ID, p.Latitude},
			column{"power_plants", "longitude", p.ID, p.Longitude},
		)
	}
	for _, tar := range tenant.Tariffs {
		distribution, reactive, vat := tar.DistributionCost, tar.ReactivePowerPrice, tar.VatRate
		columns = append(columns,
			column{"tariffs", "single_time_price", tar.ID, tar.SingleTimePrice},
			column{"tariffs", "t1_price", tar.ID, tar.T1Price},
			column{"tariffs", "t2_price", tar.ID, tar.T2Price},
			column{"tariffs", "t3_price", tar.ID, tar.T3Price},
			column{"tariffs", "distribution_cost", tar.ID, &distribution},
			column{"tariffs", "reactive_power_price", tar.ID, &reactive},
			column{"tariffs", "contracted_power_kw", tar.ID, tar.ContractedPowerKw},
			column{"tariffs", "power_unit_price", tar.ID, tar.PowerUnitPrice},
			column{"tariffs", "vat_rate", tar.ID, &vat},
			column{"tariffs", "kbk_distribution_cost_tl_per_kwh", tar.ID, tar.KbkDistributionCostTlPerKwh},
		)
	}
	for _, c := range columns {
		var got *string
		// Table and column names come from the literals above, never from
		// input, so formatting them into the statement is safe.
		require.NoError(t, pool.QueryRow(ctx,
			"select "+c.column+"::text from "+c.table+" where id = $1", c.id).Scan(&got))
		if c.want == nil {
			require.Nilf(t, got, "%s.%s (id %s): fixture is nil, column must be NULL", c.table, c.column, c.id)
			continue
		}
		require.NotNilf(t, got, "%s.%s (id %s): fixture is %s, column is NULL", c.table, c.column, c.id, c.want)
		require.Equalf(t, atOwnScale(*c.want), *got,
			"%s.%s (id %s): the column holds a different value or scale than the fixture returned",
			c.table, c.column, c.id)
	}
}

// atOwnScale renders d with exactly as many fractional digits as it carries.
// decimal.Decimal.String trims trailing zeros and StringFixed rounds to a
// chosen width; neither would reveal a fixture literal whose scale disagrees
// with its column, which is what the comparison above exists to catch.
func atOwnScale(d decimal.Decimal) string {
	places := -d.Exponent()
	if places < 0 {
		places = 0
	}
	return d.StringFixed(places)
}
