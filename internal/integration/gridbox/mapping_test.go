package gridbox

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/normalize"
)

// jn builds a *json.Number from a literal digit string, exactly as
// encoding/json's UseNumber() decoder would have produced it.
func jn(s string) *json.Number {
	n := json.Number(s)
	return &n
}

// requireReg requires *d to be non-nil and equal, by decimal.Equal (never
// by string or float comparison), to the decimal parsed from want.
func requireReg(t *testing.T, want string, d *decimal.Decimal) {
	t.Helper()
	require.NotNil(t, d, "expected %s, got nil (removed-behaviour 21: absent register)", want)
	require.True(t, decimal.RequireFromString(want).Equal(*d), "got %s, want %s", d.String(), want)
}

func requireNilReg(t *testing.T, d *decimal.Decimal) {
	t.Helper()
	require.Nil(t, d)
}

// TestGridBoxReactiveRegistersAreNotCrossed is the F2 acceptance criterion
// and removed-behaviour 19: RI/ReactiveInductiveEndex must map to
// reactive_inductive_import and RC/ReactiveCapacitiveEndex to
// reactive_capacitive_import, in both the short-name and long-name
// spellings GridBox uses — the legacy mapper crossed exactly one of the
// two paths.
func TestGridBoxReactiveRegistersAreNotCrossed(t *testing.T) {
	one := decimal.NewFromInt(1)

	t.Run("short_names", func(t *testing.T) {
		r := register{RI: jn("111"), RC: jn("222"), RIOut: jn("333"), RCOut: jn("444")}
		reading, err := mapRegisters(r, one)
		require.NoError(t, err)
		requireReg(t, "111", reading.ReactiveInductiveImport)
		requireReg(t, "222", reading.ReactiveCapacitiveImport)
		requireReg(t, "333", reading.ReactiveInductiveExport)
		requireReg(t, "444", reading.ReactiveCapacitiveExport)
	})

	t.Run("long_names", func(t *testing.T) {
		r := register{
			ReactiveInductiveEndex:     jn("111"),
			ReactiveCapacitiveEndex:    jn("222"),
			ReactiveInductiveEndexOut:  jn("333"),
			ReactiveCapacitiveEndexOut: jn("444"),
		}
		reading, err := mapRegisters(r, one)
		require.NoError(t, err)
		requireReg(t, "111", reading.ReactiveInductiveImport)
		requireReg(t, "222", reading.ReactiveCapacitiveImport)
		requireReg(t, "333", reading.ReactiveInductiveExport)
		requireReg(t, "444", reading.ReactiveCapacitiveExport)
	})
}

