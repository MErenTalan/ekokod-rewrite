//go:build integration

package renewable_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	perr "github.com/MErenTalan/ekokod-rewrite/internal/platform/errors"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/renewable"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

var loadNow = time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)

func loader(t *testing.T) (*renewable.Service, testfixtures.Tenant, testfixtures.Tenant, *postgres.BillRepository) {
	t.Helper()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	other := testfixtures.NewTenant(t, ctx, pool, 2)
	bills := postgres.NewBillRepository(pool)
	svc := renewable.New(renewable.Deps{Analyzers: postgres.NewAnalyzerRepository(pool), Buildings: postgres.NewBuildingRepository(pool),
		Analytics: postgres.NewAnalyticsRepository(pool), Bills: bills, Forecasts: postgres.NewForecastRepository(pool),
		Carbon: postgres.NewCarbonRepository(pool), Clock: clock.NewFake(loadNow)})
	return svc, tenant, other, bills
}

func query(analyzer, building *uuid.UUID) renewable.Query {
	return renewable.Query{AnalyzerID: analyzer, BuildingID: building, From: loadNow.AddDate(0, 0, -7), To: loadNow}
}

// A building admin reads their own building's analyzers and nothing else.
func TestRenewableScopeBuildingAdminOwnAnalyzerOnly(t *testing.T) {
	t.Parallel()
	svc, tenant, other, _ := loader(t)
	ctx := context.Background()
	own, foreign := tenant.Analyzers[0], tenant.Analyzers[len(tenant.Analyzers)-1]
	require.NotEqual(t, *own.BuildingID, *foreign.BuildingID)

	_, err := svc.Load(ctx, tenant.Scope, query(&own.ID, nil))
	require.NoError(t, err)
	_, err = svc.Load(ctx, tenant.Scope, query(nil, &tenant.Buildings[0].ID))
	require.NoError(t, err)

	for name, q := range map[string]renewable.Query{
		"another building's analyzer": query(&foreign.ID, nil),
		"another building":            query(nil, foreign.BuildingID),
		"another company's analyzer":  query(&other.Analyzers[0].ID, nil),
		"another company's building":  query(nil, &other.Buildings[0].ID),
	} {
		_, err := svc.Load(ctx, tenant.Scope, q)
		require.ErrorIs(t, err, store.ErrNotFound, name)
	}
	_, err = svc.Load(ctx, tenant.AdminScope, query(nil, &other.Buildings[0].ID))
	require.ErrorIs(t, err, store.ErrNotFound, "even the company scope stops at the company")
}

// R298: exactly one subject, a forward range of at most 366 days.
func TestRenewableQueryLimits(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	for name, q := range map[string]renewable.Query{
		"no subject":   {From: loadNow.AddDate(0, 0, -1), To: loadNow},
		"two subjects": {AnalyzerID: &id, BuildingID: &id, From: loadNow.AddDate(0, 0, -1), To: loadNow},
		"backwards":    {AnalyzerID: &id, From: loadNow, To: loadNow.AddDate(0, 0, -1)},
		"367 days":     {AnalyzerID: &id, From: loadNow.AddDate(0, 0, -367), To: loadNow},
	} {
		var e *perr.Error
		require.True(t, errors.As(q.Validate(), &e), name)
	}
	require.NoError(t, renewable.Query{AnalyzerID: &id, From: loadNow.AddDate(0, 0, -366), To: loadNow}.Validate())
}

// Bills follow the subject: analyzer bills for an analyzer, building bills
// for a building; the newest period is the latest bill.
func TestRenewableLoadsTheSubjectsBills(t *testing.T) {
	t.Parallel()
	svc, tenant, _, bills := loader(t)
	ctx := context.Background()
	a := tenant.Analyzers[0]
	mk := func(scope model.BillScope, analyzer *uuid.UUID, period string, credit string) {
		start, _ := time.Parse("2006-01", period)
		v := decimal.RequireFromString(credit)
		_, err := bills.Create(ctx, tenant.AdminScope, model.Bill{CompanyID: tenant.Company.ID, BuildingID: a.BuildingID, AnalyzerID: analyzer,
			Scope: scope, PeriodKey: period, PeriodStart: start, PeriodEnd: start.AddDate(0, 1, 0), DaysInPeriod: 30,
			Currency: model.CurrencyTRY, Status: model.BillStatusIssued, GenerationUsage: model.GenerationUsageNone,
			GenerationCredit: v, ComputedAt: loadNow, IndexStart: []byte(`{}`), IndexEnd: []byte(`{}`)}, nil, nil)
		require.NoError(t, err)
	}
	mk(model.BillScopeAnalyzer, &a.ID, "2026-01", "10")
	mk(model.BillScopeAnalyzer, &a.ID, "2026-02", "20")
	mk(model.BillScopeBuilding, nil, "2026-02", "99")

	in, err := svc.Load(ctx, tenant.Scope, query(&a.ID, nil))
	require.NoError(t, err)
	require.Len(t, in.Bills, 2)
	require.Equal(t, "2026-02", in.LatestBill.PeriodKey)
	require.Equal(t, model.BillScopeAnalyzer, in.LatestBill.Scope)

	in, err = svc.Load(ctx, tenant.Scope, query(nil, a.BuildingID))
	require.NoError(t, err)
	require.Len(t, in.Bills, 1)
	require.Equal(t, model.BillScopeBuilding, in.Bills[0].Scope)
}

// R262/R293: the grid factor is kg CO2e per activity unit, and the company's
// own row wins over the platform's.
func TestRenewableFactorsCompanyFirstWithUnits(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1)
	for _, row := range []struct {
		company *uuid.UUID
		key     string
		factor  string
		unit    string
	}{
		{nil, renewable.GridFactorKey, "0.469", "kWh"},
		{&tenant.Company.ID, renewable.GridFactorKey, "0.41", "kWh"},
		{nil, renewable.EquivTree, "21.77", "kg CO2/ağaç-yıl"},
	} {
		_, err := pool.Exec(ctx, `insert into emission_factors (company_id, key, label, main_category, base_factor, base_unit, source, source_year)
			values ($1, $2, $2, 'test', $3, $4, 'Kaynak', 2023)`, row.company, row.key, row.factor, row.unit)
		require.NoError(t, err)
	}
	svc := renewable.New(renewable.Deps{Analyzers: postgres.NewAnalyzerRepository(pool), Buildings: postgres.NewBuildingRepository(pool),
		Analytics: postgres.NewAnalyticsRepository(pool), Bills: postgres.NewBillRepository(pool), Forecasts: postgres.NewForecastRepository(pool),
		Carbon: postgres.NewCarbonRepository(pool), Clock: clock.NewFake(loadNow)})
	in, err := svc.Load(ctx, tenant.Scope, query(&tenant.Analyzers[0].ID, nil))
	require.NoError(t, err)
	require.NotNil(t, in.GridFactor)
	require.Equal(t, "0.41", in.GridFactor.Value.String(), "the company's own factor first")
	require.Equal(t, "kg CO2e/kWh", in.GridFactor.Unit, "the factor is per activity unit, not the unit itself")
	require.Equal(t, "kg CO2/ağaç-yıl", in.Equivalences[renewable.EquivTree].Unit)
}
