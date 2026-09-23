package carbon_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/carbon"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

type accrualWorld struct {
	*world
	analyzers *fakeAnalyzers
	analytics *fakeAnalytics
	ops       *fakeOps
	a1, a2    uuid.UUID
}

// istDay is the daily bucket start: Istanbul midnight.
func istDay(y int, m time.Month, dd int) time.Time {
	loc, _ := time.LoadLocation("Europe/Istanbul")
	return time.Date(y, m, dd, 0, 0, 0, 0, loc).UTC()
}

func newAccrualWorld() *accrualWorld {
	w := &accrualWorld{world: newWorld(), a1: uuid.New(), a2: uuid.New(), ops: &fakeOps{}, analytics: &fakeAnalytics{}}
	w.analyzers = &fakeAnalyzers{byBuilding: map[uuid.UUID][]uuid.UUID{w.b1: {w.a1, w.a2}}} // b2 has no analyzers
	return w
}

func (w *accrualWorld) bucket(a uuid.UUID, day time.Time, cons, gen *decimal.Decimal) {
	w.analytics.buckets = append(w.analytics.buckets, model.ConsumptionBucket{AnalyzerID: a, Bucket: day, ActiveConsumption: cons, ActiveGeneration: gen})
}

func (w *accrualWorld) accrual() *carbon.Service {
	return carbon.New(carbon.Deps{Carbon: w.carbon, Buildings: w.buildings, Analyzers: w.analyzers, Analytics: w.analytics,
		Ops: w.ops, Tenants: &fakeTenants{companies: []model.Company{{ID: w.company}, {ID: w.other}}}, Clock: clock.NewFake(now)})
}

func (w *accrualWorld) automated(t *testing.T) []model.CarbonActivity {
	t.Helper()
	yes := true
	list, err := w.carbon.ListActivities(context.Background(), w.admin, store.CarbonActivityFilter{IsAutomated: &yes})
	require.NoError(t, err)
	return list
}

func TestAccrualTwiceGivesOneRowPerBuildingAndType(t *testing.T) {
	w := newAccrualWorld()
	day := istDay(2026, 9, 9)
	w.bucket(w.a1, day, ptr(d("100.5")), ptr(d("20")))
	w.bucket(w.a2, day, ptr(d("50")), nil)
	w.bucket(w.a1, istDay(2026, 9, 8), ptr(d("999")), nil) // another day never counts
	ctx := context.Background()
	for range 2 {
		res, err := w.accrual().Accrue(ctx, date(2026, 9, 9))
		require.NoError(t, err)
		require.Equal(t, 1, res.GridRows)
		require.Equal(t, 1, res.GenerationRows)
		require.Equal(t, 2, res.Companies)
	}
	rows := w.automated(t)
	require.Len(t, rows, 2, "acceptance: one record per building/type/day")
	byType := map[string]model.CarbonActivity{}
	for _, r := range rows {
		byType[r.ActivityType] = r
	}
	grid := byType["sub_grid_electricity"]
	require.True(t, grid.Quantity.Equal(d("150.5")))
	require.True(t, grid.EmissionKgco2e.Equal(d("70.5845")), "150.5 × 0.469")
	require.Equal(t, model.CarbonStatusApproved, grid.Status)
	require.Equal(t, model.CarbonScope2, grid.Scope)
	require.Equal(t, date(2026, 9, 9), grid.PeriodStart)
	require.Equal(t, date(2026, 9, 9), grid.PeriodEnd)
	require.Equal(t, "grid_electricity_tr_2022", *grid.FactorKey)

	gen := byType["sub_elec_generation"]
	require.True(t, gen.Quantity.Equal(d("20")))
	require.True(t, gen.EmissionKgco2e.IsZero(), "Q-F3: rooftop export carries no emission")
	var details map[string]string
	require.NoError(t, json.Unmarshal(gen.Details, &details))
	require.Equal(t, "9.38", details["avoided_kgco2e"])
	require.Equal(t, "meter_export", details["source"])

	// Late data converges on the same row.
	w.bucket(w.a2, day, nil, nil)
	w.analytics.buckets[1].ActiveConsumption = ptr(d("60"))
	_, err := w.accrual().Accrue(ctx, date(2026, 9, 9))
	require.NoError(t, err)
	rows = w.automated(t)
	require.Len(t, rows, 2)
	for _, r := range rows {
		if r.ActivityType == "sub_grid_electricity" {
			require.True(t, r.Quantity.Equal(d("160.5")))
		}
	}
	require.Equal(t, []string{"success", "success", "success", "success", "success", "success"}, w.ops.finished)
}

func TestAccrualSkipsWithoutDataOrFactor(t *testing.T) {
	w := newAccrualWorld()
	day := istDay(2026, 9, 9)
	w.bucket(w.a1, day, ptr(d("0")), ptr(d("0")))
	res, err := w.accrual().Accrue(context.Background(), date(2026, 9, 9))
	require.NoError(t, err)
	require.Equal(t, 1, res.GridRows, "measured zero consumption is a record")
	require.Zero(t, res.GenerationRows, "export 0 → no generation row")

	w2 := newAccrualWorld()
	w2.bucket(w2.a1, day, nil, nil)
	res, err = w2.accrual().Accrue(context.Background(), date(2026, 9, 9))
	require.NoError(t, err)
	require.Zero(t, res.GridRows, "no measurement → no record")

	w3 := newAccrualWorld()
	delete(w3.carbon.factors, w3.gridID)
	w3.bucket(w3.a1, day, ptr(d("10")), ptr(d("5")))
	res, err = w3.accrual().Accrue(context.Background(), date(2026, 9, 9))
	require.NoError(t, err)
	require.Zero(t, res.GridRows)
	require.Equal(t, 1, res.Skipped["grid_factor_missing"])
	require.Equal(t, 1, res.GenerationRows, "generation is recorded without its avoided figure")
	require.NotEmpty(t, w3.ops.messages)
	require.Equal(t, "error", w3.ops.messages[0].Status)
}

func TestAccrualDayBounds(t *testing.T) {
	w := newAccrualWorld()
	_, err := w.accrual().Accrue(context.Background(), date(2026, 9, 10))
	requireValidation(t, err, "day", "range")
	_, err = w.accrual().Accrue(context.Background(), date(2025, 8, 5))
	requireValidation(t, err, "day", "range")
	_, err = w.accrual().Accrue(context.Background(), date(2025, 8, 6))
	require.NoError(t, err, "400 days back is allowed")
}

func TestAccrueTaskDefaultsToYesterday(t *testing.T) {
	w := newAccrualWorld()
	w.bucket(w.a1, istDay(2026, 9, 9), ptr(d("1")), nil)
	require.NoError(t, w.accrual().AccrueTask(context.Background(), ""))
	require.Len(t, w.automated(t), 1)
	require.Error(t, w.accrual().AccrueTask(context.Background(), "09.09.2026"))
}
