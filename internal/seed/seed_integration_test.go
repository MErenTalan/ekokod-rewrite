//go:build integration

// Task 12b's loader tests. Task 12a's datasets_test.go already proves the
// embedded data decodes and validates (no database, runs on every
// `go test`); everything here proves the LOADER's own contract — writing
// those values through store.AdminCatalogueRepository idempotently — and so
// needs a real Postgres. See internal/testfixtures for NewIsolatedDB.
package seed_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/seed"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/admin"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

// seedTargetFactorKey is emission_factors.json's first entry: it carries
// conversions ("tonne", "kg"), so the same key exercises both the factor row
// and its children in one place. Fixed here rather than re-decoded per test
// so every test in this file targets the same, known row.
const seedTargetFactorKey = "stationary_space_heating_coal_domestic"

// seedTableCounts is the set of row counts
// TestSeedRunTwiceProducesTheSameRowCounts compares before and after the
// second run — every table Load writes to, platform rows only (the
// company-owned-override test writes its own row into emission_factors and
// asserts separately that IT, specifically, is untouched; a blanket
// `count(*) from emission_factors` here would also have to account for that
// row, which is not what this test is about).
func seedTableCounts(t *testing.T, ctx context.Context, pool *pgxpool.Pool) map[string]int {
	t.Helper()
	counts := map[string]int{}
	for table, query := range map[string]string{
		"emission_factors":            `select count(*) from emission_factors where company_id is null`,
		"emission_factor_conversions": `select count(*) from emission_factor_conversions ec join emission_factors ef on ef.id = ec.factor_id where ef.company_id is null`,
		"integration_definitions":     `select count(*) from integration_definitions`,
		"national_tariff_schedule":    `select count(*) from national_tariff_schedule`,
	} {
		var n int
		require.NoError(t, pool.QueryRow(ctx, query).Scan(&n), "counting %s", table)
		counts[table] = n
	}
	return counts
}

// TestSeedRunTwiceProducesTheSameRowCounts is the F1 acceptance criterion
// (task-12 brief): the offline bundle re-runs seed on every upgrade, so a
// second run that duplicated rows would corrupt the factor catalogue every
// release.
func TestSeedRunTwiceProducesTheSameRowCounts(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	log := testfixtures.DiscardLogger()

	first, err := seed.Load(ctx, pool, log)
	require.NoError(t, err)
	countsAfterFirst := seedTableCounts(t, ctx, pool)

	second, err := seed.Load(ctx, pool, log)
	require.NoError(t, err)
	require.Equal(t, countsAfterFirst, seedTableCounts(t, ctx, pool),
		"a second seed run must not change any row count")
	require.Equal(t, first.Total(), second.Total())
	require.Equal(t, first, second, "every per-dataset count must be identical between runs")

	// Pin the shipped counts directly (task-12a report: 182 factors, 196
	// conversions, 3 integration definitions, 0 tariff rows), so a future
	// change to the embedded data that silently drops rows is caught here
	// too, not only by datasets_test.go.
	require.EqualValues(t, 182, first.EmissionFactors)
	require.EqualValues(t, 196, first.EmissionFactorConversions)
	require.EqualValues(t, 3, first.IntegrationDefinitions)
	require.EqualValues(t, 0, first.NationalTariffSchedule)
}

// TestSeedRepairsAModifiedRow proves seeding is idempotent by CONVERGENCE,
// not by skipping work: an operator who edits a master factor must get the
// shipped value back, otherwise "idempotent" would just mean "does nothing
// the second time" and a corrupted catalogue would never heal.
func TestSeedRepairsAModifiedRow(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	log := testfixtures.DiscardLogger()

	_, err := seed.Load(ctx, pool, log)
	require.NoError(t, err)

	// An operator (or a bad migration) corrupts the shipped label directly.
	tag, err := pool.Exec(ctx,
		`update emission_factors set label = 'CORRUPTED BY OPERATOR' where company_id is null and key = $1`,
		seedTargetFactorKey)
	require.NoError(t, err)
	require.EqualValues(t, 1, tag.RowsAffected())

	var corruptedLabel string
	require.NoError(t, pool.QueryRow(ctx,
		`select label from emission_factors where company_id is null and key = $1`, seedTargetFactorKey).
		Scan(&corruptedLabel))
	require.Equal(t, "CORRUPTED BY OPERATOR", corruptedLabel, "sanity: the corruption actually landed")

	_, err = seed.Load(ctx, pool, log)
	require.NoError(t, err)

	factors, err := seed.EmissionFactors()
	require.NoError(t, err)
	var want string
	for _, f := range factors {
		if f.Key == seedTargetFactorKey {
			want = f.Label
			break
		}
	}
	require.NotEmpty(t, want, "test setup: %s must exist in the shipped dataset", seedTargetFactorKey)

	var repairedLabel string
	require.NoError(t, pool.QueryRow(ctx,
		`select label from emission_factors where company_id is null and key = $1`, seedTargetFactorKey).
		Scan(&repairedLabel))
	require.Equal(t, want, repairedLabel, "re-seeding must repair a hand-edited row back to the shipped value")
}

