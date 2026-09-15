package gridbox

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/normalize"
)

// errNoTimestamp is returned by resolveTimestamp when a row carries none of
// the four timestamp candidates. It is wrapped with the row's index by the
// caller into a WarnUnparseableRow warning, never surfaced as a hard error
// (06 §1 "a row that fails to parse is skipped").
var errNoTimestamp = errors.New("gridbox: row has no ProfileDateTime/ProfileDate/EndexDate/ReadDate")

// resolveTimestamp tries r's four timestamp candidates in the order 06 §3's
// mapping table lists them and parses the first one present via
// normalize.ISO8601 (offset-less = Europe/Istanbul, removed-behaviour 20).
// field names which candidate was used or attempted, so the caller can name
// it in a WarnUnparseableRow's Detail.
func resolveTimestamp(r register) (ts time.Time, field string, err error) {
	switch {
	case r.ProfileDateTime != nil:
		field = "ProfileDateTime"
		ts, err = normalize.ISO8601(*r.ProfileDateTime)
	case r.ProfileDate != nil:
		field = "ProfileDate"
		ts, err = normalize.ISO8601(*r.ProfileDate)
	case r.EndexDate != nil:
		field = "EndexDate"
		ts, err = normalize.ISO8601(*r.EndexDate)
	case r.ReadDate != nil:
		field = "ReadDate"
		ts, err = normalize.ISO8601(*r.ReadDate)
	default:
		return time.Time{}, "", errNoTimestamp
	}
	if err != nil {
		return time.Time{}, field, err
	}
	return ts, field, nil
}

// selectRegister resolves one canonical register's value from the provider
// field candidates that all carry the same underlying quantity (06 §3
// "Field mapping"). withMult, if non-nil, is used AS-IS — it is already
// multiplied — never multiplied again. Otherwise the first non-nil entry of
// raw (the legacy mapper's alternate spellings for the same raw quantity,
// e.g. RI and ReactiveInductiveEndex) is parsed and multiplied by m exactly
// once (02 §2.2: the multiplier is applied exactly once, in the adapter).
// Absent everywhere: nil, never zero (removed-behaviour 21).
func selectRegister(withMult *json.Number, m decimal.Decimal, raw ...*json.Number) (*decimal.Decimal, error) {
	if withMult != nil {
		v, err := normalize.JSONNumber(*withMult)
		if err != nil {
			return nil, err
		}
		return &v, nil
	}
	for _, r := range raw {
		if r == nil {
			continue
		}
		v, err := normalize.JSONNumber(*r)
		if err != nil {
			return nil, err
		}
		return normalize.Multiply(&v, m), nil
	}
	return nil, nil
}

// mapRegisters is 06 §3's field-mapping table, verbatim, in one function:
// RI/ReactiveInductiveEndex → reactive_inductive_import, RC/
// ReactiveCapacitiveEndex → reactive_capacitive_import — never crossed
// (removed-behaviour 19) — and every "…WithMultiplier" field is used as-is
// while its raw counterpart is multiplied by m. It does not set
// AnalyzerID, Ts, Kind, MultiplierApplied, SourceProvider or IngestedAt;
// the caller stamps those.
func mapRegisters(r register, m decimal.Decimal) (model.MeterReading, error) {
	var reading model.MeterReading
	var err error

	fields := []struct {
		name string
		dst  **decimal.Decimal
		with *json.Number
		raw  []*json.Number
	}{
		{"ActiveEndex", &reading.ActiveImport, r.ActiveEndexWithMultiplier, []*json.Number{r.ActiveEndex}},
		{"ActiveEndexOut", &reading.ActiveExport, r.ActiveEndexOutWithMultiplier, []*json.Number{r.ActiveEndexOut}},
		{"RI", &reading.ReactiveInductiveImport, nil, []*json.Number{r.RI, r.ReactiveInductiveEndex}},
		{"RC", &reading.ReactiveCapacitiveImport, nil, []*json.Number{r.RC, r.ReactiveCapacitiveEndex}},
		{"RIOut", &reading.ReactiveInductiveExport, nil, []*json.Number{r.RIOut, r.ReactiveInductiveEndexOut}},
		{"RCOut", &reading.ReactiveCapacitiveExport, nil, []*json.Number{r.RCOut, r.ReactiveCapacitiveEndexOut}},
		{"T1", &reading.T1Import, r.T1WithMultiplier, []*json.Number{r.T1, r.T1Endex}},
		{"T2", &reading.T2Import, r.T2WithMultiplier, []*json.Number{r.T2, r.T2Endex}},
		{"T3", &reading.T3Import, r.T3WithMultiplier, []*json.Number{r.T3, r.T3Endex}},
		{"T1Out", &reading.T1Export, nil, []*json.Number{r.T1Out}},
		{"T2Out", &reading.T2Export, nil, []*json.Number{r.T2Out}},
		{"T3Out", &reading.T3Export, nil, []*json.Number{r.T3Out}},
		{"MaxDemand", &reading.MaxDemandKw, r.MaxDemandWithMultiplier, []*json.Number{r.MaxDemand}},
	}
	for _, f := range fields {
		*f.dst, err = selectRegister(f.with, m, f.raw...)
		if err != nil {
			return model.MeterReading{}, fmt.Errorf("gridbox: %s: %w", f.name, err)
		}
	}

	if r.MeterSerialNumber != nil {
		reading.MeterSerial = r.MeterSerialNumber
	}
	return reading, nil
}

