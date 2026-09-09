package middleware_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/MErenTalan/ekokod-rewrite/internal/api/middleware"
	"github.com/stretchr/testify/require"
)

func TestRecovererTurnsAPanicInto500AndLogsIt(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))

	handler := middleware.Recoverer(log)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))

	rec := httptest.NewRecorder()
	require.NotPanics(t, func() {
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	})
	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.Contains(t, rec.Body.String(), "internal")
	require.Contains(t, buf.String(), "panic")
	require.NotContains(t, rec.Body.String(), "boom", "the panic value must not reach the client")
}
