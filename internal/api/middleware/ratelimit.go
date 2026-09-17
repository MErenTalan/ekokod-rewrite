package middleware

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"golang.org/x/time/rate"
)

// RateLimit applies a per-client token bucket. The client is the peer address
// unless the peer is a trusted proxy, in which case the left-most
// X-Forwarded-For entry is used. F6 adds a per-principal bucket on top.
func RateLimit(limit config.RateLimit, trustedProxies []string) func(http.Handler) http.Handler {
	return RateLimitBy(limit, ClientIP(trustedProxies))
}

// ClientIP returns the caller-address function RateLimit keys buckets by.
func ClientIP(trustedProxies []string) func(*http.Request) string {
	trusted := parseCIDRs(trustedProxies)
	return func(r *http.Request) string { return clientKey(r, trusted) }
}

// RateLimitBy applies a token bucket per key; an empty key is not limited.
// R180 uses it per authenticated user.
func RateLimitBy(limit config.RateLimit, key func(*http.Request) string) func(http.Handler) http.Handler {
	buckets := &bucketSet{
		limiters: map[string]*bucketEntry{},
		rate:     rate.Limit(float64(limit.Limit) / limit.Window.Seconds()),
		burst:    limit.Limit,
	}
	go buckets.reapEvery(10 * time.Minute)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if k := key(r); k != "" && !buckets.allow(k) {
				w.Header().Set("Retry-After", "60")
				writeEnvelope(w, http.StatusTooManyRequests, "rate_limited",
					"Çok fazla deneme yapıldı. Lütfen daha sonra tekrar deneyin.")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

type bucketEntry struct {
	limiter *rate.Limiter
	seen    time.Time
}

type bucketSet struct {
	mu       sync.Mutex
	limiters map[string]*bucketEntry
	rate     rate.Limit
	burst    int
}

func (b *bucketSet) allow(key string) bool {
	b.mu.Lock()
	entry, ok := b.limiters[key]
	if !ok {
		entry = &bucketEntry{limiter: rate.NewLimiter(b.rate, b.burst)}
		b.limiters[key] = entry
	}
	entry.seen = time.Now()
	b.mu.Unlock()
	return entry.limiter.Allow()
}

func (b *bucketSet) reapEvery(d time.Duration) {
	for range time.Tick(d) {
		cutoff := time.Now().Add(-d)
		b.mu.Lock()
		for key, entry := range b.limiters {
			if entry.seen.Before(cutoff) {
				delete(b.limiters, key)
			}
		}
		b.mu.Unlock()
	}
}

func parseCIDRs(raw []string) []*net.IPNet {
	nets := make([]*net.IPNet, 0, len(raw))
	for _, item := range raw {
		if _, network, err := net.ParseCIDR(item); err == nil {
			nets = append(nets, network)
		}
	}
	return nets
}

// clientKey identifies the caller, honouring X-Forwarded-For only from a
// trusted proxy so a spoofed header cannot reset someone else's bucket.
func clientKey(r *http.Request, trusted []*net.IPNet) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer := net.ParseIP(host)
	for _, network := range trusted {
		if peer != nil && network.Contains(peer) {
			if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
				if comma := strings.IndexByte(forwarded, ','); comma > 0 {
					return strings.TrimSpace(forwarded[:comma])
				}
				return strings.TrimSpace(forwarded)
			}
		}
	}
	return host
}

// writeEnvelope writes the 05 §1 error envelope for the edge middleware, which
// sits outside /api/v1's localised kit (R152); the message is the Turkish default.
func writeEnvelope(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	body, _ := json.Marshal(map[string]any{"error": map[string]string{
		"code": code, "message": message, "request_id": w.Header().Get(HeaderRequestID),
	}})
	_, _ = w.Write(append(body, '\n'))
}
