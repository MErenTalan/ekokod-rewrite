package energy_test

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
)

// TestGolden is 09-implementation-plan.md §F3 acceptance criterion 1: a
// golden-file test for each of the six consumption-derivation cases named
// there (R80). Every file under testdata/golden holds `input` (raw
// readings and resets — never pre-selected boundaries) and a `want` block
// computed by hand from the spec, not by running the code under test.
// -update rewrites only `want`; it never touches `input`, and it never
// produces the first `want` for a new fixture.
var update = flag.Bool("update", false, "rewrite the want block of each golden file")

// goldenWindow is the JSON shape of a Window: a half-open [From, To).
type goldenWindow struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

// goldenReading is the JSON shape of one raw meter reading or reset row.
// Values only carries the registers the meter actually reported (an absent
// key means "not reported", exactly like energy.Reading).
type goldenReading struct {
	TS          time.Time                            `json:"ts"`
	Kind        energy.Kind                          `json:"kind"`
	Values      map[energy.Register]*decimal.Decimal `json:"values,omitempty"`
	MaxDemandKw *decimal.Decimal                     `json:"max_demand_kw,omitempty"`
}

func (r goldenReading) toEnergy() energy.Reading {
	return energy.Reading{TS: r.TS, Kind: r.Kind, Values: r.Values, MaxDemandKw: r.MaxDemandKw}
}

// goldenInput is the raw fixture input: a window, the boundary-and-priors
// pool (`readings`), and the reset evidence (`resets`), fetched and passed
// to Derive exactly as Billing.Consumption does.
type goldenInput struct {
	Window   goldenWindow    `json:"window"`
	Readings []goldenReading `json:"readings"`
	Resets   []goldenReading `json:"resets"`
}

// goldenSuspicion is the JSON shape of an energy.Suspicion. energy.Suspicion
// itself carries no json tags (internal/domain/energy does no I/O), so this
// package defines its own lowercase-field mirror for the golden files.
type goldenSuspicion struct {
	Reason    energy.Reason    `json:"reason"`
	Delta     *decimal.Decimal `json:"delta,omitempty"`
	ResetRows int              `json:"reset_rows,omitempty"`
}

// goldenValues is a Derivation.Values map with a custom JSON encoding that
// always lists every register in energy.AllRegisters() order — whether the
// file is hand-authored or rewritten by -update — rather than
// encoding/json's default alphabetical map-key order. A nil map marshals to
// JSON null, matching Derivation's documented !Emitted invariant (Values is
// nil, never a non-nil empty map).
type goldenValues map[energy.Register]*decimal.Decimal

// MarshalJSON emits the registers in canonical order. encoding/json
// compacts whatever bytes a Marshaler returns and (for MarshalIndent)
// re-indents the whole document as one final pass, so this only needs to
// get key ORDER right — not whitespace, which is reflowed regardless.
func (v goldenValues) MarshalJSON() ([]byte, error) {
	if v == nil {
		return []byte("null"), nil
	}
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, reg := range energy.AllRegisters() {
		if i > 0 {
			buf.WriteByte(',')
		}
		keyBytes, err := json.Marshal(string(reg))
		if err != nil {
			return nil, err
		}
		buf.Write(keyBytes)
		buf.WriteByte(':')
		valBytes, err := json.Marshal(v[reg])
		if err != nil {
			return nil, err
		}
		buf.Write(valBytes)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// goldenWant is the JSON shape of runCase's result: a Derivation reduced to
// its comparable fields, plus the two ratios and max demand (R80, Task 4).
type goldenWant struct {
	Emitted         bool                                `json:"emitted"`
	Values          goldenValues                        `json:"values"`
	Suspect         map[energy.Register]goldenSuspicion `json:"suspect"`
	InductiveRatio  *decimal.Decimal                    `json:"inductive_ratio"`
	CapacitiveRatio *decimal.Decimal                    `json:"capacitive_ratio"`
	MaxDemandKw     *decimal.Decimal                    `json:"max_demand_kw"`
}

// goldenCase is one golden file. Input is kept as raw JSON (not decoded
// into goldenInput at the struct level) so that rewriting the file with
// -update round-trips its bytes unchanged: json.RawMessage.MarshalJSON
// returns exactly the bytes it was unmarshalled from, so the harness never
// re-derives or reformats `input` from Go values — it only ever replaces
// the Want field before writing the file back out (R80: "-update rewrites
// only want").
type goldenCase struct {
	Name  string          `json:"name"`
	Doc   string          `json:"doc"`
	Input json.RawMessage `json:"input"`
	Want  goldenWant      `json:"want"`
}

func TestGolden(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("testdata", "golden", "*.json"))
	require.NoError(t, err)
	require.NotEmpty(t, paths, "no golden fixtures found")

	for _, path := range paths {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			raw, err := os.ReadFile(path)
			require.NoError(t, err)

			var c goldenCase
			require.NoError(t, json.Unmarshal(raw, &c))

			var in goldenInput
			require.NoError(t, json.Unmarshal(c.Input, &in))

			got := runCase(t, in)

			if *update {
				c.Want = got
				writeGoldenFile(t, path, c)
				return
			}

			requireWantEqual(t, c.Name, c.Doc, c.Want, got)
		})
	}
}

