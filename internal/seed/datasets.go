// Package seed embeds the F1 reference-data seed datasets and decodes them
// into typed, validated Go values.
//
// This is Task 12a: it produces the reviewed data files, the Go types, and
// the parsing/validation that turns embedded JSON into values a loader can
// insert. It performs NO I/O beyond reading the embedded bytes and touches no
// database — Task 12b's loader is the thing that takes the values returned
// here and writes them through internal/store/postgres/admin.
//
// Every decimal in the JSON is a STRING, decoded with decimal.NewFromString.
// A JSON number would round-trip through float64 during unmarshalling, which
// is exactly the defect internal/store/postgres/numeric.go exists to keep out
// of the money and energy paths; the seed data is not exempt just because it
// ships as a file instead of a database row.
package seed

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
)

//go:embed data/emission_factors.json
var emissionFactorsData []byte

//go:embed data/integration_definitions.json
var integrationDefinitionsData []byte

//go:embed data/national_tariff_schedule.json
var nationalTariffScheduleData []byte

// decodeStrict unmarshals raw into dst, rejecting any object field, at any
// nesting level dst itself decodes, that dst does not declare.
// json.Unmarshal accepts unknown fields silently;
// json.Decoder.DisallowUnknownFields is the only way to reject them, so every
// dataset in this package decodes through this helper rather than through
// json.Unmarshal directly. (A field typed json.RawMessage, such as
// IntegrationDefinition.Endpoints, is captured as raw bytes and is
// deliberately NOT checked by this — that field's shape is the provider's
// jsonb, not ours to constrain.)
func decodeStrict(raw []byte, dst any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if dec.More() {
		return fmt.Errorf("unexpected trailing data after the JSON value")
	}
	return nil
}

// fitsNumeric reports whether d can be written to a column declared
// numeric(precision, scale) without Postgres's typmod silently rounding it on
// insert — i.e. without the stored value differing from this JSON.
func fitsNumeric(d decimal.Decimal, precision, scale int32) bool {
	if !d.Round(scale).Equal(d) {
		return false // more fractional digits than the column's scale allows
	}
	digits := strings.TrimPrefix(d.StringFixed(scale), "-")
	digits = strings.Replace(digits, ".", "", 1)
	return int32(len(digits)) <= precision
}

// parseNumeric decodes a required decimal string and checks it fits
// numeric(precision, scale). field is used only to build error messages.
func parseNumeric(field, value string, precision, scale int32) (decimal.Decimal, error) {
	d, err := decimal.NewFromString(value)
	if err != nil {
		return decimal.Decimal{}, fmt.Errorf("%s %q is not a decimal: %w", field, value, err)
	}
	if !fitsNumeric(d, precision, scale) {
		return decimal.Decimal{}, fmt.Errorf("%s %q does not fit numeric(%d,%d)", field, value, precision, scale)
	}
	return d, nil
}

