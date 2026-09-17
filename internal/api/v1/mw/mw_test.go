package mw_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/dto"
	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/kit"
	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/mw"
	"github.com/MErenTalan/ekokod-rewrite/internal/auth"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	perr "github.com/MErenTalan/ekokod-rewrite/internal/platform/errors"
	authsvc "github.com/MErenTalan/ekokod-rewrite/internal/service/auth"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

type fakeAuth struct {
	tokens map[string]authsvc.Principal
	errs   map[string]error
	scope  func(p authsvc.Principal, param *uuid.UUID) (store.Scope, error)
	calls  []string
}

func (f *fakeAuth) Authenticate(_ context.Context, token, ua string) (authsvc.Principal, error) {
	f.calls = append(f.calls, token+"|"+ua)
	if err, ok := f.errs[token]; ok {
		return authsvc.Principal{}, err
	}
	p, ok := f.tokens[token]
	if !ok {
		return authsvc.Principal{}, authsvc.ErrTokenInvalid
	}
	return p, nil
}

func (f *fakeAuth) ResolveScope(_ context.Context, p authsvc.Principal, param *uuid.UUID) (store.Scope, error) {
	return f.scope(p, param)
}

func principal(role model.UserRole) authsvc.Principal {
	return authsvc.Principal{User: model.User{ID: uuid.New(), CompanyID: uuid.New(), Role: role}}
}

func ok(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }

func code(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body dto.Error
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body), rec.Body.String())
	return body.Error.Code
}

func do(h http.Handler, method, target string, mutate func(*http.Request)) *httptest.ResponseRecorder {
	req := httptest.NewRequestWithContext(context.Background(), method, target, nil)
	if mutate != nil {
		mutate(req)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestAuthnBearerPrecedenceAndCodes(t *testing.T) {
	good := principal(model.UserRoleCompanyAdmin)
	fa := &fakeAuth{
		tokens: map[string]authsvc.Principal{"good": good},
		errs:   map[string]error{"expired": authsvc.ErrTokenExpired, "moved": authsvc.ErrDeviceMismatch},
	}
	var seen authsvc.Principal
	h := mw.Authn(fa)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen, _ = mw.PrincipalFrom(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))

	rec := do(h, http.MethodGet, "/", func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer bad")
		r.AddCookie(&http.Cookie{Name: kit.AccessCookie, Value: "good"})
	})
	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.Equal(t, "token_invalid", code(t, rec), "an invalid bearer never falls back to the cookie")

	rec = do(h, http.MethodGet, "/", func(r *http.Request) { r.Header.Set("Authorization", "Basic abc") })
	require.Equal(t, "token_invalid", code(t, rec))

	rec = do(h, http.MethodGet, "/", func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer good")
		r.Header.Set("User-Agent", "UA-1")
	})
	require.Equal(t, http.StatusNoContent, rec.Code)
	require.Equal(t, good.User.ID, seen.User.ID)
	require.Contains(t, fa.calls, "good|UA-1", "the User-Agent reaches device binding")

	rec = do(h, http.MethodGet, "/", nil)
	require.Equal(t, "unauthorized", code(t, rec))

	rec = do(h, http.MethodGet, "/", func(r *http.Request) { r.AddCookie(&http.Cookie{Name: kit.AccessCookie, Value: "expired"}) })
	require.Equal(t, "token_expired", code(t, rec))
	require.Empty(t, rec.Result().Cookies(), "an expired access token keeps the refresh cookie")

	rec = do(h, http.MethodGet, "/", func(r *http.Request) { r.AddCookie(&http.Cookie{Name: kit.AccessCookie, Value: "moved"}) })
	require.Equal(t, "device_mismatch", code(t, rec))
	cleared := map[string]int{}
	for _, c := range rec.Result().Cookies() {
		cleared[c.Name] = c.MaxAge
	}
	require.Equal(t, map[string]int{kit.AccessCookie: -1, kit.RefreshCookie: -1}, cleared)
}

func TestRequireRoles(t *testing.T) {
	h := mw.RequireRoles(auth.Roles(model.UserRoleAdmin, model.UserRoleCompanyAdmin))(http.HandlerFunc(ok))
	for role, want := range map[model.UserRole]int{
		model.UserRoleAdmin: 204, model.UserRoleCompanyAdmin: 204, model.UserRoleCompanyReadonlyAdmin: 403, model.UserRoleDemo: 403,
	} {
		rec := do(h, http.MethodPost, "/", func(r *http.Request) {
			*r = *r.WithContext(mw.WithPrincipal(r.Context(), principal(role)))
		})
		require.Equal(t, want, rec.Code, role)
	}
	require.Equal(t, http.StatusUnauthorized, do(h, http.MethodPost, "/", nil).Code)
	empty := mw.RequireRoles(auth.Roles())(http.HandlerFunc(ok))
	rec := do(empty, http.MethodGet, "/", func(r *http.Request) {
		*r = *r.WithContext(mw.WithPrincipal(r.Context(), principal(model.UserRoleAdmin)))
	})
	require.Equal(t, http.StatusForbidden, rec.Code, "an empty role set allows nobody")
}