// runCase is the ONE place selection meets derivation, mirroring
// Billing.Consumption's query shape: select boundaries from the same pool
// that also serves as Derive's priors. The harness sorts nothing —
// requireSortedAscending fails the test outright on an unsorted fixture,
// since Derive and SelectBoundary both document unsorted input as an
// unspecified-result precondition violation, not something to silently fix
// up.
func runCase(t *testing.T, in goldenInput) goldenWant {
	t.Helper()

	readings := toEnergyReadings(in.Readings)
	resets := toEnergyReadings(in.Resets)
	requireSortedAscending(t, readings, "input.readings")
	requireSortedAscending(t, resets, "input.resets")

	window := energy.Window{From: in.Window.From, To: in.Window.To}
	start := energy.SelectBoundary(readings, window.From)
	end := energy.SelectBoundary(readings, window.To)

	d := energy.Derive(window, start, end, resets, readings)
	inductive, capacitive := energy.Ratios(d)

	return goldenWant{
		Emitted:         d.Emitted,
		Values:          valuesInAllRegistersOrder(d.Values),
		Suspect:         toGoldenSuspect(d.Suspect),
		InductiveRatio:  inductive,
		CapacitiveRatio: capacitive,
		MaxDemandKw:     energy.MaxDemand(window, readings),
	}
}

func toEnergyReadings(in []goldenReading) []energy.Reading {
	out := make([]energy.Reading, len(in))
	for i, r := range in {
		out[i] = r.toEnergy()
	}
	return out
}

// requireSortedAscending fails the test when readings is not sorted
// ascending by TS — Derive and SelectBoundary both document this as a
// caller precondition they do not enforce themselves, so a fixture that
// violates it must fail the harness rather than silently produce an
// unspecified result.
func requireSortedAscending(t *testing.T, readings []energy.Reading, label string) {
	t.Helper()
	for i := 1; i < len(readings); i++ {
		require.Falsef(t, readings[i].TS.Before(readings[i-1].TS),
			"%s is not sorted ascending by ts: index %d (%s) precedes index %d (%s)",
			label, i, readings[i].TS, i-1, readings[i-1].TS)
	}
}

// valuesInAllRegistersOrder defensively rebuilds a Values map with every
// register in energy.AllRegisters() explicit — Derive and Difference
// already guarantee this shape when Emitted, but the harness
// does not trust that invariant blindly. A nil map (the !Emitted case)
// stays nil, matching Derivation's documented invariant.
func valuesInAllRegistersOrder(values map[energy.Register]*decimal.Decimal) goldenValues {
	if values == nil {
		return nil
	}
	out := make(goldenValues, len(energy.AllRegisters()))
	for _, reg := range energy.AllRegisters() {
		out[reg] = values[reg]
	}
	return out
}

func toGoldenSuspect(susp map[energy.Register]energy.Suspicion) map[energy.Register]goldenSuspicion {
	if susp == nil {
		return nil
	}
	out := make(map[energy.Register]goldenSuspicion, len(susp))
	for reg, s := range susp {
		out[reg] = goldenSuspicion{Reason: s.Reason, Delta: s.Delta, ResetRows: s.ResetRows}
	}
	return out
}

