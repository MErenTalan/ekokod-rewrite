package osos

// mapping.go holds only pure functions from wire structs to
// model.MeterReading / integration.MeteringPoint / integration.HourlyValue.
// Nothing here makes a network call or touches a clock beyond what the
// caller passes in.

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/normalize"
)

// stringOrNil turns an empty provider string into a nil pointer
// (removed-behaviour 21's "unavailable is nil, never a zero value" applies
// to strings the same way it applies to numbers: OSOS's JSON always sends a
// string field, but an empty one means "the provider reported nothing").
func stringOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// optionalProviderNumber parses raw through normalize.ProviderNumber (R10),
// treating an empty string as "the provider reported nothing" (nil, nil)
// rather than a parse error — discovery rows leave several numeric fields
// blank for installations OSOS has no value for yet.
func optionalProviderNumber(raw string) (*decimal.Decimal, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	v, err := normalize.ProviderNumber(raw)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// tolerantOptionalProviderNumber parses raw exactly like
// optionalProviderNumber, except a value that fails to parse (not merely an
// empty one) is also treated as "the provider reported nothing" (nil)
// rather than propagated as an error.
//
// DiscoverMeteringPoints has no per-row Warnings channel — unlike
// FetchReadings, 06's MeterDataSource interface returns discovery as plain
// ([]MeteringPoint, error), so there is nowhere to attach a per-field
// warning. Per task-6-fix1-findings.md's folded minor ("discovery ...
// tolerates a bad optional numeric field as nil"), a single installation's
// unparseable optional number must not fail the entire discovery call for
// every other installation — nil is the closest available signal ("the
// provider reported nothing usable"), even though, unlike FetchReadings'
// row-level failures, it cannot also carry an operator-facing warning
// string today.
func tolerantOptionalProviderNumber(raw string) *decimal.Decimal {
	v, err := optionalProviderNumber(raw)
	if err != nil {
		return nil
	}
	return v
}

// mapInstallation converts one discovery-list row into a MeteringPoint,
// mapping every field of 06 §2's table. kuruluGucu, koordinatX/Y and
// meterMultiplier go through normalize.ProviderNumber (R10, tolerantly —
// see tolerantOptionalProviderNumber); everything else is a direct or
// nil-preserving string copy. Never errors: DiscoverMeteringPoints is
// responsible for skipping rows with no InstalationNumber before calling
// this (a row with no installation number cannot become an identifiable
// MeteringPoint at all), which is the only unrecoverable condition a
// discovery row can be in.
func mapInstallation(row wireInstallation) integration.MeteringPoint {
	installedPower := tolerantOptionalProviderNumber(row.KuruluGucu)
	lat := tolerantOptionalProviderNumber(row.KoordinatX)
	lon := tolerantOptionalProviderNumber(row.KoordinatY)
	multiplier := tolerantOptionalProviderNumber(row.MeterMultiplier)

	return integration.MeteringPoint{
		InstallationNumber: row.InstalationNumber,
		CustomerName:       stringOrNil(row.CustomerName),
		Address:            stringOrNil(row.CustomerAdress),
		Province:           stringOrNil(row.Il),
		District:           stringOrNil(row.Ilce),
		Neighbourhood:      stringOrNil(row.KoyMahallesi),
		Street:             stringOrNil(row.CaddesiSokagi),
		TariffType:         stringOrNil(row.TarifeTipi),
		TariffKind:         stringOrNil(row.TarifeTuru),
		InstallationKind:   stringOrNil(row.TesisatTurTanim),
		InstalledPowerKw:   installedPower,
		MeterNumber:        stringOrNil(row.MeterNumber),
		MeterModel:         stringOrNil(row.MeterModel),
		MeterMultiplier:    multiplier,
		CounterpartyNo:     stringOrNil(row.MuhatapNo),
		MeteringPointName:  stringOrNil(row.SayimNokTanim),
		Latitude:           lat,
		Longitude:          lon,
	}
}

// unmarshalTypeErrorField extracts the offending struct field name from a
// json.Unmarshal error when it is a *json.UnmarshalTypeError (e.g. a
// register sent as a JSON number where a string was expected) — the
// concrete manifestation of task-6-fix1-findings.md I7's "a register sent
// as a JSON number (or other type mismatch)". Any other decode error (a
// row that isn't even a JSON object, for instance) falls back to "row".
func unmarshalTypeErrorField(err error) string {
	var te *json.UnmarshalTypeError
	if errors.As(err, &te) && te.Field != "" {
		return te.Field
	}
	return "row"
}

// mapEnergyRow decodes and converts one energy_values row into a
// model.MeterReading. raw is that row's original, unmodified JSON bytes —
// decoding happens here, one row at a time (task-6-fix1-findings.md I7), so
// a row that doesn't even match wireEnergyRow's shape (a type-mismatched
// field) fails only this row, not the whole page. ok is false when the row
// failed to decode or parse (the "field" return names which one, for the
// caller's Warning.Detail); a failed row is never returned half-built.
// req.Multiplier is applied exactly once, here, via normalize.Multiply (R8)
// — never anywhere downstream. reading.Raw is raw's own bytes, unchanged
// (adapter-patterns.md item 14: never a re-marshalled struct).
func mapEnergyRow(raw json.RawMessage, req integration.FetchRequest) (reading model.MeterReading, field string, ok bool) {
	var row wireEnergyRow
	if err := json.Unmarshal(raw, &row); err != nil {
		return model.MeterReading{}, unmarshalTypeErrorField(err), false
	}

	ts, err := normalize.OSOSDate(row.MeterDate)
	if err != nil {
		return model.MeterReading{}, "meter_date", false
	}

	type regField struct {
		name string
		raw  string
		dst  **decimal.Decimal
	}
	var (
		activeImport, reactiveIndImport, reactiveCapImport *decimal.Decimal
		t1Import, t2Import, t3Import, maxDemand            *decimal.Decimal
		activeExport, reactiveIndExport, reactiveCapExport *decimal.Decimal
		t1Export, t2Export, t3Export                       *decimal.Decimal
	)
	fields := []regField{
		{"t_top_kWh", row.TTop, &activeImport},
		{"t_ri_kVarh", row.TRi, &reactiveIndImport},
		{"t_rc_kVarh", row.TRc, &reactiveCapImport},
		{"t_t1_kWh", row.TT1, &t1Import},
		{"t_t2_kWh", row.TT2, &t2Import},
		{"t_t3_kWh", row.TT3, &t3Import},
		{"t_p_kW", row.TP, &maxDemand},
		{"u_top_kWh", row.UTop, &activeExport},
		{"u_ri_kVarh", row.URi, &reactiveIndExport},
		{"u_rc_kVarh", row.URc, &reactiveCapExport},
		{"u_u1_kWh", row.UU1, &t1Export},
		{"u_u2_kWh", row.UU2, &t2Export},
		{"u_u3_kWh", row.UU3, &t3Export},
		// u_p_kW (row.UP) is deliberately absent from this list — see
		// wire.go's doc comment on wireEnergyRow.
	}
	for _, f := range fields {
		v, err := normalize.OptionalNumber(f.raw)
		if err != nil {
			return model.MeterReading{}, f.name, false
		}
		*f.dst = v
	}

	reading = model.MeterReading{
		AnalyzerID:               req.AnalyzerID,
		Ts:                       ts,
		Kind:                     req.Kind,
		ActiveImport:             normalize.Multiply(activeImport, req.Multiplier),
		ReactiveInductiveImport:  normalize.Multiply(reactiveIndImport, req.Multiplier),
		ReactiveCapacitiveImport: normalize.Multiply(reactiveCapImport, req.Multiplier),
		T1Import:                 normalize.Multiply(t1Import, req.Multiplier),
		T2Import:                 normalize.Multiply(t2Import, req.Multiplier),
		T3Import:                 normalize.Multiply(t3Import, req.Multiplier),
		ActiveExport:             normalize.Multiply(activeExport, req.Multiplier),
		ReactiveInductiveExport:  normalize.Multiply(reactiveIndExport, req.Multiplier),
		ReactiveCapacitiveExport: normalize.Multiply(reactiveCapExport, req.Multiplier),
		T1Export:                 normalize.Multiply(t1Export, req.Multiplier),
		T2Export:                 normalize.Multiply(t2Export, req.Multiplier),
		T3Export:                 normalize.Multiply(t3Export, req.Multiplier),
		MaxDemandKw:              normalize.Multiply(maxDemand, req.Multiplier),
		MeterSerial:              stringOrNil(row.MeterSerialNo),
		MultiplierApplied:        req.Multiplier,
		SourceProvider:           model.IntegrationProviderOSOS,
		// Raw is raw's own bytes, copied so the caller's buffer can't
		// alias/mutate what this reading carries — never a re-marshalled
		// struct (adapter-patterns.md item 14).
		Raw: append(json.RawMessage(nil), raw...),
	}
	return reading, "", true
}

// mapHourlyValue decodes and converts one hourly_values valueList entry
// into an integration.HourlyValue, one row at a time for the same I7 reason
// as mapEnergyRow. Unlike mapEnergyRow, this is never multiplied: 06 §2
// describes hourly_values as "already-differenced consumption values", a
// provider-computed cross-check series kept separate from the multiplied
// index readings (removed-behaviour 23), not a register that R8's
// multiply-once rule applies to.
func mapHourlyValue(raw json.RawMessage) (hv integration.HourlyValue, field string, ok bool) {
	var v wireHourlyValue
	if err := json.Unmarshal(raw, &v); err != nil {
		return integration.HourlyValue{}, unmarshalTypeErrorField(err), false
	}

	ts, err := normalize.OSOSDate(v.MeterDate)
	if err != nil {
		return integration.HourlyValue{}, "meter_date", false
	}
	consumption, err := normalize.OptionalNumber(v.ActiveConsumption)
	if err != nil {
		return integration.HourlyValue{}, "activeConsumption", false
	}
	generation, err := normalize.OptionalNumber(v.ActiveGeneration)
	if err != nil {
		return integration.HourlyValue{}, "activeGeneration", false
	}
	return integration.HourlyValue{Ts: ts, ActiveConsumption: consumption, ActiveGeneration: generation}, "", true
}
