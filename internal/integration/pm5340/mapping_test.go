package pm5340

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
)

func testAnalyzerID() uuid.UUID {
	return uuid.MustParse("33333333-3333-3333-3333-333333333333")
}

func testReq(multiplier string) integration.FetchRequest {
	return integration.FetchRequest{
		AnalyzerID: testAnalyzerID(),
		Multiplier: decimal.RequireFromString(multiplier),
		Kind:       model.ReadingKindLoadProfile,
		From:       time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC),
		To:         time.Date(2026, 1, 6, 0, 0, 0, 0, time.UTC),
	}
}

// TestMapRowMapsEveryField is adapter-patterns.md item 1 (every
// template-required field asserted by name on at least one reading) applied
// to PM5340's own reading mapping (06 §5): there is no discovery call to
// assert a metering-point mapping table against — PM5340 has none, per the
// task-9 brief's ruling — so this is that requirement's PM5340-shaped
// equivalent, and doubles as Test<P>DiscoverMapsEveryField from the shared
// adapter template.
func TestMapRowMapsEveryField(t *testing.T) {
	raw := json.RawMessage(`{
		"meterDate": "2026-01-05T08:00:00+03:00",
		"activeImport_kWh": 1523.750,
		"inductive_kvarh": 88.400,
		"capacitive_kvarh": 12.100,
		"dmdKwPeak_kW": 45.200,
		"currentGeneration": 3.600,
		"deviceId": "FIXTURE-DEVICE-01",
		"deviceIp": "127.0.0.1"
	}`)
	var row pm5340Row
	require.NoError(t, json.Unmarshal(raw, &row))

	req := testReq("2.5")
	reading, field, ok := mapRow(row, raw, req)
	require.True(t, ok, "field %q", field)

	require.Equal(t, testAnalyzerID(), reading.AnalyzerID)
	require.Equal(t, model.ReadingKindLoadProfile, reading.Kind)
	require.Equal(t, model.IntegrationProviderPM5340, reading.SourceProvider)
	require.True(t, req.Multiplier.Equal(reading.MultiplierApplied))
	require.True(t, reading.Ts.Equal(time.Date(2026, 1, 5, 5, 0, 0, 0, time.UTC)))

	// adapter-patterns.md item 2: every register = raw x multiplier (2.5),
	// applied exactly once.
	requireDecimal(t, "1523.750", "2.5", reading.ActiveImport)
	requireDecimal(t, "88.400", "2.5", reading.ReactiveInductiveImport)
	requireDecimal(t, "12.100", "2.5", reading.ReactiveCapacitiveImport)
	requireDecimal(t, "45.200", "2.5", reading.MaxDemandKw)

	// currentGeneration: interval kW -> kWh via x0.25, NOT multiplied by
	// req.Multiplier (see mapRow's doc comment and the task report).
	wantGeneration := decimal.RequireFromString("3.600").Mul(decimal.RequireFromString("0.25"))
	require.NotNil(t, reading.IntervalGenerationKwh)
	require.True(t, wantGeneration.Equal(*reading.IntervalGenerationKwh))

	// removed-behaviour 22 / TestPM5340NeverDerivesCumulativeExport: the
	// adapter NEVER writes ActiveExport.
	require.Nil(t, reading.ActiveExport)
	require.Nil(t, reading.MeterSerial)

	// adapter-patterns.md item 14: Raw is the row's OWN original bytes, not
	// a re-marshalled struct — json.RawMessage(raw) round-trips exactly.
	require.JSONEq(t, string(raw), string(reading.Raw))
	require.Equal(t, []byte(raw), []byte(reading.Raw))
}

func requireDecimal(t *testing.T, rawValue, multiplier string, got *decimal.Decimal) {
	t.Helper()
	require.NotNil(t, got)
	want := decimal.RequireFromString(rawValue).Mul(decimal.RequireFromString(multiplier))
	require.Truef(t, want.Equal(*got), "want %s, got %s", want, got)
}

// TestMapRowNilNeverZero is adapter-patterns.md item 3: a JSON `null`
// register decodes to a nil pointer (removed-behaviour 21), while a
// genuine "0" reading stays a zero decimal, never collapsed to nil.
func TestMapRowNilNeverZero(t *testing.T) {
	raw := json.RawMessage(`{
		"meterDate": "2026-01-05T08:00:00+03:00",
		"activeImport_kWh": 0,
		"inductive_kvarh": null,
		"capacitive_kvarh": null,
		"dmdKwPeak_kW": null,
		"currentGeneration": null
	}`)
	var row pm5340Row
	require.NoError(t, json.Unmarshal(raw, &row))

	reading, field, ok := mapRow(row, raw, testReq("1"))
	require.True(t, ok, "field %q", field)

	require.NotNil(t, reading.ActiveImport, "a genuine 0 must stay a zero decimal, never nil")
	require.True(t, reading.ActiveImport.IsZero())

	require.Nil(t, reading.ReactiveInductiveImport)
	require.Nil(t, reading.ReactiveCapacitiveImport)
	require.Nil(t, reading.MaxDemandKw)
	require.Nil(t, reading.IntervalGenerationKwh)
}

