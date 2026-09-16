package energy_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
)

// TestDerivationSuspectRegistersOrder pins I-9: SuspectRegisters must return
// suspect registers in AllRegisters order, regardless of the order they
// were inserted into the Suspect map (map iteration order is random, so a
// mutation that ranges over d.Suspect directly instead of allRegisters
// would still often pass — inserting the registers in reverse-of-canonical
// order gives this test the best chance of catching it).
func TestDerivationSuspectRegistersOrder(t *testing.T) {
	d := energy.Derivation{
		Suspect: map[energy.Register]energy.Suspicion{
			// Inserted out of AllRegisters order (T1Export, then T1Import,
			// then ActiveImport — the reverse of their canonical order).
			energy.T1Export:     {Reason: energy.ReasonNegativeDelta},
			energy.T1Import:     {Reason: energy.ReasonMeterReset},
			energy.ActiveImport: {Reason: energy.ReasonMissingReadings},
		},
	}

	got := d.SuspectRegisters()
	require.Equal(t, []energy.Register{energy.ActiveImport, energy.T1Import, energy.T1Export}, got,
		"must follow AllRegisters order, not map insertion order")

	// Cross-check directly against AllRegisters' own order: got must be the
	// subsequence of AllRegisters restricted to the suspect set.
	all := energy.AllRegisters()
	var wantOrder []energy.Register
	for _, reg := range all {
		if _, ok := d.Suspect[reg]; ok {
			wantOrder = append(wantOrder, reg)
		}
	}
	require.Equal(t, wantOrder, got)
}

func TestDerivationSuspectRegistersEmptyWhenNoneSuspect(t *testing.T) {
	d := energy.Derivation{}
	require.Empty(t, d.SuspectRegisters())
}
