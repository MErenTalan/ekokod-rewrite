package seed

import (
	"os"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
)

// wantEmissionFactorCount is the number of entries in the legacy
// `emission_fac` array (constants.ts, lines 925-5608 of
// bcem-energy/src/app/components/carbon-footprint/constants.ts as it stood
// when this file was converted). Counted by parsing the TS array into JSON
// with a throwaway Node script (kept out of the repository per the task
// brief) and taking len() of the result; cross-checked by counting
// `key:` occurrences in that line range and by confirming every one of the
// 182 keys is unique, which this test also asserts (F9 adds four R293
// equivalence rows on top). This is the REAL count:
// the spec (docs/rewrite/04-data-model.md §13) describes the catalogue as
// "thousands of entries", which this survey does not confirm — see the
// task-12a report.
const wantEmissionFactorCount = 186

// wantEquivalenceCount rows are F9's renewable equivalences (R293): reference
// values, not emission factors, so they carry no ISO 14064 category.
const wantEquivalenceCount = 4

// wantIntegrationDefinitionCount: the legacy code hardcodes exactly the three
// iSolar regions (ISOLAR_REGIONS in
// bcem-energy/src/utils/isolar/isolarPoints.ts, lines 197-214) as a closed,
// literal set of gateway/endpoint templates. GRIDBOX, OSOS, ARIL and PM5340
// have no equivalent hardcoded subtype or endpoint data anywhere in the
// legacy code: their `Integration` Mongo documents are entirely
// admin-authored at runtime (see task-12a report), so seeding a row for them
// would be inventing data the brief forbids.
const wantIntegrationDefinitionCount = 3

func TestEmissionFactorsDecodeAndValidate(t *testing.T) {
	t.Parallel()
	factors, err := EmissionFactors()
	require.NoError(t, err)
	require.Len(t, factors, wantEmissionFactorCount)

	seenKeys := make(map[string]bool, len(factors))
	isoFilled, isoNull := 0, 0
	for _, f := range factors {
		require.NotEmpty(t, f.Key)
		require.NotEmpty(t, f.Label)
		require.NotEmpty(t, f.MainCategory)
		require.NotEmpty(t, f.BaseUnit)
		require.False(t, seenKeys[f.Key], "duplicate key %q", f.Key)
		seenKeys[f.Key] = true

		// base_factor is numeric(18,8): every parsed value must already have
		// passed fitsNumeric, but re-assert the invariant directly here so a
		// future change to parseNumeric's call site can't silently loosen it.
		require.True(t, fitsNumeric(f.BaseFactor, 18, 8), "base_factor %s for %s", f.BaseFactor, f.Key)

		if f.Scope != nil {
			require.True(t, f.Scope.Valid(), "scope %q for %s", *f.Scope, f.Key)
		}
		if f.IsoCategory != nil {
			isoFilled++
		} else {
			isoNull++
		}

		for _, c := range f.Conversions {
			require.NotEmpty(t, c.Unit)
			require.True(t, fitsNumeric(c.Multiplier, 18, 8), "conversion multiplier for %s/%s", f.Key, c.Unit)
		}
	}

	// iso_category is filled from the legacy `category` field on every
	// entry (an explicit per-entry ISO 14064 category, category_1..
	// category_6 — see the task-12a report for why this is used directly
	// rather than derived from the coarser GHG scope). All 182 legacy entries
	// carry it; only F9's equivalence rows (R293) have none.
	require.Equal(t, wantEmissionFactorCount-wantEquivalenceCount, isoFilled, "iso_category filled count")
	require.Equal(t, wantEquivalenceCount, isoNull, "iso_category NULL count: the equivalence rows only")
}

func TestEmissionFactorsRejectsDuplicateKey(t *testing.T) {
	t.Parallel()
	broken := mustReplaceFirst(t, emissionFactorsData,
		`"key": "stationary_space_heating_coal_industrial",`,
		`"key": "stationary_space_heating_coal_domestic",`)

	_, err := parseEmissionFactors(broken)
	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicate key")
	t.Logf("got expected failure: %v", err)
}

