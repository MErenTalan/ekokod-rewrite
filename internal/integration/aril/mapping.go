package aril

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/normalize"
)

// errorCode reports the numeric ErrorCode carried by n and whether n was
// present and parsed. A nil or unparseable ErrorCode reports ok=false —
// "no code reported", not a failure on its own: 06 §4 does not document
// what a genuinely absent ErrorCode key means, and treating absence as "no
// error" (rather than as a failure) matches removed-behaviour 21's own
// logic — applied here to the envelope's own signalling field instead of a
// register — never surfacing a spurious ErrUpstreamUnavailable for a
// payload that simply never reports ErrorCode at all.
func errorCode(n *json.Number) (code int64, ok bool) {
	if n == nil {
		return 0, false
	}
	v, err := n.Int64()
	if err != nil {
		return 0, false
	}
	return v, true
}

// parseProfileDate parses a 14-digit yyyyMMddHHmmss ARIL date (06 §4 "Load
// profile mapping": "ProfileDate as a 14-digit number") as Europe/Istanbul
// local time.
func parseProfileDate(n json.Number) (time.Time, error) {
	v, err := n.Int64()
	if err != nil {
		return time.Time{}, fmt.Errorf("aril: ProfileDate %q: %w", n.String(), err)
	}
	return normalize.ARILProfileDate(v)
}

// mapRegisters is 06 §4's "Load profile mapping" table, verbatim: TSum ->
// active_import, ReactiveInductive -> reactive_inductive_import,
// ReactiveCapasitive -> reactive_capacitive_import, TSumOut ->
// active_export, ReactiveInductiveOut -> reactive_inductive_export,
// ReactiveCapasitiveOut -> reactive_capacitive_export, each multiplied by m
// exactly once (02 §2.2). T1/T2/T3 import and export are never set — ARIL
// load profiles carry no time-of-use split (06 §4 warning box,
// removed-behaviour 21): they stay at model.MeterReading's zero value,
// which is nil, never decimal.Zero.
//
// It does not set AnalyzerID, Ts, Kind, MultiplierApplied, SourceProvider,
// IngestedAt, MeterSerial or Raw — the caller (stamp) fills those.
func mapRegisters(r registers, m decimal.Decimal) (model.MeterReading, error) {
	var reading model.MeterReading

	fields := []struct {
		name string
		dst  **decimal.Decimal
		raw  *string
	}{
		{"TSum", &reading.ActiveImport, r.TSum},
		{"ReactiveInductive", &reading.ReactiveInductiveImport, r.ReactiveInductive},
		{"ReactiveCapasitive", &reading.ReactiveCapacitiveImport, r.ReactiveCapasitive},
		{"TSumOut", &reading.ActiveExport, r.TSumOut},
		{"ReactiveInductiveOut", &reading.ReactiveInductiveExport, r.ReactiveInductiveOut},
		{"ReactiveCapasitiveOut", &reading.ReactiveCapacitiveExport, r.ReactiveCapasitiveOut},
	}
	for _, f := range fields {
		v, err := optionalRegister(f.raw)
		if err != nil {
			return model.MeterReading{}, fmt.Errorf("aril: %s: %w", f.name, err)
		}
		*f.dst = normalize.Multiply(v, m)
	}
	return reading, nil
}

// mapMaxDemand parses current_endexes'/end_of_month_endexes' MaxDemand
// field and multiplies it by m exactly once, same as every other register
// (adapter review pattern 2/removed-behaviour 21 both apply to it — 06 §4
// gives it no special-cased treatment).
func mapMaxDemand(raw *string, m decimal.Decimal) (*decimal.Decimal, error) {
	v, err := optionalRegister(raw)
	if err != nil {
		return nil, fmt.Errorf("aril: MaxDemand: %w", err)
	}
	return normalize.Multiply(v, m), nil
}

// optionalRegister parses raw via normalize.OptionalNumber, treating a nil
// pointer (JSON key absent, or an explicit `null`) the same as the sentinel
// strings OptionalNumber itself already recognises ("", "-"): absent, never
// zero (removed-behaviour 21).
func optionalRegister(raw *string) (*decimal.Decimal, error) {
	if raw == nil {
		return nil, nil
	}
	return normalize.OptionalNumber(*raw)
}

