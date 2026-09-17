package v1_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	v1 "github.com/MErenTalan/ekokod-rewrite/internal/api/v1"
	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/mw"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	authsvc "github.com/MErenTalan/ekokod-rewrite/internal/service/auth"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// expectedAccess is written from 05-api-contract.md's tables, independently of
// the route table (R157): "public", "authenticated", or role abbreviations.
var expectedAccess = map[string]string{
	"GET /openapi.json": "public",

	// 05 §2
	"POST /auth/login":           "public",
	"POST /auth/refresh":         "public",
	"POST /auth/logout":          "authenticated",
	"POST /auth/logout-all":      "authenticated",
	"GET /auth/me":               "authenticated",
	"POST /auth/forgot-password": "public",
	"POST /auth/reset-password":  "public",
	"POST /auth/change-password": "A CA CR BA BR", // R150: demo excluded
	"GET /auth/sessions":         "authenticated",
	"DELETE /auth/sessions/{id}": "authenticated",
	// 05 §3
	"GET /profile":   "authenticated",
	"PATCH /profile": "A CA CR BA BR", // R150
	// 05 §18
	"POST /mobile/auth/login":   "public",
	"POST /mobile/auth/refresh": "public",
	"GET /mobile/auth/me":       "authenticated",
}

// selfServiceMutations is R150's literal exemption list.
var selfServiceMutations = map[string]bool{
	"POST /auth/logout": true, "POST /auth/logout-all": true, "POST /auth/change-password": true,
	"DELETE /auth/sessions/{id}": true, "PATCH /profile": true,
}

var abbreviations = map[string]model.UserRole{
	"A": model.UserRoleAdmin, "CA": model.UserRoleCompanyAdmin, "CR": model.UserRoleCompanyReadonlyAdmin,
	"BA": model.UserRoleBuildingAdmin, "BR": model.UserRoleBuildingReadonlyAdmin, "D": model.UserRoleDemo,
}

// passthrough authenticates from X-Test-Role and skips every other middleware.
func passthrough(scope *store.Scope) v1.Middleware {
	id := func(next http.Handler) http.Handler { return next }
	return v1.Middleware{
		Authn: func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				role := r.Header.Get("X-Test-Role")
				if role == "" {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				p := authsvc.Principal{User: model.User{ID: uuid.New(), CompanyID: uuid.New(), Role: model.UserRole(role)}}
				next.ServeHTTP(w, r.WithContext(mw.WithPrincipal(r.Context(), p)))
			})
		},
		PrincipalLimit: id,
		Scope:          id,
		AuthLimit:      func(string, bool) func(http.Handler) http.Handler { return id },
		Idempotency:    func(string) func(http.Handler) http.Handler { return id },
		Audit:          func(mw.AuditInfo) func(http.Handler) http.Handler { return id },
	}
}

func sentinelTable() []v1.Route {
	routes := v1.Table()
	for i := range routes {
		routes[i].Handler = func(_ *v1.Handlers, w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) }
	}
	return routes
}

func concretePath(pattern string) string {
	parts := strings.Split(pattern, "/")
	for i, p := range parts {
		if strings.HasPrefix(p, "{") {
			parts[i] = uuid.NewString()
		}
	}
	return strings.Join(parts, "/")
}

func call(router http.Handler, rt v1.Route, role model.UserRole) int {
	req := httptest.NewRequestWithContext(context.Background(), rt.Method, concretePath(rt.Pattern), nil)
	if role != "" {
		req.Header.Set("X-Test-Role", string(role))
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec.Code
}

func TestMatrixCoversEveryRoute(t *testing.T) {
	inTable := map[string]bool{}
	for _, rt := range v1.Table() {
		key := rt.Method + " " + rt.Pattern
		inTable[key] = true
		require.Contains(t, expectedAccess, key, "add %s to expectedAccess from 05-api-contract.md", key)
	}
	for key := range expectedAccess {
		require.True(t, inTable[key], "expectedAccess names %s, which is not in the route table", key)
	}
	for key := range selfServiceMutations {
		require.True(t, inTable[key], "selfServiceMutations names %s, which is not in the route table", key)
	}
}

func TestAuthorizationMatrix(t *testing.T) {
	router := v1.NewRouterForTable(sentinelTable(), &v1.Handlers{}, passthrough(nil), nil)
	for _, rt := range v1.Table() {
		key := rt.Method + " " + rt.Pattern
		want := expectedAccess[key]
		allowed := map[model.UserRole]bool{}
		switch want {
		case "public", "authenticated":
			for _, r := range model.UserRoles() {
				allowed[r] = true
			}
		default:
			for _, abbr := range strings.Fields(want) {
				role, ok := abbreviations[abbr]
				require.True(t, ok, "%s: unknown role abbreviation %q", key, abbr)
				allowed[role] = true
			}
		}
		for _, role := range model.UserRoles() {
			t.Run(key+"/"+string(role), func(t *testing.T) {
				got := call(router, rt, role)
				if allowed[role] {
					require.Equal(t, http.StatusTeapot, got, "%s must reach the handler", role)
				} else {
					require.Equal(t, http.StatusForbidden, got, "%s must be refused", role)
				}
			})
		}
		t.Run(key+"/anonymous", func(t *testing.T) {
			got := call(router, rt, "")
			if want == "public" {
				require.Equal(t, http.StatusTeapot, got)
			} else {
				require.Equal(t, http.StatusUnauthorized, got)
			}
		})
	}
}

func TestReadOnlyRolesForbiddenOnEveryMutation(t *testing.T) {
	router := v1.NewRouterForTable(sentinelTable(), &v1.Handlers{}, passthrough(nil), nil)
	var keys []string
	for _, rt := range v1.Table() {
		key := rt.Method + " " + rt.Pattern
		if !rt.Mutating() || rt.Access == v1.Public {
			continue
		}
		keys = append(keys, key)
		readOnly := []model.UserRole{model.UserRoleCompanyReadonlyAdmin, model.UserRoleBuildingReadonlyAdmin, model.UserRoleDemo}
		for _, role := range readOnly {
			t.Run(key+"/"+string(role), func(t *testing.T) {
				got := call(router, rt, role)
				if selfServiceMutations[key] && role != model.UserRoleDemo {
					require.Equal(t, http.StatusTeapot, got, "R150 self-service route")
					return
				}
				if selfServiceMutations[key] && strings.Contains(expectedAccess[key], "authenticated") {
					require.Equal(t, http.StatusTeapot, got, "R150 self-service route open to demo")
					return
				}
				require.Equal(t, http.StatusForbidden, got)
			})
		}
	}
	sort.Strings(keys)
	t.Logf("%d mutating routes checked", len(keys))
}
