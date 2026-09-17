package v1

import (
	"bytes"
	"encoding/json"
	"net/http"
	"reflect"
	"regexp"
	"strings"

	"github.com/swaggest/jsonschema-go"
	"github.com/swaggest/openapi-go"
	"github.com/swaggest/openapi-go/openapi31"

	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/dto"
)

var genericName = regexp.MustCompile(`^(\w+)\[.*?(\w+)\]$`)

// BuildOpenAPI renders the OpenAPI 3.1 document of routes, deterministically (R185).
func BuildOpenAPI(routes []Route) ([]byte, error) {
	r := openapi31.NewReflector()
	r.Spec.Info.WithTitle("ekokod API").WithVersion("1")
	r.Spec.SetAPIKeySecurity("cookieAuth", "ekokod_at", openapi.InCookie, "Browser session access cookie.")
	r.Spec.SetHTTPBearerTokenSecurity("bearerAuth", "JWT", "Mobile access token.")
	r.JSONSchemaReflector().DefaultOptions = append(r.JSONSchemaReflector().DefaultOptions,
		jsonschema.InterceptNullability(func(p jsonschema.InterceptNullabilityParams) {
			t := p.Type
			for t.Kind() == reflect.Pointer {
				t = t.Elem()
			}
			_, enum := reflect.New(t).Elem().Interface().(jsonschema.Enum)
			// The API never sends null collections, and an optional enum field is omitted, never null:
			// a shared enum component (Role, Locale) must not turn nullable because a PATCH field points at it.
			if t.Kind() == reflect.Slice || t.Kind() == reflect.Map || enum {
				p.Schema.RemoveType(jsonschema.Null)
			}
		}),
		jsonschema.InterceptDefName(func(_ reflect.Type, name string) string {
			for _, prefix := range []string{"Dto", "Kit", "V1"} {
				name = strings.TrimPrefix(name, prefix)
			}
			if m := genericName.FindStringSubmatch(name); m != nil {
				return strings.TrimPrefix(strings.TrimPrefix(m[2], "Dto"), "V1") + m[1]
			}
			return name
		}),
	)
	for _, rt := range routes {
		oc, err := r.NewOperationContext(rt.Method, "/api/v1"+rt.Pattern)
		if err != nil {
			return nil, err
		}
		oc.SetID(rt.OperationID)
		oc.SetSummary(rt.Summary)
		oc.SetTags(rt.Tag)
		if rt.Request != nil {
			oc.AddReqStructure(rt.Request)
		}
		switch {
		case rt.RawContentType != "":
			oc.AddRespStructure(nil, openapi.WithHTTPStatus(rt.Status), openapi.WithContentType(rt.RawContentType))
		case rt.Response == nil:
			oc.AddRespStructure(nil, openapi.WithHTTPStatus(rt.Status))
		default:
			oc.AddRespStructure(rt.Response, openapi.WithHTTPStatus(rt.Status))
		}
		for _, status := range errorStatuses(rt) {
			oc.AddRespStructure(dto.Error{}, openapi.WithHTTPStatus(status))
		}
		if rt.Access != Public {
			// R190: kit.Bind accepts company_id on every route, but only an authenticated
			// one resolves a scope from it (R139), so only those declare it.
			oc.AddReqStructure(dto.CompanyScopeQuery{})
			oc.AddSecurity("cookieAuth")
			oc.AddSecurity("bearerAuth")
		}
		if err := r.AddOperation(oc); err != nil {
			return nil, err
		}
	}
	raw, err := json.Marshal(r.Spec)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := json.Indent(&out, raw, "", "  "); err != nil {
		return nil, err
	}
	out.WriteByte('\n')
	return out.Bytes(), nil
}

func errorStatuses(rt Route) []int {
	out := []int{http.StatusBadRequest}
	if rt.Access != Public {
		out = append(out, http.StatusUnauthorized)
	}
	if rt.Access == RoleGated {
		out = append(out, http.StatusForbidden)
	}
	if strings.Contains(rt.Pattern, "{") || rt.Access != Public {
		out = append(out, http.StatusNotFound)
	}
	if rt.Mutating() {
		out = append(out, http.StatusConflict, http.StatusUnprocessableEntity)
	}
	if rt.AuthLimit != NoAuthLimit || rt.Access != Public {
		out = append(out, http.StatusTooManyRequests)
	}
	return append(out, http.StatusInternalServerError)
}
