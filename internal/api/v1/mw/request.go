package mw

import (
	"mime"
	"net/http"

	"github.com/MErenTalan/ekokod-rewrite/internal/api/middleware"
	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/kit"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
)

// BodyLimit caps request bodies; the binder reports an overflow as 413.
func BodyLimit(n int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, n)
			next.ServeHTTP(w, r)
		})
	}
}

// ContentType requires JSON (or multipart where a route allows it) on any
// request that carries a body (R144).
func ContentType(multipart bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}
			header := r.Header.Get("Content-Type")
			if header == "" && r.ContentLength == 0 {
				next.ServeHTTP(w, r)
				return
			}
			media, _, err := mime.ParseMediaType(header)
			if err != nil || (media != "application/json" && (!multipart || media != "multipart/form-data")) {
				kit.WriteError(w, r, kit.ErrUnsupportedMedia)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// PrincipalLimit applies the API bucket per authenticated user (R180).
func PrincipalLimit(limit config.RateLimit) func(http.Handler) http.Handler {
	return middleware.RateLimitBy(limit, func(r *http.Request) string {
		if p, ok := PrincipalFrom(r.Context()); ok {
			return "user:" + p.User.ID.String()
		}
		return ""
	})
}
