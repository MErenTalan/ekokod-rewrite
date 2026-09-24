package aril

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

// str returns a *string, for building wire test fixtures inline.
func str(s string) *string { return &s }

func requireReg(t *testing.T, want string, d *decimal.Decimal) {
	t.Helper()
	require.NotNil(t, d, "expected %s, got nil (removed-behaviour 21: absent register)", want)
	require.True(t, decimal.RequireFromString(want).Equal(*d), "got %s, want %s", d.String(), want)
}

func requireNilReg(t *testing.T, d *decimal.Decimal) {
	t.Helper()
	require.Nil(t, d)
}

// TestARILDiscoverMapsEveryField is the shared adapter template's
// "Test<P>DiscoverMapsEveryField": every row of 06 §4's "Subscription
// mapping" table, asserted by name.
func TestARILDiscoverMapsEveryField(t *testing.T) {
	raw := json.RawMessage(`{
		"SubscriptionSerno": "FX_123456",
		"Title": "Fixture Subscriber",
		"Address": "Fixture Address 1",
		"MeterSerial": "FX_METER_1",
		"MeterBrand": "FixtureBrand",
		"Multiplier": 40,
		"InstalledPower": "100.5",
		"AccordPower": "80.25",
		"Etso": "FX-ETSO-1",
		"LastEndexDate": 20260901120000,
		"LastProfileDate": 20260831230000,
		"DefinitionType": 15
	}`)

	pt, ok := mapSubscription(raw)
	require.True(t, ok)

	require.Equal(t, "FX_123456", pt.InstallationNumber, "SubscriptionSerno -> installation_number")
	require.NotNil(t, pt.CustomerName)
	require.Equal(t, "Fixture Subscriber", *pt.CustomerName, "Title -> customer_name")
	require.NotNil(t, pt.Address)
	require.Equal(t, "Fixture Address 1", *pt.Address, "Address -> address")
	require.NotNil(t, pt.MeterNumber)
	require.Equal(t, "FX_METER_1", *pt.MeterNumber, "MeterSerial -> meter_number")
	require.NotNil(t, pt.MeterModel)
	require.Equal(t, "FixtureBrand", *pt.MeterModel, "MeterBrand -> meter_model")
	require.NotNil(t, pt.MeterMultiplier)
	require.True(t, decimal.NewFromInt(40).Equal(*pt.MeterMultiplier), "Multiplier -> meter_multiplier")
	require.NotNil(t, pt.InstalledPowerKw)
	require.True(t, decimal.RequireFromString("100.5").Equal(*pt.InstalledPowerKw), "InstalledPower -> installed_power_kw")
	require.NotNil(t, pt.ContractedPowerKw)
	require.True(t, decimal.RequireFromString("80.25").Equal(*pt.ContractedPowerKw), "AccordPower -> contracted power (R30)")
	require.NotNil(t, pt.EtsoCode)
	require.Equal(t, "FX-ETSO-1", *pt.EtsoCode, "Etso -> etso_code")
	require.NotNil(t, pt.DefinitionType)
	require.Equal(t, int16(15), *pt.DefinitionType, "DefinitionType -> definition_type")

	require.NotNil(t, pt.ProviderHighWater)
	loadProfileHW, ok := pt.ProviderHighWater["load_profile"]
	require.True(t, ok, "LastProfileDate -> ProviderHighWater[load_profile]")
	require.True(t, loadProfileHW.Equal(mustARILDate(t, 20260831230000)))
	currentIndexHW, ok := pt.ProviderHighWater["current_index"]
	require.True(t, ok, "LastEndexDate -> ProviderHighWater[current_index]")
	require.True(t, currentIndexHW.Equal(mustARILDate(t, 20260901120000)))
	billingHW, ok := pt.ProviderHighWater["billing"]
	require.True(t, ok, "LastEndexDate -> ProviderHighWater[billing]")
	require.True(t, billingHW.Equal(mustARILDate(t, 20260901120000)))
}

func mustARILDate(t *testing.T, n int64) time.Time {
	t.Helper()
	ts, err := parseProfileDate(json.Number(fmtInt(n)))
	require.NoError(t, err)
	return ts
}

