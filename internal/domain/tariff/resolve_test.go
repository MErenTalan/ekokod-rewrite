package tariff_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/tariff"
)

func version(building *uuid.UUID, y int, m time.Month, day int, created time.Time, price string) model.Tariff {
	tr := fixedTariff()
	tr.ID = uuid.New()
	tr.BuildingID = building
	tr.EffectiveFrom = time.Date(y, m, day, 0, 0, 0, 0, time.UTC)
	tr.CreatedAt = created
	tr.SingleTimePrice = dp(price)
	return tr
}

func TestResolvePicksGreatestEffectiveFromNotAfterDate(t *testing.T) {
	loc := istanbul(t)
	b := uuid.New()
	c := time.Unix(0, 0)
	vs := []model.Tariff{version(&b, 2026, 1, 1, c, "1"), version(&b, 2026, 3, 1, c, "3"), version(&b, 2026, 2, 1, c, "2")}
	got, ok := tariff.Resolve(vs, b, time.Date(2026, 2, 28, 23, 59, 0, 0, loc), loc)
	require.True(t, ok)
	require.Equal(t, "2", got.SingleTimePrice.String())
	got, _ = tariff.Resolve(vs, b, time.Date(2026, 3, 1, 0, 0, 0, 0, loc), loc)
	require.Equal(t, "3", got.SingleTimePrice.String())
}

func TestResolveTieBreaksOnLatestCreatedAt(t *testing.T) {
	loc := istanbul(t)
	b := uuid.New()
	vs := []model.Tariff{version(&b, 2026, 1, 1, time.Unix(200, 0), "2"), version(&b, 2026, 1, 1, time.Unix(100, 0), "1")}
	got, ok := tariff.Resolve(vs, b, time.Date(2026, 1, 15, 0, 0, 0, 0, loc), loc)
	require.True(t, ok)
	require.Equal(t, "2", got.SingleTimePrice.String())
}

func TestResolveUsesIstanbulDate(t *testing.T) {
	loc := istanbul(t)
	b := uuid.New()
	vs := []model.Tariff{version(&b, 2026, 2, 1, time.Unix(0, 0), "1"), version(&b, 2026, 3, 1, time.Unix(0, 0), "3")}
	got, ok := tariff.Resolve(vs, b, time.Date(2026, 2, 28, 21, 30, 0, 0, time.UTC), loc)
	require.True(t, ok)
	require.Equal(t, "3", got.SingleTimePrice.String(), "21:30Z is 00:30 on 03-01 in Istanbul")
}

func TestResolveNoTariffReturnsFalse(t *testing.T) {
	loc := istanbul(t)
	b := uuid.New()
	other := uuid.New()
	deleted := version(&b, 2025, 1, 1, time.Unix(0, 0), "1")
	now := time.Now()
	deleted.DeletedAt = &now
	vs := []model.Tariff{version(&b, 2027, 1, 1, time.Unix(0, 0), "1"), version(&other, 2025, 1, 1, time.Unix(0, 0), "1"), deleted}
	_, ok := tariff.Resolve(vs, b, time.Date(2026, 1, 1, 0, 0, 0, 0, loc), loc)
	require.False(t, ok)
}

func TestResolveBuildingRowBeatsNewerCompanyWideRow(t *testing.T) {
	loc := istanbul(t)
	b := uuid.New()
	vs := []model.Tariff{version(nil, 2026, 2, 1, time.Unix(0, 0), "9"), version(&b, 2026, 1, 1, time.Unix(0, 0), "1")}
	got, ok := tariff.Resolve(vs, b, time.Date(2026, 2, 15, 0, 0, 0, 0, loc), loc)
	require.True(t, ok)
	require.Equal(t, "1", got.SingleTimePrice.String())
	got, ok = tariff.Resolve(vs, uuid.New(), time.Date(2026, 2, 15, 0, 0, 0, 0, loc), loc)
	require.True(t, ok)
	require.Equal(t, "9", got.SingleTimePrice.String(), "company-wide row when the building has none")
}

func TestResolveParamsPicksByDate(t *testing.T) {
	loc := istanbul(t)
	a, b := seedParams(), seedParams()
	b.EffectiveFrom = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	b.DemandOverrunMultiplier = d("3")
	got, ok := tariff.ResolveParams([]model.BillingParameters{b, a}, time.Date(2025, 12, 31, 21, 0, 0, 0, time.UTC), loc)
	require.True(t, ok)
	require.Equal(t, "3", got.DemandOverrunMultiplier.String())
	_, ok = tariff.ResolveParams(nil, time.Now(), loc)
	require.False(t, ok)
}
