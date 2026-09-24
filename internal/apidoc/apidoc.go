// Package apidoc renders the generated OpenAPI document as the static API
// reference in docs/api/reference.md (F15c R482).
package apidoc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// genericPrefix is what Go generics give a DTO's schema name (Page[T] types).
const genericPrefix = "GithubComMErenTalanEkokodRewriteInternalApiV1Dto"

// genericErrors are listed by nearly every operation; the reference leaves
// them to its header and keeps the codes that say something (404, 409, 422…).
var genericErrors = map[string]bool{"400": true, "401": true, "403": true, "429": true, "500": true}

var methodOrder = []string{"get", "post", "put", "patch", "delete"}

type schema struct {
	Ref        string             `json:"$ref"`
	Type       any                `json:"type"`
	Format     string             `json:"format"`
	Items      *schema            `json:"items"`
	Properties map[string]*schema `json:"properties"`
	Required   []string           `json:"required"`
	Enum       []any              `json:"enum"`
}

type media struct {
	Schema *schema `json:"schema"`
}

type operation struct {
	OperationID string `json:"operationId"`
	Summary     string `json:"summary"`
	Tags        []string
	Security    []map[string][]string `json:"security"`
	Parameters  []struct {
		In       string  `json:"in"`
		Name     string  `json:"name"`
		Required bool    `json:"required"`
		Schema   *schema `json:"schema"`
	} `json:"parameters"`
	RequestBody *struct {
		Content map[string]media `json:"content"`
	} `json:"requestBody"`
	Responses map[string]struct {
		Content map[string]media `json:"content"`
	} `json:"responses"`
}

type document struct {
	Info struct {
		Version string `json:"version"`
	} `json:"info"`
	Paths      map[string]map[string]operation `json:"paths"`
	Components struct {
		Schemas map[string]*schema `json:"schemas"`
	} `json:"components"`
}

type entry struct {
	path, method string
	op           operation
}

// Markdown renders doc: operations grouped by tag (or the first path segment),
// then every component schema with its fields. Output is deterministic.
func Markdown(doc []byte) ([]byte, error) {
	var d document
	if err := json.Unmarshal(doc, &d); err != nil {
		return nil, fmt.Errorf("apidoc: %w", err)
	}
	names := shortNames(d.Components.Schemas)

	groups := map[string][]entry{}
	count := 0
	for path, ops := range d.Paths {
		for method, op := range ops {
			g := groupOf(path, op)
			groups[g] = append(groups[g], entry{path, method, op})
			count++
		}
	}
	var b bytes.Buffer
	fmt.Fprintf(&b, "# ekokod API reference\n\n")
	fmt.Fprintf(&b, "Generated from the OpenAPI document (`ekokod tool openapi`) by `make api-docs`; do not edit.\n")
	fmt.Fprintf(&b, "Version %s · %d operations · %d schemas. Authentication: a session cookie (`ekokod_at`) or a mobile bearer token; operations marked *public* need neither.\n"+
		"Errors 400, 401, 403, 429 and 500 apply to every operation and use the `Error` schema.\n\n",
		d.Info.Version, count, len(d.Components.Schemas))

	for _, g := range sortedKeys(groups) {
		es := groups[g]
		sort.Slice(es, func(i, j int) bool {
			if es[i].path != es[j].path {
				return es[i].path < es[j].path
			}
			return methodRank(es[i].method) < methodRank(es[j].method)
		})
		fmt.Fprintf(&b, "## %s\n\n", g)
		for _, e := range es {
			writeOperation(&b, e, names)
		}
	}

	fmt.Fprintf(&b, "## Schemas\n\n")
	byShort := make(map[string]string, len(names))
	for full, short := range names {
		byShort[short] = full
	}
	for _, short := range sortedKeys(byShort) {
		s := d.Components.Schemas[byShort[short]]
		fmt.Fprintf(&b, "### <a id=\"%s\"></a>`%s`\n\n", anchor(short), short)
		if len(s.Properties) == 0 {
			fmt.Fprintf(&b, "%s\n\n", typeOf(s, names))
			continue
		}
		required := map[string]bool{}
		for _, r := range s.Required {
			required[r] = true
		}
		fmt.Fprintf(&b, "| field | type | required |\n|---|---|---|\n")
		for _, f := range sortedKeys(s.Properties) {
			fmt.Fprintf(&b, "| `%s` | %s | %s |\n", f, typeOf(s.Properties[f], names), yes(required[f]))
		}
		b.WriteString("\n")
	}
	return b.Bytes(), nil
}

