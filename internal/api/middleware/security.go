package middleware

import (
	"net/http"
	"strings"
)

const apiCSP = "default-src 'none'; frame-ancestors 'none'"

// SecurityHeaders hardens every API response (F15a R451): nothing the API
// returns may be sniffed, framed or leak a referrer. JSON gets a CSP that runs
// nothing; files do not, because Chrome's PDF viewer refuses to render under
// one. HSTS is sent only when the public URL is HTTPS (Q-L4).
func SecurityHeaders(https bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("Referrer-Policy", "no-referrer")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Cross-Origin-Resource-Policy", "same-origin")
			if https {
				h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}
			next.ServeHTTP(&cspWriter{ResponseWriter: w}, r)
		})
	}
}

// cspWriter adds the CSP once the handler has chosen its Content-Type.
type cspWriter struct {
	http.ResponseWriter
	decided bool
}

func (c *cspWriter) decide() {
	if c.decided {
		return
	}
	c.decided = true
	ct := c.Header().Get("Content-Type")
	if ct == "" || strings.Contains(ct, "json") || strings.HasPrefix(ct, "text/") {
		c.Header().Set("Content-Security-Policy", apiCSP)
	}
}

func (c *cspWriter) WriteHeader(code int) {
	c.decide()
	c.ResponseWriter.WriteHeader(code)
}

func (c *cspWriter) Write(b []byte) (int, error) {
	c.decide()
	return c.ResponseWriter.Write(b)
}

// Flush keeps streaming responses streaming.
func (c *cspWriter) Flush() {
	if f, ok := c.ResponseWriter.(http.Flusher); ok {
		c.decide()
		f.Flush()
	}
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (c *cspWriter) Unwrap() http.ResponseWriter { return c.ResponseWriter }
