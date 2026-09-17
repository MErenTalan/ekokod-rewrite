package v1

import (
	_ "embed"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/kit"
	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/mw"
	perr "github.com/MErenTalan/ekokod-rewrite/internal/platform/errors"
)

//go:embed openapi.json
var openAPIDocument []byte

// OpenAPIDocument is the committed document GET /openapi.json serves.
func OpenAPIDocument() []byte { return openAPIDocument }

func serveOpenAPI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(openAPIDocument)
}

// Handlers holds the services the endpoint handlers call.
type Handlers struct{}

// Middleware is the per-route middleware the router composes; tests swap it.
type Middleware struct {
	Authn          func(http.Handler) http.Handler
	PrincipalLimit func(http.Handler) http.Handler
	Scope          func(http.Handler) http.Handler
	AuthLimit      func(op string, byEmail bool) func(http.Handler) http.Handler
	Idempotency    func(op string) func(http.Handler) http.Handler
	Audit          func(mw.AuditInfo) func(http.Handler) http.Handler
}

const maxBody = 1 << 20

// NewRouter builds /api/v1 from Table.
func NewRouter(h *Handlers, m Middleware, log *slog.Logger) http.Handler {
	return NewRouterForTable(Table(), h, m, log)
}

// NewRouterForTable builds a router for routes, in the R157 middleware order.
func NewRouterForTable(routes []Route, h *Handlers, m Middleware, log *slog.Logger) http.Handler {
	if log != nil {
		kit.Logger = log
	}
	r := chi.NewRouter()
	r.Use(mw.BodyLimit(maxBody))
	r.NotFound(func(w http.ResponseWriter, req *http.Request) { kit.WriteError(w, req, perr.NotFound) })
	r.MethodNotAllowed(func(w http.ResponseWriter, req *http.Request) { kit.WriteError(w, req, kit.ErrMethodNotAllowed) })
	for _, rt := range routes {
		chain := []func(http.Handler) http.Handler{mw.ContentType(rt.Multipart)}
		switch rt.AuthLimit {
		case AuthLimitByEmail:
			chain = append(chain, m.AuthLimit(rt.OperationID, true))
		case AuthLimitByIP:
			chain = append(chain, m.AuthLimit(rt.OperationID, false))
		}
		if rt.Access != Public {
			chain = append(chain, m.Authn, m.PrincipalLimit)
			if rt.Access == RoleGated {
				chain = append(chain, mw.RequireRoles(rt.Roles))
			}
			chain = append(chain, m.Scope)
			if rt.Mutating() {
				if !rt.NoIdempotency {
					chain = append(chain, m.Idempotency(rt.OperationID))
				}
				chain = append(chain, m.Audit(mw.AuditInfo{Operation: rt.OperationID, Entity: rt.Entity, Platform: rt.PlatformAudit}))
			}
		}
		r.With(chain...).Method(rt.Method, rt.Pattern, rt.Handler(h))
	}
	return r
}
