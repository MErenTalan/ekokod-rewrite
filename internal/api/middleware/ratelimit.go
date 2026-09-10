package middleware

import (
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
	trusted := parseCIDRs(trustedProxies)
	buckets := &bucketSet{
		limiters: map[string]*bucketEntry{},
		rate:     rate.Limit(float64(limit.Limit) / limit.Window.Seconds()),
		burst:    limit.Limit,
	}
	go buckets.reapEvery(10 * time.Minute)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !buckets.allow(clientKey(r, trusted)) {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.Header().Set("Retry-After", "60")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"code":"rate_limited","message_key":"errors.generic.rateLimited"}`))
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
