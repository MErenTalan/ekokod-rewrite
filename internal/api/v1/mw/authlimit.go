package mw

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	goredis "github.com/redis/go-redis/v9"

	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/kit"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
)

// AuthLimiter is R147's fixed-window limiter for credential endpoints, in
// Redis so every API instance shares the count.
type AuthLimiter struct {
	Redis    *goredis.Client
	Limit    config.RateLimit
	ClientIP func(*http.Request) string
	Prefix   string
}

// Middleware counts one attempt per request; byEmail adds the body's e-mail to the key.
func (l AuthLimiter) Middleware(op string, byEmail bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := l.Prefix + "authlimit:" + op + ":" + l.ClientIP(r)
			if byEmail {
				raw, err := io.ReadAll(r.Body)
				if err != nil {
					kit.WriteError(w, r, kit.ErrBodyTooLarge)
					return
				}
				r.Body = io.NopCloser(bytes.NewReader(raw))
				var body struct {
					Email string `json:"email"`
				}
				_ = json.Unmarshal(raw, &body)
				key += ":" + strings.ToLower(strings.TrimSpace(body.Email))
			}
			ctx := r.Context()
			pipe := l.Redis.TxPipeline()
			count := pipe.Incr(ctx, key)
			pipe.ExpireNX(ctx, key, l.Limit.Window)
			ttl := pipe.TTL(ctx, key)
			if _, err := pipe.Exec(ctx); err != nil {
				kit.WriteError(w, r, err)
				return
			}
			if count.Val() > int64(l.Limit.Limit) {
				w.Header().Set("Retry-After", strconv.Itoa(max(1, int(ttl.Val().Seconds()))))
				kit.WriteError(w, r, kit.ErrRateLimited)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
