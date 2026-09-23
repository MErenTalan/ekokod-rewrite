package carbon_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/carbon"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

func date(y int, m time.Month, dd int) time.Time { return time.Date(y, m, dd, 0, 0, 0, 0, time.UTC) }

func (w *world) selectAll(t *testing.T, b uuid.UUID) {
	t.Helper()
	_, err := w.svc().SetSelected(context.Background(), w.admin, b, []string{"sub_space_heating", "sub_grid_electricity", "sub_waste_disposal"})
	require.NoError(t, err)
}

func gasInput(b uuid.UUID) carbon.ActivityInput {
	return carbon.ActivityInput{BuildingID: b, SubCategory: "sub_space_heating", FactorKey: "natural_gas", Unit: "m3",
		Quantity: d("1000"), PeriodStart: date(2026, 8, 1), PeriodEnd: date(2026, 8, 31), Description: ptr("Kazan"),
		Details: map[string]string{"level1": "natural_gas"}}
}

var user = uuid.New()

func TestCreateComputesServerSide(t *testing.T) {
	w := newWorld()
	w.selectAll(t, w.b1)
	a, err := w.svc().CreateActivity(context.Background(), w.admin, user, gasInput(w.b1))
	require.NoError(t, err)
	require.True(t, a.EmissionKgco2e.Equal(d("2066.72")))
	require.Equal(t, "cat_stationary", a.MainCategory)
	require.Equal(t, "sub_space_heating", a.ActivityType)
	require.Equal(t, model.CarbonScope1, a.Scope)
	require.Equal(t, "category_1", a.IsoCategory)
	require.Equal(t, model.CarbonStatusPending, a.Status)
	require.Equal(t, w.gasID, *a.FactorID)
	require.True(t, a.FactorValue.Equal(d("2.06672")))
	require.True(t, a.ConversionMultiplier.Equal(d("1")))
	require.Equal(t, user, *a.CreatedBy)
	var details map[string]string
	require.NoError(t, json.Unmarshal(a.Details, &details))
	require.Equal(t, map[string]string{"level1": "natural_gas", "factor_source": "Defra", "factor_source_year": "2025"}, details)

	in := gasInput(w.b1)
	in.Unit, in.Quantity = "kWh", d("10000")
	kwh, err := w.svc().CreateActivity(context.Background(), w.admin, user, in)
	require.NoError(t, err)
	require.True(t, kwh.ConversionMultiplier.Equal(d("0.0948")))
	require.True(t, kwh.EmissionKgco2e.Equal(d("1959.25056")), "10000 × 0.0948 × 2.06672")
}

func TestCreateRefusals(t *testing.T) {
	w := newWorld()
	w.selectAll(t, w.b1)
	ctx := context.Background()
	coalID := uuid.New()
	w.carbon.factors[coalID] = model.EmissionFactor{ID: coalID, Key: "old_coal", MainCategory: "cat_stationary",
		SubCategories: []string{"sub_space_heating"}, BaseFactor: d("2"), BaseUnit: "kg", Status: ptr("archived")}
	for name, c := range map[string]struct {
		mutate      func(*carbon.ActivityInput)
		field, code string
	}{
		"unknown sub":          {func(i *carbon.ActivityInput) { i.SubCategory = "sub_nope" }, "sub_category", "unknown"},
		"not selected":         {func(i *carbon.ActivityInput) { i.SubCategory = "sub_process_combustion" }, "sub_category", "not_selected"},
		"unknown factor":       {func(i *carbon.ActivityInput) { i.FactorKey = "nope" }, "factor_key", "not_found"},
		"archived factor":      {func(i *carbon.ActivityInput) { i.FactorKey = "old_coal"; i.Unit = "kg" }, "factor_key", "archived"},
		"factor of other sub":  {func(i *carbon.ActivityInput) { i.FactorKey = "grid_electricity_tr_2022"; i.Unit = "kWh" }, "factor_key", "not_applicable"},
		"unit not on factor":   {func(i *carbon.ActivityInput) { i.Unit = "litre" }, "unit", "not_supported"},
		"zero quantity":        {func(i *carbon.ActivityInput) { i.Quantity = d("0") }, "quantity", "range"},
		"huge quantity":        {func(i *carbon.ActivityInput) { i.Quantity = d("1000000000001") }, "quantity", "range"},
		"end before start":     {func(i *carbon.ActivityInput) { i.PeriodEnd = date(2026, 7, 31) }, "period_end", "invalid_range"},
		"span over 366 days":   {func(i *carbon.ActivityInput) { i.PeriodStart = date(2025, 8, 29) }, "period_end", "range_too_long"},
		"future end":           {func(i *carbon.ActivityInput) { i.PeriodEnd = date(2026, 9, 11) }, "period_end", "future"},
		"description too long": {func(i *carbon.ActivityInput) { i.Description = ptr(strings.Repeat("a", 501)) }, "description", "too_long"},
		"detail too long":      {func(i *carbon.ActivityInput) { i.Details = map[string]string{"x": strings.Repeat("a", 101)} }, "details", "invalid"},
	} {
		in := gasInput(w.b1)
		c.mutate(&in)
		_, err := w.svc().CreateActivity(ctx, w.admin, user, in)
		requireValidation(t, err, c.field, c.code)
		_ = name
	}
	eleven := map[string]string{}
	for i := 0; i < 11; i++ {
		eleven[string(rune('a'+i))] = "v"
	}
	in := gasInput(w.b1)
	in.Details = eleven
	_, err := w.svc().CreateActivity(ctx, w.admin, user, in)
	requireValidation(t, err, "details", "invalid")

	in = gasInput(w.b2)
	_, err = w.svc().CreateActivity(ctx, w.ba, user, in)
	require.ErrorIs(t, err, store.ErrNotFound, "a building admin outside their building")
	in = gasInput(w.otherB)
	_, err = w.svc().CreateActivity(ctx, w.admin, user, in)
	require.ErrorIs(t, err, store.ErrNotFound, "another company's building")

	// 366 days exactly is allowed; today is allowed.
	in = gasInput(w.b1)
	in.PeriodStart, in.PeriodEnd = date(2025, 9, 10), date(2026, 9, 10)
	_, err = w.svc().CreateActivity(ctx, w.admin, user, in)
	require.NoError(t, err)
}

