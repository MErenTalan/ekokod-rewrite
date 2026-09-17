package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/MErenTalan/ekokod-rewrite/internal/api"
	v1 "github.com/MErenTalan/ekokod-rewrite/internal/api/v1"
	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/mw"
	"github.com/MErenTalan/ekokod-rewrite/internal/buildinfo"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/health"
	"github.com/stretchr/testify/require"
)

func testConfig(t *testing.T) *config.Config {
	t.Helper()
	env := map[string]string{
		"EKOKOD_PUBLIC_URL":                "http://localhost:3000",
		"EKOKOD_DB_URL":                    "postgres://u:p@localhost:5432/ekokod?sslmode=disable",
		"EKOKOD_REDIS_URL":                 "redis://localhost:6379/0",
		"EKOKOD_ENCRYPTION_KEY":            "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=",
		"EKOKOD_JWT_SIGNING_KEY":           "jwt-signing-key-at-least-32-chars-long!!",
		"EKOKOD_PASSWORD_PEPPER":           "password-pepper-at-least-32-chars-long!!",
		"EKOKOD_DEVICE_FINGERPRINT_SECRET": "device-fingerprint-secret-32-chars-min!!",
		"EKOKOD_STORAGE_ROOT":              "/tmp/ekokod",
		"EKOKOD_EPIAS_USERNAME":            "u",
		"EKOKOD_EPIAS_PASSWORD":            "p",
		"EKOKOD_ML_API_KEY":                "k",
		"EKOKOD_CORS_ORIGINS":              "http://localhost:3000",
	}
	cfg, err := config.Load(func(k string) (string, bool) { v, ok := env[k]; return v, ok })
	require.NoError(t, err)
	return cfg
}

func newTestRouter(t *testing.T, checks ...health.Check) http.Handler {
	t.Helper()
	return api.NewRouter(api.Deps{
		Cfg:         testConfig(t),
		Log:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		Build:       buildinfo.Get(),
		ReadyChecks: checks,
	})
}

func TestHealthLiveAlwaysReturns200(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestRouter(t).ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/health/live", nil))
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestHealthReadyReportsEachDependency(t *testing.T) {
	router := newTestRouter(t,
		health.Check{Name: "database", Fn: func(context.Context) error { return nil }},
		health.Check{Name: "redis", Fn: func(context.Context) error { return nil }},
		health.Check{Name: "migrations", Fn: func(context.Context) error { return nil }},
	)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/health/ready", nil))
	require.Equal(t, http.StatusOK, rec.Code)

	var report health.Report
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &report))
	require.Equal(t, "ok", report.Status)
	require.Len(t, report.Checks, 3)
}

func TestHealthReadyReturns503WhenADependencyIsDown(t *testing.T) {
	router := newTestRouter(t,
		health.Check{Name: "database", Fn: func(context.Context) error { return errors.New("connection refused") }},
	)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/health/ready", nil))
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.Contains(t, rec.Body.String(), "connection refused")
}

func TestVersionEndpoint(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestRouter(t).ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/version", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "version")
}

func TestMetricsEndpointExposesHTTPMetrics(t *testing.T) {
	router := newTestRouter(t)

	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/health/live", nil))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/metrics", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "ekokod_http_request_duration_seconds")
}

func TestRequestIDIsEchoedAndGenerated(t *testing.T) {
	router := newTestRouter(t)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/health/live", nil))
	require.NotEmpty(t, rec.Header().Get("X-Request-Id"))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/health/live", nil)
	req.Header.Set("X-Request-Id", "caller-supplied-id")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, "caller-supplied-id", rec.Header().Get("X-Request-Id"))
}

func TestCORSAllowsTheConfiguredOriginOnly(t *testing.T) {
	router := newTestRouter(t)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodOptions, "/health/live", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Access-Control-Request-Method", "GET")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, "http://localhost:3000", rec.Header().Get("Access-Control-Allow-Origin"))

	req = httptest.NewRequestWithContext(context.Background(), http.MethodOptions, "/health/live", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	req.Header.Set("Access-Control-Request-Method", "GET")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Empty(t, rec.Header().Get("Access-Control-Allow-Origin"))
}

func TestAPIV1MountedWithErrorEnvelope(t *testing.T) {
	router := api.NewRouter(api.Deps{
		Cfg: testConfig(t), Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Build: buildinfo.Get(),
		V1: v1.NewRouterForTable(v1.Table(), &v1.Handlers{}, identityMiddleware(), nil),
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/nope", nil))
	require.Equal(t, http.StatusNotFound, rec.Code)
	var body struct {
		Error struct {
			Code      string `json:"code"`
			Message   string `json:"message"`
			RequestID string `json:"request_id"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "not_found", body.Error.Code)
	require.NotEmpty(t, body.Error.RequestID)

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodDelete, "/api/v1/openapi.json", nil))
	require.Equal(t, http.StatusMethodNotAllowed, rec.Code)

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/openapi.json", nil))
	require.Equal(t, http.StatusOK, rec.Code)
}

func identityMiddleware() v1.Middleware {
	id := func(next http.Handler) http.Handler { return next }
	return v1.Middleware{
		Authn: id, PrincipalLimit: id, Scope: id,
		AuthLimit:   func(string, bool) func(http.Handler) http.Handler { return id },
		Idempotency: func(string) func(http.Handler) http.Handler { return id },
		Audit:       func(mw.AuditInfo) func(http.Handler) http.Handler { return id },
	}
}