func writeOperation(b *bytes.Buffer, e entry, names map[string]string) {
	fmt.Fprintf(b, "### `%s %s`\n\n", strings.ToUpper(e.method), e.path)
	public := ""
	if len(e.op.Security) == 0 {
		public = "*public* · "
	}
	fmt.Fprintf(b, "%s %s`%s`\n\n", e.op.Summary, public, e.op.OperationID)
	if len(e.op.Parameters) > 0 {
		fmt.Fprintf(b, "| parameter | in | required | type |\n|---|---|---|---|\n")
		for _, p := range e.op.Parameters {
			fmt.Fprintf(b, "| `%s` | %s | %s | %s |\n", p.Name, p.In, yes(p.Required), typeOf(p.Schema, names))
		}
		b.WriteString("\n")
	}
	if e.op.RequestBody != nil {
		var parts []string
		for _, ct := range sortedKeys(e.op.RequestBody.Content) {
			t := typeOf(e.op.RequestBody.Content[ct].Schema, names)
			if ct != "application/json" {
				t += " (" + ct + ")"
			}
			parts = append(parts, t)
		}
		fmt.Fprintf(b, "Body: %s\n\n", strings.Join(parts, ", "))
	}
	var rs []string
	for _, code := range sortedKeys(e.op.Responses) {
		r := code
		if strings.HasPrefix(code, "2") {
			if m, ok := e.op.Responses[code].Content["application/json"]; ok && m.Schema != nil {
				r += " " + typeOf(m.Schema, names)
			}
		} else if genericErrors[code] {
			continue
		}
		rs = append(rs, r)
	}
	fmt.Fprintf(b, "Responses: %s\n\n", strings.Join(rs, ", "))
}

func typeOf(s *schema, names map[string]string) string {
	if s == nil {
		return "any"
	}
	if s.Ref != "" {
		name := strings.TrimPrefix(s.Ref, "#/components/schemas/")
		short := names[name]
		if short == "" {
			short = name
		}
		if short == "UuidUUID" {
			return "`UuidUUID`"
		}
		return fmt.Sprintf("[`%s`](#%s)", short, anchor(short))
	}
	nullable := false
	var t string
	switch v := s.Type.(type) {
	case string:
		t = v
	case []any:
		for _, x := range v {
			if x == "null" {
				nullable = true
			} else if str, ok := x.(string); ok {
				t = str
			}
		}
	}
	switch {
	case t == "array":
		t = typeOf(s.Items, names) + "[]"
	case t == "":
		t = "any"
	case s.Format != "":
		t += " (" + s.Format + ")"
	}
	if len(s.Enum) > 0 {
		vals := make([]string, 0, len(s.Enum))
		for _, v := range s.Enum {
			if v != nil {
				vals = append(vals, fmt.Sprint(v))
			}
		}
		t += ": " + strings.Join(vals, " \\| ")
	}
	if nullable {
		t += "?"
	}
	return t
}

func shortNames(schemas map[string]*schema) map[string]string {
	out := make(map[string]string, len(schemas))
	for name := range schemas {
		short := strings.TrimPrefix(name, genericPrefix)
		if _, clash := schemas[short]; clash && short != name {
			short = name
		}
		out[name] = short
	}
	return out
}

func groupOf(path string, op operation) string {
	if len(op.Tags) > 0 {
		return op.Tags[0]
	}
	rest := strings.TrimPrefix(path, "/api/v1/")
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		rest = rest[:i]
	}
	return rest
}

func methodRank(m string) int {
	for i, x := range methodOrder {
		if x == m {
			return i
		}
	}
	return len(methodOrder)
}

func anchor(short string) string { return "schema-" + strings.ToLower(short) }

func yes(b bool) string {
	if b {
		return "yes"
	}
	return ""
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
