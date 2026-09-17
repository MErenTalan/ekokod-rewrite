package comparison_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/comparison"
)

func d(s string) *decimal.Decimal { v := decimal.RequireFromString(s); return &v }
func n(v int32) *int32            { return &v }

var factor = decimal.RequireFromString("0.469")

func str(p *decimal.Decimal) string {
	if p == nil {
		return "<nil>"
	}
	return p.String()
}

func TestCompareRanksAscendingWithTies(t *testing.T) {
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	peers := []comparison.Figures{
		{BuildingID: a, Monthly: d("100"), Daily: d("3")},
		{BuildingID: b, Monthly: d("100"), Daily: d("4")},
		{BuildingID: c, Monthly: d("300"), Daily: d("10")},
	}
	res := comparison.Compare(a, peers, &factor)
	require.True(t, res.Available)
	require.Equal(t, 3, res.Peers)
	require.Equal(t, comparison.Metric{Value: d("100"), Average: res.Monthly.Average, Rank: 1, Ranked: 3}, res.Monthly)
	require.Equal(t, "166.666667", str(res.Monthly.Average))
	require.Equal(t, 1, res.Daily.Rank)

	res = comparison.Compare(b, peers, &factor)
	require.Equal(t, 1, res.Monthly.Rank, "ties share the lower rank")
	require.Equal(t, 2, res.Daily.Rank)
	res = comparison.Compare(c, peers, &factor)
	require.Equal(t, 3, res.Monthly.Rank, "competition ranking: 1, 1, 3")
}

func TestCompareExcludesZeroDenominators(t *testing.T) {
	self, zero, missing, other := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	peers := []comparison.Figures{
		{BuildingID: self, Monthly: d("1000"), Personnel: n(10), AreaM2: d("500")},
		{BuildingID: zero, Monthly: d("2000"), Personnel: n(0), AreaM2: d("0")},
		{BuildingID: missing, Monthly: d("3000")},
		{BuildingID: other, Monthly: d("900"), Personnel: n(3), AreaM2: d("100")},
	}
	res := comparison.Compare(self, peers, &factor)
	require.Equal(t, "100", str(res.PerCapita.Value))
	require.Equal(t, "200", str(res.PerCapita.Average), "(100 + 300) / 2: zero and missing personnel are excluded")
	require.Equal(t, 1, res.PerCapita.Rank)
	require.Equal(t, 2, res.PerCapita.Ranked)
	require.Equal(t, "2", str(res.PerArea.Value))
	require.Equal(t, "5.5", str(res.PerArea.Average))
	require.Equal(t, 2, res.PerArea.Ranked)

	res = comparison.Compare(zero, peers, &factor)
	require.Nil(t, res.PerCapita.Value)
	require.Zero(t, res.PerCapita.Rank, "a building without a value is unranked")
	require.Equal(t, 2, res.PerCapita.Ranked)
}

func TestCompareSmallSector(t *testing.T) {
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	res := comparison.Compare(a, []comparison.Figures{
		{BuildingID: a, Monthly: d("100")}, {BuildingID: b, Monthly: d("200")}, {BuildingID: c},
	}, &factor)
	require.False(t, res.Available)
	require.Equal(t, comparison.ReasonSectorTooSmall, res.Reason)
	require.Nil(t, res.Monthly.Average, "nothing about the peers is disclosed")
	require.Nil(t, res.Monthly.Value)
	require.Equal(t, 3, res.Peers)
}

func TestCompareCO2(t *testing.T) {
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	peers := []comparison.Figures{{BuildingID: a, Monthly: d("1000")}, {BuildingID: b, Monthly: d("2000")}, {BuildingID: c, Monthly: d("3000")}}
	res := comparison.Compare(a, peers, &factor)
	require.Equal(t, "469", str(res.CO2.Value))
	require.Equal(t, "938", str(res.CO2.Average))
	require.Equal(t, 1, res.CO2.Rank)

	res = comparison.Compare(a, peers, nil)
	require.True(t, res.Available)
	require.Nil(t, res.CO2.Value, "no grid factor: CO2 is unknown, not zero")
	require.Nil(t, res.CO2.Average)
}

func TestCompareSelfMissingFromPeers(t *testing.T) {
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	res := comparison.Compare(uuid.New(), []comparison.Figures{{BuildingID: a, Monthly: d("1")}, {BuildingID: b, Monthly: d("2")}, {BuildingID: c, Monthly: d("3")}}, &factor)
	require.True(t, res.Available)
	require.Nil(t, res.Monthly.Value)
	require.Zero(t, res.Monthly.Rank)
}
