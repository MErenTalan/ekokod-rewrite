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

// TestBalanceNeverRendersASuspectRegisterAsANumber proves a hand-built Row
// whose Values entry is non-nil for a suspect register — something the
// real pipeline (Difference/Derive) never produces, but nothing about
// Row's shape prevents a caller from building — must still come back nil
// from Balance, never the raw 999.
func TestBalanceNeverRendersASuspectRegisterAsANumber(t *testing.T) {
	w := energy.Window{From: summaryT0, To: summaryT0.Add(time.Hour)}
	rows := []consumption.Row{
		{
			Window: w,
			Values: map[energy.Register]*decimal.Decimal{
				energy.ActiveImport: dec("999"),
				energy.ActiveExport: dec("999"),
			},
			Suspect: map[energy.Register]energy.Suspicion{
				energy.ActiveImport: {Reason: energy.ReasonNegativeDelta},
				energy.ActiveExport: {Reason: energy.ReasonNegativeDelta},
			},
		},
	}
	b := consumption.Balance(rows)
	require.Nil(t, b[0].Consumption, "active_import is suspect: must never render as 999")
	require.Nil(t, b[0].GridImport, "grid_import mirrors consumption")
	require.Nil(t, b[0].Generation, "active_export is suspect: must never render as 999")
	require.Nil(t, b[0].GridExport, "grid_export mirrors generation")
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
