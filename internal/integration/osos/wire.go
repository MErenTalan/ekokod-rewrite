package osos

// wire.go holds only provider-shaped structs, decoded directly from OSOS's
// JSON responses. Every numeric field is a string (06 §2 "numbers … arrive
// as strings, sometimes with thousands separators") — never float64 or
// `any` — so TestIntegrationTreesDoNotParseFloats has nothing to flag here.

import "encoding/json"

// authResponse is POST authentication's response body.
type authResponse struct {
	AccessToken string `json:"access_token"`
}

// discoverResponse is GET analyzers_list's response body (06 §2 "Discover").
// InstalationList is a pointer so a top-level body with no "instalation_list"
// key at all, or an explicit JSON null, both decode to nil — indistinguishable
// from each other, and both distinguishable from a present-but-empty "[]",
// which decodes to a non-nil pointer to a zero-length slice. The former is
// ErrMalformedPayload (adapter-patterns.md item 6: a missing/null top-level
// key is never a silent empty success); the latter is a genuine empty
// result. See DiscoverMeteringPoints.
//
//nolint:misspell // OSOS's own field spelling ("instalation_list"), not an English typo
type discoverResponse struct {
	InstalationList *[]wireInstallation `json:"instalation_list"` //nolint:misspell // OSOS's own field spelling, not an English typo
}

// wireInstallation is one row of the discovery list, field names exactly as
// 06 §2's mapping table and analyzers_list gives them (including OSOS's own
// misspelled field names: one 'l' on the installation number field, one
// 'd' on the customer address field).
type wireInstallation struct {
	InstalationNumber string `json:"instalationNumber"`
	CustomerName      string `json:"customerName"`
	CustomerAdress    string `json:"customerAdress"`
	Il                string `json:"il"`
	Ilce              string `json:"ilce"`
	KoyMahallesi      string `json:"koyMahallesi"`
	CaddesiSokagi     string `json:"caddesiSokagi"`
	TarifeTipi        string `json:"tarifeTipi"`
	TarifeTuru        string `json:"tarifeTuru"`
	TesisatTurTanim   string `json:"tesisatTurTanim"`
	KuruluGucu        string `json:"kuruluGucu"`
	KoordinatX        string `json:"koordinatX"`
	KoordinatY        string `json:"koordinatY"`
	MeterNumber       string `json:"meterNumber"`
	MeterModel        string `json:"meterModel"`
	MeterMultiplier   string `json:"meterMultiplier"`
	MuhatapNo         string `json:"muhatapNo"`
	SayimNokTanim     string `json:"sayimNokTanim"`
}

// energyResponse is GET energy_values' response body (06 §2 "Fetch energy
// values"; field list per task-6-brief.md, citing legacy
// osos/refresh/route.ts:314-331).
//
// Energy is *[]json.RawMessage, not []wireEnergyRow, for two reasons
// (task-6-fix1-findings.md I6, I7):
//   - the pointer distinguishes a missing/null "energy" key (nil —
//     ErrMalformedPayload) from a present-but-empty "[]" (non-nil,
//     zero-length — a genuine empty result), exactly as discoverResponse
//     does above;
//   - keeping rows as raw JSON lets the caller decode one row at a time, so
//     a single row with a type-mismatched field (e.g. a register sent as a
//     JSON number instead of a string) produces one WarnUnparseableRow for
//     that row instead of failing json.Unmarshal for the whole page (which
//     would turn one bad row into a whole-page ErrMalformedPayload — exactly
//     the non-retryable-failure-from-one-row bug adapter-patterns.md item 7
//     warns against). See mapEnergyRow, which does the per-row decode.
type energyResponse struct {
	Energy *[]json.RawMessage `json:"energy"`
}

// wireEnergyRow is one cumulative-index reading. UP (u_p_kW) has no
// canonical register — 02-domain-rules.md §2.1 defines a single, import-side
// max_demand register, with no export-side counterpart — so mapEnergyRow
// reads it (for Raw/forensics only) but never maps it to a
// model.MeterReading field.
type wireEnergyRow struct {
	MeterDate     string `json:"meter_date"`
	MeterSerialNo string `json:"meter_serial_no"`

	TTop string `json:"t_top_kWh"`
	TRi  string `json:"t_ri_kVarh"`
	TRc  string `json:"t_rc_kVarh"`
	TT1  string `json:"t_t1_kWh"`
	TT2  string `json:"t_t2_kWh"`
	TT3  string `json:"t_t3_kWh"`
	TP   string `json:"t_p_kW"`

	UTop string `json:"u_top_kWh"`
	URi  string `json:"u_ri_kVarh"`
	URc  string `json:"u_rc_kVarh"`
	UU1  string `json:"u_u1_kWh"`
	UU2  string `json:"u_u2_kWh"`
	UU3  string `json:"u_u3_kWh"`
	UP   string `json:"u_p_kW"` // dropped — see 02-domain-rules.md §2.1
}

// hourlyResponse is GET hourly_values' response body (06 §2 "Fetch hourly
// values"): items keyed by installation number, each holding one or more
// meter blocks whose valueList carries the already-differenced
// consumption/generation series. Items is a pointer for the same
// missing/null-vs-empty reason as discoverResponse.InstalationList and
// energyResponse.Energy above: a missing/null "items" key is
// ErrMalformedPayload; a present "items":{} is a genuine empty result (no
// installation has hourly data this call).
type hourlyResponse struct {
	Items *map[string][]wireHourlyBlock `json:"items"`
}

// wireHourlyBlock's ValueList is raw, per-row, for the same reason
// energyResponse.Energy is: one row with a type-mismatched field must not
// fail every other row in the block. See mapHourlyValue.
type wireHourlyBlock struct {
	ValueList []json.RawMessage `json:"valueList"`
}

type wireHourlyValue struct {
	MeterDate         string `json:"meter_date"`
	ActiveConsumption string `json:"activeConsumption"`
	ActiveGeneration  string `json:"activeGeneration"`
}
