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
	commentRe   = regexp.MustCompile(`(?m)--.*$`)
	createTabRe = regexp.MustCompile(`(?is)create table\s+(\w+)\s*\((.*?)\n\);`)
	columnRe    = regexp.MustCompile(`(?i)^\s*(\w+)\s+([a-z0-9_]+)`)
	matViewRe   = regexp.MustCompile(`(?is)create materialized view\s+(\w+)(.*?)\ngroup by`)
	castAliasRe = regexp.MustCompile(`(?i)::numeric\s+as\s+(\w+)`)
	declRe      = regexp.MustCompile(`(?i)create function\s+(\w+)\s*\(`)
	// matViewBodyRe captures a whole view body, unlike matViewRe which stops at
	// `group by` because that is all the numeric-alias scan needs. Terminating
	// on the first `;` is safe here only because no view body in this schema
	// contains a semicolon inside a string literal; if one ever does, this
	// under-reads rather than over-reads, and the per-function floors in
	// TestEveryFunctionSQLcMustTypeIsDeclared are what would notice.
	matViewBodyRe = regexp.MustCompile(`(?is)create materialized view\s+(\w+)(.*?);`)
	// blockCommentRe matches a /* ... */ comment. (?s) lets "." match a
	// newline, because a review found `locf/* x */(ts)` — a block comment
	// sitting between a function name and its opening parenthesis — hiding
	// the call from functionCalls entirely: unlike commentRe's line
	// comments, nothing here stripped it before the scan.
	blockCommentRe = regexp.MustCompile(`(?s)/\*.*?\*/`)
	tableKeyword   = map[string]bool{
		"primary": true, "unique": true, "check": true,
		"foreign": true, "constraint": true, "exclude": true,
	}
)

const (
	shimPath   = "timescale-shims.sql"
	queriesDir = "queries"
)

// nativeFunctions are functions sqlc's own catalogue already types, so calling
// one in a view body or a query needs no shim declaration. It is deliberately
// generous about ordinary Postgres built-ins that Tasks 9-11 are likely to
// reach for: a name wrongly listed here is a missed mistype, but a name merely
// absent is a loud, one-line fix, so the cost of the two mistakes is not
// symmetric and this list should only ever gain names that Postgres really does
// provide.
var nativeFunctions = map[string]bool{
	"count": true, "sum": true, "avg": true, "min": true, "max": true,
	"stddev": true, "variance": true, "bool_and": true, "bool_or": true,
	"abs": true, "ceil": true, "ceiling": true, "floor": true, "round": true,
	"trunc": true, "greatest": true, "least": true, "coalesce": true,
	"nullif": true, "mod": true, "power": true, "sqrt": true,
	"now": true, "date_trunc": true, "date_part": true, "extract": true,
	"age": true, "to_char": true, "to_date": true, "to_timestamp": true,
	"make_timestamptz": true, "timezone": true,
	"lower": true, "upper": true, "trim": true, "btrim": true, "length": true,
	"concat": true, "concat_ws": true, "substring": true, "replace": true,
	"split_part": true, "format": true, "md5": true, "encode": true, "decode": true,
	"array_agg": true, "string_agg": true, "unnest": true, "cardinality": true,
	"array_length": true, "array_position": true,
	"jsonb_agg": true, "json_agg": true, "jsonb_build_object": true,
	"json_build_object": true, "jsonb_object_agg": true, "to_jsonb": true,
	"gen_random_uuid": true, "generate_series": true,
	"row_number": true, "rank": true, "dense_rank": true, "lag": true, "lead": true,
	"percentile_cont": true, "percentile_disc": true,
}

// notFunctions are SQL keywords and type names that can be followed by an
// opening parenthesis without being a function call at all — `= any($3::uuid[])`,
// `cast(x as y)`, `numeric(18,4)`, a window's `over (...)`. Skipping them keeps
// the default-deny rule above from producing nonsense failures.
var notFunctions = map[string]bool{
	"any": true, "all": true, "in": true, "exists": true, "values": true,
	"array": true, "row": true, "cast": true, "case": true, "when": true,
	"select": true, "from": true, "where": true, "and": true, "or": true,
	"not": true, "on": true, "using": true, "over": true, "partition": true,
	"filter": true, "within": true, "group": true, "order": true, "by": true,
	"having": true, "union": true, "intersect": true, "except": true,
	"distinct": true, "as": true, "with": true, "returns": true, "function": true,
	"numeric": true, "decimal": true, "varchar": true, "char": true,
	"timestamp": true, "timestamptz": true, "interval": true, "time": true,
}

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

