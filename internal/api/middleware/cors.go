package middleware

import (
	"net/http"
	"strings"

	"github.com/go-chi/cors"
)

// CORS allows exactly the configured origins. With none configured, no
// cross-origin request is allowed.
//
// Origins are matched against an explicit allow-list built with
// AllowOriginFunc rather than handed to the library's AllowedOrigins list:
// go-chi/cors treats an empty AllowedOrigins (or a literal "*" entry) as
// "allow every origin", which combined with AllowCredentials would emit
// Access-Control-Allow-Origin: * alongside Access-Control-Allow-Credentials:
// true — a wildcard-with-credentials response that must never be sent. A
// "*" entry in the configured origins is therefore ignored rather than
// honoured, and an arbitrary Origin header is never reflected back — only a
// value present in the allow-list is ever echoed.
func CORS(origins []string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(origins))
	for _, origin := range origins {
		if origin == "*" {
			continue
		}
		allowed[strings.ToLower(origin)] = struct{}{}
	}

	return cors.Handler(cors.Options{
		AllowOriginFunc: func(_ *http.Request, origin string) bool {
			_, ok := allowed[strings.ToLower(origin)]
			return ok
		},
		AllowedMethods:   []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", HeaderRequestID, "Accept-Language"},
		ExposedHeaders:   []string{HeaderRequestID},
		AllowCredentials: true,
		MaxAge:           300,
	})
}
