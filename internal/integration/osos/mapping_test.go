package osos

// Internal (white-box) unit tests for the pure functions in mapping.go and
// source.go's windowing helpers. source_test.go covers the adapter
// end-to-end through the fake HTTP server; this file tests the pure
// conversions directly, with no network involved.

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

// mustRaw marshals v to json.RawMessage, failing the test on error. Used
// throughout this file since mapEnergyRow/mapHourlyValue decode raw JSON
// bytes directly (I7's per-row decoding), not wire structs.
func mustRaw(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

func TestStringOrNil(t *testing.T) {
	require.Nil(t, stringOrNil(""))
	got := stringOrNil("Fixture Value")
	require.NotNil(t, got)
	require.Equal(t, "Fixture Value", *got)
}

func TestOptionalProviderNumber(t *testing.T) {
	v, err := optionalProviderNumber("")
	require.NoError(t, err)
	require.Nil(t, v)

	v, err = optionalProviderNumber("  ")
	require.NoError(t, err)
	require.Nil(t, v)

	v, err = optionalProviderNumber("12.345,678")
	require.NoError(t, err)
	require.NotNil(t, v)
	require.True(t, decimal.RequireFromString("12345.678").Equal(*v))

	_, err = optionalProviderNumber("not-a-number")
	require.Error(t, err)
}

func TestDataTypeParam(t *testing.T) {
	for _, tc := range []struct {
		kind model.ReadingKind
		want string
	}{
		{model.ReadingKindLoadProfile, "1"},
		{model.ReadingKindDaily, "2"},
		{model.ReadingKindReset, "3"},
	} {
		got, err := dataTypeParam(tc.kind)
		require.NoError(t, err)
		require.Equal(t, tc.want, got)
	}

	_, err := dataTypeParam(model.ReadingKindBilling)
	require.Error(t, err)
}

func TestWindowChunksSplits30Plus15(t *testing.T) {
	from := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC) // 45 days

	chunks := windowChunks(from, to, 30*24*time.Hour)
	require.Len(t, chunks, 2)
	require.Equal(t, from, chunks[0].From)
	require.Equal(t, 30*24*time.Hour, chunks[0].To.Sub(chunks[0].From))
	require.Equal(t, chunks[0].To, chunks[1].From)
	require.Equal(t, to, chunks[1].To)
	require.Equal(t, 15*24*time.Hour, chunks[1].To.Sub(chunks[1].From))
}

func TestWindowChunksEmptyOrInvertedYieldsNone(t *testing.T) {
	from := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	require.Nil(t, windowChunks(from, from, 30*24*time.Hour))
	require.Nil(t, windowChunks(from.Add(time.Hour), from, 30*24*time.Hour))
	require.Nil(t, windowChunks(from, from.Add(time.Hour), 0))
}

func TestMonthsIntersecting(t *testing.T) {
	// A window entirely inside August (Istanbul-local) touches one month.
	from := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC) // 03:00 Istanbul
	to := time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)
	months := monthsIntersecting(from, to)
	require.Len(t, months, 1)
	require.Equal(t, "2026-08", months[0].Label)

	// A 45-day window from 1 Aug touches August and September.
	to45 := time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC)
	months = monthsIntersecting(from, to45)
	require.Len(t, months, 2)
	require.Equal(t, "2026-08", months[0].Label)
	require.Equal(t, "2026-09", months[1].Label)
}

func TestMapInstallationEmptyOptionalFieldsAreNil(t *testing.T) {
	pt := mapInstallation(wireInstallation{InstalationNumber: "FX0001"})
	require.Equal(t, "FX0001", pt.InstallationNumber)
	require.Nil(t, pt.CustomerName)
	require.Nil(t, pt.Address)
	require.Nil(t, pt.InstalledPowerKw)
	require.Nil(t, pt.Latitude)
	require.Nil(t, pt.Longitude)
	require.Nil(t, pt.MeterMultiplier)
}

// TestMapInstallationToleratesUnparseableNumberAsNil: task-6-fix1-findings.md
// folded minor — mapInstallation never errors (DiscoverMeteringPoints has no
// per-row Warnings channel to attach a field-level warning to), so a bad
// optional numeric field becomes nil rather than failing every other
// installation in the response.
func TestMapInstallationToleratesUnparseableNumberAsNil(t *testing.T) {
	pt := mapInstallation(wireInstallation{InstalationNumber: "FX0001", KuruluGucu: "not-a-number"})
	require.Equal(t, "FX0001", pt.InstallationNumber)
	require.Nil(t, pt.InstalledPowerKw)
}

