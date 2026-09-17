// Package v1 is the /api/v1 HTTP surface. One route table (Table) drives the
// router, authorisation, OpenAPI, idempotency and audit (R157).
package v1

import (
	"net/http"
	"slices"

	"github.com/MErenTalan/ekokod-rewrite/internal/auth"
)

// Access says who may reach a route before its role set is consulted.
type Access int

// The three access levels.
const (
	Public Access = iota
	Authenticated
	RoleGated
)

// AuthLimitMode selects R147's credential rate limit for a route.
type AuthLimitMode int

// The limiter keys.
const (
	NoAuthLimit AuthLimitMode = iota
	AuthLimitByEmail
	AuthLimitByIP
)

// Route is one endpoint.
type Route struct {
	Method, Pattern, OperationID, Summary, Tag string
	// Entity names the audited entity type of a mutating route.
	Entity string
	Access Access
	Roles  auth.RoleSet
	// SelfService routes touch only the caller's own account (R150).
	SelfService bool
	// NoIdempotency opts a mutating route out of Idempotency-Key replay (R155).
	NoIdempotency bool
	// PlatformAudit writes the audit row with company_id NULL (R156).
	PlatformAudit bool
	AuthLimit     AuthLimitMode
	Multipart     bool

	Request, Response any
	Status            int
	RawContentType    string

	Handler func(h *Handlers) http.HandlerFunc
}

// Mutating reports whether the route changes state.
func (rt Route) Mutating() bool {
	return rt.Method != http.MethodGet && rt.Method != http.MethodHead
}

// Table is every /api/v1 route.
func Table() []Route {
	return slices.Concat(systemRoutes())
}

func systemRoutes() []Route {
	return []Route{{
		Method: http.MethodGet, Pattern: "/openapi.json", OperationID: "system.openapi", Tag: "system",
		Summary: "The OpenAPI 3.1 document of this API.", Access: Public, Status: http.StatusOK,
		RawContentType: "application/json",
		Handler:        func(*Handlers) http.HandlerFunc { return serveOpenAPI },
	}}
}
