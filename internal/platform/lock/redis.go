package lock

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// redisKeyPrefix namespaces every Acquire/Release key in the shared Redis
// keyspace. redisConsumePrefix is a SEPARATE namespace for Consume (M2): it
// must not share redisKeyPrefix with Acquire/Release, or Consume("x") and
// Acquire("x") would collide on the same Redis key — Consume marking "x" as
// used would make Acquire("x") block (and a held Acquire("x") lease would
// make Consume("x") report found=false), even though the two are unrelated
// operations. This mirrors Memory, whose held and consumed maps are already
// separate.
const (
	redisKeyPrefix     = "ekokod:lock:"
	redisConsumePrefix = "ekokod:consume:"
)

// redisPollStart and redisPollMax bound Acquire's polling backoff against a
// busy key: it waits redisPollStart, doubling on every retry, capped at
// redisPollMax, until ctx is done.
const (
	redisPollStart = 50 * time.Millisecond
	redisPollMax   = time.Second
)

// redisReleaseScript deletes KEYS[1] only if its current value is still
// ARGV[1] — the compare-and-delete that makes Release owner-only. A holder
// whose lease already expired (and was reacquired by someone else) then
// calling Release must NOT delete the new holder's key.
var redisReleaseScript = goredis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("DEL", KEYS[1])
end
return 0
`)

// Redis is a Locker and Consumer backed by a shared Redis instance.
type Redis struct {
	client goredis.UniversalClient
}

var (
	_ Locker   = (*Redis)(nil)
	_ Consumer = (*Redis)(nil)
	_ Lease    = (*redisLease)(nil)
)

// NewRedis returns a Redis locker over client.
func NewRedis(client goredis.UniversalClient) *Redis {
	return &Redis{client: client}
}

// Acquire issues SET key token NX PX ttl, retrying with doubling backoff
// (redisPollStart to redisPollMax) until it succeeds or ctx is done.
func (r *Redis) Acquire(ctx context.Context, key string, ttl time.Duration) (Lease, error) {
	if ttl <= 0 {
		return nil, ErrInvalidTTL
	}

	token, err := randomToken()
	if err != nil {
		return nil, fmt.Errorf("lock: generate token: %w", err)
	}
	fullKey := redisKeyPrefix + key
	wait := redisPollStart

	for {
		ok, err := r.client.SetNX(ctx, fullKey, token, ttl).Result()
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, fmt.Errorf("%w: %w", ErrNotAcquired, ctxErr)
			}
			return nil, fmt.Errorf("lock: acquire %q: %w", key, err)
		}
		if ok {
			return &redisLease{client: r.client, key: fullKey, token: token}, nil
		}

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("%w: %w", ErrNotAcquired, ctx.Err())
		case <-time.After(wait):
		}
		wait *= 2
		if wait > redisPollMax {
			wait = redisPollMax
		}
	}
}

// Consume issues SET key 1 NX PX ttl. The NX failure IS the "already used"
// signal — there is no separate poll/wait path here to accidentally
// introduce, unlike Acquire.
func (r *Redis) Consume(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	if ttl <= 0 {
		return false, ErrInvalidTTL
	}

	fullKey := redisConsumePrefix + key
	ok, err := r.client.SetNX(ctx, fullKey, "1", ttl).Result()
	if err != nil {
		return false, fmt.Errorf("lock: consume %q: %w", key, err)
	}
	return ok, nil
}

// redisLease is a lease held against a Redis locker.
type redisLease struct {
	client goredis.UniversalClient
	key    string
	token  string
}

// Release runs redisReleaseScript so a lease deletes its key only if the
// stored token still matches — a stale Release (TTL already expired, key
// reacquired by someone else) must never delete the new holder's key.
func (l *redisLease) Release(ctx context.Context) error {
	if err := redisReleaseScript.Run(ctx, l.client, []string{l.key}, l.token).Err(); err != nil {
		return fmt.Errorf("lock: release %q: %w", l.key, err)
	}
	return nil
}

// randomToken returns a random lease token unique enough across processes
// to make Redis's compare-and-delete meaningful.
func randomToken() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