// TestEveryFunctionSQLcMustTypeIsDeclared is the guard for the failure that
// actually happened, generalised to the class rather than the instance.
//
// WHAT HAPPENED. A merge changed five continuous aggregates from
// `time_bucket('1 day', ts)` to the timezone-aware
// `time_bucket('1 day', ts, 'Europe/Istanbul')`, because a day or month
// boundary must be evaluated in Istanbul local time rather than UTC. To sqlc's
// catalogue that is a different overload, timescale-shims.sql declared only the
// two-argument form, and all five views regenerated with `Bucket interface{}`.
//
// Nothing in `make test` caught it, and that was structural rather than bad
// luck: every other test in this file compares COMMITTED Go against CURRENT
// SQL, and a schema change that has not been regenerated yet appears on neither
// side. This test compares SQL against SQL, so it does not care.
//
// WHAT IT CHECKS, and why the scope is what it is. sqlc only has to know a
// function's signature where it must INFER A RESULT TYPE: inside a view body,
// and inside a query file. A bare `select create_hypertable(...)` or
// `select add_compression_policy(...)` statement produces no Go type at all —
// sqlc ignores those outright, which is the whole reason `schema:` can point at
// the migrations directory — so requiring shim declarations for them would be a
// false failure. Column defaults such as `default gen_random_uuid()` are the
// same. Hence: materialized-view bodies and internal/store/postgres/queries.
//
// WHY DEFAULT-DENY rather than a list of known TimescaleDB names. An allowlist
// of `time_bucket`/`locf`/`interpolate`/... only catches functions somebody
// remembered to enumerate; the day TimescaleDB ships `time_bucket_ng` and
// nobody updates the list, the guard goes quietly blind — which is precisely
// the "guard narrower than its name" defect this phase keeps finding, including
// in the first version of this very test, which looped over the functions the
// shim already declared and so could never notice a new one. So the rule is
// inverted: every function called in a type-inferring context must be either a
// built-in sqlc already knows (nativeFunctions) or declared in the shim, at the
// arity it is called with. An unknown function fails loudly and is fixed by one
// line in whichever of the two lists is correct.
func TestEveryFunctionSQLcMustTypeIsDeclared(t *testing.T) {
	t.Parallel()

	declared := shimDeclarations(t)
	require.Contains(t, declared, "time_bucket", "parsed no time_bucket declaration out of "+shimPath)

	byFn := map[string]int{}
	for _, src := range typeInferringSQL(t) {
		for _, call := range functionCalls(src.sql) {
			if notFunctions[call.name] {
				continue
			}
			byFn[call.name]++

			if nativeFunctions[call.name] {
				continue
			}
			arities, known := declared[call.name]
			require.True(t, known,
				"%s calls %s(), which sqlc's catalogue does not know: it is neither a "+
					"built-in in nativeFunctions nor declared in %s. In a %s an undeclared "+
					"function makes sqlc guess the column type — that is how an energy value "+
					"once generated as int32. Declare it in %s, or, if it really is a Postgres "+
					"built-in sqlc types on its own, add it to nativeFunctions.",
				src.name, call.name, shimPath, src.kind, shimPath)
			require.True(t, arities[call.arity],
				"%s calls %s() with %d argument(s), but %s declares only %v. sqlc treats "+
					"each arity as a distinct overload: an undeclared one regenerates as "+
					"`interface{}`. Add the missing declaration to %s — extend it, never "+
					"weaken an existing return type.",
				src.name, call.name, call.arity, shimPath, sortedKeys(arities), shimPath)
		}
	}

	// Anti-vacuity, PER FUNCTION rather than pooled. A pooled floor cannot
	// notice the one function that matters going blind: the migrations hold
	// ~136 first()/last() call sites against 6 time_bucket() ones, so a scanner
	// that stopped seeing time_bucket specifically would still clear a total of
	// twenty and pass in silence.
	for fn, atLeast := range map[string]int{"time_bucket": 6, "first": 20, "last": 20} {
		require.GreaterOrEqual(t, byFn[fn], atLeast,
			"found only %d call site(s) for %s(); the call scanner has probably "+
				"stopped matching it, which would make this guard blind to exactly "+
				"the function it was written for", byFn[fn], fn)
	}
}

// sqlSource is one block of SQL in which sqlc must infer result types.
type sqlSource struct {
	name string
	kind string
	sql  string
}

