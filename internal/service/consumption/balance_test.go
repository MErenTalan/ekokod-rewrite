package consumption_test

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/consumption"
)

func TestBalanceDerivesFromRegistersOnly(t *testing.T) {
	w := energy.Window{From: summaryT0, To: summaryT0.Add(time.Hour)}
	rows := []consumption.Row{
		{
			Window: w,
			Values: map[energy.Register]*decimal.Decimal{
				energy.ActiveImport: dec("12.5"),
				energy.ActiveExport: dec("3.2"),
			},
		},
	}
	b := consumption.Balance(rows)
	require.Len(t, b, 1)
	require.Equal(t, w, b[0].Window)
	require.Equal(t, "12.5", b[0].Consumption.String())
	require.Equal(t, "3.2", b[0].Generation.String())
	require.Equal(t, "12.5", b[0].GridImport.String(), "grid_import is identically consumption until F9")
	require.Equal(t, "3.2", b[0].GridExport.String(), "grid_export is identically generation until F9")
}

func TestBalanceIsNilWhenARegisterIsUnavailable(t *testing.T) {
	w := energy.Window{From: summaryT0, To: summaryT0.Add(time.Hour)}
	rows := []consumption.Row{
		{
			Window: w,
			Values: map[energy.Register]*decimal.Decimal{}, // both nil
		},
	}
	b := consumption.Balance(rows)
	require.Nil(t, b[0].Consumption)
	require.Nil(t, b[0].Generation)
	require.Nil(t, b[0].GridImport)
	require.Nil(t, b[0].GridExport)
}