// ResolveMultiplier decides GridBox's meter multiplier per 06 §3
// "Multiplier resolution", in order:
//  1. lastEndex.Multiplier, when lastEndex is non-nil, it is present, and
//     it is strictly positive. A last_endex.Multiplier of exactly 0 (or
//     negative) is treated as unusable, not "the multiplier is zero" —
//     accepting it as-is would silently zero out every register it is
//     applied to, the same class of silent misprice 06 §3 calls out for
//     the fallback-to-1 case (adapter review pattern 12's "zero multiplier
//     … error/skip before use"). ProviderResolved: true.
//  2. Derived from the first row of profiles where both
//     ActiveEndexWithMultiplier and a non-zero ActiveEndex are present, as
//     ActiveEndexWithMultiplier / ActiveEndex (decimal division,
//     DivRound(…, 6)). A row with a zero or absent ActiveEndex, or an
//     absent ActiveEndexWithMultiplier, is skipped in favour of the next.
//     ProviderResolved: true.
//  3. stored (FetchRequest.Multiplier — R51/I2), when it is strictly
//     positive: the analyzer's own already-known multiplier, reused rather
//     than assumed-to-be-1 when this call's kind (daily, reset,
//     current_index, billing — none of which carries its own multiplier
//     source) has neither of the two provider sources above.
//     ProviderResolved: FALSE — this did not come from the provider THIS
//     call, so the pipeline must never treat it as new information to
//     persist (see ResolvedMultiplier's doc).
//  4. Fall back to 1, when stored is also zero/unset. ProviderResolved:
//     false. The caller (not this pure function) is responsible for
//     recording WarnMultiplierFallback — 06 §3 "silently assuming 1 can
//     misprice an entire account".
//
// Exported so it is tested directly, independent of any HTTP call.
func ResolveMultiplier(lastEndex *LastEndex, profiles []LoadProfileRow, stored decimal.Decimal) integration.ResolvedMultiplier {
	if lastEndex != nil && lastEndex.Multiplier != nil {
		if v, err := normalize.JSONNumber(*lastEndex.Multiplier); err == nil && v.IsPositive() {
			return integration.ResolvedMultiplier{Value: v, Source: integration.MultiplierFromLastEndex, ProviderResolved: true}
		}
	}

	for _, row := range profiles {
		if row.ActiveEndexWithMultiplier == nil || row.ActiveEndex == nil {
			continue
		}
		num, err := normalize.JSONNumber(*row.ActiveEndexWithMultiplier)
		if err != nil {
			continue
		}
		den, err := normalize.JSONNumber(*row.ActiveEndex)
		if err != nil || den.IsZero() {
			continue
		}
		return integration.ResolvedMultiplier{Value: num.DivRound(den, 6), Source: integration.MultiplierFromLoadProfile, ProviderResolved: true}
	}

	if stored.IsPositive() {
		return integration.ResolvedMultiplier{Value: stored, Source: integration.MultiplierFromRequest, ProviderResolved: false}
	}

	return integration.ResolvedMultiplier{Value: decimal.NewFromInt(1), Source: integration.MultiplierFallbackOne, ProviderResolved: false}
}