func TestEmissionFactorsRejectsInvalidScope(t *testing.T) {
	t.Parallel()
	broken := mustReplaceFirst(t, emissionFactorsData, `"scope": "scope_1",`, `"scope": "scope_9",`)

	_, err := parseEmissionFactors(broken)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not a valid carbon_scope")
	t.Logf("got expected failure: %v", err)
}

func TestEmissionFactorsRejectsScaleOverflow(t *testing.T) {
	t.Parallel()
	// numeric(18,8): nine fractional digits is one more than the column
	// allows. This is not a float-vs-string question (both "2.904" and
	// "2.1234567890" are valid decimal STRINGS); it's a precision question,
	// which is exactly why fitsNumeric exists as a check independent of
	// decimal.NewFromString succeeding.
	broken := mustReplaceFirst(t, emissionFactorsData, `"base_factor": "2.904",`, `"base_factor": "2.123456789",`)

	_, err := parseEmissionFactors(broken)
	require.Error(t, err)
	require.Contains(t, err.Error(), "does not fit numeric(18,8)")
	t.Logf("got expected failure: %v", err)
}

func TestEmissionFactorsRejectsUnknownField(t *testing.T) {
	t.Parallel()
	broken := mustReplaceFirst(t, emissionFactorsData,
		`"key": "stationary_space_heating_coal_domestic",`,
		`"key": "stationary_space_heating_coal_domestic", "not_a_real_column": "surprise",`)

	_, err := parseEmissionFactors(broken)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not_a_real_column")
	t.Logf("got expected failure: %v", err)
}

func TestEmissionFactorsRejectsMissingRequiredField(t *testing.T) {
	t.Parallel()
	broken := mustReplaceFirst(t, emissionFactorsData,
		`"key": "stationary_space_heating_coal_domestic",`, `"key": "",`)

	_, err := parseEmissionFactors(broken)
	require.Error(t, err)
	require.Contains(t, err.Error(), "key is required")
	t.Logf("got expected failure: %v", err)
}

func TestIntegrationDefinitionsDecodeAndValidate(t *testing.T) {
	t.Parallel()
	defs, err := IntegrationDefinitions()
	require.NoError(t, err)
	require.Len(t, defs, wantIntegrationDefinitionCount)

	type key struct{ provider, subtype string }
	seen := make(map[key]bool, len(defs))
	for _, d := range defs {
		require.True(t, d.Provider.Valid(), "provider %q", d.Provider)
		require.Equal(t, model.IntegrationProviderISolar, d.Provider, "task-12a seeds only iSolar; see report")
		require.NotEmpty(t, d.Subtype)
		require.NotEmpty(t, d.Endpoints)

		k := key{string(d.Provider), d.Subtype}
		require.False(t, seen[k], "duplicate (provider, subtype) %+v", k)
		seen[k] = true
	}
}

func TestIntegrationDefinitionsRejectsInvalidProvider(t *testing.T) {
	t.Parallel()
	broken := mustReplaceFirst(t, integrationDefinitionsData, `"provider": "isolar",`, `"provider": "not_a_provider",`)

	_, err := parseIntegrationDefinitions(broken)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not a valid integration_provider")
	t.Logf("got expected failure: %v", err)
}

func TestIntegrationDefinitionsRejectsDuplicateNaturalKey(t *testing.T) {
	t.Parallel()
	broken := mustReplaceFirst(t, integrationDefinitionsData, `"subtype": "CN",`, `"subtype": "EU",`)

	_, err := parseIntegrationDefinitions(broken)
	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicate (provider, subtype)")
	t.Logf("got expected failure: %v", err)
}

func TestIntegrationDefinitionsRejectsUnknownField(t *testing.T) {
	t.Parallel()
	broken := mustReplaceFirst(t, integrationDefinitionsData, `"subtype": "EU",`, `"subtype": "EU", "bogus_field": true,`)

	_, err := parseIntegrationDefinitions(broken)
	require.Error(t, err)
	require.Contains(t, err.Error(), "bogus_field")
	t.Logf("got expected failure: %v", err)
}