func TestScopeMiddlewarePassesCompanyParam(t *testing.T) {
	p := principal(model.UserRoleAdmin)
	other := uuid.New()
	fa := &fakeAuth{scope: func(_ authsvc.Principal, param *uuid.UUID) (store.Scope, error) {
		if param == nil {
			return store.Scope{CompanyID: p.User.CompanyID, AllBuildings: true}, nil
		}
		if *param == other {
			return store.Scope{CompanyID: other, AllBuildings: true}, nil
		}
		return store.Scope{}, perr.NotFound
	}}
	var got store.Scope
	h := mw.Scope(fa)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = mw.ScopeFrom(r)
		w.WriteHeader(http.StatusNoContent)
	}))
	withP := func(r *http.Request) { *r = *r.WithContext(mw.WithPrincipal(r.Context(), p)) }

	require.Equal(t, 204, do(h, http.MethodGet, "/", withP).Code)
	require.Equal(t, p.User.CompanyID, got.CompanyID)
	require.Equal(t, 204, do(h, http.MethodGet, "/?company_id="+other.String(), withP).Code)
	require.Equal(t, other, got.CompanyID)
	rec := do(h, http.MethodGet, "/?company_id="+uuid.NewString(), withP)
	require.Equal(t, 404, rec.Code)
	rec = do(h, http.MethodGet, "/?company_id=nope", withP)
	require.Equal(t, "invalid_parameters", code(t, rec))
	require.False(t, mw.ScopeFrom(httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)).Valid())
}

func TestContentTypeRequired(t *testing.T) {
	h := mw.ContentType(false)(http.HandlerFunc(ok))
	post := func(ct, body string) int {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", strings.NewReader(body))
		if ct != "" {
			req.Header.Set("Content-Type", ct)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	require.Equal(t, http.StatusUnsupportedMediaType, post("text/plain", "x"))
	require.Equal(t, http.StatusUnsupportedMediaType, post("application/x-www-form-urlencoded", "a=b"))
	require.Equal(t, http.StatusUnsupportedMediaType, post("", "{}"))
	require.Equal(t, http.StatusUnsupportedMediaType, post("multipart/form-data; boundary=x", "--x"))
	require.Equal(t, http.StatusNoContent, post("application/json; charset=utf-8", "{}"))
	require.Equal(t, http.StatusNoContent, post("", ""))
	h = mw.ContentType(true)(http.HandlerFunc(ok))
	require.Equal(t, http.StatusNoContent, post("multipart/form-data; boundary=x", "--x"))
}

func TestPrincipalRateLimit(t *testing.T) {
	a, b := principal(model.UserRoleCompanyAdmin), principal(model.UserRoleCompanyAdmin)
	h := mw.PrincipalLimit(config.RateLimit{Limit: 2, Window: time.Minute})(http.HandlerFunc(ok))
	as := func(p authsvc.Principal) int {
		return do(h, http.MethodGet, "/", func(r *http.Request) { *r = *r.WithContext(mw.WithPrincipal(r.Context(), p)) }).Code
	}
	require.Equal(t, 204, as(a))
	require.Equal(t, 204, as(a))
	require.Equal(t, 429, as(a))
	require.Equal(t, 204, as(b), "buckets are per user")
}

type fakeAudit struct {
	entries  []model.AuditEntry
	platform []model.AuditEntry
	scopes   []store.Scope
}

func (f *fakeAudit) Append(_ context.Context, s store.Scope, e model.AuditEntry) (model.AuditEntry, error) {
	f.entries = append(f.entries, e)
	f.scopes = append(f.scopes, s)
	return e, nil
}

func (f *fakeAudit) List(context.Context, store.Scope, store.AuditFilter) ([]model.AuditEntry, error) {
	return nil, nil
}

func (f *fakeAudit) AppendPlatform(_ context.Context, e model.AuditEntry) (model.AuditEntry, error) {
	f.platform = append(f.platform, e)
	return e, nil
}

func TestAuditAppendsOnSuccessOnly(t *testing.T) {
	fa := &fakeAudit{}
	auditor := mw.Auditor{Tenant: fa, Platform: fa, ClientIP: func(*http.Request) string { return "10.0.0.7" }, Log: nil}
	p := principal(model.UserRoleCompanyAdmin)
	sc := store.Scope{CompanyID: p.User.CompanyID, AllBuildings: true}
	status := http.StatusCreated
	r := chi.NewRouter()
	r.With(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx := mw.WithScope(mw.WithPrincipal(req.Context(), p), sc)
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	}, auditor.Middleware(mw.AuditInfo{Operation: "buildings.update", Entity: "building"})).
		Patch("/buildings/{id}", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) })
	id := uuid.New()

	require.Equal(t, 201, do(r, http.MethodPatch, "/buildings/"+id.String(), nil).Code)
	require.Len(t, fa.entries, 1)
	e := fa.entries[0]
	require.Equal(t, "buildings.update", e.Action)
	require.Equal(t, "building", e.EntityType)
	require.Equal(t, id, *e.EntityID)
	require.Equal(t, p.User.ID, *e.UserID)
	require.Equal(t, p.User.CompanyID, *e.CompanyID)
	require.Equal(t, "10.0.0.7", e.IP.String())
	require.Equal(t, sc, fa.scopes[0])

	status = http.StatusUnprocessableEntity
	require.Equal(t, 422, do(r, http.MethodPatch, "/buildings/"+id.String(), nil).Code)
	require.Len(t, fa.entries, 1, "a failed mutation is not audited")
}
