package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/api/middleware"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/stretchr/testify/require"
)

func TestRateLimitBlocksAfterTheLimitAndIsPerClient(t *testing.T) {
	limited := middleware.RateLimit(config.RateLimit{Limit: 2, Window: time.Minute}, nil)(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))

	call := func(ip string) int {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
		req.RemoteAddr = ip + ":12345"
		rec := httptest.NewRecorder()
		limited.ServeHTTP(rec, req)
		return rec.Code
	}

	require.Equal(t, http.StatusOK, call("10.0.0.1"))
	require.Equal(t, http.StatusOK, call("10.0.0.1"))
	require.Equal(t, http.StatusTooManyRequests, call("10.0.0.1"))
	require.Equal(t, http.StatusOK, call("10.0.0.2"), "the limit is per client, not global")
}

func TestRateLimitIgnoresForwardedForFromUntrustedProxies(t *testing.T) {
	limited := middleware.RateLimit(config.RateLimit{Limit: 1, Window: time.Minute}, nil)(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))

	call := func(forwarded string) int {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
		req.RemoteAddr = "10.0.0.9:1111"
		req.Header.Set("X-Forwarded-For", forwarded)
		rec := httptest.NewRecorder()
		limited.ServeHTTP(rec, req)
		return rec.Code
	}

	require.Equal(t, http.StatusOK, call("1.1.1.1"))
	require.Equal(t, http.StatusTooManyRequests, call("2.2.2.2"),
		"a spoofed X-Forwarded-For must not reset the bucket when the proxy is untrusted")
}