// stripComments removes every SQL comment from sql: block comments first,
// since one can itself contain "--" and because a block comment may span
// lines that a line-by-line strip would otherwise leave half-stripped, then
// line comments. Every caller that feeds functionCalls must go through this,
// not commentRe alone, or a block comment between a function name and its
// parenthesis hides the call.
func stripComments(sql string) string {
	return commentRe.ReplaceAllString(blockCommentRe.ReplaceAllString(sql, ""), "")
}

// typeInferringSQL returns every such block: the body of each materialized
// view in the migrations, and each query file. Everything else in a migration —
// bare `select create_hypertable(...)` calls, column defaults, index
// expressions — yields no Go type, so an unknown function there is harmless.
func typeInferringSQL(t *testing.T) []sqlSource {
	t.Helper()

	var out []sqlSource

	entries, err := migrationsFS.ReadDir(migrationsDir)
	require.NoError(t, err)
	for _, entry := range entries {
		name := migrationsDir + "/" + entry.Name()
		raw, err := migrationsFS.ReadFile(name)
		require.NoError(t, err)
		sql := stripComments(string(raw))
		for _, block := range matViewBodyRe.FindAllStringSubmatch(sql, -1) {
			out = append(out, sqlSource{
				name: name + " (view " + block[1] + ")",
				kind: "continuous aggregate",
				sql:  block[2],
			})
		}
	}

	queryFiles, err := os.ReadDir(queriesDir)
	require.NoError(t, err)
	for _, entry := range queryFiles {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		name := queriesDir + "/" + entry.Name()
		raw, err := os.ReadFile(name)
		require.NoError(t, err)
		out = append(out, sqlSource{
			name: name,
			kind: "query file",
			sql:  stripComments(string(raw)),
		})
	}

	require.GreaterOrEqual(t, len(out), 7,
		"expected at least the six continuous aggregates and one query file; "+
			"the block scanner has probably stopped matching")
	return out
}

// shimDeclarations parses timescale-shims.sql into name -> set of arities.
func shimDeclarations(t *testing.T) map[string]map[int]bool {
	t.Helper()

	shim, err := os.ReadFile(shimPath)
	require.NoError(t, err, "the sqlc-only Timescale shim is missing")

	declared := map[string]map[int]bool{}
	for _, m := range declRe.FindAllStringSubmatchIndex(string(shim), -1) {
		name := strings.ToLower(string(shim[m[2]:m[3]]))
		if declared[name] == nil {
			declared[name] = map[int]bool{}
		}
		declared[name][argCount(string(shim), m[1]-1)] = true
	}
	require.NotEmpty(t, declared, "parsed no declarations out of "+shimPath)
	return declared
}

type funcCall struct {
	name  string
	arity int
}

// functionCalls returns every function call in sql: an identifier — or a
// "double quoted identifier" — followed by optional whitespace and an
// opening parenthesis. Callers must run sql through stripComments first, so
// that a comment sitting between the name and the parenthesis cannot hide a
// call.
//
// Whitespace here is ANY whitespace, newlines included. The first draft skipped
// only spaces and tabs, which meant a migration formatted as "time_bucket\n("
// would have been invisible to the very guard written to watch time_bucket.
//
// A name preceded by "." is a qualified call, and only the SQLC qualifier is
// skipped: sqlc.arg, sqlc.narg and sqlc.slice are sqlc's own pseudo-functions,
// not something its catalogue types. The first draft skipped a name preceded
// by ANY qualifier, meant only to cover that case, and so let a
// schema-qualified real call such as `public.locf(ts)` slip past the guard
// entirely — the same silent `interface{}` this file exists to prevent. Any
// other qualifier (a schema, or a table/alias as in `b.name`) is checked like
// an unqualified call; `b.name` is still never reported, because nothing
// follows it with "(".
func functionCalls(sql string) []funcCall {
	var out []funcCall
	for i := 0; i < len(sql); i++ {
		if sql[i] == '"' {
			j := i + 1
			for j < len(sql) && sql[j] != '"' {
				j++
			}
			if j >= len(sql) {
				break // unterminated quote: nothing more to scan
			}
			name := strings.ToLower(sql[i+1 : j])
			out = appendIfCall(out, sql, name, j+1)
			i = j
			continue
		}
		if !isIdentStart(sql[i]) || (i > 0 && isIdentByte(sql[i-1])) {
			continue
		}
		j := i
		for j < len(sql) && isIdentByte(sql[j]) {
			j++
		}
		if i > 0 && sql[i-1] == '.' && qualifierIsSQLC(sql, i-1) {
			i = j - 1
			continue
		}
		name := strings.ToLower(sql[i:j])
		out = appendIfCall(out, sql, name, j)
		i = j - 1
	}
	return out
}