// TestNationalTariffScheduleShipsLegacyCalculatorTable pins F12a Q-H1/Q-H2:
// the shipped schedule is the legacy public calculator's active constant
// table, 17 rows, with the low/high daily-use prices as base and *_plus rows.
func TestNationalTariffScheduleShipsLegacyCalculatorTable(t *testing.T) {
	t.Parallel()
	rows, err := NationalTariffSchedule()
	require.NoError(t, err)
	require.Len(t, rows, 17)
	find := func(group, level, term string) NationalTariffScheduleEntry {
		for _, r := range rows {
			if string(r.UserGroup) == group && string(r.VoltageLevel) == level && string(r.Term) == term {
				return r
			}
		}
		t.Fatalf("no row %s/%s/%s", group, level, term)
		return NationalTariffScheduleEntry{}
	}
	res := find("residential", "lv", "monomial")
	require.Equal(t, "0.494065", res.EnergyPrice.StringFixed(6))
	require.Equal(t, "8", res.DailyThresholdKwh.String())
	require.Equal(t, "1.895808", find("residential_plus", "lv", "monomial").EnergyPrice.StringFixed(6))
	com := find("commercial", "lv", "monomial")
	require.Equal(t, "30", com.DailyThresholdKwh.String())
	require.Equal(t, "3.454688", find("commercial_plus", "lv", "monomial").EnergyPrice.StringFixed(6))
	bi := find("commercial", "mv", "binomial")
	require.Equal(t, "89.14752", bi.PowerPrice.String())
	require.Equal(t, "178.29504", bi.OverusePrice.String())
	require.Nil(t, find("lighting", "lv", "monomial").T1Price, "lighting has no time-of-use prices")
	for _, r := range rows {
		require.Equal(t, "20", r.VatRate.String())
		require.Equal(t, "2025-01-01", r.EffectiveFrom.Format("2006-01-02"))
	}
}

// TestNationalTariffScheduleFixtureDecodesAndValidates exercises the SAME
// parser against internal/seed/testdata/national_tariff_schedule_fixture.json
// — small, obviously-synthetic rows — because the shipped production file is
// empty and cannot exercise multi-row validation (duplicate natural keys,
// enum checks against real values, decimal precision) on its own. Task 12b's
// loader tests read this fixture directly; this test proves the fixture
// itself is well-formed and that this package's parser handles real rows, not
// only the empty case.
func TestNationalTariffScheduleFixtureDecodesAndValidates(t *testing.T) {
	t.Parallel()
	raw := mustReadFile(t, "testdata/national_tariff_schedule_fixture.json")

	rows, err := parseNationalTariffSchedule(raw)
	require.NoError(t, err)
	require.Len(t, rows, 3)

	type key struct {
		effectiveFrom, userGroup, voltageLevel, term string
	}
	seen := make(map[key]bool, len(rows))
	for _, r := range rows {
		require.True(t, r.UserGroup.Valid())
		require.True(t, r.VoltageLevel.Valid())
		require.True(t, r.Term.Valid())
		require.True(t, fitsNumeric(r.EnergyPrice, 18, 6))
		require.True(t, fitsNumeric(r.DistributionPrice, 18, 6))
		require.True(t, fitsNumeric(r.VatRate, 6, 3))

		k := key{r.EffectiveFrom.Format("2006-01-02"), string(r.UserGroup), string(r.VoltageLevel), string(r.Term)}
		require.False(t, seen[k], "duplicate natural key %+v", k)
		seen[k] = true
	}

	// The second fixture row exercises the nullable fields being non-nil.
	require.NotNil(t, rows[1].T1Price)
	require.True(t, decimal.RequireFromString("1.500000").Equal(*rows[1].T1Price))
	require.Nil(t, rows[0].T1Price)
}

func TestNationalTariffScheduleFixtureRejectsDuplicateNaturalKey(t *testing.T) {
	t.Parallel()
	raw := mustReadFile(t, "testdata/national_tariff_schedule_fixture.json")
	broken := mustReplaceFirst(t, raw, `"effective_from": "2021-06-01",`, `"effective_from": "2020-01-01",`)

	_, err := parseNationalTariffSchedule(broken)
	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicate (effective_from, user_group, voltage_level, term)")
	t.Logf("got expected failure: %v", err)
}

