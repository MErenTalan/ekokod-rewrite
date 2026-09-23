package carbon_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	perr "github.com/MErenTalan/ekokod-rewrite/internal/platform/errors"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/carbon"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

var now = time.Date(2026, 9, 10, 9, 0, 0, 0, time.UTC)

func (w *world) svc() *carbon.Service {
	return carbon.New(carbon.Deps{Carbon: w.carbon, Buildings: w.buildings, Clock: clock.NewFake(now)})
}

func requireValidation(t *testing.T, err error, field, code string) {
	t.Helper()
	var pe *perr.Error
	require.True(t, errors.As(err, &pe), "want a validation error on %s, got %v", field, err)
	require.Equal(t, []string{code}, pe.Params[field], "params %v", pe.Params)
}

func TestFactorsShadowPlatformRows(t *testing.T) {
	w := newWorld()
	ctx := context.Background()
	_, err := w.svc().OverrideFactor(ctx, w.admin, w.gasID, carbon.FactorOverride{BaseFactor: d("2.1"), SourceYear: ptr(int16(2026))})
	require.NoError(t, err)

	list, err := w.svc().Factors(ctx, w.admin, carbon.FactorQuery{})
	require.NoError(t, err)
	require.Len(t, list, 2, "the shadow replaces the platform row, it is not listed twice")
	gas := list[1]
	require.Equal(t, "natural_gas", gas.Key)
	require.True(t, gas.Overridden)
	require.True(t, gas.BaseFactor.Equal(d("2.1")))
	require.True(t, gas.PlatformBaseFactor.Equal(d("2.06672")))
	require.EqualValues(t, 2026, *gas.SourceYear)
	require.Equal(t, "Defra", *gas.Source, "unset metadata is copied from the platform row")
	require.Len(t, gas.Conversions, 2, "conversions are copied to the shadow")
	require.NotEqual(t, w.gasID, gas.ID)

	other, err := w.svc().Factors(ctx, w.otherSc, carbon.FactorQuery{})
	require.NoError(t, err)
	require.False(t, other[1].Overridden, "another company still sees the platform value")
	require.True(t, other[1].BaseFactor.Equal(d("2.06672")))

	grid := list[0]
	require.False(t, grid.Overridden)
	require.True(t, grid.PlatformBaseFactor.Equal(d("0.469")))
}

func TestFactorsFilterBySubCategoryAndText(t *testing.T) {
	w := newWorld()
	list, err := w.svc().Factors(context.Background(), w.admin, carbon.FactorQuery{SubCategory: "sub_process_combustion"})
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, "natural_gas", list[0].Key)
	list, err = w.svc().Factors(context.Background(), w.admin, carbon.FactorQuery{Q: "ŞEBEKE"})
	require.NoError(t, err)
	require.Len(t, list, 1, "case-insensitive on label and key")
}

func TestOverrideRefusals(t *testing.T) {
	w := newWorld()
	ctx := context.Background()
	_, err := w.svc().OverrideFactor(ctx, w.admin, w.gasID, carbon.FactorOverride{BaseFactor: d("0")})
	requireValidation(t, err, "base_factor", "range")
	_, err = w.svc().OverrideFactor(ctx, w.admin, w.gasID, carbon.FactorOverride{BaseFactor: d("1000000.1")})
	requireValidation(t, err, "base_factor", "range")
	_, err = w.svc().OverrideFactor(ctx, w.admin, w.gasID, carbon.FactorOverride{BaseFactor: d("1"), SourceYear: ptr(int16(1989))})
	requireValidation(t, err, "source_year", "range")

	own, err := w.svc().OverrideFactor(ctx, w.otherSc, w.gasID, carbon.FactorOverride{BaseFactor: d("3")})
	require.NoError(t, err)
	_, err = w.svc().OverrideFactor(ctx, w.admin, own.ID, carbon.FactorOverride{BaseFactor: d("4")})
	require.ErrorIs(t, err, store.ErrNotFound, "another company's own factor")
}

func TestOverrideOwnFactorUpdatesInPlace(t *testing.T) {
	w := newWorld()
	ctx := context.Background()
	first, err := w.svc().OverrideFactor(ctx, w.admin, w.gasID, carbon.FactorOverride{BaseFactor: d("2.1")})
	require.NoError(t, err)
	second, err := w.svc().OverrideFactor(ctx, w.admin, first.ID, carbon.FactorOverride{BaseFactor: d("2.2"), Source: ptr("Our lab")})
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID)
	require.True(t, second.BaseFactor.Equal(d("2.2")))
	require.Equal(t, "Our lab", *second.Source)
}

func TestResetRestoresPlatformValues(t *testing.T) {
	w := newWorld()
	ctx := context.Background()
	_, err := w.svc().OverrideFactor(ctx, w.admin, w.gasID, carbon.FactorOverride{BaseFactor: d("2.1")})
	require.NoError(t, err)
	require.NoError(t, w.svc().ResetFactors(ctx, w.admin))
	list, err := w.svc().Factors(ctx, w.admin, carbon.FactorQuery{})
	require.NoError(t, err)
	require.False(t, list[1].Overridden)
	require.True(t, list[1].BaseFactor.Equal(d("2.06672")))
}

func TestGridFactorPrefersCompanyOverride(t *testing.T) {
	w := newWorld()
	ctx := context.Background()
	g, err := w.svc().GridFactor(ctx, w.admin)
	require.NoError(t, err)
	require.True(t, g.BaseFactor.Equal(d("0.469")))
	_, err = w.svc().OverrideFactor(ctx, w.admin, w.gridID, carbon.FactorOverride{BaseFactor: d("0.44"), SourceYear: ptr(int16(2026))})
	require.NoError(t, err)
	g, err = w.svc().GridFactor(ctx, w.admin)
	require.NoError(t, err)
	require.True(t, g.BaseFactor.Equal(d("0.44")))

	delete(w.carbon.factors, w.gridID)
	g, err = w.svc().GridFactor(ctx, w.otherSc)
	require.NoError(t, err)
	require.Nil(t, g, "no grid factor at all is nil, not an error")
}

func TestSelection(t *testing.T) {
	w := newWorld()
	ctx := context.Background()
	got, err := w.svc().SetSelected(ctx, w.admin, w.b1, []string{"sub_waste_disposal", "sub_space_heating", "sub_waste_disposal"})
	require.NoError(t, err)
	require.Equal(t, []string{"sub_space_heating", "sub_waste_disposal"}, got, "deduplicated and sorted")
	read, err := w.svc().Selected(ctx, w.ba, w.b1)
	require.NoError(t, err)
	require.Equal(t, got, read)

	empty, err := w.svc().Selected(ctx, w.admin, w.b2)
	require.NoError(t, err)
	require.Empty(t, empty, "never declared means none (Q-F5)")

	_, err = w.svc().SetSelected(ctx, w.admin, w.b1, []string{"sub_space_heating", "sub_nope"})
	requireValidation(t, err, "activity_keys", "unknown")
	_, err = w.svc().Selected(ctx, w.ba, w.b2)
	require.ErrorIs(t, err, store.ErrNotFound, "a building admin outside their building")
	_, err = w.svc().SetSelected(ctx, w.admin, w.otherB, nil)
	require.ErrorIs(t, err, store.ErrNotFound, "another company's building")
	_, err = w.svc().Selected(ctx, w.admin, uuid.New())
	require.ErrorIs(t, err, store.ErrNotFound)
}
