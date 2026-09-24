package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/api/middleware"
)

func TestSecurityHeaders(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		w.WriteHeader(http.StatusOK)
	})
	for _, https := range []bool{false, true} {
		rec := httptest.NewRecorder()
		middleware.SecurityHeaders(https)(ok).ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/x", nil))
		h := rec.Header()
		require.Equal(t, "nosniff", h.Get("X-Content-Type-Options"))
		require.Equal(t, "no-referrer", h.Get("Referrer-Policy"))
		require.Equal(t, "DENY", h.Get("X-Frame-Options"))
		require.Empty(t, h.Get("Content-Security-Policy"), "a file keeps no CSP: Chrome's PDF viewer would not render it")
		require.Equal(t, "same-origin", h.Get("Cross-Origin-Resource-Policy"))
		require.Equal(t, "application/pdf", h.Get("Content-Type"), "a download keeps its own type")
		if https {
			require.Equal(t, "max-age=31536000; includeSubDomains", h.Get("Strict-Transport-Security"))
		} else {
			require.Empty(t, h.Get("Strict-Transport-Security"), "HSTS over plain HTTP is meaningless (Q-L4)")
		}
	}
}

func TestAJSONResponseGetsTheLockedDownCSP(t *testing.T) {
	for _, write := range []func(w http.ResponseWriter){
		func(w http.ResponseWriter) {
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(http.StatusNotFound)
		},
		func(w http.ResponseWriter) { _, _ = w.Write([]byte(`{"ok":true}`)) }, // implicit 200, sniffed type
	} {
		rec := httptest.NewRecorder()
		middleware.SecurityHeaders(false)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { write(w) })).
			ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/x", nil))
		require.Equal(t, "default-src 'none'; frame-ancestors 'none'", rec.Header().Get("Content-Security-Policy"))
	}
}