// TestMapRowRejectsMissingOrUnparseableMeterDate covers both "no reading
// without a timestamp" branches: an absent meterDate key and one that
// parses as neither ISO 8601 nor DD/MM/YYYY.
func TestMapRowRejectsMissingOrUnparseableMeterDate(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
	}{
		{"missing", `{"activeImport_kWh": 1}`},
		{"unparseable", `{"meterDate": "not-a-date", "activeImport_kWh": 1}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := json.RawMessage(tc.raw)
			var row pm5340Row
			require.NoError(t, json.Unmarshal(raw, &row))

			_, field, ok := mapRow(row, raw, testReq("1"))
			require.False(t, ok)
			require.Equal(t, "meterDate", field)
		})
	}
}

// TestMapRowRejectsUnparseableRegister names the offending field, per the
// task-9 brief's "<field> at row <n>" detail format.
func TestMapRowRejectsUnparseableRegister(t *testing.T) {
	raw := json.RawMessage(`{"meterDate": "2026-01-05T08:00:00+03:00", "activeImport_kWh": "N/A"}`)
	var row pm5340Row
	require.NoError(t, json.Unmarshal(raw, &row))

	_, field, ok := mapRow(row, raw, testReq("1"))
	require.False(t, ok)
	require.Equal(t, "activeImport_kWh", field)
}

// TestPM5340AcceptsBothDateFormats is the task-9 brief's named behaviour
// test: meterDate parses whether it is ISO 8601 (with an explicit offset)
// or "DD/MM/YYYY HH:mm[:ss]" (Europe/Istanbul local, removed-behaviour 20).
func TestPM5340AcceptsBothDateFormats(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
		want time.Time
	}{
		{"iso8601_offset", `{"meterDate": "2026-01-05T08:00:00+03:00"}`, time.Date(2026, 1, 5, 5, 0, 0, 0, time.UTC)},
		{"ddmmyyyy_with_seconds", `{"meterDate": "05/01/2026 08:00:00"}`, time.Date(2026, 1, 5, 5, 0, 0, 0, time.UTC)},
		{"ddmmyyyy_without_seconds", `{"meterDate": "05/01/2026 08:15"}`, time.Date(2026, 1, 5, 5, 15, 0, 0, time.UTC)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := json.RawMessage(tc.raw)
			var row pm5340Row
			require.NoError(t, json.Unmarshal(raw, &row))

			reading, field, ok := mapRow(row, raw, testReq("1"))
			require.True(t, ok, "field %q", field)
			require.True(t, tc.want.Equal(reading.Ts), "want %s, got %s", tc.want, reading.Ts)
		})
	}
}

// TestDecodeEnvelopeMalformedWhenItemsMissingOrNull is adapter-patterns.md
// item 6: a 200 body whose "items" key is missing, or explicitly JSON
// null, is malformed; an explicit empty array is a valid empty result.
func TestDecodeEnvelopeMalformedWhenItemsMissingOrNull(t *testing.T) {
	for _, tc := range []struct {
		name    string
		body    string
		wantOK  bool
		wantLen int
	}{
		{"missing_key", `{"limit": 500}`, false, 0},
		{"null_items", `{"items": null}`, false, 0},
		{"error_envelope", `{"error": "boom"}`, false, 0},
		{"not_json", `not json at all`, false, 0},
		{"empty_array", `{"items": []}`, true, 0},
		{"one_item", `{"items": [{"meterDate":"2026-01-05T08:00:00+03:00"}]}`, true, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env, ok := decodeEnvelope([]byte(tc.body))
			require.Equal(t, tc.wantOK, ok)
			if tc.wantOK {
				require.Len(t, *env.Items, tc.wantLen)
			}
		})
	}
}

func TestDecodeRowErrorOnMalformedJSON(t *testing.T) {
	_, err := decodeRow(json.RawMessage(`{not valid json`))
	require.Error(t, err)
}

// TestRegistersEqualDetectsConflict backs adapter-patterns.md item 13
// (duplicate timestamps with DIFFERENT values -> warning).
func TestRegistersEqualDetectsConflict(t *testing.T) {
	v1 := decimal.RequireFromString("1.0")
	v2 := decimal.RequireFromString("2.0")

	a := model.MeterReading{ActiveImport: &v1}
	b := model.MeterReading{ActiveImport: &v1}
	require.True(t, registersEqual(a, b))

	c := model.MeterReading{ActiveImport: &v2}
	require.False(t, registersEqual(a, c))

	// nil vs nil is equal; nil vs a value is not.
	require.True(t, registersEqual(model.MeterReading{}, model.MeterReading{}))
	require.False(t, registersEqual(model.MeterReading{}, a))
}