func TestMapEnergyRowUnparseableMeterDate(t *testing.T) {
	req := integration.FetchRequest{AnalyzerID: uuid.New(), Multiplier: decimal.NewFromInt(1)}
	_, field, ok := mapEnergyRow(mustRaw(t, wireEnergyRow{MeterDate: "not-a-date"}), req)
	require.False(t, ok)
	require.Equal(t, "meter_date", field)
}

func TestMapEnergyRowUnparseableRegister(t *testing.T) {
	req := integration.FetchRequest{AnalyzerID: uuid.New(), Multiplier: decimal.NewFromInt(1)}
	_, field, ok := mapEnergyRow(mustRaw(t, wireEnergyRow{
		MeterDate: "01/08/2026 10:00:00",
		TTop:      "not-a-number",
	}), req)
	require.False(t, ok)
	require.Equal(t, "t_top_kWh", field)
}

// TestMapEnergyRowRejectsTypeMismatchedRow proves I7's decode-shaped
// failure specifically: a register sent as a JSON *number* (not a string at
// all — t_top_kWh's wire type is string) fails json.Unmarshal itself, and
// mapEnergyRow reports the offending field name from
// *json.UnmarshalTypeError, not merely "row failed to parse".
func TestMapEnergyRowRejectsTypeMismatchedRow(t *testing.T) {
	req := integration.FetchRequest{AnalyzerID: uuid.New(), Multiplier: decimal.NewFromInt(1)}
	raw := json.RawMessage(`{"meter_date":"01/08/2026 10:00:00","meter_serial_no":"SN0001","t_top_kWh":123}`)
	_, field, ok := mapEnergyRow(raw, req)
	require.False(t, ok)
	require.Equal(t, "t_top_kWh", field)
}

func TestMapHourlyValueUnparseable(t *testing.T) {
	_, field, ok := mapHourlyValue(mustRaw(t, wireHourlyValue{MeterDate: "garbage"}))
	require.False(t, ok)
	require.Equal(t, "meter_date", field)

	_, field, ok = mapHourlyValue(mustRaw(t, wireHourlyValue{MeterDate: "01/08/2026 10:00:00", ActiveConsumption: "not-a-number"}))
	require.False(t, ok)
	require.Equal(t, "activeConsumption", field)
}

// TestMapHourlyValueRejectsTypeMismatchedRow is TestMapEnergyRowRejectsTypeMismatchedRow's
// hourly_values counterpart (I7).
func TestMapHourlyValueRejectsTypeMismatchedRow(t *testing.T) {
	raw := json.RawMessage(`{"meter_date":"01/08/2026 10:00:00","activeConsumption":42}`)
	_, field, ok := mapHourlyValue(raw)
	require.False(t, ok)
	require.Equal(t, "activeConsumption", field)
}

// TestMapEnergyRowNilVsZero pins I3 at the mapping boundary: "", "null" (the
// JSON literal — Go's json.Unmarshal leaves a string field "" for a null
// value, exercised here through raw JSON, not a Go literal) and "-" all mean
// nil; a genuine "0,000" is a real, present zero, never coerced to nil.
func TestMapEnergyRowNilVsZero(t *testing.T) {
	req := integration.FetchRequest{AnalyzerID: uuid.New(), Multiplier: decimal.NewFromInt(1)}
	raw := json.RawMessage(`{
		"meter_date":"01/08/2026 10:00:00",
		"meter_serial_no":"SN0001",
		"t_top_kWh":"",
		"t_ri_kVarh":null,
		"t_rc_kVarh":"-",
		"t_t1_kWh":"0,000"
	}`)
	reading, field, ok := mapEnergyRow(raw, req)
	require.True(t, ok, "unexpected unparseable field %q", field)
	require.Nil(t, reading.ActiveImport, "empty string t_top_kWh must be nil, not zero")
	require.Nil(t, reading.ReactiveInductiveImport, "JSON null t_ri_kVarh must be nil, not zero")
	require.Nil(t, reading.ReactiveCapacitiveImport, `"-" t_rc_kVarh must be nil, not zero`)
	require.NotNil(t, reading.T1Import, `"0,000" t_t1_kWh is a real, present zero`)
	require.True(t, decimal.Zero.Equal(*reading.T1Import), "got %s", reading.T1Import.String())
}

