package pm5340

import "encoding/json"

// readingsEnvelope is the top-level shape of a PM5340
// GET .../api/v1/readings response (06-integrations.md §5):
//
//	{ items: [...], limit, cursorNext, hasMore, total, sort, filters }
//
// Items is a pointer so a missing or JSON-null "items" key (the ONLY key
// this adapter treats as gating malformed-vs-empty, per adapter-patterns.md
// item 6) is distinguishable from a present, empty array. Every other field
// is metadata this adapter either does not need (Limit, Total, Sort,
// Filters) or reads defensively (CursorNext, HasMore) — their absence is
// never treated as malformed, only as "no further page".
type readingsEnvelope struct {
	Items      *[]json.RawMessage `json:"items"`
	Limit      *json.Number       `json:"limit"`
	CursorNext *string            `json:"cursorNext"`
	HasMore    *bool              `json:"hasMore"`
	Total      *json.Number       `json:"total"`
	Sort       *string            `json:"sort"`
	Filters    json.RawMessage    `json:"filters"`
}

// pm5340Row is one entry of readingsEnvelope.Items (06 §5 "Reading
// mapping"). Every register is json.RawMessage rather than *json.Number:
// encoding/json validates numeric SYNTAX even for a json.Number-typed
// field (a quoted non-numeric string like "N/A" fails to decode at all,
// which would fail the WHOLE row rather than naming the one offending
// field), so registers are captured as raw bytes here and parsed field by
// field in mapping.go's decodeOptionalNumber — accepting a bare JSON
// number, a quoted numeric string (some device firmwares stringify
// numbers) or JSON null uniformly, with no float64 anywhere in the path.
// DeviceID/DeviceIP are diagnostic metadata only (06 §5): they never feed
// a canonical register, they only ever reach Raw via the row's own
// original bytes.
type pm5340Row struct {
	MeterDate         *string         `json:"meterDate"`
	ActiveImportKwh   json.RawMessage `json:"activeImport_kWh"`
	InductiveKvarh    json.RawMessage `json:"inductive_kvarh"`
	CapacitiveKvarh   json.RawMessage `json:"capacitive_kvarh"`
	DmdKwPeakKw       json.RawMessage `json:"dmdKwPeak_kW"`
	CurrentGeneration json.RawMessage `json:"currentGeneration"`
	DeviceID          *string         `json:"deviceId"`
	DeviceIP          *string         `json:"deviceIp"`
}
