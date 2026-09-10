package postgres

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"sort"
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
//
// The checks run in BOTH directions, deliberately. Walking generated fields and
// looking each one up in the schema catches a column that generated as the
// wrong type. It does NOT catch a numeric column that generated no field at all
// -- the symptom of a committed sqlcgen/ that has gone stale against a newer
// migration -- so TestEveryNumericColumnHasAGeneratedField walks the other way.
//
// ONE ASSUMPTION, stated because a later task can break it: columns are matched
// to Go fields by name alone, with no table-to-struct mapping, which is what
// spares this file from reimplementing sqlc's inflection and initialism rules.
// That is sound only while column names are globally unambiguous across the
// schema -- no name may be `numeric` in one table and an integer type in
// another. That holds today with zero collisions, and
// TestNumericColumnNamesDoNotCollideWithIntegerColumns fails if a later
// migration breaks it, rather than letting the expectation set quietly rot.

const generatedModelsPath = "sqlcgen/models.go"

var (
	commentRe    = regexp.MustCompile(`(?m)--.*$`)
	createTabRe  = regexp.MustCompile(`(?is)create table\s+(\w+)\s*\((.*?)\n\);`)
	columnRe     = regexp.MustCompile(`(?i)^\s*(\w+)\s+([a-z0-9_]+)`)
	matViewRe    = regexp.MustCompile(`(?is)create materialized view\s+(\w+)(.*?)\ngroup by`)
	castAliasRe  = regexp.MustCompile(`(?i)::numeric\s+as\s+(\w+)`)
	declRe       = regexp.MustCompile(`(?i)create function\s+(\w+)\s*\(`)
	tableKeyword = map[string]bool{
		"primary": true, "unique": true, "check": true,
		"foreign": true, "constraint": true, "exclude": true,
	}
)

const shimPath = "timescale-shims.sql"

// schema is what the migrations say, as opposed to what the generated models
// say. numeric maps a normalised column name to the migration it was declared
// in; integer does the same for every column declared with an integer type, and
// exists only so the name-collision assumption can be checked rather than
// assumed.
type schema struct {
	numeric map[string]string
	integer map[string]string
}

var integerTypes = map[string]bool{
	"smallint": true, "integer": true, "int": true, "int2": true,
	"int4": true, "int8": true, "bigint": true, "smallserial": true,
	"serial": true, "bigserial": true,
}

// parseSchema reads the EMBEDDED migrations — the same bytes that ship — and
// extracts every `numeric` column declaration plus every `::numeric as` alias in
// the continuous aggregates.
//
// Columns are keyed by name alone, with no table-to-struct mapping, which is
// what spares this file from reimplementing sqlc's inflection and initialism
// rules. See TestNumericColumnNamesDoNotCollideWithIntegerColumns for the
// assumption that makes sound.
func parseSchema(t *testing.T) schema {
	t.Helper()

	entries, err := migrationsFS.ReadDir(migrationsDir)
	require.NoError(t, err)

	out := schema{numeric: map[string]string{}, integer: map[string]string{}}
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
				switch typ := strings.ToLower(m[2]); {
				case typ == "numeric", typ == "decimal":
					out.numeric[normalise(m[1])] = name
				case integerTypes[typ]:
					out.integer[normalise(m[1])] = name
				}
			}
		}

		// Continuous aggregates: every computed column carries an explicit
		// `::numeric` cast, which is both what sqlc needs and what this reads.
		for _, block := range matViewRe.FindAllStringSubmatch(sql, -1) {
			for _, m := range castAliasRe.FindAllStringSubmatch(block[2], -1) {
				out.numeric[normalise(m[1])] = name
			}
		}
	}

	// Guard against a vacuous pass: a regex that silently stops matching would
	// otherwise turn every assertion below into a no-op.
	require.GreaterOrEqual(t, tables, 40, "parsed implausibly few CREATE TABLE blocks")
	require.GreaterOrEqual(t, len(out.numeric), 140, "parsed implausibly few numeric columns")
	require.GreaterOrEqual(t, len(out.integer), 15, "parsed implausibly few integer columns")
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
		require.Contains(t, out.numeric, must, "expected numeric column missing from the parsed schema")
	}

	return out
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

	cols := parseSchema(t).numeric
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

// TestEveryNumericColumnHasAGeneratedField is the reverse direction, and it
// closes a real gap rather than restating the test above.
//
// TestGeneratedNumericColumnsAreNumeric walks GENERATED FIELDS and looks each
// one up in the schema. A numeric column with no generated field at all is
// therefore invisible to it — the field simply never comes up — which is the
// exact symptom of a committed sqlcgen/ that has gone stale against a newer
// migration. The floors on the forward test do not save it either: with 251
// fields checked against a floor of 200, roughly fifty columns could vanish
// before anything complained.
//
// So this walks the schema and requires a generated field for every numeric
// column. A table added in a later task without regenerating fails here, in
// `make test`, instead of only in CI's drift check.
func TestEveryNumericColumnHasAGeneratedField(t *testing.T) {
	t.Parallel()

	cols := parseSchema(t).numeric

	generated := map[string]bool{}
	for _, f := range generatedFields(t) {
		generated[normalise(f.name)] = true
	}

	var missing []string
	for col, src := range cols {
		if !generated[col] {
			missing = append(missing, col+" (declared in "+src+")")
		}
	}
	sort.Strings(missing)

	require.Empty(t, missing,
		"%d numeric column(s) have no field in the generated models — "+
			"sqlcgen/ is stale against the migrations; run `make generate` and commit the result",
		len(missing))
}