// TestGridBoxDiscoverMapsEveryField adapts the shared adapter template's
// "Test<P>DiscoverMapsEveryField" (every row of the 06 mapping table
// asserted by name) to GridBox: 06 §3's Endpoints table has no
// discovery/listing endpoint for GridBox (see source.go's
// DiscoverMeteringPoints doc), so there is no MeteringPoint discovery
// mapping table to assert here. 06 §3's ONE mapping table — the
// field-mapping table mapRegisters implements — is what this test asserts,
// row by row, by canonical register name.
func TestGridBoxDiscoverMapsEveryField(t *testing.T) {
	m := decimal.NewFromInt(10)

	t.Run("raw_values_are_multiplied", func(t *testing.T) {
		r := register{
			ActiveEndex:    jn("100"),
			ActiveEndexOut: jn("90"),
			RI:             jn("11"),
			RC:             jn("12"),
			RIOut:          jn("13"),
			RCOut:          jn("14"),
			T1:             jn("21"),
			T2:             jn("22"),
			T3:             jn("23"),
			T1Out:          jn("31"),
			T2Out:          jn("32"),
			T3Out:          jn("33"),
			MaxDemand:      jn("40"),
		}
		reading, err := mapRegisters(r, m)
		require.NoError(t, err)
		requireReg(t, "1000", reading.ActiveImport)
		requireReg(t, "900", reading.ActiveExport)
		requireReg(t, "110", reading.ReactiveInductiveImport)
		requireReg(t, "120", reading.ReactiveCapacitiveImport)
		requireReg(t, "130", reading.ReactiveInductiveExport)
		requireReg(t, "140", reading.ReactiveCapacitiveExport)
		requireReg(t, "210", reading.T1Import)
		requireReg(t, "220", reading.T2Import)
		requireReg(t, "230", reading.T3Import)
		requireReg(t, "310", reading.T1Export)
		requireReg(t, "320", reading.T2Export)
		requireReg(t, "330", reading.T3Export)
		requireReg(t, "400", reading.MaxDemandKw)
	})

	// Adapter review pattern 2: a NON-integer multiplier ("2.5") proves the
	// multiplication is exact decimal arithmetic, not an integer-only path
	// that would happen to look right with m=10.
	t.Run("raw_values_are_multiplied_by_a_decimal_multiplier", func(t *testing.T) {
		m25 := decimal.RequireFromString("2.5")
		r := register{
			ActiveEndex:    jn("100"),
			ActiveEndexOut: jn("90"),
			RI:             jn("11"),
			RC:             jn("12"),
			RIOut:          jn("13"),
			RCOut:          jn("14"),
			T1:             jn("21"),
			T2:             jn("22"),
			T3:             jn("23"),
			T1Out:          jn("31"),
			T2Out:          jn("32"),
			T3Out:          jn("33"),
			MaxDemand:      jn("40"),
		}
		reading, err := mapRegisters(r, m25)
		require.NoError(t, err)
		requireReg(t, "250", reading.ActiveImport)
		requireReg(t, "225", reading.ActiveExport)
		requireReg(t, "27.5", reading.ReactiveInductiveImport)
		requireReg(t, "30", reading.ReactiveCapacitiveImport)
		requireReg(t, "32.5", reading.ReactiveInductiveExport)
		requireReg(t, "35", reading.ReactiveCapacitiveExport)
		requireReg(t, "52.5", reading.T1Import)
		requireReg(t, "55", reading.T2Import)
		requireReg(t, "57.5", reading.T3Import)
		requireReg(t, "77.5", reading.T1Export)
		requireReg(t, "80", reading.T2Export)
		requireReg(t, "82.5", reading.T3Export)
		requireReg(t, "100", reading.MaxDemandKw)
	})

	t.Run("with_multiplier_fields_used_as_is", func(t *testing.T) {
		r := register{
			ActiveEndexWithMultiplier:    jn("1000"),
			ActiveEndexOutWithMultiplier: jn("900"),
			T1WithMultiplier:             jn("210"),
			T2WithMultiplier:             jn("220"),
			T3WithMultiplier:             jn("230"),
			MaxDemandWithMultiplier:      jn("400"),
			// Their raw counterparts are also present, with DIFFERENT
			// values, to prove the WithMultiplier field wins and is never
			// itself multiplied again.
			ActiveEndex:    jn("999"),
			ActiveEndexOut: jn("999"),
			T1:             jn("999"),
			T2:             jn("999"),
			T3:             jn("999"),
			MaxDemand:      jn("999"),
		}
		reading, err := mapRegisters(r, m)
		require.NoError(t, err)
		requireReg(t, "1000", reading.ActiveImport)
		requireReg(t, "900", reading.ActiveExport)
		requireReg(t, "210", reading.T1Import)
		requireReg(t, "220", reading.T2Import)
		requireReg(t, "230", reading.T3Import)
		requireReg(t, "400", reading.MaxDemandKw)
	})

	t.Run("t1_t2_t3_endex_long_names", func(t *testing.T) {
		one := decimal.NewFromInt(1)
		r := register{T1Endex: jn("1"), T2Endex: jn("2"), T3Endex: jn("3")}
		reading, err := mapRegisters(r, one)
		require.NoError(t, err)
		requireReg(t, "1", reading.T1Import)
		requireReg(t, "2", reading.T2Import)
		requireReg(t, "3", reading.T3Import)
	})

	t.Run("meter_serial", func(t *testing.T) {
		serial := "SN-1"
		r := register{MeterSerialNumber: &serial}
		reading, err := mapRegisters(r, decimal.NewFromInt(1))
		require.NoError(t, err)
		require.NotNil(t, reading.MeterSerial)
		require.Equal(t, "SN-1", *reading.MeterSerial)
	})

	t.Run("absent_registers_are_nil_never_zero", func(t *testing.T) {
		reading, err := mapRegisters(register{}, decimal.NewFromInt(1))
		require.NoError(t, err)
		requireNilReg(t, reading.ActiveImport)
		requireNilReg(t, reading.ActiveExport)
		requireNilReg(t, reading.ReactiveInductiveImport)
		requireNilReg(t, reading.ReactiveCapacitiveImport)
		requireNilReg(t, reading.ReactiveInductiveExport)
		requireNilReg(t, reading.ReactiveCapacitiveExport)
		requireNilReg(t, reading.T1Import)
		requireNilReg(t, reading.T2Import)
		requireNilReg(t, reading.T3Import)
		requireNilReg(t, reading.T1Export)
		requireNilReg(t, reading.T2Export)
		requireNilReg(t, reading.T3Export)
		requireNilReg(t, reading.MaxDemandKw)
		require.Nil(t, reading.MeterSerial)
	})

	t.Run("timestamp_field_precedence", func(t *testing.T) {
		profileDateTime := "2026-09-14T10:00:00Z"
		profileDate := "2026-09-13T10:00:00Z"
		endexDate := "2026-09-12T10:00:00Z"
		readDate := "2026-09-11T10:00:00Z"

		ts, field, err := resolveTimestamp(register{ProfileDateTime: &profileDateTime, ProfileDate: &profileDate, EndexDate: &endexDate, ReadDate: &readDate})
		require.NoError(t, err)
		require.Equal(t, "ProfileDateTime", field)
		require.Equal(t, "2026-09-14T10:00:00Z", ts.UTC().Format("2006-01-02T15:04:05Z"))

		ts, field, err = resolveTimestamp(register{ProfileDate: &profileDate, EndexDate: &endexDate, ReadDate: &readDate})
		require.NoError(t, err)
		require.Equal(t, "ProfileDate", field)
		require.Equal(t, "2026-09-13T10:00:00Z", ts.UTC().Format("2006-01-02T15:04:05Z"))

		ts, field, err = resolveTimestamp(register{EndexDate: &endexDate, ReadDate: &readDate})
		require.NoError(t, err)
		require.Equal(t, "EndexDate", field)
		require.Equal(t, "2026-09-12T10:00:00Z", ts.UTC().Format("2006-01-02T15:04:05Z"))

		ts, field, err = resolveTimestamp(register{ReadDate: &readDate})
		require.NoError(t, err)
		require.Equal(t, "ReadDate", field)
		require.Equal(t, "2026-09-11T10:00:00Z", ts.UTC().Format("2006-01-02T15:04:05Z"))

		_, _, err = resolveTimestamp(register{})
		require.ErrorIs(t, err, errNoTimestamp)
	})
}