// appendIfCall appends a funcCall named name to out if sql, starting at
// from, is optional whitespace followed by "(".
func appendIfCall(out []funcCall, sql, name string, from int) []funcCall {
	k := from
	for k < len(sql) && isSpaceByte(sql[k]) {
		k++
	}
	if k < len(sql) && sql[k] == '(' {
		out = append(out, funcCall{name: name, arity: argCount(sql, k)})
	}
	return out
}

// qualifierIsSQLC reports whether the identifier immediately before the "."
// at index dot is "sqlc" (case-insensitively) — sqlc's own pseudo-function
// namespace, and the only qualifier this scanner may skip. Any other
// qualifier (a schema such as public, or a table/alias) must still have its
// unqualified name checked against nativeFunctions and the shim.
func qualifierIsSQLC(sql string, dot int) bool {
	end := dot
	start := dot
	for start > 0 && isIdentByte(sql[start-1]) {
		start--
	}
	return strings.EqualFold(sql[start:end], "sqlc")
}

func isIdentStart(b byte) bool {
	return b == '_' || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

func isSpaceByte(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
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

// TestFunctionCallsRecognisesEveryCallSpelling pins the scanner directly,
// independent of TestEveryFunctionSQLcMustTypeIsDeclared's real migrations
// and query files, against the three spellings a review found slipping past
// it — each one confirmed, with a real `sqlc generate`, to regenerate the
// column as `interface{}`:
//
//  1. schema-qualified: `public.locf(ts)` — the old rule skipped every
//     qualified name, meant only to skip sqlc's own sqlc.arg/narg/slice.
//  2. quoted identifier: `"locf"(ts)`.
//  3. a block comment between the name and the parenthesis: `locf/* x */(ts)`
//     — handled by stripping block comments before the scan, which is the
//     same treatment `--` line comments already got.
//
// Alongside each, it pins the false-positive guards that made the old rule
// look reasonable in the first place: sqlc's own pseudo-namespace
// (sqlc.arg/narg/slice) must still be skipped, and a plain column reference
// such as `b.name` must still never be reported.
func TestFunctionCallsRecognisesEveryCallSpelling(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		sql   string
		calls []funcCall
	}{
		{
			name:  "schema-qualified call is reported by its unqualified name",
			sql:   `select public.locf(ts) from consumption_hourly`,
			calls: []funcCall{{name: "locf", arity: 1}},
		},
		{
			name:  "table-alias-qualified call is reported the same way",
			sql:   `select b.locf(ts) from consumption_hourly b`,
			calls: []funcCall{{name: "locf", arity: 1}},
		},
		{
			name:  "double-quoted identifier is treated as the name",
			sql:   `select "locf"(ts) from consumption_hourly`,
			calls: []funcCall{{name: "locf", arity: 1}},
		},
		{
			name:  "sqlc's own pseudo-namespace is still skipped",
			sql:   `select * from buildings where id = sqlc.arg(id)`,
			calls: nil,
		},
		{
			// any() is a real call as far as functionCalls is concerned —
			// notFunctions is what the guard filters it with, one layer up
			// — so it is expected here too. The point of this case is
			// sqlc.narg and sqlc.slice: neither may appear.
			name:  "sqlc.narg and sqlc.slice are still skipped too",
			sql:   `select * from buildings where id = sqlc.narg(id) and company_id = any(sqlc.slice(ids))`,
			calls: []funcCall{{name: "any", arity: 1}},
		},
		{
			name:  "a plain column reference is never reported",
			sql:   `select b.name from buildings b`,
			calls: nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.calls, functionCalls(tc.sql))
		})
	}

	t.Run("block comment between name and parenthesis is stripped first", func(t *testing.T) {
		sql := stripComments("select locf/* x */(ts) from consumption_hourly")
		require.NotContains(t, sql, "/*", "the block comment must be gone before the scanner ever runs")
		require.Equal(t, []funcCall{{name: "locf", arity: 1}}, functionCalls(sql))
	})

	t.Run("a block comment spanning multiple lines is stripped too", func(t *testing.T) {
		sql := stripComments("select locf/* spans\nmultiple\nlines */(ts) from consumption_hourly")
		require.Equal(t, []funcCall{{name: "locf", arity: 1}}, functionCalls(sql))
	})
}