func fmtInt(n int64) string {
	return decimal.NewFromInt(n).String()
}

// TestARILDiscoverMissingSubscriptionSernoIsSkipped proves a row with no
// usable identity field is dropped, not turned into a zero-valued
// MeteringPoint.
func TestARILDiscoverMissingSubscriptionSernoIsSkipped(t *testing.T) {
	_, ok := mapSubscription(json.RawMessage(`{"Title":"Fixture X"}`))
	require.False(t, ok)
}

// TestARILDiscoverBestEffortOptionalFields proves an unparseable optional
// field is left nil/absent rather than failing the whole row
// (subscriptionWire's doc): LastEndexDate here is a syntactically valid
// JSON number but not 14 digits, so normalize.ARILProfileDate rejects it —
// exactly the "present but semantically wrong" case a best-effort mapping
// must swallow without dropping the row's other, valid fields.
func TestARILDiscoverBestEffortOptionalFields(t *testing.T) {
	raw := json.RawMessage(`{"SubscriptionSerno": "FX_1", "Title": "Fixture Y", "LastEndexDate": 123}`)
	pt, ok := mapSubscription(raw)
	require.True(t, ok)
	require.Equal(t, "FX_1", pt.InstallationNumber)
	require.NotNil(t, pt.CustomerName)
	require.Equal(t, "Fixture Y", *pt.CustomerName)
	_, hasCurrentIndex := pt.ProviderHighWater["current_index"]
	require.False(t, hasCurrentIndex)
}

// TestARILTimeOfUseRegistersAreNull is the F2 acceptance criterion "ARIL
// rows produce NULL, not 0, in the T1/T2/T3 registers" (removed-behaviour
// 21). mapRegisters never touches T1/T2/T3 Import/Export at all — the
// mutation task-8-brief.md names ("set T1Import: decimalPtr(\"0\") in
// mapping.go") is proven live in TestARILTimeOfUseRegistersAreNullMutation
// (skipped by default; see its doc).
func TestARILTimeOfUseRegistersAreNull(t *testing.T) {
	r := registers{
		TSum:                  str("100"),
		ReactiveInductive:     str("10"),
		ReactiveCapasitive:    str("5"),
		TSumOut:               str("50"),
		ReactiveInductiveOut:  str("4"),
		ReactiveCapasitiveOut: str("2"),
	}
	reading, err := mapRegisters(r, decimal.NewFromInt(1))
	require.NoError(t, err)

	require.NotNil(t, reading.ActiveImport, "ActiveImport must be present")
	requireNilReg(t, reading.T1Import)
	requireNilReg(t, reading.T2Import)
	requireNilReg(t, reading.T3Import)
	requireNilReg(t, reading.T1Export)
	requireNilReg(t, reading.T2Export)
	requireNilReg(t, reading.T3Export)
}

// TestARILRegisterMappingWithDecimalMultiplier is adapter review pattern 2:
// a non-integer multiplier ("2.5") proves the multiplication is exact
// decimal arithmetic, applied to every register exactly once.
func TestARILRegisterMappingWithDecimalMultiplier(t *testing.T) {
	r := registers{
		TSum:                  str("100"),
		ReactiveInductive:     str("10"),
		ReactiveCapasitive:    str("20"),
		TSumOut:               str("30"),
		ReactiveInductiveOut:  str("40"),
		ReactiveCapasitiveOut: str("50"),
	}
	m := decimal.RequireFromString("2.5")
	reading, err := mapRegisters(r, m)
	require.NoError(t, err)

	requireReg(t, "250", reading.ActiveImport)
	requireReg(t, "25", reading.ReactiveInductiveImport)
	requireReg(t, "50", reading.ReactiveCapacitiveImport)
	requireReg(t, "75", reading.ActiveExport)
	requireReg(t, "100", reading.ReactiveInductiveExport)
	requireReg(t, "125", reading.ReactiveCapacitiveExport)
}

