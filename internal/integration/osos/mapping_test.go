package osos

// Internal (white-box) unit tests for the pure functions in mapping.go and
// source.go's windowing helpers. source_test.go covers the adapter
// end-to-end through the fake HTTP server; this file tests the pure
// conversions directly, with no network involved.

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
)

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
	pt, err := mapInstallation(wireInstallation{InstalationNumber: "FX0001"})
	require.NoError(t, err)
	require.Equal(t, "FX0001", pt.InstallationNumber)
	require.Nil(t, pt.CustomerName)
	require.Nil(t, pt.Address)
	require.Nil(t, pt.InstalledPowerKw)
	require.Nil(t, pt.Latitude)
	require.Nil(t, pt.Longitude)
	require.Nil(t, pt.MeterMultiplier)
}

func TestMapInstallationRejectsUnparseableNumber(t *testing.T) {
	_, err := mapInstallation(wireInstallation{InstalationNumber: "FX0001", KuruluGucu: "not-a-number"})
	require.Error(t, err)
}

func TestMapEnergyRowUnparseableMeterDate(t *testing.T) {
	req := integration.FetchRequest{AnalyzerID: uuid.New(), Multiplier: decimal.NewFromInt(1)}
	_, field, ok := mapEnergyRow(wireEnergyRow{MeterDate: "not-a-date"}, req)
	require.False(t, ok)
	require.Equal(t, "meter_date", field)
}

func TestMapEnergyRowUnparseableRegister(t *testing.T) {
	req := integration.FetchRequest{AnalyzerID: uuid.New(), Multiplier: decimal.NewFromInt(1)}
	_, field, ok := mapEnergyRow(wireEnergyRow{
		MeterDate: "01/08/2026 10:00:00",
		TTop:      "not-a-number",
	}, req)
	require.False(t, ok)
	require.Equal(t, "t_top_kWh", field)
}

func TestMapHourlyValueUnparseable(t *testing.T) {
	_, field, ok := mapHourlyValue(wireHourlyValue{MeterDate: "garbage"})
	require.False(t, ok)
	require.Equal(t, "meter_date", field)

	_, field, ok = mapHourlyValue(wireHourlyValue{MeterDate: "01/08/2026 10:00:00", ActiveConsumption: "not-a-number"})
	require.False(t, ok)
	require.Equal(t, "activeConsumption", field)
}

// TestOSOSRegisterMapping is the reading-side companion to
// Test<P>DiscoverMapsEveryField (task-6-brief.md): one fixture row with 14
// distinct raw values (13 mapped registers + u_p_kW, which is deliberately
// dropped — see wire.go), asserted register by register. The task brief's
// named mutation — mapping u_ri (which belongs to
// ReactiveInductiveExport) to ReactiveCapacitiveExport instead — must turn
// this test red; see mapping_test_mutation_proof.md-equivalent note in the
// task-6 report for the observed failure.
func TestOSOSRegisterMapping(t *testing.T) {
	row := wireEnergyRow{
		MeterDate: "01/08/2026 10:00:00",
		TTop:      "1",
		TRi:       "2",
		TRc:       "3",
		TT1:       "4",
		TT2:       "5",
		TT3:       "6",
		TP:        "7",
		UTop:      "8",
		URi:       "9",
		URc:       "10",
		UU1:       "11",
		UU2:       "12",
		UU3:       "13",
		UP:        "14", // dropped — asserted absent nowhere below by design
	}
	req := integration.FetchRequest{AnalyzerID: uuid.New(), Multiplier: decimal.NewFromInt(1)}

	reading, field, ok := mapEnergyRow(row, req)
	require.True(t, ok, "unexpected unparseable field %q", field)

	requireDecimalEqual(t, "ActiveImport (t_top_kWh)", "1", reading.ActiveImport)
	requireDecimalEqual(t, "ReactiveInductiveImport (t_ri_kVarh)", "2", reading.ReactiveInductiveImport)
	requireDecimalEqual(t, "ReactiveCapacitiveImport (t_rc_kVarh)", "3", reading.ReactiveCapacitiveImport)
	requireDecimalEqual(t, "T1Import (t_t1_kWh)", "4", reading.T1Import)
	requireDecimalEqual(t, "T2Import (t_t2_kWh)", "5", reading.T2Import)
	requireDecimalEqual(t, "T3Import (t_t3_kWh)", "6", reading.T3Import)
	requireDecimalEqual(t, "MaxDemandKw (t_p_kW)", "7", reading.MaxDemandKw)
	requireDecimalEqual(t, "ActiveExport (u_top_kWh)", "8", reading.ActiveExport)
	requireDecimalEqual(t, "ReactiveInductiveExport (u_ri_kVarh)", "9", reading.ReactiveInductiveExport)
	requireDecimalEqual(t, "ReactiveCapacitiveExport (u_rc_kVarh)", "10", reading.ReactiveCapacitiveExport)
	requireDecimalEqual(t, "T1Export (u_u1_kWh)", "11", reading.T1Export)
	requireDecimalEqual(t, "T2Export (u_u2_kWh)", "12", reading.T2Export)
	requireDecimalEqual(t, "T3Export (u_u3_kWh)", "13", reading.T3Export)
}

func requireDecimalEqual(t *testing.T, label, want string, got *decimal.Decimal) {
	t.Helper()
	require.NotNil(t, got, "%s: expected %s, got nil", label, want)
	require.True(t, decimal.RequireFromString(want).Equal(*got), "%s: expected %s, got %s", label, want, got.String())
}
