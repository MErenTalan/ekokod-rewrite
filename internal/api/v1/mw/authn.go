package mw

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/kit"
	"github.com/MErenTalan/ekokod-rewrite/internal/auth"
	perr "github.com/MErenTalan/ekokod-rewrite/internal/platform/errors"
	authsvc "github.com/MErenTalan/ekokod-rewrite/internal/service/auth"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// Authenticator is the slice of the auth service authentication needs.
type Authenticator interface {
	Authenticate(ctx context.Context, accessToken, userAgent string) (authsvc.Principal, error)
	ResolveScope(ctx context.Context, p authsvc.Principal, companyParam *uuid.UUID) (store.Scope, error)
}

// Authn authenticates a bearer token or the access cookie (R143). An invalid
// bearer never falls back to the cookie.
func Authn(a Authenticator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, fromCookie := "", false
			if header := r.Header.Get("Authorization"); header != "" {
				bearer, ok := strings.CutPrefix(header, "Bearer ")
				if !ok || strings.TrimSpace(bearer) == "" {
					kit.WriteError(w, r, authsvc.ErrTokenInvalid)
					return
				}
				token = strings.TrimSpace(bearer)
			} else if c, err := r.Cookie(kit.AccessCookie); err == nil && c.Value != "" {
				token, fromCookie = c.Value, true
			}
			if token == "" {
				kit.WriteError(w, r, perr.Unauthorized)
				return
			}
			p, err := a.Authenticate(r.Context(), token, r.UserAgent())
			if err != nil {
				if fromCookie && !errors.Is(err, authsvc.ErrTokenExpired) {
					kit.ClearAuthCookies(w)
				}
				kit.WriteError(w, r, err)
				return
			}
			next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), p)))
		})
	}
}

// RequireRoles refuses callers whose role is outside roles with 403.
func RequireRoles(roles auth.RoleSet) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, ok := PrincipalFrom(r.Context())
			if !ok {
				kit.WriteError(w, r, perr.Unauthorized)
				return
			}
			if !roles.Has(p.User.Role) {
				kit.WriteError(w, r, perr.Forbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// Scope resolves the request's Scope from the caller and company_id (R139, R158).
func Scope(a Authenticator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, ok := PrincipalFrom(r.Context())
			if !ok {
				kit.WriteError(w, r, perr.Unauthorized)
				return
			}
			var param *uuid.UUID
			if values, present := r.URL.Query()[kit.CompanyParam]; present {
				id, err := uuid.Parse(strings.Join(values, ","))
				if err != nil {
					kit.WriteError(w, r, kit.ErrInvalidParameters.WithParams(map[string]any{kit.CompanyParam: []string{"invalid"}}))
					return
				}
				param = &id
			}
			sc, err := a.ResolveScope(r.Context(), p, param)
			if err != nil {
				kit.WriteError(w, r, err)
				return
			}
			next.ServeHTTP(w, r.WithContext(WithScope(r.Context(), sc)))
		})
	}
}
