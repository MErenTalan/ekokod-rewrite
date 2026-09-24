package consumption

// This file is package consumption (internal, not consumption_test):
// toEnergyReading and toModelKind are unexported by design (F3
// exposes only Analytics, Billing and their Deps/Row/SeriesRequest types),
// so this is the only test file in the package allowed to call them
// directly. Every other *_test.go file in this package is external
// (package consumption_test) and exercises the package through
// NewAnalytics/NewBilling only.

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
)

// convertSetRegister sets one energy.Register field on a model.MeterReading
// by name. It exists only for TestConvertRoundTripsEveryRegisterAndNever
// ReappliesTheMultiplier, which needs to hit every one of the twelve
// registers generically via energy.AllRegisters() rather than one field at
// a time.
func convertSetRegister(mr *model.MeterReading, reg energy.Register, v *decimal.Decimal) {
	switch reg {
	case energy.ActiveImport:
		mr.ActiveImport = v
	case energy.ReactiveInductiveImport:
		mr.ReactiveInductiveImport = v
	case energy.ReactiveCapacitiveImport:
		mr.ReactiveCapacitiveImport = v
	case energy.T1Import:
		mr.T1Import = v
	case energy.T2Import:
		mr.T2Import = v
	case energy.T3Import:
		mr.T3Import = v
	case energy.ActiveExport:
		mr.ActiveExport = v
	case energy.ReactiveInductiveExport:
		mr.ReactiveInductiveExport = v
	case energy.ReactiveCapacitiveExport:
		mr.ReactiveCapacitiveExport = v
	case energy.T1Export:
		mr.T1Export = v
	case energy.T2Export:
		mr.T2Export = v
	case energy.T3Export:
		mr.T3Export = v
	default:
		panic("convertSetRegister: unknown register " + string(reg))
	}
}

// TestConvertRoundTripsEveryRegisterAndNeverReappliesTheMultiplier proves
// every energy.Register round-trips through
// toEnergyReading, and MultiplierApplied is read for audit only, never
// reapplied — 02 §2.2 says the multiplier is applied exactly once, at
// ingestion, so a stored reading's register values are ALREADY the true
// physical quantity regardless of what MultiplierApplied says. Setting it
// to 10 here and expecting "123.45" back (not "1234.5") is the whole test:
// if toEnergyReading multiplied by MultiplierApplied again, this would fail
// with "1234.5" instead.
func TestConvertRoundTripsEveryRegisterAndNeverReappliesTheMultiplier(t *testing.T) {
	ts := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	for _, reg := range energy.AllRegisters() {
		mr := model.MeterReading{
			AnalyzerID:        uuid.New(),
			Ts:                ts,
			Kind:              model.ReadingKindLoadProfile,
			MultiplierApplied: decimal.RequireFromString("10"),
		}
		convertSetRegister(&mr, reg, decPtr("123.4500"))

		er := toEnergyReading(mr)
		require.NotNil(t, er.Value(reg))
		require.Equal(t, "123.45", er.Value(reg).String(),
			"MultiplierApplied=10 leaves the value unchanged — it was applied once, at ingestion")
	}

	require.Equal(t, model.ReadingKindBilling, toModelKind(energy.KindBilling))
	require.Equal(t, model.ReadingKindLoadProfile, toModelKind(energy.KindLoadProfile))
	require.Equal(t, model.ReadingKindDaily, toModelKind(energy.KindDaily))
	require.Equal(t, model.ReadingKindReset, toModelKind(energy.KindReset))
	require.Equal(t, model.ReadingKindCurrentIndex, toModelKind(energy.KindCurrentIndex))
}

func decPtr(s string) *decimal.Decimal {
	d := decimal.RequireFromString(s)
	return &d
}
