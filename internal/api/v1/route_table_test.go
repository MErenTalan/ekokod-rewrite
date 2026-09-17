package v1_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	v1 "github.com/MErenTalan/ekokod-rewrite/internal/api/v1"
	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/dto"
	"github.com/MErenTalan/ekokod-rewrite/internal/auth"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
)

func TestRouteTableMatchesRouter(t *testing.T) {
	table := map[string]bool{}
	ops := map[string]bool{}
	for _, rt := range v1.Table() {
		key := rt.Method + " " + rt.Pattern
		require.False(t, table[key], "duplicate route %s", key)
		require.False(t, ops[rt.OperationID], "duplicate operation id %s", rt.OperationID)
		require.NotEmpty(t, rt.OperationID, key)
		require.NotEmpty(t, rt.Tag, key)
		require.NotZero(t, rt.Status, key)
		require.NotNil(t, rt.Handler, key)
		if rt.Access == v1.RoleGated {
			require.NotEmpty(t, rt.Roles.List(), "%s: a role-gated route with no roles", key)
		}
		if rt.Mutating() && rt.Access != v1.Public {
			require.NotEmpty(t, rt.Entity, "%s: a mutating route must name its audited entity", key)
		}
		table[key] = true
		ops[rt.OperationID] = true
	}
	router := v1.NewRouterForTable(v1.Table(), &v1.Handlers{}, passthrough(nil), nil).(chi.Routes)
	mounted := map[string]bool{}
	require.NoError(t, chi.Walk(router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		mounted[method+" "+strings.TrimSuffix(route, "/")] = true
		return nil
	}))
	for key := range table {
		require.True(t, mounted[key], "%s is in the table but not routed", key)
	}
	for key := range mounted {
		require.True(t, table[key], "%s is routed but not in the table", key)
	}
}

func TestOpenAPIDocumentIsCurrent(t *testing.T) {
	doc, err := v1.BuildOpenAPI(v1.Table())
	require.NoError(t, err)
	require.True(t, bytes.Equal(doc, v1.OpenAPIDocument()), "internal/api/v1/openapi.json is stale: run `make openapi` and commit it")
	again, err := v1.BuildOpenAPI(v1.Table())
	require.NoError(t, err)
	require.Equal(t, doc, again, "generation must be deterministic")
	require.Contains(t, string(doc), `"openapi": "3.1.0"`)
}

// A nullable shared enum types every response field that uses it as nullable in the web client.
func TestEnumComponentsAreNotNullable(t *testing.T) {
	var doc struct {
		Components struct {
			Schemas map[string]struct {
				Enum []any `json:"enum"`
				Type any   `json:"type"`
			} `json:"schemas"`
		} `json:"components"`
	}
	require.NoError(t, json.Unmarshal(v1.OpenAPIDocument(), &doc))
	enums := 0
	for name, s := range doc.Components.Schemas {
		if len(s.Enum) == 0 {
			continue
		}
		enums++
		require.Equal(t, "string", s.Type, "%s must not be nullable", name)
	}
	require.GreaterOrEqual(t, enums, 3)
}

func TestOpenAPIServed(t *testing.T) {
	router := v1.NewRouterForTable(v1.Table(), &v1.Handlers{}, passthrough(nil), nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/openapi.json", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, v1.OpenAPIDocument(), rec.Body.Bytes())
}

// Request types keep path/query fields out of the JSON body and keep
// `validate:"required"` and the OpenAPI `required:"true"` in step.
func TestRequestTypesAreConsistent(t *testing.T) {
	var check func(route string, typ reflect.Type)
	check = func(route string, typ reflect.Type) {
		for typ.Kind() == reflect.Pointer || typ.Kind() == reflect.Slice {
			typ = typ.Elem()
		}
		if typ.Kind() != reflect.Struct || typ.PkgPath() == "time" || strings.Contains(typ.PkgPath(), "decimal") {
			return
		}
		for i := range typ.NumField() {
			f := typ.Field(i)
			if (f.Tag.Get("path") != "" || f.Tag.Get("query") != "") && f.Tag.Get("json") != "-" {
				t.Errorf("%s: %s.%s is a path/query field without json:\"-\"", route, typ.Name(), f.Name)
			}
			validateRequired := strings.Contains(","+f.Tag.Get("validate")+",", ",required,")
			schemaRequired := f.Tag.Get("required") == "true"
			if f.Tag.Get("path") == "" && validateRequired != schemaRequired {
				t.Errorf("%s: %s.%s validate required=%v but required tag=%v", route, typ.Name(), f.Name, validateRequired, schemaRequired)
			}
			if f.Tag.Get("json") != "-" {
				check(route, f.Type)
			}
		}
	}
	for _, rt := range v1.Table() {
		if rt.Request != nil {
			check(rt.Method+" "+rt.Pattern, reflect.TypeOf(rt.Request))
		}
	}
}

func TestPermissionEnumMatchesTable(t *testing.T) {
	var got []string
	for _, v := range (dto.Permission("")).Enum() {
		got = append(got, v.(string))
	}
	require.Equal(t, auth.AllPermissions(), got)
	var roles []string
	for _, v := range (dto.Role("")).Enum() {
		roles = append(roles, v.(string))
	}
	var want []string
	for _, r := range model.UserRoles() {
		want = append(want, string(r))
	}
	require.Equal(t, want, roles)
}

// R190: the web sends `company_id` on every authenticated route (R139), so the generated client must type it.
func TestOpenAPIDeclaresCompanyIDOnScopedRoutes(t *testing.T) {
	var doc struct {
		Paths map[string]map[string]struct {
			OperationID string `json:"operationId"`
			Parameters  []struct {
				Name   string `json:"name"`
				In     string `json:"in"`
				Schema struct {
					Ref    string `json:"$ref"`
					Format string `json:"format"`
				} `json:"schema"`
			} `json:"parameters"`
		} `json:"paths"`
	}
	require.NoError(t, json.Unmarshal(v1.OpenAPIDocument(), &doc))
	scoped, public := 0, 0
	for _, rt := range v1.Table() {
		op, ok := doc.Paths["/api/v1"+rt.Pattern][strings.ToLower(rt.Method)]
		require.True(t, ok, "%s %s missing from the document", rt.Method, rt.Pattern)
		var found int
		for _, p := range op.Parameters {
			if p.Name != "company_id" {
				continue
			}
			found++
			require.Equal(t, "query", p.In, rt.OperationID)
			require.True(t, strings.HasSuffix(p.Schema.Ref, "/UuidUUID") || p.Schema.Format == "uuid",
				"%s: company_id must be a uuid, got ref %q format %q", rt.OperationID, p.Schema.Ref, p.Schema.Format)
		}
		if rt.Access == v1.Public {
			require.Zero(t, found, "%s is public and must not declare company_id", rt.OperationID)
			public++
			continue
		}
		require.Equal(t, 1, found, "%s must declare exactly one company_id parameter", rt.OperationID)
		scoped++
	}
	require.Greater(t, scoped, 70)
	require.GreaterOrEqual(t, public, 5)
}
