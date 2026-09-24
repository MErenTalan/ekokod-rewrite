package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/api/middleware"
)

// TestMetricsRecordsUnmatchedRoutesUnderAConstantLabel guards against
// unbounded Prometheus label cardinality: a request to a path that never
// matches a registered chi route (a 404, or an unauthenticated caller
// probing arbitrary paths) must be recorded under a single constant "route"
// label rather than the raw request path, otherwise every distinct path an
// attacker sends mints a new, permanent label combination.
func TestMetricsRecordsUnmatchedRoutesUnderAConstantLabel(t *testing.T) {
	r := chi.NewRouter()
	r.Use(middleware.Metrics)
	r.Get("/known", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	unmatchedPath := "/this-path-is-never-registered-" + t.Name()
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, unmatchedPath, nil))
	require.Equal(t, http.StatusNotFound, rec.Code)

	scrape := httptest.NewRecorder()
	promhttp.Handler().ServeHTTP(scrape, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/metrics", nil))
	body := scrape.Body.String()

	require.Contains(t, body, `route="unmatched"`,
		"an unmatched request must be recorded under the constant \"unmatched\" label")
	require.NotContains(t, body, `route="`+unmatchedPath+`"`,
		"the raw unmatched path must never become a Prometheus label value")
}
