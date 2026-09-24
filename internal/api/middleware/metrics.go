package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var requestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "ekokod_http_request_duration_seconds",
	Help:    "HTTP request duration by route, method and status.",
	Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
}, []string{"route", "method", "status"})

// unmatchedRoute is the constant label used for requests that never matched
// a registered chi route (404s, vulnerability-scanner probes, arbitrary
// paths). chi only populates RoutePattern() on a successful route match, so
// falling back to the raw request path here would let an unauthenticated
// caller mint one Prometheus label combination per distinct path they probe
// — an unbounded, attacker-controlled cardinality (and memory) leak, since
// HistogramVec entries are never evicted. Recording every unmatched request
// under a single constant label keeps cardinality bounded regardless of what
// a caller sends.
const unmatchedRoute = "unmatched"

// Metrics records request duration per route.
func Metrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		ww := chimiddleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)

		pattern := chiRoutePattern(r)
		if pattern == "" {
			pattern = unmatchedRoute
		}
		requestDuration.WithLabelValues(pattern, r.Method, strconv.Itoa(ww.Status())).
			Observe(time.Since(started).Seconds())
	})
}

func chiRoutePattern(r *http.Request) string {
	if rctx := chi.RouteContext(r.Context()); rctx != nil {
		return rctx.RoutePattern()
	}
	return ""
}