// TestOSOSRegisterMapping is the reading-side companion to
// Test<P>DiscoverMapsEveryField (task-6-brief.md): one fixture row with 14
// distinct raw values (13 mapped registers + u_p_kW, which is deliberately
// dropped — see wire.go), asserted register by register, with a multiplier
// != 1 ("2.5", task-6-fix1-findings.md I1) so a bug that fails to multiply
// one register, multiplies it twice, or reuses another register's raw
// value cannot hide behind a multiplier of 1 (reviewer mutations a2/a3).
// It also asserts every other template-required field on the same reading
// (I2: Ts, Kind, AnalyzerID, SourceProvider, MultiplierApplied, MeterSerial
// — reviewer mutation a4). The task brief's named mutation — mapping u_ri
// (which belongs to ReactiveInductiveExport) to ReactiveCapacitiveExport
// instead — must turn this test red; see task-6-report.md's "Fix round 1"
// section for the observed failure (re-proven below this test's original
// baseline in the fix round).
func TestOSOSRegisterMapping(t *testing.T) {
	row := wireEnergyRow{
		MeterDate:     "01/08/2026 10:00:00",
		MeterSerialNo: "SN-FIXTURE-0001",
		TTop:          "1",
		TRi:           "2",
		TRc:           "3",
		TT1:           "4",
		TT2:           "5",
		TT3:           "6",
		TP:            "7",
		UTop:          "8",
		URi:           "9",
		URc:           "10",
		UU1:           "11",
		UU2:           "12",
		UU3:           "13",
		UP:            "14", // dropped — asserted absent nowhere below by design
	}
	analyzerID := uuid.New()
	multiplier := decimal.RequireFromString("2.5")
	req := integration.FetchRequest{AnalyzerID: analyzerID, Kind: model.ReadingKindDaily, Multiplier: multiplier}

	reading, field, ok := mapEnergyRow(mustRaw(t, row), req)
	require.True(t, ok, "unexpected unparseable field %q", field)

	// I2: every other template-required field on this same reading.
	require.Equal(t, time.Date(2026, 8, 1, 7, 0, 0, 0, time.UTC), reading.Ts.UTC(), "meter_date -> Ts (Istanbul local -> UTC)")
	require.Equal(t, model.ReadingKindDaily, reading.Kind)
	require.Equal(t, analyzerID, reading.AnalyzerID)
	require.Equal(t, model.IntegrationProviderOSOS, reading.SourceProvider)
	require.True(t, multiplier.Equal(reading.MultiplierApplied), "MultiplierApplied: expected %s, got %s", multiplier, reading.MultiplierApplied)
	require.NotNil(t, reading.MeterSerial)
	require.Equal(t, "SN-FIXTURE-0001", *reading.MeterSerial)

	// I1: every register = raw x 2.5, applied exactly once.
	requireDecimalEqual(t, "ActiveImport (t_top_kWh)", "2.5", reading.ActiveImport)
	requireDecimalEqual(t, "ReactiveInductiveImport (t_ri_kVarh)", "5", reading.ReactiveInductiveImport)
	requireDecimalEqual(t, "ReactiveCapacitiveImport (t_rc_kVarh)", "7.5", reading.ReactiveCapacitiveImport)
	requireDecimalEqual(t, "T1Import (t_t1_kWh)", "10", reading.T1Import)
	requireDecimalEqual(t, "T2Import (t_t2_kWh)", "12.5", reading.T2Import)
	requireDecimalEqual(t, "T3Import (t_t3_kWh)", "15", reading.T3Import)
	requireDecimalEqual(t, "MaxDemandKw (t_p_kW)", "17.5", reading.MaxDemandKw)
	requireDecimalEqual(t, "ActiveExport (u_top_kWh)", "20", reading.ActiveExport)
	requireDecimalEqual(t, "ReactiveInductiveExport (u_ri_kVarh)", "22.5", reading.ReactiveInductiveExport)
	requireDecimalEqual(t, "ReactiveCapacitiveExport (u_rc_kVarh)", "25", reading.ReactiveCapacitiveExport)
	requireDecimalEqual(t, "T1Export (u_u1_kWh)", "27.5", reading.T1Export)
	requireDecimalEqual(t, "T2Export (u_u2_kWh)", "30", reading.T2Export)
	requireDecimalEqual(t, "T3Export (u_u3_kWh)", "32.5", reading.T3Export)
}

func requireDecimalEqual(t *testing.T, label, want string, got *decimal.Decimal) {
	t.Helper()
	require.NotNil(t, got, "%s: expected %s, got nil", label, want)
	require.True(t, decimal.RequireFromString(want).Equal(*got), "%s: expected %s, got %s", label, want, got.String())
}
