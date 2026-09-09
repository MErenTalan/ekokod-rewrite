package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var requestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "ekokod_http_request_duration_seconds",
	Help:    "HTTP request duration by route, method and status.",
	Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
}, []string{"route", "method", "status"})

// Metrics records request duration per route.
func Metrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		ww := chimiddleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)

		pattern := r.URL.Path
		if rc := chiRoutePattern(r); rc != "" {
			pattern = rc
		}
		requestDuration.WithLabelValues(pattern, r.Method, strconv.Itoa(ww.Status())).
			Observe(time.Since(started).Seconds())
	})
}

// MetricsHandler serves the Prometheus scrape endpoint.
func MetricsHandler() http.Handler { return promhttp.Handler() }

func chiRoutePattern(r *http.Request) string {
	if rctx := chi.RouteContext(r.Context()); rctx != nil {
		return rctx.RoutePattern()
	}
	return ""
}