func TestNationalTariffScheduleFixtureRejectsInvalidEnum(t *testing.T) {
	t.Parallel()
	raw := mustReadFile(t, "testdata/national_tariff_schedule_fixture.json")
	broken := mustReplaceFirst(t, raw, `"voltage_level": "lv",`, `"voltage_level": "extra_high_voltage",`)

	_, err := parseNationalTariffSchedule(broken)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not a valid voltage_level")
	t.Logf("got expected failure: %v", err)
}

func TestNationalTariffScheduleFixtureRejectsScaleOverflow(t *testing.T) {
	t.Parallel()
	raw := mustReadFile(t, "testdata/national_tariff_schedule_fixture.json")
	// numeric(6,3): four integer digits plus three fractional digits is 7
	// significant digits, one over precision 6.
	broken := mustReplaceFirst(t, raw, `"vat_rate": "18.000",`, `"vat_rate": "1234.567",`)

	_, err := parseNationalTariffSchedule(broken)
	require.Error(t, err)
	require.Contains(t, err.Error(), "does not fit numeric(6,3)")
	t.Logf("got expected failure: %v", err)
}

func TestNationalTariffScheduleFixtureRejectsUnknownField(t *testing.T) {
	t.Parallel()
	raw := mustReadFile(t, "testdata/national_tariff_schedule_fixture.json")
	broken := mustReplaceFirst(t, raw, `"vat_rate": "18.000",`, `"vat_rate": "18.000", "unexpected": "field",`)

	_, err := parseNationalTariffSchedule(broken)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unexpected")
	t.Logf("got expected failure: %v", err)
}

// TestFitsNumeric is a focused unit test on the precision/scale guard itself,
// independent of any dataset, so a future change to it is caught even if
// every shipped value happens to stay within bounds.
func TestFitsNumeric(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		value     string
		precision int32
		scale     int32
		want      bool
	}{
		{"exact fit", "123456789012.345678", 18, 6, true},
		{"too many fractional digits", "1.1234567", 18, 6, false},
		{"too many total digits", "1234567890123.456789", 18, 6, false}, // 19 significant digits
		{"negative sign is not a digit", "-1.500000", 18, 6, true},
		{"trailing zero at scale is fine", "1.500", 18, 6, true},
		{"zero", "0", 18, 6, true},
		{"fewer fractional digits than scale", "1", 6, 3, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d := decimal.RequireFromString(tc.value)
			require.Equal(t, tc.want, fitsNumeric(d, tc.precision, tc.scale))
		})
	}
}

// --- test helpers -----------------------------------------------------------

// mustReplaceFirst returns a COPY of raw with the first occurrence of old
// replaced by new, failing the test if old does not appear at all. Operating
// on a copy in memory (never on the embedded data or the files on disk) is
// what lets these "prove it fails" tests run as an ordinary part of
// `go test`, rather than as a manual edit-run-revert cycle against the real
// dataset files. Only the first occurrence is touched, deterministically,
// which is all a single-value corruption needs.
func mustReplaceFirst(t *testing.T, raw []byte, old, new string) []byte {
	t.Helper()
	s := string(raw)
	require.Contains(t, s, old, "fixture string %q not found", old)
	return []byte(strings.Replace(s, old, new, 1))
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	return raw
}

// R293: the renewable panel's equivalences are seeded platform rows with a
// source and a year, so a missing or changed factor is data, not code.
func TestEquivalenceFactorsAreSeededWithASource(t *testing.T) {
	factors, err := EmissionFactors()
	require.NoError(t, err)
	byKey := map[string]EmissionFactor{}
	for _, f := range factors {
		byKey[f.Key] = f
	}
	for _, key := range []string{"equiv_tree_co2_kg_per_year", "equiv_coal_kg_per_kwh", "equiv_car_co2_kg_per_km", "equiv_home_heating_kwh_per_year"} {
		f, ok := byKey[key]
		require.True(t, ok, key)
		require.Equal(t, "equivalence", f.MainCategory, key)
		require.NotNil(t, f.Source, key)
		require.NotNil(t, f.SourceYear, key)
		require.True(t, f.BaseFactor.IsPositive(), key)
	}
}