// requireWantEqual compares want and got field by field rather than with a
// single require.Equal(t, want, got): decimal.Decimal's internal
// coefficient/exponent representation depends on how a value was produced
// (e.g. a Sub of two 4-decimal-place operands prints "12.2500", not the
// hand-authored "12.25"), so a struct-level DeepEqual would fail on
// cosmetic scale differences alone. Comparing through decimal.Equal (value
// equality) still fails on every genuine numeric mismatch — including the
// mutations Step 5 proves — while tolerating a fixture written with fewer
// trailing zeros than the internal representation happens to carry.
func requireWantEqual(t *testing.T, name, doc string, want, got goldenWant) {
	t.Helper()
	msg := fmt.Sprintf("%s: %s", name, doc)

	require.Equal(t, want.Emitted, got.Emitted, "%s: emitted", msg)
	requireValuesEqual(t, want.Values, got.Values, msg)
	requireSuspectEqual(t, want.Suspect, got.Suspect, msg)
	requireDecimalEqual(t, want.InductiveRatio, got.InductiveRatio, msg+": inductive_ratio")
	requireDecimalEqual(t, want.CapacitiveRatio, got.CapacitiveRatio, msg+": capacitive_ratio")
	requireDecimalEqual(t, want.MaxDemandKw, got.MaxDemandKw, msg+": max_demand_kw")
}

func requireValuesEqual(t *testing.T, want, got goldenValues, msg string) {
	t.Helper()
	if want == nil || got == nil {
		require.Equalf(t, want == nil, got == nil, "%s: values nil-ness (want nil=%v, got nil=%v)", msg, want == nil, got == nil)
		return
	}
	for _, reg := range energy.AllRegisters() {
		requireDecimalEqual(t, want[reg], got[reg], fmt.Sprintf("%s: values.%s", msg, reg))
	}
}

func requireSuspectEqual(t *testing.T, want, got map[energy.Register]goldenSuspicion, msg string) {
	t.Helper()
	if want == nil || got == nil {
		require.Equalf(t, want == nil, got == nil, "%s: suspect nil-ness (want nil=%v, got nil=%v)", msg, want == nil, got == nil)
		return
	}
	require.Equalf(t, len(want), len(got), "%s: suspect key count (want %v, got %v)", msg, suspectKeys(want), suspectKeys(got))
	for reg, w := range want {
		g, ok := got[reg]
		require.Truef(t, ok, "%s: suspect missing entry for %s", msg, reg)
		require.Equalf(t, w.Reason, g.Reason, "%s: suspect.%s.reason", msg, reg)
		require.Equalf(t, w.ResetRows, g.ResetRows, "%s: suspect.%s.reset_rows", msg, reg)
		requireDecimalEqual(t, w.Delta, g.Delta, fmt.Sprintf("%s: suspect.%s.delta", msg, reg))
	}
}

func suspectKeys(m map[energy.Register]goldenSuspicion) []energy.Register {
	out := make([]energy.Register, 0, len(m))
	for reg := range m {
		out = append(out, reg)
	}
	return out
}

func requireDecimalEqual(t *testing.T, want, got *decimal.Decimal, msg string) {
	t.Helper()
	if want == nil || got == nil {
		require.Equalf(t, want == nil, got == nil, "%s (want nil=%v, got nil=%v)", msg, want == nil, got == nil)
		return
	}
	require.Truef(t, want.Equal(*got), "%s: want %s, got %s", msg, want.String(), got.String())
}

// writeGoldenFile rewrites path with c's current Name/Doc/Input/Want,
// two-space indented, matching the indentation the fixtures are
// hand-authored in so that an -update run that changes nothing produces a
// byte-identical file. Input is a json.RawMessage: MarshalJSON returns its
// original bytes verbatim, so this never rewrites `input` — only `want`
// changes, exactly as R80 requires.
func writeGoldenFile(t *testing.T, path string, c goldenCase) {
	t.Helper()
	buf, err := json.MarshalIndent(c, "", "  ")
	require.NoError(t, err)
	buf = append(buf, '\n')
	require.NoError(t, os.WriteFile(path, buf, 0o644))
}
