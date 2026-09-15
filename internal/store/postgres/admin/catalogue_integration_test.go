//go:build integration

package admin_test

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

func catalogueDec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func TestAdminUpsertNationalTariffScheduleIsIdempotent(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	repo := admin.NewCatalogueRepository(pool)

	entry := model.NationalTariffScheduleEntry{
		EffectiveFrom:     time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		UserGroup:         model.UserGroupCommercial,
		VoltageLevel:      model.VoltageLevelLV,
		Term:              model.TariffTermMonomial,
		EnergyPrice:       catalogueDec("2.500000"),
		DistributionPrice: catalogueDec("0.500000"),
		VatRate:           catalogueDec("20.000"),
	}
	n, err := repo.UpsertNationalTariffSchedule(ctx, []model.NationalTariffScheduleEntry{entry})
	require.NoError(t, err)
	require.EqualValues(t, 1, n)

	// Re-upserting the same natural key converges rather than duplicating.
	entry.EnergyPrice = catalogueDec("2.750000")
	n, err = repo.UpsertNationalTariffSchedule(ctx, []model.NationalTariffScheduleEntry{entry})
	require.NoError(t, err)
	require.EqualValues(t, 1, n)

	var count int
	require.NoError(t, pool.QueryRow(ctx, `select count(*) from national_tariff_schedule`).Scan(&count))
	require.Equal(t, 1, count, "the unique (effective_from, user_group, voltage_level, term) index must converge on one row")

	var storedPrice string
	require.NoError(t, pool.QueryRow(ctx, `select energy_price::text from national_tariff_schedule`).Scan(&storedPrice))
	require.Equal(t, "2.750000", storedPrice)
}

func TestAdminUpsertPlatformFactorNeverTouchesACompanyOwnedRow(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	catalogueRepo := admin.NewCatalogueRepository(pool)
	carbonRepo := postgres.NewCarbonRepository(pool)
	tenant := testfixtures.NewTenant(t, ctx, pool, 9001)

	// A CompanyID-bearing factor is refused before any database call.
	companyID := tenant.Company.ID
	_, err := catalogueRepo.UpsertPlatformFactor(ctx, model.EmissionFactor{
		CompanyID: &companyID, Key: "diesel", Label: "x", MainCategory: "fuel",
		BaseFactor: catalogueDec("1"), BaseUnit: "kg",
	})
	require.ErrorIs(t, err, store.ErrNotFound)

	platform, err := catalogueRepo.UpsertPlatformFactor(ctx, model.EmissionFactor{
		Key: "diesel", Label: "Diesel", MainCategory: "fuel", BaseFactor: catalogueDec("2.68000000"), BaseUnit: "kg",
	})
	require.NoError(t, err)
	require.Nil(t, platform.CompanyID)

	// The tenant shadows the same key with its own row.
	own, err := carbonRepo.UpsertFactor(ctx, tenant.Scope, model.EmissionFactor{
		CompanyID: &companyID, Key: "diesel", Label: "Our Diesel", MainCategory: "fuel",
		BaseFactor: catalogueDec("2.90000000"), BaseUnit: "kg",
	})
	require.NoError(t, err)
	require.NotEqual(t, platform.ID, own.ID)

	// Re-upserting the platform key must not touch the company's shadowing
	// row.
	updatedPlatform, err := catalogueRepo.UpsertPlatformFactor(ctx, model.EmissionFactor{
		Key: "diesel", Label: "Diesel Updated", MainCategory: "fuel", BaseFactor: catalogueDec("2.70000000"), BaseUnit: "kg",
	})
	require.NoError(t, err)
	require.Equal(t, platform.ID, updatedPlatform.ID)

	ownStill, err := carbonRepo.Factor(ctx, tenant.Scope, own.ID)
	require.NoError(t, err)
	require.Equal(t, "Our Diesel", ownStill.Label, "the company's own row must be untouched by a platform upsert")
}

func TestAdminReplacePlatformConversionsRefusesACompanyOwnedFactor(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	catalogueRepo := admin.NewCatalogueRepository(pool)
	carbonRepo := postgres.NewCarbonRepository(pool)
	tenant := testfixtures.NewTenant(t, ctx, pool, 9002)

	companyID := tenant.Company.ID
	own, err := carbonRepo.UpsertFactor(ctx, tenant.Scope, model.EmissionFactor{
		CompanyID: &companyID, Key: "propane", Label: "Propane", MainCategory: "fuel",
		BaseFactor: catalogueDec("1.51000000"), BaseUnit: "kg",
	})
	require.NoError(t, err)

	err = catalogueRepo.ReplacePlatformConversions(ctx, own.ID, []model.EmissionFactorConversion{
		{Unit: "litre", Multiplier: catalogueDec("0.55000000"), Label: "per litre"},
	})
	require.ErrorIs(t, err, store.ErrNotFound, "a company-owned factor's id must be refused, not replaced")

	conversions, err := carbonRepo.Conversions(ctx, tenant.Scope, own.ID)
	require.NoError(t, err)
	require.Empty(t, conversions, "the refused admin replace must not have written anything")

	platform, err := catalogueRepo.UpsertPlatformFactor(ctx, model.EmissionFactor{
		Key: "butane", Label: "Butane", MainCategory: "fuel", BaseFactor: catalogueDec("1.60000000"), BaseUnit: "kg",
	})
	require.NoError(t, err)

	require.NoError(t, catalogueRepo.ReplacePlatformConversions(ctx, platform.ID, []model.EmissionFactorConversion{
		{Unit: "litre", Multiplier: catalogueDec("0.58000000"), Label: "per litre"},
	}))

	platformConversions, err := carbonRepo.Conversions(ctx, tenant.Scope, platform.ID)
	require.NoError(t, err)
	require.Len(t, platformConversions, 1)
}

func TestAdminUpsertIntegrationDefinitionsIsIdempotent(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	repo := admin.NewCatalogueRepository(pool)

	defs := []model.IntegrationDefinition{
		{Provider: model.IntegrationProviderOSOS, Subtype: "default", Endpoints: []byte(`{"a":1}`)},
	}
	n, err := repo.UpsertIntegrationDefinitions(ctx, defs)
	require.NoError(t, err)
	require.EqualValues(t, 1, n)

	defs[0].Endpoints = []byte(`{"a":2}`)
	n, err = repo.UpsertIntegrationDefinitions(ctx, defs)
	require.NoError(t, err)
	require.EqualValues(t, 1, n)

	var count int
	require.NoError(t, pool.QueryRow(ctx, `select count(*) from integration_definitions`).Scan(&count))
	require.Equal(t, 1, count)
}
