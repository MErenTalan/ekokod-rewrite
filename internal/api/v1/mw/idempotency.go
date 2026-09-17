package mw

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/MErenTalan/ekokod-rewrite/internal/api/v1/kit"
)

const (
	idempotencyTTL     = 24 * time.Hour
	idempotencyMaxKey  = 128
	idempotencyMaxBody = 1 << 20
)

// Idempotency replays a mutating response for a repeated Idempotency-Key (R155).
type Idempotency struct {
	Redis  *goredis.Client
	Prefix string
}

type idemRecord struct {
	State       string `json:"state"` // pending | done
	BodyHash    string `json:"body_hash"`
	Status      int    `json:"status,omitempty"`
	ContentType string `json:"content_type,omitempty"`
	Body        []byte `json:"body,omitempty"`
}

type capture struct {
	http.ResponseWriter
	status int
	buf    bytes.Buffer
	over   bool
}

func (c *capture) WriteHeader(status int) {
	if c.status == 0 {
		c.status = status
	}
	c.ResponseWriter.WriteHeader(status)
}

func (c *capture) Write(b []byte) (int, error) {
	if c.status == 0 {
		c.status = http.StatusOK
	}
	if !c.over {
		if c.buf.Len()+len(b) > idempotencyMaxBody {
			c.over = true
			c.buf.Reset()
		} else {
			c.buf.Write(b)
		}
	}
	return c.ResponseWriter.Write(b)
}

// Middleware applies to one operation.
func (m Idempotency) Middleware(op string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := r.Header.Get("Idempotency-Key")
			p, authenticated := PrincipalFrom(r.Context())
			if key == "" || !authenticated {
				next.ServeHTTP(w, r)
				return
			}
			if len(key) > idempotencyMaxKey {
				kit.WriteError(w, r, kit.ErrInvalidParameters.WithParams(map[string]any{"Idempotency-Key": []string{"max"}}))
				return
			}
			raw, err := io.ReadAll(r.Body)
			if err != nil {
				kit.WriteError(w, r, kit.ErrBodyTooLarge)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(raw))
			bodySum := sha256.Sum256(raw)
			bodyHash := hex.EncodeToString(bodySum[:])
			keySum := sha256.Sum256([]byte(key))
			redisKey := m.Prefix + "idem:" + p.User.ID.String() + ":" + op + ":" + hex.EncodeToString(keySum[:])
			ctx := r.Context()

			pending, _ := json.Marshal(idemRecord{State: "pending", BodyHash: bodyHash})
			created, err := m.Redis.SetNX(ctx, redisKey, pending, idempotencyTTL).Result()
			if err != nil {
				kit.WriteError(w, r, err)
				return
			}
			if !created {
				m.replay(w, r, redisKey, bodyHash)
				return
			}
			c := &capture{ResponseWriter: w}
			completed := false
			defer func() {
				if !completed {
					m.Redis.Del(ctx, redisKey)
				}
			}()
			next.ServeHTTP(c, r)
			if c.status >= 500 || c.over || c.status == 0 {
				return
			}
			done, _ := json.Marshal(idemRecord{State: "done", BodyHash: bodyHash, Status: c.status,
				ContentType: c.Header().Get("Content-Type"), Body: c.buf.Bytes()})
			if err := m.Redis.Set(ctx, redisKey, done, idempotencyTTL).Err(); err == nil {
				completed = true
			}
		})
	}
}

func (m Idempotency) replay(w http.ResponseWriter, r *http.Request, redisKey, bodyHash string) {
	raw, err := m.Redis.Get(r.Context(), redisKey).Bytes()
	if errors.Is(err, goredis.Nil) {
		kit.WriteError(w, r, kit.ErrIdempotencyBusy)
		return
	}
	if err != nil {
		kit.WriteError(w, r, err)
		return
	}
	var rec idemRecord
	if err := json.Unmarshal(raw, &rec); err != nil {
		kit.WriteError(w, r, err)
		return
	}
	switch {
	case rec.BodyHash != bodyHash:
		kit.WriteError(w, r, kit.ErrIdempotencyReuse)
	case rec.State != "done":
		kit.WriteError(w, r, kit.ErrIdempotencyBusy)
	default:
		if rec.ContentType != "" {
			w.Header().Set("Content-Type", rec.ContentType)
		}
		w.Header().Set("Idempotent-Replay", "true")
		w.WriteHeader(rec.Status)
		_, _ = w.Write(rec.Body)
	}
}
