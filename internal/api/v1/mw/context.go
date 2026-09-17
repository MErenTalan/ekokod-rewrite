// Package mw is the /api/v1 middleware: authentication, role checks, scope
// resolution, content-type and body limits, auth rate limiting, idempotency
// and audit (R139–R158, R180).
package mw

import (
	"context"
	"net/http"

	authsvc "github.com/MErenTalan/ekokod-rewrite/internal/service/auth"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

type ctxKey int

const (
	principalKey ctxKey = iota
	scopeKey
)

// WithPrincipal stores the authenticated caller.
func WithPrincipal(ctx context.Context, p authsvc.Principal) context.Context {
	return context.WithValue(ctx, principalKey, p)
}

// PrincipalFrom returns the authenticated caller.
func PrincipalFrom(ctx context.Context) (authsvc.Principal, bool) {
	p, ok := ctx.Value(principalKey).(authsvc.Principal)
	return p, ok
}

// WithScope stores the request's resolved scope.
func WithScope(ctx context.Context, sc store.Scope) context.Context {
	return context.WithValue(ctx, scopeKey, sc)
}

// ScopeFrom returns the resolved scope; a zero (invalid) scope when absent,
// which every repository refuses.
func ScopeFrom(r *http.Request) store.Scope {
	sc, _ := r.Context().Value(scopeKey).(store.Scope)
	return sc
}
