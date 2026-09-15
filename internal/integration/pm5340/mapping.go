package pm5340

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/normalize"
)

// decodeEnvelope decodes body into a readingsEnvelope. A body that is not
// valid JSON at all, or whose "items" key is missing or JSON-null, reports
// ok=false; the caller turns that into ErrMalformedPayload (adapter-
// patterns.md item 6). An explicit empty array is a valid, non-error empty
// result.
func decodeEnvelope(body []byte) (readingsEnvelope, bool) {
	var env readingsEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return readingsEnvelope{}, false
	}
	if env.Items == nil {
		return readingsEnvelope{}, false
	}
	return env, true
}

// decodeRow decodes one raw item into a pm5340Row.
func decodeRow(raw json.RawMessage) (pm5340Row, error) {
	var row pm5340Row
	if err := json.Unmarshal(raw, &row); err != nil {
		return pm5340Row{}, err
	}
	return row, nil
}

// mapRow converts one decoded pm5340Row into a model.MeterReading, applying
// req's AnalyzerID/Kind/Multiplier (06 §1 normalisation obligations) and
// leaving ActiveExport nil always (removed-behaviour 22: the cumulative
// export register is derived by the ingestion pipeline, never here).
//
// ok is false when the row cannot be mapped at all (an absent or
// unparseable meterDate) or when one register's value fails to parse;
// field names the offending field for the caller's WarnUnparseableRow
// detail ("<field> at row <n>", per the task brief verbatim).
func mapRow(row pm5340Row, raw json.RawMessage, req integration.FetchRequest) (reading model.MeterReading, field string, ok bool) {
	if row.MeterDate == nil {
		return model.MeterReading{}, "meterDate", false
	}
	ts, err := normalize.PM5340Date(*row.MeterDate)
	if err != nil {
		return model.MeterReading{}, "meterDate", false
	}

	activeImport, field, valid := optionalRegister(row.ActiveImportKwh, "activeImport_kWh")
	if !valid {
		return model.MeterReading{}, field, false
	}
	inductive, field, valid := optionalRegister(row.InductiveKvarh, "inductive_kvarh")
	if !valid {
		return model.MeterReading{}, field, false
	}
	capacitive, field, valid := optionalRegister(row.CapacitiveKvarh, "capacitive_kvarh")
	if !valid {
		return model.MeterReading{}, field, false
	}
	maxDemand, field, valid := optionalRegister(row.DmdKwPeakKw, "dmdKwPeak_kW")
	if !valid {
		return model.MeterReading{}, field, false
	}
	// currentGeneration deliberately has NO req.Multiplier applied: the
	// task-9 brief's mapping list ("activeImport_kWh, inductive_kvarh,
	// capacitive_kvarh, dmdKwPeak_kW, each x req.Multiplier") names only
	// those four registers for multiplication; currentGeneration's own
	// sentence ("currentGeneration (kW) -> IntervalGenerationKwh =
	// normalize.KWFor15MinToKWh") names only the 0.25 interval-to-energy
	// conversion. See the task report's "Spec gaps and interpretation
	// choices".
	generation, field, valid := optionalRegister(row.CurrentGeneration, "currentGeneration")
	if !valid {
		return model.MeterReading{}, field, false
	}

	reading = model.MeterReading{
		AnalyzerID:               req.AnalyzerID,
		Ts:                       ts,
		Kind:                     req.Kind,
		ActiveImport:             normalize.Multiply(activeImport, req.Multiplier),
		ReactiveInductiveImport:  normalize.Multiply(inductive, req.Multiplier),
		ReactiveCapacitiveImport: normalize.Multiply(capacitive, req.Multiplier),
		MaxDemandKw:              normalize.Multiply(maxDemand, req.Multiplier),
		IntervalGenerationKwh:    normalize.KWFor15MinToKWh(generation),
		MultiplierApplied:        req.Multiplier,
		SourceProvider:           model.IntegrationProviderPM5340,
		Raw:                      raw,
	}
	return reading, "", true
}

// optionalRegister parses raw (nil, empty, or JSON `null` all mean
// "unavailable" — removed-behaviour 21) into a *decimal.Decimal, naming
// field for the caller's WarnUnparseableRow detail when raw is present but
// not a valid number. raw may be a bare JSON number or a quoted numeric
// string — see pm5340Row's doc comment for why registers are decoded as
// json.RawMessage rather than *json.Number.
func optionalRegister(raw json.RawMessage, field string) (*decimal.Decimal, string, bool) {
	v, err := decodeOptionalNumber(raw)
	if err != nil {
		return nil, field, false
	}
	return v, "", true
}

// decodeOptionalNumber parses one register's raw JSON bytes into a
// *decimal.Decimal, or nil for an absent field, a JSON `null`, or (via
// normalize.OptionalNumber) an empty/"-" string content once unquoted.
func decodeOptionalNumber(raw json.RawMessage) (*decimal.Decimal, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return nil, nil
	}

	s := string(trimmed)
	if len(trimmed) >= 2 && trimmed[0] == '"' && trimmed[len(trimmed)-1] == '"' {
		var unquoted string
		if err := json.Unmarshal(trimmed, &unquoted); err != nil {
			return nil, err
		}
		s = unquoted
	}
	return normalize.OptionalNumber(s)
}

// registersEqual reports whether two readings carry identical register
// values — used to detect a duplicate timestamp whose rows genuinely
// disagree (adapter-patterns.md item 13), as opposed to the provider simply
// repeating the same row (e.g. a page-boundary re-fetch), which is not a
// conflict worth warning about.
func registersEqual(a, b model.MeterReading) bool {
	return decimalPtrEqual(a.ActiveImport, b.ActiveImport) &&
		decimalPtrEqual(a.ReactiveInductiveImport, b.ReactiveInductiveImport) &&
		decimalPtrEqual(a.ReactiveCapacitiveImport, b.ReactiveCapacitiveImport) &&
		decimalPtrEqual(a.MaxDemandKw, b.MaxDemandKw) &&
		decimalPtrEqual(a.IntervalGenerationKwh, b.IntervalGenerationKwh)
}

func decimalPtrEqual(a, b *decimal.Decimal) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}

// unparseableRowWarning builds the exact "<field> at row <n>" detail the
// task-9 brief specifies for a WarnUnparseableRow.
func unparseableRowWarning(field string, row int) integration.Warning {
	return integration.Warning{Code: integration.WarnUnparseableRow, Detail: fmt.Sprintf("%s at row %d", field, row)}
}

// duplicateTimestampWarning marks a later row at the same timestamp whose
// register values disagree with an earlier one already collected. There is
// no dedicated Warning code for this case among package integration's
// closed Warn* set (owned by Task 1); WarnUnparseableRow is reused since it
// is, like this case, a per-row data-quality note carrying a "<...> at row
// <n>" detail, not a claim that persistence should treat the row as
// unusable. See the task report's "Spec gaps and interpretation choices".
func duplicateTimestampWarning(row int) integration.Warning {
	return integration.Warning{Code: integration.WarnUnparseableRow, Detail: fmt.Sprintf("duplicate timestamp with differing register values at row %d", row)}
}

// generationNilWarning marks a row whose currentGeneration was JSON null,
// per the task-9 brief ("nil stays nil, plus WarnGenerationIntervalNil").
func generationNilWarning(row int) integration.Warning {
	return integration.Warning{Code: integration.WarnGenerationIntervalNil, Detail: fmt.Sprintf("currentGeneration at row %d", row)}
}
