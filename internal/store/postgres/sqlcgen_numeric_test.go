package postgres

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The whole point of this file.
//
// Money and energy are `numeric` in SQL and must never reach Go as a float or
// an integer. sqlc gets that right for ordinary tables on its own, but the six
// continuous aggregates in 00005 are built on TimescaleDB functions
// (time_bucket, first, last) that sqlc's catalogue does not know. Left to
// guess, it produced `ActiveConsumption int32` for an energy quantity — a
// silent 32-bit truncation that no build, linter or migration test would have
// caught, and that would have surfaced as wrong kWh on a customer invoice.
//
// Two things prevent that: the explicit `::numeric` casts on every computed
// column in 00005, and internal/store/postgres/timescale-shims.sql, which
// declares the function signatures for sqlc's catalogue only. Both are easy to
// remove by accident — the casts look redundant (in Postgres they are), and the
// shim file looks like dead SQL that is never applied to a database. These
// tests are what makes removing either one fail loudly.
//
// The expectation is derived from the migrations themselves rather than from a
// hand-written list, so a table added in a later task is covered the moment its
// migration lands, with nothing to remember to update.

const generatedModelsPath = "sqlcgen/models.go"

var (
	commentRe    = regexp.MustCompile(`(?m)--.*$`)
	createTabRe  = regexp.MustCompile(`(?is)create table\s+(\w+)\s*\((.*?)\n\);`)
	columnRe     = regexp.MustCompile(`(?i)^\s*(\w+)\s+([a-z0-9_]+)`)
	matViewRe    = regexp.MustCompile(`(?is)create materialized view\s+(\w+)(.*?)\ngroup by`)
	castAliasRe  = regexp.MustCompile(`(?i)::numeric\s+as\s+(\w+)`)
	tableKeyword = map[string]bool{
		"primary": true, "unique": true, "check": true,
		"foreign": true, "constraint": true, "exclude": true,
	}
)

// numericColumns returns every column the migrations declare as `numeric`,
// keyed by its lower-cased name with underscores removed, mapped to the
// migration file it came from. Column names are globally unambiguous across the
// schema — no name is numeric in one table and integer in another — which is
// what lets the lookup be by name alone, with no table-to-struct name mapping
// (and therefore no need to reimplement sqlc's inflection rules).
func numericColumns(t *testing.T) map[string]string {
	t.Helper()

	entries, err := migrationsFS.ReadDir(migrationsDir)
	require.NoError(t, err)

	cols := make(map[string]string)
	tables := 0

	for _, entry := range entries {
		name := migrationsDir + "/" + entry.Name()
		raw, err := migrationsFS.ReadFile(name)
		require.NoError(t, err)
		sql := commentRe.ReplaceAllString(string(raw), "")

		for _, block := range createTabRe.FindAllStringSubmatch(sql, -1) {
			tables++
			for _, line := range strings.Split(block[2], "\n") {
				m := columnRe.FindStringSubmatch(line)
				if m == nil || tableKeyword[strings.ToLower(m[1])] {
					continue
				}
				if typ := strings.ToLower(m[2]); typ == "numeric" || typ == "decimal" {
					cols[normalise(m[1])] = name
				}
			}
		}

		// Continuous aggregates: every computed column carries an explicit
		// `::numeric` cast, which is both what sqlc needs and what this reads.
		for _, block := range matViewRe.FindAllStringSubmatch(sql, -1) {
			for _, m := range castAliasRe.FindAllStringSubmatch(block[2], -1) {
				cols[normalise(m[1])] = name
			}
		}
	}

	// Guard against a vacuous pass: a regex that silently stops matching would
	// otherwise turn every assertion below into a no-op.
	require.GreaterOrEqual(t, tables, 40, "parsed implausibly few CREATE TABLE blocks")
	require.GreaterOrEqual(t, len(cols), 140, "parsed implausibly few numeric columns")
	for _, must := range []string{
		"activeconsumption", // consumption_hourly, the column the spike broke
		"productionkwh",     // plant_production_daily
		"totalcost",         // bills
		"energyprice",       // tariffs, national_tariff_schedule
		"ptf",               // market_prices_hourly
		"emissionkgco2e",    // carbon_activities
		"basefactor",        // emission_factors
		"maxdemandkw",       // meter_readings and every consumption aggregate
		"activeimport",      // meter_readings
		"avgefficiencypct",  // plant_production_monthly
	} {
		require.Contains(t, cols, must, "expected numeric column missing from the parsed schema")
	}

	return cols
}