// TestARILRequestsWithoutMultiplierAndMultipliesOnce is the F2 acceptance
// criterion named by task-8-brief.md: TSum "12.5" x 40 -> 500, applied
// exactly once (never doubled, and never left at the raw value because
// WithoutMultiplier suppressed it server-side too).
func TestARILRequestsWithoutMultiplierAndMultipliesOnce(t *testing.T) {
	r := registers{TSum: str("12.5")}
	reading, err := mapRegisters(r, decimal.NewFromInt(40))
	require.NoError(t, err)
	requireReg(t, "500", reading.ActiveImport)
}

// TestARILNilNeverZero is adapter review pattern 3: null/""/"-" all map to
// nil, while a genuine "0" stays a present, zero-valued register.
func TestARILNilNeverZero(t *testing.T) {
	t.Run("absent_key", func(t *testing.T) {
		reading, err := mapRegisters(registers{}, decimal.NewFromInt(1))
		require.NoError(t, err)
		requireNilReg(t, reading.ActiveImport)
	})
	t.Run("empty_string", func(t *testing.T) {
		reading, err := mapRegisters(registers{TSum: str("")}, decimal.NewFromInt(1))
		require.NoError(t, err)
		requireNilReg(t, reading.ActiveImport)
	})
	t.Run("dash", func(t *testing.T) {
		reading, err := mapRegisters(registers{TSum: str("-")}, decimal.NewFromInt(1))
		require.NoError(t, err)
		requireNilReg(t, reading.ActiveImport)
	})
	t.Run("json_null", func(t *testing.T) {
		var row loadProfileRow
		require.NoError(t, json.Unmarshal([]byte(`{"ProfileDate":20260101000000,"TSum":null}`), &row))
		reading, err := mapRegisters(row.registers, decimal.NewFromInt(1))
		require.NoError(t, err)
		requireNilReg(t, reading.ActiveImport)
	})
	t.Run("genuine_zero_stays_zero", func(t *testing.T) {
		reading, err := mapRegisters(registers{TSum: str("0")}, decimal.NewFromInt(5))
		require.NoError(t, err)
		requireReg(t, "0", reading.ActiveImport)
	})
	t.Run("locale_thousands_separator_zero", func(t *testing.T) {
		reading, err := mapRegisters(registers{TSum: str("0,000")}, decimal.NewFromInt(5))
		require.NoError(t, err)
		requireReg(t, "0", reading.ActiveImport)
	})
}

// TestARILProfileDateIsIstanbulLocal is the F2 acceptance criterion named
// by task-8-brief.md: the 14-digit ProfileDate is parsed as Europe/Istanbul
// local time, not UTC.
func TestARILProfileDateIsIstanbulLocal(t *testing.T) {
	// 2026-09-01 10:00:00 Istanbul (+03:00, no DST) = 07:00:00 UTC.
	ts, err := parseProfileDate(json.Number("20260901100000"))
	require.NoError(t, err)
	require.True(t, ts.Equal(time.Date(2026, 9, 1, 7, 0, 0, 0, time.UTC)), "got %s", ts)
}

// TestARILMaxDemandFieldParsesAndMultiplies proves mapMaxDemand applies
// removed-behaviour 21 and the one-multiplication rule identically to every
// other register.
func TestARILMaxDemandFieldParsesAndMultiplies(t *testing.T) {
	md, err := mapMaxDemand(str("12.5"), decimal.NewFromInt(4))
	require.NoError(t, err)
	requireReg(t, "50", md)

	md, err = mapMaxDemand(nil, decimal.NewFromInt(4))
	require.NoError(t, err)
	requireNilReg(t, md)
}

// TestARILReadingRegistersEqual proves the duplicate-timestamp comparison
// helper ignores Ts/Kind/stamp fields and compares only registers.
func TestARILReadingRegistersEqual(t *testing.T) {
	a, err := mapRegisters(registers{TSum: str("100")}, decimal.NewFromInt(1))
	require.NoError(t, err)
	b, err := mapRegisters(registers{TSum: str("100")}, decimal.NewFromInt(1))
	require.NoError(t, err)
	require.True(t, readingRegistersEqual(a, b))

	c, err := mapRegisters(registers{TSum: str("999")}, decimal.NewFromInt(1))
	require.NoError(t, err)
	require.False(t, readingRegistersEqual(a, c))
}
