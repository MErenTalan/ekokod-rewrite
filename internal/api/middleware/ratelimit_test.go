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

// Behind trusted proxies (front proxy → Next → API) the client is the rightmost untrusted hop:
// anything to its left was written by the client itself.
func TestClientIPTakesTheRightmostUntrustedHop(t *testing.T) {
	clientIP := middleware.ClientIP([]string{"10.0.0.0/8", "127.0.0.1/32"})
	ip := func(forwarded string) string {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
		req.RemoteAddr = "127.0.0.1:1111"
		if forwarded != "" {
			req.Header.Set("X-Forwarded-For", forwarded)
		}
		return clientIP(req)
	}
	require.Equal(t, "203.0.113.7", ip("203.0.113.7"))
	require.Equal(t, "203.0.113.7", ip("6.6.6.6, 203.0.113.7"), "a client-written entry must not win")
	require.Equal(t, "203.0.113.7", ip("6.6.6.6, 203.0.113.7, 10.1.2.3"), "trusted hops are skipped")
	require.Equal(t, "203.0.113.7", ip(" garbage , 203.0.113.7 "))
	require.Equal(t, "10.9.9.9", ip("10.9.9.9, 10.1.2.3"), "all trusted: the leftmost is the best we know")
	require.Equal(t, "127.0.0.1", ip("203.0.113.7, not-an-ip"), "an unparseable nearest hop falls back to the peer")
	require.Equal(t, "127.0.0.1", ip(""))
}
