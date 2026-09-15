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
	tableKeyword  = map[string]bool{
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
	// DDL / constraint clause keywords: each one is, by SQL grammar, the
	// word that sits directly before the "(" in question — "primary
	// KEY (id)", "UNIQUE (id)", "foreign KEY (id) references t (id)",
	// "CHECK (price > 0)", "on CONFLICT (id)", "as MATERIALIZED (...)".
	// Unlike a table name (see isRelationBeforeParen below), these are a
	// closed, fixed set of reserved words, so a keyword list is the right
	// tool here, not a growing one.
	"check": true, "unique": true, "key": true, "foreign": true,
	"primary": true, "constraint": true, "conflict": true, "materialized": true,
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
		calls, truncated := functionCalls(src.sql)
		require.False(t, truncated,
			"%s has an unterminated '...' literal; functionCalls cannot tell what is real "+
				"code after that point, so a call written after it would be silently invisible "+
				"to this guard", src.name)
		for _, call := range calls {
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

// stripComments removes every SQL comment from sql — "--" to end of line,
// and NESTED "/* */" block comments, exactly as Postgres itself lexes them —
// while leaving every quoted literal ('...' strings, "..." identifiers,
// $tag$...$tag$ dollar-quoted strings) untouched, byte for byte.
//
// It is a hand-written lexer, not a regex pre-pass, because a regex pre-pass
// cannot tell a comment DELIMITER from the same two characters appearing
// inside a literal, and a review found three ways that gap lets an
// undeclared function slip past TestEveryFunctionSQLcMustTypeIsDeclared
// silently:
//
//   - A "--" line comment containing "/*" (e.g. `-- serves /api/* routes`),
//     with an unrelated "*/" appearing later in real code. A regex block-
//     comment pass run before the line-comment pass treats the "/*" inside
//     the line comment as a real opener and deletes everything up to that
//     unrelated "*/" — which can delete the very call the guard exists to
//     catch. A lexer never has this problem: once it is inside a line
//     comment it skips to the next "\n" and nothing inside that span is
//     ever reinterpreted, because states are entered in strict left-to-right
//     order rather than by an independent regex search.
//   - String literals containing '/*' … '*/' as plain text: the same
//     regex-based block-comment pass treats those as real delimiters and
//     deletes whatever real code sits between them.
//   - A string literal containing a lone '"' (e.g. `'"'`): a scanner that
//     does not track '...' string state at all misreads that quote as the
//     START of a double-quoted identifier and skips ahead to the next '"'
//     it can find, silently swallowing real code (and any call in it).
//
// This function and functionCalls below therefore share one lexing
// primitive, skipNonCode, so both agree on where a comment or a literal
// begins and ends.
//
// truncated reports whether any comment or literal ran off the end of sql
// unterminated — see skipNonCode's EOF discussion. That matters here more
// than it sounds: an unterminated /* */ comment is DROPPED, not kept
// verbatim like a literal, so if it swallows real code containing a call,
// that call is gone from the output entirely, not merely re-scanned later.
// Callers must treat truncated=true as a failure, not a silent pass.
func stripComments(sql string) (out string, truncated bool) {
	var b strings.Builder
	b.Grow(len(sql))
	for i := 0; i < len(sql); {
		if end, isComment, terminated, ok := skipNonCode(sql, i); ok {
			if !terminated {
				truncated = true
			}
			if !isComment {
				b.WriteString(sql[i:end]) // a literal: keep it verbatim
			}
			i = end
			continue
		}
		b.WriteByte(sql[i])
		i++
	}
	return b.String(), truncated
}

// skipNonCode reports whether sql, starting at i, is a "--" line comment, a
// NESTED "/* */" block comment (Postgres nests them, unlike the ANSI
// standard), a '...' string literal, or a $tag$...$tag$ dollar-quoted string
// (including the common bare $$...$$) — and if so, the index just past it,
// whether it was a COMMENT (dropped by stripComments) as opposed to a
// LITERAL (kept verbatim, and never scanned for an identifier by
// functionCalls), and whether it was properly TERMINATED before sql ran out.
//
// '...' STRING LITERALS AND BACKSLASH ESCAPES. Postgres has two dialects of
// '...' literal. A STANDARD literal (`'...'`) follows
// standard_conforming_strings: ” is the only escape (a doubled quote) and a
// bare backslash is just a literal character, so `'C:\'` is a complete,
// two-character-content literal that closes at that quote. An EXTENDED
// literal — one whose opening quote is immediately preceded by `E` or `e`,
// as in `E'it\'s'` — additionally treats `\` as an escape character, so
// `\'` inside it is an escaped quote, not the closing one. Task 8c fix round
// 2's finding: the first version of this scanner applied ONLY the ”-escape
// rule to every '...' literal, so in `E'it\'s'` it read the `\'` as the real
// close, then re-entered string state at the literal `'` after `s` and
// swallowed the rest of the file as "still inside a string" — hiding every
// call after it. The `E`/`e` prefix is checked against the byte immediately
// before it too (isIdentByte), so an identifier merely ENDING in `e`, as in
// `where name='x'`, is never mistaken for the extended-string prefix.
//
// TERMINATION AND EOF. An unterminated literal or comment consumes the rest
// of sql — there is nothing safe to do with malformed input other than
// stop — but the caller must not treat that silently: any call already
// collected before the unterminated span stays collected (functionCalls
// appends as it scans left to right, so nothing already found is lost), but
// a call written AFTER an unterminated '...' literal or /* */ comment is
// invisible to this scan, exactly like the finding above before its fix. So
// `terminated=false` is reported all the way out (through stripComments and
// functionCalls) and both tests in this file that drive them turn it into a
// failure instead of a silent, possibly-vacuous pass. A "--" line comment is
// the one exception: it is defined to run to end of line OR end of file, so
// hitting EOF ends it cleanly and is always reported as terminated.
func skipNonCode(sql string, i int) (end int, isComment bool, terminated bool, ok bool) {
	n := len(sql)
	switch {
	case sql[i] == '-' && i+1 < n && sql[i+1] == '-':
		j := i + 2
		for j < n && sql[j] != '\n' {
			j++
		}
		return j, true, true, true

	case sql[i] == '/' && i+1 < n && sql[i+1] == '*':
		depth := 1
		j := i + 2
		for j < n && depth > 0 {
			switch {
			case sql[j] == '/' && j+1 < n && sql[j+1] == '*':
				depth++
				j += 2
			case sql[j] == '*' && j+1 < n && sql[j+1] == '/':
				depth--
				j += 2
			default:
				j++
			}
		}
		return j, true, depth == 0, true

	case sql[i] == '\'':
		extended := i > 0 && (sql[i-1] == 'E' || sql[i-1] == 'e') &&
			(i < 2 || !isIdentByte(sql[i-2]))
		j := i + 1
		for j < n {
			if extended && sql[j] == '\\' && j+1 < n {
				j += 2 // extended-string escape: the next byte, even a ', does not close it
				continue
			}
			if sql[j] == '\'' {
				if j+1 < n && sql[j+1] == '\'' {
					j += 2 // '' escape: still inside the literal
					continue
				}
				j++
				return j, false, true, true
			}
			j++
		}
		return j, false, false, true // ran off the end: unterminated

	case sql[i] == '$':
		if contentStart, ok := dollarQuoteTag(sql, i); ok {
			closer := sql[i:contentStart] // "$tag$" or "$$", byte-identical to the opener
			if idx := strings.Index(sql[contentStart:], closer); idx >= 0 {
				return contentStart + idx + len(closer), false, true, true
			}
			return n, false, false, true // ran off the end: unterminated
		}
	}
	return 0, false, false, false
}

// dollarQuoteTag reports whether sql opens a dollar-quoted string at the '$'
// index i, returning the index just past the OPENING delimiter ("$$" or
// "$tag$"). The tag, if any, is whatever identifier-byte run sits between
// the two '$' characters — Postgres requires it to start with a letter or
// underscore, which this does not re-check, since a tag this permissive
// rejects is simply not a dollar-quote opener at all (see the "$1" case
// below) and falls through as an ordinary byte.
//
// "$1" (a positional query parameter) is the case this must NOT match: it
// scans past the digit, finds no closing '$', and reports ok=false.
func dollarQuoteTag(sql string, i int) (contentStart int, ok bool) {
	j := i + 1
	for j < len(sql) && isIdentByte(sql[j]) {
		j++
	}
	if j < len(sql) && sql[j] == '$' {
		return j + 1, true
	}
	return 0, false
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
		sql, truncated := stripComments(string(raw))
		require.False(t, truncated,
			"%s has an unterminated '...' literal or /* */ comment; stripComments cannot "+
				"tell what is real code after that point, so this migration cannot be scanned "+
				"for calls at all", name)
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
		sql, truncated := stripComments(string(raw))
		require.False(t, truncated,
			"%s has an unterminated '...' literal or /* */ comment; stripComments cannot "+
				"tell what is real code after that point, so this file cannot be scanned for "+
				"calls at all", name)
		out = append(out, sqlSource{
			name: name,
			kind: "query file",
			sql:  sql,
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

// sqlcPseudoFunctions are the ONLY names sqlc itself recognises inside its
// own "sqlc." namespace — sqlc.arg and sqlc.narg for named parameters,
// sqlc.slice for an IN-list parameter, sqlc.embed for embedding a whole
// struct. The first draft skipped `sqlc.` + ANY name, on the theory that
// nothing else lives in that namespace; that theory is exactly what an
// undeclared `sqlc.wobble_undeclared(id)` would exploit, so the qualifier
// check below additionally requires the name itself be one of these four.
var sqlcPseudoFunctions = map[string]bool{
	"arg": true, "narg": true, "slice": true, "embed": true,
}

// functionCalls returns every function call in sql: an identifier — or a
// "double quoted identifier" — followed by optional whitespace and an
// opening parenthesis. It shares skipNonCode with stripComments, so it never
// needs sql pre-stripped to avoid the block-comment evasion: it recognises a
// comment (of either kind) and skips it in the same pass. It also tracks
// '...' string and $tag$...$tag$ literal boundaries with that same
// function, so identifier-like text INSIDE one of those is never mistaken
// for a call, and a lone '"' inside one is never mistaken for the start of a
// quoted identifier. (typeInferringSQL still runs stripComments first, for
// an unrelated reason: matViewBodyRe needs comment-free text to find a view
// body's real terminating ";" rather than one inside a comment.)
//
// Whitespace here is ANY whitespace, newlines included. The first draft skipped
// only spaces and tabs, which meant a migration formatted as "time_bucket\n("
// would have been invisible to the very guard written to watch time_bucket.
//
// A DOUBLE-QUOTED identifier's case is preserved exactly as written, unlike
// an unquoted one (lowercased below, matching how Postgres folds an
// unquoted identifier). Postgres treats a quoted identifier as
// case-SENSITIVE: `"LOCF"` names a different object than `locf` or `"locf"`,
// so lower-casing it here would make `"LOCF"(ts)` match this file's (all
// lowercase) nativeFunctions/shim entries when the two are not the same
// name at all — a false "it's declared" that defeats the whole guard.
//
// A name preceded by "." is a qualified call, and only sqlc's own
// pseudo-functions (sqlcPseudoFunctions) are skipped. The first draft
// skipped a name preceded by ANY qualifier, meant only to cover
// `sqlc.arg`/`narg`/`slice`, and so let a schema-qualified real call such as
// `public.locf(ts)` slip past the guard entirely — the same silent
// `interface{}` this file exists to prevent. Any other qualifier (a schema,
// or a table/alias as in `b.name`) is checked like an unqualified call;
// `b.name` is still never reported, because nothing follows it with "(".
//
// truncated reports whether scanning ran into an unterminated '...' or
// $tag$...$tag$ literal — see skipNonCode's EOF discussion and
// stripComments' truncated. Every caller in this file treats it as a
// failure: a call written after the unterminated span would otherwise be
// silently invisible, exactly the hole Task 8c fix round 2 closed for
// `E'it\'s'`.
func functionCalls(sql string) (out []funcCall, truncated bool) {
	for i := 0; i < len(sql); {
		if end, _, terminated, ok := skipNonCode(sql, i); ok {
			// A comment (already stripped by the caller, but harmless to
			// skip again) or a literal — either way, not a place a call's
			// name can start.
			if !terminated {
				truncated = true
			}
			i = end
			continue
		}

		if sql[i] == '"' {
			j, name, closed := scanQuotedIdentifier(sql, i)
			if !closed {
				break // unterminated quote: nothing more to scan
			}
			out = appendIfCall(out, sql, i, name, j)
			i = j
			continue
		}

		if !isIdentStart(sql[i]) {
			i++
			continue
		}
		j := i
		for j < len(sql) && isIdentByte(sql[j]) {
			j++
		}
		name := strings.ToLower(sql[i:j])
		if i > 0 && sql[i-1] == '.' && qualifierIsSQLC(sql, i-1) && sqlcPseudoFunctions[name] {
			i = j
			continue
		}
		out = appendIfCall(out, sql, i, name, j)
		i = j
	}
	return out, truncated
}

// createIndexOnRe matches a CREATE INDEX statement's "... on " immediately
// before the table name that follows it — the one place "on" genuinely
// introduces a relation rather than a join or WHERE condition. It is
// deliberately narrow, not a blanket "on precedes a relation" rule: "on" is
// also how a join condition or WHERE clause is spelled, as in
// `join meters m on wobble_undeclared(m.id) = readings.meter_id`, and a
// blanket rule would hide a real call written there. Anchoring on `on\s*$`
// against the text immediately preceding the identifier means it only
// matches when "create ... index ... <name> on" reads contiguously right up
// to this exact position, which a join's "on" never does.
var createIndexOnRe = regexp.MustCompile(
	`(?is)create\s+(unique\s+)?index\s+(concurrently\s+)?(if\s+not\s+exists\s+)?\S+\s+on\s*$`)

// isRelationBeforeParen reports whether the identifier that starts at
// nameStart — which the caller already knows is followed by "(" — names a
// RELATION rather than a function: `insert into t (...)`, `create table t
// (...)`, `references t (...)`, or the one legitimate `on`-introduced
// relation, `create index ... on t (...)`. It walks back over any "schema."
// qualification first, so `insert into public.buildings (` and `insert into
// "Buildings" (` are recognised exactly like the unqualified form.
//
// This is a POSITIONAL rule, not a keyword list, and deliberately so: a
// table name is arbitrary user schema, not a fixed SQL vocabulary, so no
// list of names could ever cover every table this guard will ever see —
// that is exactly the "growing keyword list" this file's own nativeFunctions
// comment warns against. Constraint/clause keywords that sit directly
// before the "(" themselves (`unique (`, `check (`, `primary key (`, `on
// conflict (`) don't need this: they're in notFunctions instead, because
// there the word immediately before "(" already IS the fixed keyword.
func isRelationBeforeParen(sql string, nameStart int) bool {
	start := nameStart
	for start > 0 && sql[start-1] == '.' {
		start-- // step back onto the '.'
		for start > 0 && isIdentByte(sql[start-1]) {
			start--
		}
	}

	k := start
	for k > 0 && isSpaceByte(sql[k-1]) {
		k--
	}
	wordEnd := k
	for k > 0 && isIdentByte(sql[k-1]) {
		k--
	}
	switch strings.ToLower(sql[k:wordEnd]) {
	case "into", "table", "references":
		return true
	case "on":
		return createIndexOnRe.MatchString(sql[:wordEnd])
	}
	return false
}

// scanQuotedIdentifier reads a "..." double-quoted identifier starting at
// the opening quote index i, unescaping "" to a single ". It returns the
// index just past the closing quote, the identifier's exact text (case
// preserved), and whether a closing quote was found at all.
func scanQuotedIdentifier(sql string, i int) (end int, name string, closed bool) {
	var b strings.Builder
	j := i + 1
	for j < len(sql) {
		if sql[j] == '"' {
			if j+1 < len(sql) && sql[j+1] == '"' {
				b.WriteByte('"')
				j += 2
				continue
			}
			return j + 1, b.String(), true
		}
		b.WriteByte(sql[j])
		j++
	}
	return j, b.String(), false
}

// appendIfCall appends a funcCall named name to out if sql, starting at
// from, is optional whitespace followed by "(" — UNLESS nameStart (name's
// own start index, before qualification) marks a relation reference rather
// than a function call; see isRelationBeforeParen.
func appendIfCall(out []funcCall, sql string, nameStart int, name string, from int) []funcCall {
	k := from
	for k < len(sql) && isSpaceByte(sql[k]) {
		k++
	}
	if k < len(sql) && sql[k] == '(' && !isRelationBeforeParen(sql, nameStart) {
		out = append(out, funcCall{name: name, arity: argCount(sql, k)})
	}
	return out
}

// qualifierIsSQLC reports whether the identifier immediately before the "."
// at index dot is "sqlc" (case-insensitively). It is only half of the check
// that lets a qualified call be skipped — see sqlcPseudoFunctions for the
// other half, which the name itself must also satisfy.
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
// callsOrFail runs the real pipeline — stripComments then functionCalls —
// and fails the (sub)test outright if either stage reports an unterminated
// literal or comment, instead of letting a truncated scan masquerade as a
// clean "no calls found". Nothing in the table below expects truncation, so
// every case in it is entitled to that assertion for free.
func callsOrFail(t *testing.T, sql string) []funcCall {
	t.Helper()
	stripped, commentsTruncated := stripComments(sql)
	require.False(t, commentsTruncated, "stripComments hit an unterminated literal or comment in %q", sql)
	calls, callsTruncated := functionCalls(stripped)
	require.False(t, callsTruncated, "functionCalls hit an unterminated literal in %q", sql)
	return calls
}

// callsNeedingDeclaration mirrors the ONE layer of filtering
// TestEveryFunctionSQLcMustTypeIsDeclared applies on top of functionCalls's
// raw output — dropping notFunctions AND nativeFunctions — so a
// false-positive test case below can pin exactly what the real guard would
// see, not merely what functionCalls returns before that filter runs. (A
// keyword such as "conflict" or "check" still comes back from functionCalls
// itself, same as "any" always has — see the sqlc.narg case below — and is
// only removed here, same as it only is one layer up in the real guard.)
func callsNeedingDeclaration(t *testing.T, sql string) []funcCall {
	t.Helper()
	var out []funcCall
	for _, c := range callsOrFail(t, sql) {
		if notFunctions[c.name] || nativeFunctions[c.name] {
			continue
		}
		out = append(out, c)
	}
	return out
}

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
			// "LOCF" and locf are DIFFERENT identifiers in Postgres — a
			// quoted identifier is case-sensitive, an unquoted one is
			// folded to lower case. Lower-casing this one would make it
			// match this file's (all-lowercase) nativeFunctions/shim
			// entries for a function it does not actually name.
			name:  "a quoted identifier's case is preserved, not folded",
			sql:   `select "LOCF"(ts) from consumption_hourly`,
			calls: []funcCall{{name: "LOCF", arity: 1}},
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
			// Task 8c fix round 1: the qualifier check used to skip
			// `sqlc.` + ANY name, which an undeclared
			// `sqlc.wobble_undeclared(id)` would have exploited outright.
			name:  "an undeclared name under the sqlc qualifier is NOT skipped",
			sql:   `select * from buildings where id = sqlc.wobble_undeclared(id)`,
			calls: []funcCall{{name: "wobble_undeclared", arity: 1}},
		},
		{
			name:  "a plain column reference is never reported",
			sql:   `select b.name from buildings b`,
			calls: nil,
		},
		{
			// Task 8c fix round 1, evasion 1: a review found the OLD
			// stripComments — a blockCommentRe pass followed by a
			// commentRe pass — treated the "/*" inside this LINE comment
			// as a real block-comment opener, and then, being non-greedy,
			// deleted everything up to the unrelated "*/" closing the real
			// block comment near the end, taking the real call with it.
			name: "a line comment containing /* does not hide a later real call",
			sql: "-- serves /api/* routes\n" +
				"select wobble_undeclared(id) from buildings /* trailing */;",
			calls: []funcCall{{name: "wobble_undeclared", arity: 1}},
		},
		{
			// Task 8c fix round 1, evasion 2: the same blockCommentRe pass
			// cannot tell a '/*'...'*/' pair of STRING LITERALS from a real
			// block comment, and deletes whatever real code — including a
			// call — sits between them.
			name: "a string literal containing /* or */ does not hide a call between two such literals",
			sql: `select case when note = '/*' then wobble_undeclared(id) else 0 end ` +
				`from buildings where flag = '*/';`,
			calls: []funcCall{{name: "wobble_undeclared", arity: 1}},
		},
		{
			// Task 8c fix round 1, evasion 3: a scanner with no '...'
			// string-tracking misreads a lone '"' inside a string literal
			// as the START of a quoted identifier, scans for the next '"'
			// it can find (there may be none), and — in the old
			// implementation — gives up on the rest of sql entirely,
			// hiding every call after it.
			name:  `a lone " inside a string literal does not hide a later real call`,
			sql:   `select id from buildings where note = '"' and wobble_undeclared(id) > 0;`,
			calls: []funcCall{{name: "wobble_undeclared", arity: 1}},
		},
		{
			// Dollar-quoted strings are the one literal form the ORIGINAL
			// (round 0) scanner never had to face, since none of this
			// schema's migrations or query files used one — but the fix
			// round's lexer must still treat $tag$...$tag$ (and the bare
			// $$...$$ form) as a literal, not as call-bearing code, or as
			// two independent, unmatched positional parameters.
			name:  "a dollar-quoted string is not scanned for calls, and $1/$2 are not mistaken for one",
			sql:   `select $tag$ wobble_undeclared($1) $tag$, $$ another_undeclared($2) $$ from buildings where id = $1`,
			calls: nil,
		},
		{
			// Task 8c fix round 2, finding 1: the OLD '...' state applied the
			// ''-doubling escape to every literal, including an EXTENDED
			// (E'...') one, so it read the \' here as the real closing quote,
			// then re-entered string state at the '  after "s" and swallowed
			// the rest of the file as "still inside a string" — hiding
			// wobble_undeclared entirely.
			name:  `E'it\'s' (an extended-string backslash escape) does not hide a later real call`,
			sql:   `select E'it\'s', wobble_undeclared(id) from buildings;`,
			calls: []funcCall{{name: "wobble_undeclared", arity: 1}},
		},
		{
			// e'a\\' is an ESCAPED backslash (\\), so the literal's content
			// is "a\" and the very next ' really does close it — unlike the
			// case above, the closing quote here is genuine, not escaped.
			name:  `e'a\\' (an escaped backslash, then a real close) does not hide a later real call`,
			sql:   `select e'a\\', wobble_undeclared(id) from buildings;`,
			calls: []funcCall{{name: "wobble_undeclared", arity: 1}},
		},
		{
			// A STANDARD (non-E-prefixed) literal never treats \ as an
			// escape at all — standard_conforming_strings semantics — so
			// 'C:\' is a complete, two-byte-content literal that closes at
			// that very quote, exactly as it always has.
			name:  `a standard 'C:\' literal (backslash is a literal character) does not hide a later real call`,
			sql:   `select 'C:\', wobble_undeclared(id) from buildings;`,
			calls: []funcCall{{name: "wobble_undeclared", arity: 1}},
		},
		{
			// The realistic shape from the findings doc: an ordinary
			// column='literal' comparison, nowhere near an E-prefix, keeps
			// working — included for direct traceability to the finding.
			name:  "an ordinary column='literal' comparison still works",
			sql:   `select * from buildings where name='x' and wobble_undeclared(id) > 0;`,
			calls: []funcCall{{name: "wobble_undeclared", arity: 1}},
		},
		{
			// The load-bearing boundary case: "name" ENDS in 'e' and is
			// immediately (no operator) followed by a literal containing a
			// backslash-then-quote. If the extended-prefix check only looked
			// at the byte before the quote (the 'e'), this would be
			// wrongly treated as extended: the \' would be read as an escape
			// rather than the close, the literal would run unterminated to
			// EOF, and wobble_undeclared would never be seen. The check
			// additionally requires the byte before THAT ('m', part of the
			// same identifier) not be an identifier byte, which correctly
			// makes this a STANDARD literal instead.
			name:  "an identifier merely ending in e is not mistaken for the E-prefix",
			sql:   `select name'a\', wobble_undeclared(id) from buildings;`,
			calls: []funcCall{{name: "wobble_undeclared", arity: 1}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.calls, callsOrFail(t, tc.sql))
		})
	}

	t.Run("block comment between name and parenthesis is stripped first", func(t *testing.T) {
		sql, truncated := stripComments("select locf/* x */(ts) from consumption_hourly")
		require.False(t, truncated)
		require.NotContains(t, sql, "/*", "the block comment must be gone before the scanner ever runs")
		calls, truncated := functionCalls(sql)
		require.False(t, truncated)
		require.Equal(t, []funcCall{{name: "locf", arity: 1}}, calls)
	})

	t.Run("a block comment spanning multiple lines is stripped too", func(t *testing.T) {
		require.Equal(t, []funcCall{{name: "locf", arity: 1}},
			callsOrFail(t, "select locf/* spans\nmultiple\nlines */(ts) from consumption_hourly"))
	})

	// A NON-nested reading (Postgres nests /* */; the ANSI standard does
	// not) would treat "/* inner */" as closing the WHOLE comment, exposing
	// "text_that_looks_like_a_call(x)" as real code — a false positive
	// this asserts against directly, not just "the real call still
	// appears", which nesting-blind stripping got right here by accident
	// (the real call sits after the true end, so hiding it was never the
	// risk this particular case tests).
	t.Run("nested block comments close only at the matching depth", func(t *testing.T) {
		require.Equal(t, []funcCall{{name: "locf", arity: 1}}, callsOrFail(t,
			"/* outer /* inner */ text_that_looks_like_a_call(x) */ select locf(ts) from consumption_hourly"))
	})

	// Task 8c fix round 2, finding 1 (EOF discussion): calls collected
	// BEFORE an unterminated literal or comment are never lost — they were
	// already appended while scanning left to right — but a call written
	// AFTER one is invisible to the scan. That must surface as a reported
	// failure, not a silent, possibly-vacuous pass.
	t.Run("an unterminated string literal is reported as truncated, not silently swallowed", func(t *testing.T) {
		calls, truncated := functionCalls(`select wobble_undeclared(id), 'unterminated`)
		require.True(t, truncated, "an unterminated '...' literal must be reported")
		require.Equal(t, []funcCall{{name: "wobble_undeclared", arity: 1}}, calls,
			"the call collected before the unterminated literal must still come back")
	})

	t.Run("an unterminated block comment is reported as truncated by stripComments", func(t *testing.T) {
		_, truncated := stripComments("select wobble_undeclared(id) from buildings; /* unterminated")
		require.True(t, truncated, "an unterminated /* */ comment must be reported")
	})

	t.Run("an unterminated dollar-quoted string is reported as truncated", func(t *testing.T) {
		_, truncated := functionCalls(`select $tag$ unterminated`)
		require.True(t, truncated, "an unterminated $tag$...$tag$ literal must be reported")
	})

	// Task 8c fix round 2, finding 2: ordinary DML and DDL syntax that is
	// shaped exactly like a call — an identifier (or reserved keyword)
	// directly followed by "(" — must not be reported as one. Each case
	// pairs the false-positive shape (must NOT be reported, checked against
	// callsNeedingDeclaration — the same filter the real guard applies) with
	// a same-shape REAL call (must still be reported) proving the fix does
	// not overreach.
	dmlTests := []struct {
		name  string
		sql   string
		calls []funcCall
	}{
		{
			name:  "insert into <table> (<cols>) is not a call",
			sql:   `insert into buildings (id, company_id, name) values ($1, $2, $3);`,
			calls: nil,
		},
		{
			name:  "insert into <schema>.<table> (<cols>) is not a call",
			sql:   `insert into public.buildings (id) values ($1);`,
			calls: nil,
		},
		{
			name:  `insert into "<quoted table>" (<cols>) is not a call`,
			sql:   `insert into "Buildings" (id) values ($1);`,
			calls: nil,
		},
		{
			name:  "on conflict (<cols>) is not a call",
			sql:   `insert into buildings (id) values ($1) on conflict (id) do nothing;`,
			calls: nil,
		},
		{
			name: "on conflict on constraint <name> is not a call",
			sql: `insert into buildings (id) values ($1) ` +
				`on conflict on constraint buildings_pkey do nothing;`,
			calls: nil,
		},
		{
			name:  "multi-row values (...), (...) is not a call",
			sql:   `insert into buildings (id, name) values ($1, $2), ($3, $4);`,
			calls: nil,
		},
		{
			name: "in (...), exists (...), any (...) and all (...) are not calls",
			sql: `select * from buildings where id in ($1, $2) and exists (select 1) ` +
				`and company_id = any($3) and company_id = all($4);`,
			calls: nil,
		},
		{
			name:  "using (<cols>) in a join is not a call",
			sql:   `select * from buildings b join companies c using (company_id);`,
			calls: nil,
		},
		{
			name: "over (...), filter (where ...) and within group (...) are not calls",
			sql: `select sum(x) over (partition by y), count(*) filter (where y > 0), ` +
				`percentile_cont(0.5) within group (order by x) from buildings;`,
			calls: nil,
		},
		{
			name:  "a CTE's as (...) body is not a call",
			sql:   `with x as (select 1) select * from x;`,
			calls: nil,
		},
		{
			name:  "a CTE's as materialized (...) body is not a call",
			sql:   `with x as materialized (select 1) select * from x;`,
			calls: nil,
		},
		{
			name: "numeric(18,4), varchar(10) and timestamp(3) type modifiers are not calls",
			sql: `select cast(x as numeric(18,4)), cast(y as varchar(10)), ` +
				`cast(z as timestamp(3)) from buildings;`,
			calls: nil,
		},
		{
			name:  "row (...) and array[...] are not calls",
			sql:   `select row(1,2,3), array[1,2,3] from buildings;`,
			calls: nil,
		},
		{
			name:  "create index ... on t (...) is not a call",
			sql:   `create index idx_buildings_name on buildings (name);`,
			calls: nil,
		},
		{
			name:  "primary key (...) as a table constraint is not a call",
			sql:   `create table t (id uuid, primary key (id));`,
			calls: nil,
		},
		{
			name:  "unique (...) as a table constraint is not a call",
			sql:   `create table t (id uuid, name text, unique (name));`,
			calls: nil,
		},
		{
			name:  "foreign key (...) references t (...) is not a call",
			sql:   `create table t (building_id uuid, foreign key (building_id) references buildings (id));`,
			calls: nil,
		},
		{
			name:  "check (...) is not a call",
			sql:   `create table t (price numeric, check (price > 0));`,
			calls: nil,
		},
		{
			// The real-call pins: the same shapes above, but with an
			// undeclared function written in the exact positions the fix
			// must not blind itself to.
			name:  "a real call inside values (...) is still reported",
			sql:   `insert into buildings (id) values (wobble_undeclared($1));`,
			calls: []funcCall{{name: "wobble_undeclared", arity: 1}},
		},
		{
			name: "a real call in an on conflict do update SET expression is still reported",
			sql: `insert into buildings (id, v) values (wobble_undeclared($1), $2) ` +
				`on conflict (id) do update set v = wobble_undeclared(excluded.v);`,
			calls: []funcCall{{name: "wobble_undeclared", arity: 1}, {name: "wobble_undeclared", arity: 1}},
		},
		{
			name:  "a real call inside a CTE body is still reported",
			sql:   `with x as (select wobble_undeclared(id) from buildings) select * from x;`,
			calls: []funcCall{{name: "wobble_undeclared", arity: 1}},
		},
		{
			// The case the "on" heuristic must not break: a JOIN condition
			// also puts an identifier directly after "on" and directly
			// before "(" — the same shape as "create index ... on t (" —
			// but here it is a real call and must still be caught. See
			// createIndexOnRe's own comment for why these are
			// distinguishable.
			name:  `a real call directly after "on" in a join condition is still reported`,
			sql:   `select * from meters m join readings r on wobble_undeclared(m.id) = r.meter_id;`,
			calls: []funcCall{{name: "wobble_undeclared", arity: 1}},
		},
	}
	for _, tc := range dmlTests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.calls, callsNeedingDeclaration(t, tc.sql))
		})
	}
}