// TestGridBoxResolveMultiplierPureCases covers ResolveMultiplier directly,
// independent of any HTTP call, for the two paths whose acceptance-test
// prose (task-7-brief.md) names no FetchResult ("res") — the HTTP-level
// TestGridBoxMultiplierResolution in source_test.go covers the "last_endex"
// and "fallback_to_one_warns" paths, which the brief explicitly ties to a
// FetchResult ("readings stamped 80", "res.Warnings").
func TestGridBoxResolveMultiplierPureCases(t *testing.T) {
	t.Run("derived_from_load_profile", func(t *testing.T) {
		profiles := []LoadProfileRow{
			{register: register{ActiveEndex: jn("12.5"), ActiveEndexWithMultiplier: jn("500")}},
		}
		res := ResolveMultiplier(nil, profiles)
		require.Equal(t, integration.MultiplierFromLoadProfile, res.Source)
		require.True(t, decimal.RequireFromString("40").Equal(res.Value), "got %s", res.Value)
	})

	t.Run("zero_denominator_skips_to_next_row", func(t *testing.T) {
		profiles := []LoadProfileRow{
			{register: register{ActiveEndex: jn("0"), ActiveEndexWithMultiplier: jn("999")}},
			{register: register{ActiveEndex: jn("10"), ActiveEndexWithMultiplier: jn("100")}},
		}
		res := ResolveMultiplier(nil, profiles)
		require.Equal(t, integration.MultiplierFromLoadProfile, res.Source)
		require.True(t, decimal.RequireFromString("10").Equal(res.Value), "got %s", res.Value)
	})

	t.Run("last_endex_multiplier_wins_over_load_profile", func(t *testing.T) {
		le := &LastEndex{Multiplier: jn("80")}
		profiles := []LoadProfileRow{
			{register: register{ActiveEndex: jn("12.5"), ActiveEndexWithMultiplier: jn("500")}},
		}
		res := ResolveMultiplier(le, profiles)
		require.Equal(t, integration.MultiplierFromLastEndex, res.Source)
		require.True(t, decimal.RequireFromString("80").Equal(res.Value))
	})

	t.Run("fallback_to_one", func(t *testing.T) {
		res := ResolveMultiplier(nil, nil)
		require.Equal(t, integration.MultiplierFallbackOne, res.Source)
		require.True(t, decimal.NewFromInt(1).Equal(res.Value))
	})

	// Adapter review pattern 12: a zero (or negative) last_endex.Multiplier
	// must not be accepted as-is — it would silently zero out every
	// register — so resolution falls through to priority 2/3 exactly as if
	// no Multiplier had been reported at all.
	t.Run("zero_last_endex_multiplier_is_rejected", func(t *testing.T) {
		le := &LastEndex{Multiplier: jn("0")}
		profiles := []LoadProfileRow{
			{register: register{ActiveEndex: jn("12.5"), ActiveEndexWithMultiplier: jn("500")}},
		}
		res := ResolveMultiplier(le, profiles)
		require.Equal(t, integration.MultiplierFromLoadProfile, res.Source, "a zero Multiplier must fall through, not be used")
		require.True(t, decimal.RequireFromString("40").Equal(res.Value))

		res = ResolveMultiplier(le, nil)
		require.Equal(t, integration.MultiplierFallbackOne, res.Source, "a zero Multiplier with nothing else available falls all the way back to 1")
	})

	t.Run("negative_last_endex_multiplier_is_rejected", func(t *testing.T) {
		le := &LastEndex{Multiplier: jn("-5")}
		res := ResolveMultiplier(le, nil)
		require.Equal(t, integration.MultiplierFallbackOne, res.Source)
	})
}