// mapSubscription decodes one analyzers_list row into an
// integration.MeteringPoint (06 §4 "Subscription mapping"). ok is false
// only when SubscriptionSerno — the identity field a MeteringPoint cannot
// be built without — is missing or unparseable; every other field is
// mapped best-effort (see subscriptionWire's doc: DiscoverMeteringPoints
// carries no per-row Warning channel).
func mapSubscription(raw json.RawMessage) (integration.MeteringPoint, bool) {
	var w subscriptionWire
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&w); err != nil {
		return integration.MeteringPoint{}, false
	}

	serno := strings.TrimSpace(w.SubscriptionSerno)
	if serno == "" {
		return integration.MeteringPoint{}, false
	}

	pt := integration.MeteringPoint{
		InstallationNumber: serno,
		CustomerName:       w.Title,
		Address:            w.Address,
		MeterNumber:        w.MeterSerial,
		MeterModel:         w.MeterBrand,
		EtsoCode:           w.Etso,
	}

	if w.Multiplier != nil {
		if v, jErr := normalize.JSONNumber(*w.Multiplier); jErr == nil {
			pt.MeterMultiplier = &v
		}
	}
	if w.InstalledPower != nil {
		if v, jErr := normalize.JSONNumber(*w.InstalledPower); jErr == nil {
			pt.InstalledPowerKw = &v
		}
	}
	if w.AccordPower != nil {
		if v, jErr := normalize.JSONNumber(*w.AccordPower); jErr == nil {
			// R30: captured here on MeteringPoint; Task 10's SyncAnalyzers
			// does not persist it (model.Analyzer has no matching column).
			pt.ContractedPowerKw = &v
		}
	}
	if w.DefinitionType != nil {
		if v, jErr := w.DefinitionType.Int64(); jErr == nil {
			dt := int16(v)
			pt.DefinitionType = &dt
		}
	}

	if hw := mapProviderHighWater(w); len(hw) > 0 {
		pt.ProviderHighWater = hw
	}

	return pt, true
}

// mapProviderHighWater maps LastProfileDate -> load_profile and
// LastEndexDate -> {current_index, billing} (both current_endexes and
// end_of_month_endexes are endex-style calls; 06 §4 gives ARIL no separate
// high-water field for billing specifically — a spec gap noted in the task
// report). A present-but-unparseable date is simply omitted, never a
// discovery failure (see mapSubscription's doc).
func mapProviderHighWater(w subscriptionWire) map[model.ReadingKind]time.Time {
	hw := map[model.ReadingKind]time.Time{}
	if w.LastProfileDate != nil {
		if ts, ok := parseHighWaterDate(*w.LastProfileDate); ok {
			hw[model.ReadingKindLoadProfile] = ts
		}
	}
	if w.LastEndexDate != nil {
		if ts, ok := parseHighWaterDate(*w.LastEndexDate); ok {
			hw[model.ReadingKindCurrentIndex] = ts
			hw[model.ReadingKindBilling] = ts
		}
	}
	return hw
}

func parseHighWaterDate(n json.Number) (time.Time, bool) {
	ts, err := parseProfileDate(n)
	if err != nil {
		return time.Time{}, false
	}
	return ts, true
}

// readingRegistersEqual compares every register field of a and b — never
// Ts, Kind, MultiplierApplied, SourceProvider, IngestedAt, MeterSerial or
// Raw, which a legitimate re-send of the same reading may render
// differently — for dedupeByTimestamp's duplicate-timestamp conflict
// check (adapter review pattern 13).
func readingRegistersEqual(a, b model.MeterReading) bool {
	pairs := [][2]*decimal.Decimal{
		{a.ActiveImport, b.ActiveImport}, {a.ActiveExport, b.ActiveExport},
		{a.ReactiveInductiveImport, b.ReactiveInductiveImport}, {a.ReactiveCapacitiveImport, b.ReactiveCapacitiveImport},
		{a.ReactiveInductiveExport, b.ReactiveInductiveExport}, {a.ReactiveCapacitiveExport, b.ReactiveCapacitiveExport},
		{a.T1Import, b.T1Import}, {a.T2Import, b.T2Import}, {a.T3Import, b.T3Import},
		{a.T1Export, b.T1Export}, {a.T2Export, b.T2Export}, {a.T3Export, b.T3Export},
		{a.MaxDemandKw, b.MaxDemandKw},
	}
	for _, p := range pairs {
		if !decimalPtrEqual(p[0], p[1]) {
			return false
		}
	}
	return true
}

func decimalPtrEqual(a, b *decimal.Decimal) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}