func normalise(s string) string {
	return strings.ToLower(strings.ReplaceAll(s, "_", ""))
}

type generatedField struct {
	structName string
	name       string
	typ        string
}

// generatedFields walks every struct declared in the generated models.go.
func generatedFields(t *testing.T) []generatedField {
	t.Helper()

	src, err := os.ReadFile(generatedModelsPath)
	require.NoError(t, err, "generated models are missing; run `make generate`")

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, generatedModelsPath, src, 0)
	require.NoError(t, err)

	var out []generatedField
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.TypeSpec)
		if !ok {
			return true
		}
		st, ok := spec.Type.(*ast.StructType)
		if !ok {
			return true
		}
		for _, f := range st.Fields.List {
			typ := strings.TrimSpace(string(src[f.Type.Pos()-1 : f.Type.End()-1]))
			for _, ident := range f.Names {
				out = append(out, generatedField{spec.Name.Name, ident.Name, typ})
			}
		}
		return true
	})

	require.GreaterOrEqual(t, len(out), 400, "parsed implausibly few generated struct fields")
	return out
}

// TestGeneratedNumericColumnsAreNumeric is the guard the shim file and the
// ::numeric casts in 00005 exist to satisfy.
func TestGeneratedNumericColumnsAreNumeric(t *testing.T) {
	t.Parallel()

	cols := numericColumns(t)
	checked := 0

	for _, f := range generatedFields(t) {
		src, ok := cols[normalise(f.name)]
		if !ok {
			continue
		}
		checked++
		require.Contains(t, []string{"pgtype.Numeric", "*pgtype.Numeric"}, f.typ,
			"%s.%s came from a numeric column in %s but generated as %s; "+
				"money and energy must be pgtype.Numeric — check "+
				"internal/store/postgres/timescale-shims.sql and the ::numeric casts in 00005",
			f.structName, f.name, src, f.typ)
	}

	require.GreaterOrEqual(t, checked, 200,
		"checked implausibly few generated fields against numeric columns")
}

// TestGeneratedModelsHaveNoLossyNumberTypes is the belt to the previous test's
// braces: it needs no schema knowledge at all, so it still fires if the schema
// parsing above ever stops finding a column.
func TestGeneratedModelsHaveNoLossyNumberTypes(t *testing.T) {
	t.Parallel()

	banned := map[string]string{
		"float64":     "money and energy are numeric, never a float",
		"float32":     "money and energy are numeric, never a float",
		"*float64":    "money and energy are numeric, never a float",
		"*float32":    "money and energy are numeric, never a float",
		"interface{}": "sqlc could not type this column; declare the function in timescale-shims.sql",
		"any":         "sqlc could not type this column; declare the function in timescale-shims.sql",
	}

	for _, f := range generatedFields(t) {
		if why, bad := banned[f.typ]; bad {
			t.Errorf("%s.%s generated as %s: %s", f.structName, f.name, f.typ, why)
		}
	}
}

// TestContinuousAggregateBucketsAreTimestamptz pins the other half of the shim
// file's job: without the time_bucket declaration `bucket` generates as
// interface{}, which compiles and is useless.
func TestContinuousAggregateBucketsAreTimestamptz(t *testing.T) {
	t.Parallel()

	aggregates := map[string]bool{
		"ConsumptionHourly": false, "ConsumptionDaily": false,
		"ConsumptionMonthly": false, "ConsumptionYearly": false,
		"PlantProductionDaily": false, "PlantProductionMonthly": false,
	}

	for _, f := range generatedFields(t) {
		if _, ok := aggregates[f.structName]; !ok || f.name != "Bucket" {
			continue
		}
		aggregates[f.structName] = true
		require.Equal(t, "pgtype.Timestamptz", f.typ,
			"%s.Bucket generated as %s; add the time_bucket declaration to "+
				"internal/store/postgres/timescale-shims.sql", f.structName, f.typ)
	}

	for name, seen := range aggregates {
		require.True(t, seen, "generated models have no %s.Bucket field", name)
	}
}
