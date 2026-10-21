package v1_test

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	v1 "github.com/MErenTalan/ekokod-rewrite/internal/api/v1"
)

var (
	uuidType      = reflect.TypeOf(uuid.UUID{})
	uuidSliceType = reflect.TypeOf([]uuid.UUID{})
)

// bodyIDFields returns the JSON names of every uuid a request body carries,
// nested structs included; path and query fields (json:"-") are not body.
func bodyIDFields(t reflect.Type, prefix string) []string {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil
	}
	var out []string
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "-" {
			continue
		}
		ft := f.Type
		for ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}
		switch {
		case ft == uuidType || ft == uuidSliceType:
			out = append(out, prefix+name)
		case ft.Kind() == reflect.Struct && f.Anonymous:
			out = append(out, bodyIDFields(ft, prefix)...)
		case ft.Kind() == reflect.Struct && name != "":
			out = append(out, bodyIDFields(ft, prefix+name+".")...)
		}
	}
	return out
}

// bodyFKRoutes are the mutations whose body names another object: the routes
// where an AllBuildings scope alone cannot stop a foreign id (store.Scope.AllowsBuilding).
func bodyFKRoutes() map[string][]string {
	out := map[string][]string{}
	for _, rt := range v1.Table() {
		if rt.Access == v1.Public || !rt.Mutating() || rt.Request == nil {
			continue
		}
		if fields := bodyIDFields(reflect.TypeOf(rt.Request), ""); len(fields) > 0 {
			sort.Strings(fields)
			out[rt.OperationID] = fields
		}
	}
	return out
}

// TestEveryForeignKeyRouteHasACase keeps the FK suite complete: a new body
// field that names an object needs a cross-tenant case or a written exemption.
// Path ids need no entry: the sweeps iterate every {param} route themselves.
func TestEveryForeignKeyRouteHasACase(t *testing.T) {
	ops := map[string]bool{}
	for _, rt := range v1.Table() {
		ops[rt.OperationID] = true
	}
	var missing []string
	for op, fields := range bodyFKRoutes() {
		_, covered := fkCovered[op]
		_, exempt := fkExempt[op]
		require.False(t, covered && exempt, "%s is both covered and exempt", op)
		if !covered && !exempt {
			missing = append(missing, op+" "+strings.Join(fields, ","))
		}
	}
	sort.Strings(missing)
	require.Empty(t, missing, "add a foreign-id case to TestForeignKeysAreRefused (fkCovered) or a reasoned fkExempt entry")
	for op := range fkCovered {
		require.True(t, ops[op], "fkCovered names %s, which is not in the route table", op)
	}
	for op, why := range fkExempt {
		require.True(t, ops[op], "fkExempt names %s, which is not in the route table", op)
		require.NotEmpty(t, why, "fkExempt %s needs a reason", op)
	}
}