// Review Focus 1: a later catalogue change never alters a stored record.
func TestStoredFiguresSurviveFactorChanges(t *testing.T) {
	w := newWorld()
	w.selectAll(t, w.b1)
	ctx := context.Background()
	a, err := w.svc().CreateActivity(ctx, w.admin, user, gasInput(w.b1))
	require.NoError(t, err)
	_, err = w.svc().OverrideFactor(ctx, w.admin, w.gasID, carbon.FactorOverride{BaseFactor: d("9")})
	require.NoError(t, err)
	again, err := w.carbon.Activity(ctx, w.admin, a.ID)
	require.NoError(t, err)
	require.True(t, again.EmissionKgco2e.Equal(d("2066.72")))
	require.True(t, again.FactorValue.Equal(d("2.06672")))

	// An edit recomputes with the factor in force now and needs re-approval.
	_, err = w.svc().SetStatus(ctx, w.admin, a.ID, model.CarbonStatusApproved)
	require.NoError(t, err)
	edited, err := w.svc().UpdateActivity(ctx, w.admin, a.ID, gasInput(w.b1))
	require.NoError(t, err)
	require.True(t, edited.EmissionKgco2e.Equal(d("9000")))
	require.Equal(t, model.CarbonStatusPending, edited.Status)
	require.Equal(t, a.CreatedBy, edited.CreatedBy)
}

func TestAutomatedRecordsAreReadOnlyExceptStatus(t *testing.T) {
	w := newWorld()
	w.selectAll(t, w.b1)
	ctx := context.Background()
	auto, err := w.carbon.UpsertAutomatedActivity(ctx, w.admin, model.CarbonActivity{CompanyID: w.company, BuildingID: w.b1,
		SubCategory: "sub_grid_electricity", ActivityType: "sub_grid_electricity", PeriodStart: date(2026, 9, 1),
		PeriodEnd: date(2026, 9, 1), Quantity: d("100"), Unit: "kWh", EmissionKgco2e: d("46.9"), Status: model.CarbonStatusApproved})
	require.NoError(t, err)
	_, err = w.svc().UpdateActivity(ctx, w.admin, auto.ID, gasInput(w.b1))
	require.ErrorIs(t, err, carbon.ErrAutomated)
	require.ErrorIs(t, w.svc().DeleteActivity(ctx, w.admin, auto.ID), carbon.ErrAutomated)
	st, err := w.svc().SetStatus(ctx, w.admin, auto.ID, model.CarbonStatusRejected)
	require.NoError(t, err)
	require.Equal(t, model.CarbonStatusRejected, st.Status)
}

