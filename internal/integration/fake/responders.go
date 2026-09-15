package fake

import (
	"net/http"
	"sync"
)

// JSON responds with status and body verbatim, Content-Type application/json.
func JSON(status int, body []byte) Responder {
	return Raw(status, "application/json", body)
}

// Raw responds with status, contentType (when non-empty) and body verbatim.
func Raw(status int, contentType string, body []byte) Responder {
	return func(w http.ResponseWriter, _ *http.Request) {
		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}
}

// RateLimited responds 429, setting Retry-After when retryAfter is
// non-empty.
func RateLimited(retryAfter string) Responder {
	return func(w http.ResponseWriter, _ *http.Request) {
		if retryAfter != "" {
			w.Header().Set("Retry-After", retryAfter)
		}
		w.WriteHeader(http.StatusTooManyRequests)
	}
}

// Sequence returns a Responder whose n-th call invokes the n-th of rs; once
// rs is exhausted, every further call invokes the last one.
func Sequence(rs ...Responder) Responder {
	var (
		mu   sync.Mutex
		call int
	)
	return func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		idx := call
		if idx >= len(rs) {
			idx = len(rs) - 1
		}
		call++
		mu.Unlock()
		rs[idx](w, r)
	}
}
