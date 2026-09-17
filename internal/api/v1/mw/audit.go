package mw

import (
	"log/slog"
	"net/http"
	"net/netip"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// Auditor appends an audit row for every successful mutation (R156).
type Auditor struct {
	Tenant   store.AuditRepository
	Platform store.AdminAuditRepository
	ClientIP func(*http.Request) string
	Log      *slog.Logger
}

// AuditInfo names what a route mutates.
type AuditInfo struct {
	Operation, Entity string
	Platform          bool
}

// Middleware records the mutation after the handler succeeds; a failed audit
// write is logged and never fails the request.
func (a Auditor) Middleware(info AuditInfo) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			if ww.Status() < 200 || ww.Status() > 299 {
				return
			}
			entry := model.AuditEntry{Action: info.Operation, EntityType: info.Entity}
			if id, err := uuid.Parse(chi.URLParam(r, "id")); err == nil {
				entry.EntityID = &id
			}
			if ip, err := netip.ParseAddr(a.ClientIP(r)); err == nil {
				entry.IP = &ip
			}
			if p, ok := PrincipalFrom(r.Context()); ok {
				userID := p.User.ID
				entry.UserID = &userID
			}
			var err error
			if info.Platform {
				_, err = a.Platform.AppendPlatform(r.Context(), entry)
			} else {
				sc := ScopeFrom(r)
				company := sc.CompanyID
				entry.CompanyID = &company
				_, err = a.Tenant.Append(r.Context(), sc, entry)
			}
			if err != nil {
				a.Log.WarnContext(r.Context(), "api: audit append failed", slog.String("action", info.Operation), slog.String("error", err.Error()))
			}
		})
	}
}
