package analysis

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
)

// R511: the reactive screen divides by the net the invoice divides by.
func TestReactiveNetFollowsTheTariff(t *testing.T) {
	t.Parallel()
	active, overlap := decimal.RequireFromString("1847.26"), decimal.RequireFromString("690.24")

	require.Equal(t, "1847.26", reactiveNet(active, &overlap, model.GenerationUsageNone).String())
	require.Equal(t, "1847.26", reactiveNet(active, &overlap, model.GenerationUsageSubtractFromTotal).String())
	require.Equal(t, "1157.02", reactiveNet(active, &overlap, model.GenerationUsageSubtractFromConsumption).String(), "R531: minus the hourly overlap")
	require.Equal(t, "1847.26", reactiveNet(active, nil, model.GenerationUsageSubtractFromConsumption).String(), "no hourly data: the import")

	more := decimal.RequireFromString("2000")
	require.Equal(t, "0", reactiveNet(active, &more, model.GenerationUsageSubtractFromConsumption).String(), "never negative")
}
