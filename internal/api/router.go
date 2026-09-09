// Package api exposes the HTTP surface. Handlers decode, authorise, call a
// service and encode — they contain no business logic.
package api

import (
	"log/slog"
	"net/http"

	"github.com/MErenTalan/ekokod-rewrite/internal/api/middleware"
	"github.com/MErenTalan/ekokod-rewrite/internal/buildinfo"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/health"
	"github.com/go-chi/chi/v5"
)

// Deps are everything the HTTP surface needs.
type Deps struct {
	Cfg         *config.Config
	Log         *slog.Logger
	Build       buildinfo.Info
	ReadyChecks []health.Check
}

// NewRouter builds the HTTP handler.
func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer(d.Log))
	r.Use(middleware.Logger(d.Log))
	r.Use(middleware.CORS(d.Cfg.HTTP.CORSOrigins))
	r.Use(middleware.Metrics)
	r.Use(middleware.RateLimit(d.Cfg.HTTP.RateLimitAPI, d.Cfg.HTTP.TrustedProxies))

	r.Get("/health/live", liveHandler())
	r.Get("/health/ready", readyHandler(d.ReadyChecks))
	r.Get("/version", versionHandler(d.Build))
	r.Handle("/metrics", middleware.MetricsHandler())

	return r
}