// TestSeedCompanyOwnedOverrideSurvivesSeeding proves the loader never
// touches a company-owned row: a company shadows a platform key (the same
// key exists in both), and seeding must leave the company's own row and
// values completely alone while still (re)writing the platform row under
// the same key.
func TestSeedCompanyOwnedOverrideSurvivesSeeding(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	log := testfixtures.DiscardLogger()
	tenant := testfixtures.NewTenant(t, ctx, pool, 12002)
	carbonRepo := postgres.NewCarbonRepository(pool)
	companyID := tenant.Company.ID

	own, err := carbonRepo.UpsertFactor(ctx, tenant.Scope, model.EmissionFactor{
		CompanyID:    &companyID,
		Key:          seedTargetFactorKey,
		Label:        "Our Own Override — do not touch",
		MainCategory: "company_specific",
		BaseFactor:   decimal.RequireFromString("999.99999999"),
		BaseUnit:     "custom_unit",
	})
	require.NoError(t, err)

	_, err = seed.Load(ctx, pool, log)
	require.NoError(t, err)

	ownStill, err := carbonRepo.Factor(ctx, tenant.Scope, own.ID)
	require.NoError(t, err)
	require.Equal(t, "Our Own Override — do not touch", ownStill.Label,
		"seeding must never modify a company-owned row sharing the platform's key")
	require.True(t, ownStill.BaseFactor.Equal(decimal.RequireFromString("999.99999999")))
	require.NotNil(t, ownStill.CompanyID)
	require.Equal(t, companyID, *ownStill.CompanyID)

	// The platform row for the same key must ALSO exist, as its own
	// distinct row, carrying the shipped dataset's values (not the
	// company's).
	factors, err := seed.EmissionFactors()
	require.NoError(t, err)
	var wantLabel string
	for _, f := range factors {
		if f.Key == seedTargetFactorKey {
			wantLabel = f.Label
			break
		}
	}
	var platformID uuid.UUID
	var platformLabel string
	require.NoError(t, pool.QueryRow(ctx,
		`select id, label from emission_factors where company_id is null and key = $1`, seedTargetFactorKey).
		Scan(&platformID, &platformLabel))
	require.Equal(t, wantLabel, platformLabel)
	require.NotEqual(t, own.ID, platformID, "the platform row and the company's row must be two distinct rows")
}

// TestSeedReplacesConversionsExactly proves a removed conversion actually
// disappears when re-seeding, rather than accumulating: it pre-seeds a
// "ghost" conversion unit (one the shipped dataset does not carry) onto the
// real platform factor row, then re-runs Load and asserts the ghost is gone
// while the dataset's real conversions remain.
func TestSeedReplacesConversionsExactly(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	log := testfixtures.DiscardLogger()
	catalogue := admin.NewCatalogueRepository(pool)
	tenant := testfixtures.NewTenant(t, ctx, pool, 12003)
	carbonRepo := postgres.NewCarbonRepository(pool)

	_, err := seed.Load(ctx, pool, log)
	require.NoError(t, err)

	factors, err := seed.EmissionFactors()
	require.NoError(t, err)
	var target seed.EmissionFactor
	for _, f := range factors {
		if f.Key == seedTargetFactorKey {
			target = f
			break
		}
	}
	require.NotEmpty(t, target.Key, "test setup: %s must exist in the shipped dataset", seedTargetFactorKey)
	require.NotEmpty(t, target.Conversions, "test setup: %s must ship with at least one conversion", seedTargetFactorKey)

	var factorID uuid.UUID
	require.NoError(t, pool.QueryRow(ctx,
		`select id from emission_factors where company_id is null and key = $1`, seedTargetFactorKey).
		Scan(&factorID))

	ghostConversions := make([]model.EmissionFactorConversion, 0, len(target.Conversions)+1)
	for _, c := range target.Conversions {
		ghostConversions = append(ghostConversions, model.EmissionFactorConversion{
			Unit: c.Unit, Multiplier: c.Multiplier, Label: c.Label,
		})
	}
	ghostConversions = append(ghostConversions, model.EmissionFactorConversion{
		Unit: "ghost_unit_not_in_dataset", Multiplier: decimal.RequireFromString("999.00000000"), Label: "ghost",
	})
	require.NoError(t, catalogue.ReplacePlatformConversions(ctx, factorID, ghostConversions))

	before, err := carbonRepo.Conversions(ctx, tenant.Scope, factorID)
	require.NoError(t, err)
	require.Len(t, before, len(target.Conversions)+1, "sanity: the ghost conversion actually landed")

	_, err = seed.Load(ctx, pool, log)
	require.NoError(t, err)

	after, err := carbonRepo.Conversions(ctx, tenant.Scope, factorID)
	require.NoError(t, err)
	require.Len(t, after, len(target.Conversions), "the ghost conversion must be gone, not just left alongside the real ones")
	for _, c := range after {
		require.NotEqual(t, "ghost_unit_not_in_dataset", c.Unit, "re-seeding must have removed the ghost conversion")
	}
}

// TestSeedLoadsNationalTariffScheduleFixture used to live here, calling
// UpsertNationalTariffSchedule directly with one hand-written
// model.NationalTariffScheduleEntry. It never read
// testdata/national_tariff_schedule_fixture.json and never called this
// package's loadNationalTariffSchedule or toModelNationalTariffSchedule, so
// the loader's real 14-field mapping never ran against non-empty data (the
// shipped dataset is `[]`). Fix round 1 (task-12b-fix1-findings.md, Important
// 1) replaced it with TestLoadNationalTariffScheduleFixtureMapsEveryField in
// the white-box seed_national_tariff_fixture_test.go (package seed, not
// seed_test), which reads the fixture, parses it with the unexported
// parseNationalTariffSchedule, and drives it through the real
// loadNationalTariffSchedule function.
