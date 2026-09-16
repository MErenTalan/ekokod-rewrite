package loadprofile

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestDefaultWeekendDaysCopyIsNotAliased proves the mutation R69's ruling
// forbids: DefaultWeekendDays must not be mutated by a caller that receives
// a Config built from it. defaultWeekendDaysCopy is the ONLY place a domain
// Config's WeekendDays map is populated from the default, so pinning IT
// directly is what actually protects the package variable — mutating the
// derived []time.Weekday on a Result.Config would prove nothing, since a
// slice of weekdays cannot alias a map.
func TestDefaultWeekendDaysCopyIsNotAliased(t *testing.T) {
	original := map[time.Weekday]bool{time.Saturday: true, time.Sunday: true}
	require.Equal(t, original, DefaultWeekendDays, "precondition: the documented default is Saturday+Sunday")

	cp := defaultWeekendDaysCopy()
	cp[time.Monday] = true
	delete(cp, time.Saturday)

	require.Equal(t, original, DefaultWeekendDays, "mutating a copy must never change DefaultWeekendDays")
}

// TestAllProfileKeysAreExactlyTheTenProfileKeys pins the set Request.Keys is
// validated against: the two bare day types plus the eight season/day-type
// combinations, and nothing else.
func TestAllProfileKeysAreExactlyTheTenProfileKeys(t *testing.T) {
	want := map[string]bool{
		"weekday": true, "weekend": true,
		"winter_weekday": true, "winter_weekend": true,
		"spring_weekday": true, "spring_weekend": true,
		"summer_weekday": true, "summer_weekend": true,
		"autumn_weekday": true, "autumn_weekend": true,
	}
	require.Len(t, allProfileKeys, len(want))
	require.Len(t, validProfileKeys, len(want))
	for _, k := range allProfileKeys {
		require.True(t, want[string(k)], "unexpected profile key %q", k)
		require.True(t, validProfileKeys[k])
	}
}
