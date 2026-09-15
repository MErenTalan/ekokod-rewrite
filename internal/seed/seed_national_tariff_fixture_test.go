//go:build integration

// Task 12b fix round 1 (task-12b-fix1-findings.md, Important 1): the
// original TestSeedLoadsNationalTariffScheduleFixture (seed_integration_test.go)
// never read testdata/national_tariff_schedule_fixture.json — it hand-wrote
// one model.NationalTariffScheduleEntry and called
// admin.NewCatalogueRepository(pool).UpsertNationalTariffSchedule directly,
// bypassing seed.go's loadNationalTariffSchedule and
// toModelNationalTariffSchedule entirely. Since the shipped
// data/national_tariff_schedule.json is `[]`, that left the loader's
// 14-field mapping untested against any real input.
//
// This file is a WHITE-BOX test (package seed, not seed_test, unlike the
// rest of this package's integration tests) specifically so it can reach the
// unexported parseNationalTariffSchedule and loadNationalTariffSchedule
// directly: it reads the fixture file, parses it with the real parser, then
// drives it through the real loadNationalTariffSchedule (the exact function
// seed.Load calls, via a parameter seam — see seed.go's doc comment on
// loadNationalTariffSchedule for why entries is a parameter rather than
// loaded internally), then reads every stored row back through the
// platform-wide NationalTariffRepository and asserts each of the 14 mapped
// fields against the fixture's own value.
package seed

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/admin"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

// TestLoadNationalTariffScheduleFixtureMapsEveryField is the fix-round-1
// replacement for the old bypassing test. It proves the loader's mapping is
// correct by construction, not merely by the admin SQL converging (which
// Task 11b's own tests already prove independently): swap two mapped fields
// in toModelNationalTariffSchedule (seed.go) and this test fails — see the
// fix-round-1 report for the observed failure.
func TestLoadNationalTariffScheduleFixtureMapsEveryField(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	log := testfixtures.DiscardLogger()
	catalogue := admin.NewCatalogueRepository(pool)
	tenant := testfixtures.NewTenant(t, ctx, pool, 12005)
	tariffRepo := postgres.NewNationalTariffRepository(pool)

	raw := mustReadFile(t, "testdata/national_tariff_schedule_fixture.json")
	fixtureEntries, err := parseNationalTariffSchedule(raw)
	require.NoError(t, err)
	require.Len(t, fixtureEntries, 3, "test setup: the fixture file must have 3 rows")

	// Drive the REAL loader function, not a hand-built model value and not a
	// direct catalogue.UpsertNationalTariffSchedule call — this is exactly
	// what Load itself calls (seed.go), including toModelNationalTariffSchedule's
	// full 14-field mapping.
	n, err := loadNationalTariffSchedule(ctx, catalogue, log, fixtureEntries)
	require.NoError(t, err)
	require.EqualValues(t, 3, n)

	stored, err := tariffRepo.List(ctx, tenant.Scope, store.NationalTariffFilter{Page: store.Page{Limit: 50}})
	require.NoError(t, err)
	require.Len(t, stored, 3, "all 3 fixture rows must have been written")

	byKey := make(map[string]model.NationalTariffScheduleEntry, len(stored))
	for _, s := range stored {
		byKey[nationalTariffTestKey(s.EffectiveFrom, s.UserGroup, s.VoltageLevel, s.Term)] = s
	}

	for _, want := range fixtureEntries {
		got, ok := byKey[nationalTariffTestKey(want.EffectiveFrom, want.UserGroup, want.VoltageLevel, want.Term)]
		require.True(t, ok, "no stored row for fixture key (%s, %s, %s, %s)",
			want.EffectiveFrom.Format("2006-01-02"), want.UserGroup, want.VoltageLevel, want.Term)

		require.True(t, want.EffectiveFrom.Equal(got.EffectiveFrom), "effective_from")
		require.Equal(t, want.UserGroup, got.UserGroup, "user_group")
		require.Equal(t, want.VoltageLevel, got.VoltageLevel, "voltage_level")
		require.Equal(t, want.Term, got.Term, "term")
		require.True(t, want.EnergyPrice.Equal(got.EnergyPrice), "energy_price")
		requireOptionalDecimalEqual(t, "t1_price", want.T1Price, got.T1Price)
		requireOptionalDecimalEqual(t, "t2_price", want.T2Price, got.T2Price)
		requireOptionalDecimalEqual(t, "t3_price", want.T3Price, got.T3Price)
		require.True(t, want.DistributionPrice.Equal(got.DistributionPrice), "distribution_price")
		requireOptionalDecimalEqual(t, "power_price", want.PowerPrice, got.PowerPrice)
		requireOptionalDecimalEqual(t, "overuse_price", want.OverusePrice, got.OverusePrice)
		requireOptionalDecimalEqual(t, "daily_threshold_kwh", want.DailyThresholdKwh, got.DailyThresholdKwh)
		require.True(t, want.VatRate.Equal(got.VatRate), "vat_rate")
		require.NotNil(t, want.Source, "test setup: fixture rows all carry a source")
		require.NotNil(t, got.Source, "source")
		if want.Source != nil && got.Source != nil {
			require.Equal(t, *want.Source, *got.Source, "source")
		}
	}

	// Idempotency at the loader-function level, mirroring
	// TestSeedRunTwiceProducesTheSameRowCounts's proof for the other two
	// datasets: re-running the loader against the same fixture converges,
	// it does not duplicate.
	n, err = loadNationalTariffSchedule(ctx, catalogue, log, fixtureEntries)
	require.NoError(t, err)
	require.EqualValues(t, 3, n)
	stored, err = tariffRepo.List(ctx, tenant.Scope, store.NationalTariffFilter{Page: store.Page{Limit: 50}})
	require.NoError(t, err)
	require.Len(t, stored, 3, "re-running the loader must converge, not duplicate")
}

func nationalTariffTestKey(effectiveFrom time.Time, userGroup model.DistributionUserGroup, voltageLevel model.VoltageLevel, term model.TariffTerm) string {
	return effectiveFrom.Format("2006-01-02") + "|" + string(userGroup) + "|" + string(voltageLevel) + "|" + string(term)
}

func requireOptionalDecimalEqual(t *testing.T, field string, want, got *decimal.Decimal) {
	t.Helper()
	if want == nil {
		require.Nil(t, got, "%s: want nil", field)
		return
	}
	require.NotNil(t, got, "%s: want non-nil", field)
	require.True(t, want.Equal(*got), "%s: want %s got %s", field, want.String(), got.String())
}