// TestGridBoxJSONNullAndZeroAreDistinguished decodes actual JSON bytes
// (never a Go struct literal) to prove removed-behaviour 21 end to end
// through encoding/json itself: a register whose value is absent, JSON
// `null`, or the sentinel strings normalize.OptionalNumber recognises maps
// to a nil pointer, while a genuine JSON `0` decodes to a present,
// non-nil, zero-valued register (adapter review pattern 3).
func TestGridBoxJSONNullAndZeroAreDistinguished(t *testing.T) {
	t.Run("absent_key", func(t *testing.T) {
		r, err := decodeRegisterRow(json.RawMessage(`{}`))
		require.NoError(t, err)
		require.Nil(t, r.ActiveEndex)
	})

	t.Run("explicit_json_null", func(t *testing.T) {
		r, err := decodeRegisterRow(json.RawMessage(`{"ActiveEndex":null}`))
		require.NoError(t, err)
		require.Nil(t, r.ActiveEndex)
	})

	t.Run("genuine_zero_stays_zero", func(t *testing.T) {
		r, err := decodeRegisterRow(json.RawMessage(`{"ActiveEndex":0}`))
		require.NoError(t, err)
		require.NotNil(t, r.ActiveEndex)
		reading, mapErr := mapRegisters(r, decimal.NewFromInt(5))
		require.NoError(t, mapErr)
		requireReg(t, "0", reading.ActiveImport)
	})
}

// TestGridBoxSplitWindow is Ruling R36's required unit test for the
// adapter-local window splitter that replaces normalize.Chunk for
// GridBox's multi-day MaxWindow: gapless, non-overlapping coverage of
// [from, to), every window <= max, and — where a whole local day fits —
// boundaries aligned to Europe/Istanbul local midnight.
func TestGridBoxSplitWindow(t *testing.T) {
	t.Run("thirty_day_max_over_a_45_day_range_makes_two_chunks", func(t *testing.T) {
		from := time.Date(2026, 1, 1, 0, 0, 0, 0, normalize.Istanbul).UTC()
		to := time.Date(2026, 2, 15, 0, 0, 0, 0, normalize.Istanbul).UTC()
		windows := splitWindow(from, to, 30*24*time.Hour)
		require.Len(t, windows, 2)
		require.True(t, windows[0].From.Equal(from))
		require.True(t, windows[0].To.Equal(time.Date(2026, 1, 31, 0, 0, 0, 0, normalize.Istanbul).UTC()))
		require.True(t, windows[1].From.Equal(windows[0].To), "contiguous: no gap or overlap")
		require.True(t, windows[1].To.Equal(to))
		for _, w := range windows {
			require.LessOrEqual(t, w.To.Sub(w.From), 30*24*time.Hour)
		}
	})

	t.Run("a_range_under_max_is_one_chunk", func(t *testing.T) {
		from := time.Date(2026, 9, 1, 0, 0, 0, 0, normalize.Istanbul).UTC()
		to := time.Date(2026, 9, 3, 0, 0, 0, 0, normalize.Istanbul).UTC()
		windows := splitWindow(from, to, 30*24*time.Hour)
		require.Len(t, windows, 1)
		require.True(t, windows[0].From.Equal(from))
		require.True(t, windows[0].To.Equal(to))
	})

	t.Run("empty_and_inverted_ranges_are_empty", func(t *testing.T) {
		now := time.Now().UTC()
		require.Empty(t, splitWindow(now, now, time.Hour))
		require.Empty(t, splitWindow(now, now.Add(-time.Hour), time.Hour))
		require.Empty(t, splitWindow(now, now.Add(time.Hour), 0))
	})

	t.Run("property_gapless_coverage_and_max_respected", func(t *testing.T) {
		base := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
		for i := 0; i < 50; i++ {
			from := base.Add(time.Duration(i) * 37 * time.Hour)
			to := from.Add(time.Duration(1+i%90) * 24 * time.Hour)
			max := time.Duration(1+i%40) * 24 * time.Hour

			windows := splitWindow(from, to, max)
			require.NotEmpty(t, windows)
			require.True(t, windows[0].From.Equal(from))
			require.True(t, windows[len(windows)-1].To.Equal(to))
			for j, w := range windows {
				require.True(t, w.From.Before(w.To))
				require.LessOrEqual(t, w.To.Sub(w.From), max)
				if j > 0 {
					require.True(t, w.From.Equal(windows[j-1].To))
				}
			}
		}
	})
}
