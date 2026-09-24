package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"
)

// Recoverer converts a panic into a 500 without leaking the panic value.
func Recoverer(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if recovered := recover(); recovered != nil {
					log.ErrorContext(r.Context(), "panic recovered",
						slog.Any("panic", recovered),
						slog.String("stack", string(debug.Stack())),
						slog.String("path", r.URL.Path))

					writeEnvelope(w, http.StatusInternalServerError, "internal", "Beklenmeyen bir hata oluştu.")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