// TestNumericColumnNamesDoNotCollideWithIntegerColumns pins the assumption the
// name-keyed matching rests on.
//
// Both directions above look a column up by name alone, with no table-to-struct
// mapping — which is what spares this file from reimplementing sqlc's
// inflection and initialism rules, and what lets it cover tables that have no
// query file yet. That is sound only while a given column name means the same
// thing everywhere in the schema. The day a later task adds, say, a `value`
// column that is numeric in one table and integer in another, the expectation
// set silently acquires a wrong entry and one of the two tables stops being
// checked properly.
//
// There are zero collisions today. This fails the moment there is one, with a
// message saying what to do about it, rather than letting the guard rot quietly.
func TestNumericColumnNamesDoNotCollideWithIntegerColumns(t *testing.T) {
	t.Parallel()

	parsed := parseSchema(t)

	var collisions []string
	for col, numericSrc := range parsed.numeric {
		if intSrc, clash := parsed.integer[col]; clash {
			collisions = append(collisions,
				col+" (numeric in "+numericSrc+", integer in "+intSrc+")")
		}
	}
	sort.Strings(collisions)

	require.Empty(t, collisions,
		"column name(s) mean different things in different tables, which breaks the "+
			"name-keyed matching the tests in this file rely on. Either rename one of "+
			"the columns, or teach parseSchema and generatedFields a table-to-struct "+
			"mapping (which means reimplementing sqlc's inflection rules).")
}

// TestShimDeclaresEveryTimescaleFunctionTheMigrationsUse is the guard for the
// failure that actually happened.
//
// A merge changed five continuous aggregates from `time_bucket('1 day', ts)` to
// the timezone-aware `time_bucket('1 day', ts, 'Europe/Istanbul')`, because a
// day or month boundary must be evaluated in Europe/Istanbul rather than UTC.
// To sqlc's catalogue that is a different overload, and timescale-shims.sql
// declared only the two-argument form — so all five regenerated with
// `Bucket interface{}` and CI's drift check went red.
//
// Nothing in `make test` caught it, and that is structural rather than bad
// luck: every other test in this file reads the COMMITTED models.go, which
// still said pgtype.Timestamptz. Comparing committed Go against current SQL
// cannot see a schema change that has not been regenerated yet.
//
// This test compares SQL against SQL, so it does not care. For every function
// the shim declares, every arity that function is CALLED with anywhere in the
// migrations must also be DECLARED. Adding a Timescale call the shim does not
// cover now fails locally, at the moment the migration is written.
func TestShimDeclaresEveryTimescaleFunctionTheMigrationsUse(t *testing.T) {
	t.Parallel()

	shim, err := os.ReadFile(shimPath)
	require.NoError(t, err, "the sqlc-only Timescale shim is missing")

	// name -> set of declared arities.
	declared := map[string]map[int]bool{}
	for _, m := range declRe.FindAllStringSubmatchIndex(string(shim), -1) {
		name := strings.ToLower(string(shim[m[2]:m[3]]))
		arity := argCount(string(shim), m[1]-1)
		if declared[name] == nil {
			declared[name] = map[int]bool{}
		}
		declared[name][arity] = true
	}
	require.NotEmpty(t, declared, "parsed no declarations out of "+shimPath)
	require.Contains(t, declared, "time_bucket")

	entries, err := migrationsFS.ReadDir(migrationsDir)
	require.NoError(t, err)

	calls := 0
	for _, entry := range entries {
		name := migrationsDir + "/" + entry.Name()
		raw, err := migrationsFS.ReadFile(name)
		require.NoError(t, err)
		sql := commentRe.ReplaceAllString(string(raw), "")

		for fn, arities := range declared {
			for _, at := range callSites(sql, fn) {
				calls++
				require.True(t, arities[at],
					"%s calls %s() with %d argument(s), but %s declares only %v. "+
						"sqlc treats each arity as a distinct overload: an undeclared one "+
						"regenerates as `interface{}`. Add the missing declaration to %s "+
						"— extend it, never weaken an existing return type.",
					name, fn, at, shimPath, sortedKeys(arities), shimPath)
			}
		}
	}

	// Anti-vacuity: 00005 calls time_bucket, first and last many times over.
	require.GreaterOrEqual(t, calls, 20,
		"found implausibly few Timescale function calls in the migrations; "+
			"the call scanner has probably stopped matching")
}

// callSites returns the argument count of every call to fn in sql. Matching is
// on a word boundary so that `last(` does not also match `atlast(`.
func callSites(sql, fn string) []int {
	var out []int
	for i := 0; i+len(fn) < len(sql); i++ {
		if !strings.HasPrefix(strings.ToLower(sql[i:]), fn) {
			continue
		}
		if i > 0 && isIdentByte(sql[i-1]) {
			continue
		}
		j := i + len(fn)
		for j < len(sql) && (sql[j] == ' ' || sql[j] == '\t') {
			j++
		}
		if j >= len(sql) || sql[j] != '(' {
			continue
		}
		out = append(out, argCount(sql, j))
		i = j
	}
	return out
}

// argCount counts the arguments of the call whose opening parenthesis is at
// open, ignoring commas inside nested calls and inside quoted literals. A call
// with an empty argument list has zero arguments.
func argCount(sql string, open int) int {
	depth, args := 0, 0
	seenArg := false
	inQuote := false
	for i := open; i < len(sql); i++ {
		c := sql[i]
		if inQuote {
			if c == '\'' {
				inQuote = false
			}
			continue
		}
		switch c {
		case '\'':
			inQuote = true
			seenArg = true
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				if seenArg {
					args++
				}
				return args
			}
		case ',':
			if depth == 1 {
				args++
			}
		case ' ', '\t', '\n', '\r':
		default:
			if depth >= 1 {
				seenArg = true
			}
		}
	}
	return args
}

func isIdentByte(b byte) bool {
	return b == '_' ||
		(b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

func sortedKeys(m map[int]bool) []int {
	out := make([]int, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}