func TestStatusAndDelete(t *testing.T) {
	w := newWorld()
	w.selectAll(t, w.b1)
	ctx := context.Background()
	a, err := w.svc().CreateActivity(ctx, w.admin, user, gasInput(w.b1))
	require.NoError(t, err)
	_, err = w.svc().SetStatus(ctx, w.admin, a.ID, model.CarbonStatusPending)
	requireValidation(t, err, "status", "invalid")
	for range 2 {
		st, err := w.svc().SetStatus(ctx, w.admin, a.ID, model.CarbonStatusApproved)
		require.NoError(t, err, "idempotent")
		require.Equal(t, model.CarbonStatusApproved, st.Status)
	}
	_, err = w.svc().SetStatus(ctx, w.otherSc, a.ID, model.CarbonStatusRejected)
	require.ErrorIs(t, err, store.ErrNotFound)
	require.ErrorIs(t, w.svc().DeleteActivity(ctx, w.otherSc, a.ID), store.ErrNotFound)
	require.NoError(t, w.svc().DeleteActivity(ctx, w.ba, a.ID))
}

func TestOverviewReconcilesWithRecords(t *testing.T) {
	w := newWorld()
	w.selectAll(t, w.b1)
	ctx := context.Background()
	mk := func(start, end time.Time, q string, st model.CarbonStatus) {
		in := gasInput(w.b1)
		in.PeriodStart, in.PeriodEnd, in.Quantity = start, end, d(q)
		a, err := w.svc().CreateActivity(ctx, w.admin, user, in)
		require.NoError(t, err)
		if st != model.CarbonStatusPending {
			_, err = w.svc().SetStatus(ctx, w.admin, a.ID, st)
			require.NoError(t, err)
		}
	}
	mk(date(2026, 1, 1), date(2026, 1, 31), "100", model.CarbonStatusApproved) // 206.672
	mk(date(2026, 3, 1), date(2026, 3, 1), "10", model.CarbonStatusPending)    // 20.6672
	mk(date(2026, 4, 1), date(2026, 4, 1), "999", model.CarbonStatusRejected)
	mk(date(2025, 6, 1), date(2025, 6, 1), "50", model.CarbonStatusApproved) // 103.336, previous year
	in := gasInput(w.b2)
	_, err := w.svc().SetSelected(ctx, w.admin, w.b2, []string{"sub_space_heating"})
	require.NoError(t, err)
	_, err = w.svc().CreateActivity(ctx, w.admin, user, in) // another building never counts
	require.NoError(t, err)

	o, err := w.svc().Overview(ctx, w.admin, w.b1, 2026)
	require.NoError(t, err)
	require.True(t, o.Total.Equal(d("227.3392")), o.Total.String())
	require.Equal(t, 2, o.ActivityCount)
	require.Equal(t, 1, o.PendingCount)
	require.Equal(t, 3, o.RegisteredCount)
	require.True(t, o.Monthly[0].Current.Equal(d("206.672")))
	require.True(t, o.Monthly[5].Previous.Equal(d("103.336")))
	require.Equal(t, "sub_space_heating", o.Highest.Sub)
	require.Len(t, o.ByCategory, 8)
	require.Equal(t, "cat_stationary", o.ByCategory[0].Key)
	require.True(t, o.ByCategory[0].KgCO2e.Equal(o.Total))
	require.Len(t, o.ByScope, 3)
	require.Len(t, o.Recent, 4, "the building's records in the two-year window, newest created first")
	require.True(t, o.Recent[0].CreatedAt.After(o.Recent[1].CreatedAt))

	_, err = w.svc().Overview(ctx, w.ba, w.b2, 2026)
	require.ErrorIs(t, err, store.ErrNotFound)
}

func TestActivitiesListFiltersAndScope(t *testing.T) {
	w := newWorld()
	w.selectAll(t, w.b1)
	ctx := context.Background()
	_, err := w.svc().CreateActivity(ctx, w.admin, user, gasInput(w.b1))
	require.NoError(t, err)
	from, to := date(2026, 8, 15), date(2026, 8, 20)
	list, err := w.svc().Activities(ctx, w.ba, carbon.ActivityQuery{BuildingID: &w.b1, From: &from, To: &to})
	require.NoError(t, err)
	require.Len(t, list, 1, "overlap, not containment")
	approved := model.CarbonStatusApproved
	list, err = w.svc().Activities(ctx, w.ba, carbon.ActivityQuery{Status: &approved})
	require.NoError(t, err)
	require.Empty(t, list)
	_, err = w.svc().Activities(ctx, w.ba, carbon.ActivityQuery{BuildingID: &w.b2})
	require.ErrorIs(t, err, store.ErrNotFound, "a named building outside the scope")
}