// parseOptionalNumeric is parseNumeric for a nullable column: a nil pointer
// stays nil rather than becoming zero.
func parseOptionalNumeric(field string, value *string, precision, scale int32) (*decimal.Decimal, error) {
	if value == nil {
		return nil, nil
	}
	d, err := parseNumeric(field, *value, precision, scale)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// ---------------------------------------------------------------------------
// Emission factor master catalogue — mirrors `emission_factors` and
// `emission_factor_conversions` (migration 00007_carbon_iso.sql).
// ---------------------------------------------------------------------------

// EmissionFactorConversion is one unit conversion for a factor. Mirrors table
// `emission_factor_conversions`, minus FactorID: the loader assigns that
// after inserting the parent factor and learning its generated id.
type EmissionFactorConversion struct {
	Unit       string
	Multiplier decimal.Decimal
	Label      string
}

// EmissionFactor is one platform master-catalogue entry: CompanyID is always
// nil for a seeded row, meaning the platform catalogue rather than a
// company's override — the loader must never touch a row whose CompanyID is
// set. ID, CreatedAt and UpdatedAt are the database's to assign and so are
// not part of this type.
type EmissionFactor struct {
	Key           string
	Label         string
	MainCategory  string
	SubCategories []string
	CategoryPath  []string
	BaseFactor    decimal.Decimal
	BaseUnit      string
	FuelType      *string
	VehicleType   *string
	Scope         *model.CarbonScope
	// IsoCategory is filled from the legacy per-entry `category` field
	// (EmissionCategory, constants.ts:550-557) on every entry. Provenance
	// correction (task 12b): an earlier version of this package's
	// documentation (see the task-12a report) said no legacy structure maps
	// a GHG sub-clause to an ISO 14064 category, based only on reading the
	// i18n display text in tr.json. That was incomplete — the legacy
	// codebase DOES carry a structured mapping,
	// `GHG_ISO_MAPPING: Record<SubCategory, {scope, category}>`
	// (bcem-energy/src/utils/types.ts:104-220) — and cross-checking it
	// confirms every (scope, category) pair used across the 182 shipped
	// entries agrees with what GHG_ISO_MAPPING assigns to that entry's
	// SubCategory (e.g. BUSINESS_TRAVEL/EMPLOYEE_COMMUTING and both freight
	// directions all map to scope_3/category_3, matching the 105
	// scope_3/category_3 entries; PURCHASED_GOODS/CAPITAL_GOODS to
	// scope_3/category_4, matching 16; END_OF_LIFE_SOLD to
	// scope_3/category_5, matching 11; WASTE_DISPOSAL to scope_3/category_6,
	// matching 5). So the per-entry `category` field used to fill this
	// column is independently validated by GHG_ISO_MAPPING, not merely "the
	// only value available" — the two legacy sources agree.
	IsoCategory *string
	Status      *string
	Source      *string
	SourceYear  *int16
	SourceURL   *string
	Conversions []EmissionFactorConversion
}

type jsonEmissionFactorConversion struct {
	Unit       string `json:"unit"`
	Multiplier string `json:"multiplier"`
	Label      string `json:"label"`
}

type jsonEmissionFactor struct {
	Key           string                         `json:"key"`
	Label         string                         `json:"label"`
	MainCategory  string                         `json:"main_category"`
	SubCategories []string                       `json:"sub_categories"`
	CategoryPath  []string                       `json:"category_path"`
	BaseFactor    string                         `json:"base_factor"`
	BaseUnit      string                         `json:"base_unit"`
	FuelType      *string                        `json:"fuel_type"`
	VehicleType   *string                        `json:"vehicle_type"`
	Scope         *string                        `json:"scope"`
	IsoCategory   *string                        `json:"iso_category"`
	Status        *string                        `json:"status"`
	Source        *string                        `json:"source"`
	SourceYear    *int16                         `json:"source_year"`
	SourceURL     *string                        `json:"source_url"`
	Conversions   []jsonEmissionFactorConversion `json:"conversions"`
}

// EmissionFactors decodes and validates the embedded platform emission-factor
// master catalogue (internal/seed/data/emission_factors.json). It never
// returns a partially valid result: any decode or validation failure comes
// back as a single wrapped error and a nil slice.
func EmissionFactors() ([]EmissionFactor, error) {
	return parseEmissionFactors(emissionFactorsData)
}

func parseEmissionFactors(raw []byte) ([]EmissionFactor, error) {
	var rows []jsonEmissionFactor
	if err := decodeStrict(raw, &rows); err != nil {
		return nil, fmt.Errorf("emission_factors: %w", err)
	}

	out := make([]EmissionFactor, 0, len(rows))
	seenKeys := make(map[string]int, len(rows))
	for i, r := range rows {
		if r.Key == "" {
			return nil, fmt.Errorf("emission_factors[%d]: key is required", i)
		}
		if r.Label == "" {
			return nil, fmt.Errorf("emission_factors[%d] (%s): label is required", i, r.Key)
		}
		if r.MainCategory == "" {
			return nil, fmt.Errorf("emission_factors[%d] (%s): main_category is required", i, r.Key)
		}
		if r.BaseUnit == "" {
			return nil, fmt.Errorf("emission_factors[%d] (%s): base_unit is required", i, r.Key)
		}

		// Natural key: the unique index is on
		// (coalesce(company_id, nil-uuid), key). Every seeded row has a nil
		// company_id (the platform catalogue), so key alone must be unique
		// among them.
		if prev, dup := seenKeys[r.Key]; dup {
			return nil, fmt.Errorf(
				"emission_factors: duplicate key %q at rows %d and %d", r.Key, prev, i)
		}
		seenKeys[r.Key] = i

		baseFactor, err := parseNumeric(
			fmt.Sprintf("emission_factors[%d] (%s): base_factor", i, r.Key), r.BaseFactor, 18, 8)
		if err != nil {
			return nil, err
		}

		var scope *model.CarbonScope
		if r.Scope != nil {
			s := model.CarbonScope(*r.Scope)
			if !s.Valid() {
				return nil, fmt.Errorf(
					"emission_factors[%d] (%s): scope %q is not a valid carbon_scope",
					i, r.Key, *r.Scope)
			}
			scope = &s
		}

		conversions := make([]EmissionFactorConversion, 0, len(r.Conversions))
		seenUnits := make(map[string]bool, len(r.Conversions))
		for j, c := range r.Conversions {
			if c.Unit == "" {
				return nil, fmt.Errorf(
					"emission_factors[%d] (%s): conversions[%d]: unit is required", i, r.Key, j)
			}
			// emission_factor_conversions' primary key is (factor_id, unit).
			if seenUnits[c.Unit] {
				return nil, fmt.Errorf(
					"emission_factors[%d] (%s): duplicate conversion unit %q", i, r.Key, c.Unit)
			}
			seenUnits[c.Unit] = true

			mult, err := parseNumeric(
				fmt.Sprintf("emission_factors[%d] (%s): conversions[%d]: multiplier", i, r.Key, j),
				c.Multiplier, 18, 8)
			if err != nil {
				return nil, err
			}
			conversions = append(conversions, EmissionFactorConversion{
				Unit:       c.Unit,
				Multiplier: mult,
				Label:      c.Label,
			})
		}

		out = append(out, EmissionFactor{
			Key:           r.Key,
			Label:         r.Label,
			MainCategory:  r.MainCategory,
			SubCategories: r.SubCategories,
			CategoryPath:  r.CategoryPath,
			BaseFactor:    baseFactor,
			BaseUnit:      r.BaseUnit,
			FuelType:      r.FuelType,
			VehicleType:   r.VehicleType,
			Scope:         scope,
			IsoCategory:   r.IsoCategory,
			Status:        r.Status,
			Source:        r.Source,
			SourceYear:    r.SourceYear,
			SourceURL:     r.SourceURL,
			Conversions:   conversions,
		})
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Integration provider definitions — mirrors `integration_definitions`
// (migration 00008_operations.sql).
// ---------------------------------------------------------------------------

// IntegrationDefinition is the platform catalogue entry for one provider and
// subtype. Endpoints is the provider's own jsonb shape and is passed through
// unvalidated beyond being well-formed JSON: this package constrains what IT
// states (provider, subtype), not the provider-specific endpoint schema.
type IntegrationDefinition struct {
	Provider  model.IntegrationProvider
	Subtype   string
	Endpoints json.RawMessage
}

type jsonIntegrationDefinition struct {
	Provider  string          `json:"provider"`
	Subtype   string          `json:"subtype"`
	Endpoints json.RawMessage `json:"endpoints"`
}

// IntegrationDefinitions decodes and validates the embedded integration
// provider catalogue (internal/seed/data/integration_definitions.json).
func IntegrationDefinitions() ([]IntegrationDefinition, error) {
	return parseIntegrationDefinitions(integrationDefinitionsData)
}

func parseIntegrationDefinitions(raw []byte) ([]IntegrationDefinition, error) {
	var rows []jsonIntegrationDefinition
	if err := decodeStrict(raw, &rows); err != nil {
		return nil, fmt.Errorf("integration_definitions: %w", err)
	}

	out := make([]IntegrationDefinition, 0, len(rows))
	type natKey struct{ provider, subtype string }
	seen := make(map[natKey]int, len(rows))
	for i, r := range rows {
		provider := model.IntegrationProvider(r.Provider)
		if !provider.Valid() {
			return nil, fmt.Errorf(
				"integration_definitions[%d]: provider %q is not a valid integration_provider",
				i, r.Provider)
		}
		if r.Subtype == "" {
			return nil, fmt.Errorf("integration_definitions[%d]: subtype is required", i)
		}

		k := natKey{r.Provider, r.Subtype}
		if prev, dup := seen[k]; dup {
			return nil, fmt.Errorf(
				"integration_definitions: duplicate (provider, subtype) (%q, %q) at rows %d and %d",
				r.Provider, r.Subtype, prev, i)
		}
		seen[k] = i

		if len(r.Endpoints) == 0 {
			return nil, fmt.Errorf("integration_definitions[%d]: endpoints is required", i)
		}
		if !json.Valid(r.Endpoints) {
			return nil, fmt.Errorf("integration_definitions[%d]: endpoints is not valid JSON", i)
		}

		out = append(out, IntegrationDefinition{
			Provider:  provider,
			Subtype:   r.Subtype,
			Endpoints: append(json.RawMessage(nil), r.Endpoints...),
		})
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// National tariff schedule — mirrors `national_tariff_schedule`
// (migration 00006_tariffs.sql). The shipped data file is an empty array (see
// internal/seed/data/national_tariff_schedule.json); the source data lives
// only in the legacy MongoDB and is not in this repository. The parser below
// is exercised by internal/seed/testdata/national_tariff_schedule_fixture.json
// so that its validation is proven against real rows, not only against an
// empty file.
// ---------------------------------------------------------------------------

// NationalTariffScheduleEntry mirrors table `national_tariff_schedule`,
// minus ID and CreatedAt, which the database assigns.
type NationalTariffScheduleEntry struct {
	EffectiveFrom     time.Time
	UserGroup         model.DistributionUserGroup
	VoltageLevel      model.VoltageLevel
	Term              model.TariffTerm
	EnergyPrice       decimal.Decimal
	T1Price           *decimal.Decimal
	T2Price           *decimal.Decimal
	T3Price           *decimal.Decimal
	DistributionPrice decimal.Decimal
	PowerPrice        *decimal.Decimal
	OverusePrice      *decimal.Decimal
	DailyThresholdKwh *decimal.Decimal
	VatRate           decimal.Decimal
	Source            *string
}

type jsonNationalTariffScheduleEntry struct {
	EffectiveFrom     string  `json:"effective_from"`
	UserGroup         string  `json:"user_group"`
	VoltageLevel      string  `json:"voltage_level"`
	Term              string  `json:"term"`
	EnergyPrice       string  `json:"energy_price"`
	T1Price           *string `json:"t1_price"`
	T2Price           *string `json:"t2_price"`
	T3Price           *string `json:"t3_price"`
	DistributionPrice string  `json:"distribution_price"`
	PowerPrice        *string `json:"power_price"`
	OverusePrice      *string `json:"overuse_price"`
	DailyThresholdKwh *string `json:"daily_threshold_kwh"`
	VatRate           string  `json:"vat_rate"`
	Source            *string `json:"source"`
}

// NationalTariffSchedule decodes and validates the embedded national tariff
// schedule (internal/seed/data/national_tariff_schedule.json). The shipped
// file is an empty array, so this returns an empty, non-nil slice and a nil
// error; ekokod seed reports "national tariff schedule: 0 rows (source data
// not in repository)" rather than treating the empty result as a failure.
func NationalTariffSchedule() ([]NationalTariffScheduleEntry, error) {
	return parseNationalTariffSchedule(nationalTariffScheduleData)
}

func parseNationalTariffSchedule(raw []byte) ([]NationalTariffScheduleEntry, error) {
	var rows []jsonNationalTariffScheduleEntry
	if err := decodeStrict(raw, &rows); err != nil {
		return nil, fmt.Errorf("national_tariff_schedule: %w", err)
	}

	out := make([]NationalTariffScheduleEntry, 0, len(rows))
	type natKey struct{ effectiveFrom, userGroup, voltageLevel, term string }
	seen := make(map[natKey]int, len(rows))
	for i, r := range rows {
		effectiveFrom, err := time.Parse("2006-01-02", r.EffectiveFrom)
		if err != nil {
			return nil, fmt.Errorf(
				"national_tariff_schedule[%d]: effective_from %q: %w", i, r.EffectiveFrom, err)
		}

		userGroup := model.DistributionUserGroup(r.UserGroup)
		if !userGroup.Valid() {
			return nil, fmt.Errorf(
				"national_tariff_schedule[%d]: user_group %q is not a valid distribution_user_group",
				i, r.UserGroup)
		}
		voltageLevel := model.VoltageLevel(r.VoltageLevel)
		if !voltageLevel.Valid() {
			return nil, fmt.Errorf(
				"national_tariff_schedule[%d]: voltage_level %q is not a valid voltage_level",
				i, r.VoltageLevel)
		}
		term := model.TariffTerm(r.Term)
		if !term.Valid() {
			return nil, fmt.Errorf(
				"national_tariff_schedule[%d]: term %q is not a valid tariff_term", i, r.Term)
		}

		// The unique index is (effective_from, user_group, voltage_level, term).
		k := natKey{r.EffectiveFrom, r.UserGroup, r.VoltageLevel, r.Term}
		if prev, dup := seen[k]; dup {
			return nil, fmt.Errorf(
				"national_tariff_schedule: duplicate (effective_from, user_group, voltage_level, term) "+
					"(%q, %q, %q, %q) at rows %d and %d",
				r.EffectiveFrom, r.UserGroup, r.VoltageLevel, r.Term, prev, i)
		}
		seen[k] = i

		prefix := fmt.Sprintf("national_tariff_schedule[%d]", i)
		energyPrice, err := parseNumeric(prefix+": energy_price", r.EnergyPrice, 18, 6)
		if err != nil {
			return nil, err
		}
		distributionPrice, err := parseNumeric(prefix+": distribution_price", r.DistributionPrice, 18, 6)
		if err != nil {
			return nil, err
		}
		vatRate, err := parseNumeric(prefix+": vat_rate", r.VatRate, 6, 3)
		if err != nil {
			return nil, err
		}
		t1Price, err := parseOptionalNumeric(prefix+": t1_price", r.T1Price, 18, 6)
		if err != nil {
			return nil, err
		}
		t2Price, err := parseOptionalNumeric(prefix+": t2_price", r.T2Price, 18, 6)
		if err != nil {
			return nil, err
		}
		t3Price, err := parseOptionalNumeric(prefix+": t3_price", r.T3Price, 18, 6)
		if err != nil {
			return nil, err
		}
		powerPrice, err := parseOptionalNumeric(prefix+": power_price", r.PowerPrice, 18, 6)
		if err != nil {
			return nil, err
		}
		overusePrice, err := parseOptionalNumeric(prefix+": overuse_price", r.OverusePrice, 18, 6)
		if err != nil {
			return nil, err
		}
		dailyThresholdKwh, err := parseOptionalNumeric(prefix+": daily_threshold_kwh", r.DailyThresholdKwh, 12, 3)
		if err != nil {
			return nil, err
		}

		out = append(out, NationalTariffScheduleEntry{
			EffectiveFrom:     effectiveFrom,
			UserGroup:         userGroup,
			VoltageLevel:      voltageLevel,
			Term:              term,
			EnergyPrice:       energyPrice,
			T1Price:           t1Price,
			T2Price:           t2Price,
			T3Price:           t3Price,
			DistributionPrice: distributionPrice,
			PowerPrice:        powerPrice,
			OverusePrice:      overusePrice,
			DailyThresholdKwh: dailyThresholdKwh,
			VatRate:           vatRate,
			Source:            r.Source,
		})
	}
	return out, nil
}
