package energy_test

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
)

func TestRatioIsNilWhenConsumptionIsZero(t *testing.T) {
	require.Nil(t, energy.Ratio(dec("12.5"), dec("0")))
	require.Nil(t, energy.Ratio(dec("12.5"), nil))
	require.Nil(t, energy.Ratio(nil, dec("100")))
}

func TestRatioIsNilWhenDenominatorIsNegative(t *testing.T) {
	require.Nil(t, energy.Ratio(dec("12.5"), dec("-5")))
}

func TestRatioIsAnUnroundedFraction(t *testing.T) {
	requireDecimal(t, energy.Ratio(dec("4.045"), dec("12.25")), "0.33020408163265306122")
	requireDecimal(t, energy.Ratio(dec("1"), dec("3")), "0.33333333333333333333")
}

func TestRatiosOfANormalDerivation(t *testing.T) {
	d := energy.Derivation{
		Emitted: true,
		Values: map[energy.Register]*decimal.Decimal{
			energy.ActiveImport:             dec("12.25"),
			energy.ReactiveInductiveImport:  dec("4.045"),
			energy.ReactiveCapacitiveImport: dec("1"),
		},
	}
	inductive, capacitive := energy.Ratios(d)
	requireDecimal(t, inductive, "0.33020408163265306122")
	requireDecimal(t, capacitive, "0.0816326530612244898")
}

// TestRatiosWhenInductiveRegisterIsSuspectOnlyInductiveIsNil proves Ratios
// operates per-register: active_import is sound so the ratio call proceeds,
// but Difference's contract (Values[reg]=nil whenever Suspect[reg] is set)
// means a suspect reactive_inductive_import arrives as a nil numerator, so
// Ratio's nil-numerator rule makes the inductive ratio nil while the
// capacitive ratio — whose register is sound — is still computed normally.
func TestRatiosWhenInductiveRegisterIsSuspectOnlyInductiveIsNil(t *testing.T) {
	delta := dec("-1")
	d := energy.Derivation{
		Emitted: true,
		Values: map[energy.Register]*decimal.Decimal{
			energy.ActiveImport:             dec("12.25"),
			energy.ReactiveInductiveImport:  nil,
			energy.ReactiveCapacitiveImport: dec("1"),
		},
		Suspect: map[energy.Register]energy.Suspicion{
			energy.ReactiveInductiveImport: {Reason: energy.ReasonNegativeDelta, Delta: delta},
		},
	}
	inductive, capacitive := energy.Ratios(d)
	require.Nil(t, inductive)
	requireDecimal(t, capacitive, "0.0816326530612244898")
}

func TestRatiosOfADerivationWhoseActiveImportIsSuspectAreNil(t *testing.T) {
	delta := dec("-5")
	d := energy.Derivation{
		Emitted: true,
		Values: map[energy.Register]*decimal.Decimal{
			energy.ActiveImport:            nil,
			energy.ReactiveInductiveImport: dec("4.045"),
		},
		Suspect: map[energy.Register]energy.Suspicion{
			energy.ActiveImport: {Reason: energy.ReasonNegativeDelta, Delta: delta},
		},
	}
	inductive, capacitive := energy.Ratios(d)
	require.Nil(t, inductive)
	require.Nil(t, capacitive)
}

func TestRatiosOfANotEmittedDerivationAreNil(t *testing.T) {
	d := energy.Derivation{Emitted: false}
	inductive, capacitive := energy.Ratios(d)
	require.Nil(t, inductive)
	require.Nil(t, capacitive)
}

func TestRatiosOfZeroActiveConsumptionAreNil(t *testing.T) {
	d := energy.Derivation{
		Emitted: true,
		Values: map[energy.Register]*decimal.Decimal{
			energy.ActiveImport:             dec("0"),
			energy.ReactiveInductiveImport:  dec("4.045"),
			energy.ReactiveCapacitiveImport: dec("1"),
		},
	}
	inductive, capacitive := energy.Ratios(d)
	require.Nil(t, inductive)
	require.Nil(t, capacitive)
}
