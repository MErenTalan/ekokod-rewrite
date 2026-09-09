// Package middleware holds the HTTP middleware chain. Nothing here contains
// business logic.
package middleware

import (
	"net/http"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/id"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/logging"
)

// HeaderRequestID is the header carrying the correlation id.
const HeaderRequestID = "X-Request-Id"

// RequestID reads or generates a request id, echoes it and puts it in context.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get(HeaderRequestID)
		if requestID == "" {
			requestID = id.New()
		}
		w.Header().Set(HeaderRequestID, requestID)
		ctx := logging.WithField(r.Context(), logging.FieldRequestID, requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
