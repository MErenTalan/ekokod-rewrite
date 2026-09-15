// Package lock is the CANONICAL owner of the Locker/Lease contract (F2 plan
// ruling R2). internal/integration/httpx (Task 2) imports this package and
// aliases httpx.Locker = lock.Locker, httpx.Lease = lock.Lease — a type
// alias, not a structurally-identical redeclaration, which is what lets
// *Memory and *Redis satisfy httpx.Locker with no adapter anywhere: Go does
// NOT treat two separately named interfaces with identical method sets as
// satisfying each other when a method's return type is itself one of those
// named interfaces.
//
// Ownership sits here, not in httpx, because after the wave-scheduling move
// this package has zero F2-task dependencies and starts in Wave A; if httpx
// (Wave B) owned the canonical type, this package would need to import a
// not-yet-merged httpx package to implement it.
package lock

import (
	"context"
	"errors"
	"time"
)

// ErrNotAcquired is returned when Acquire could not obtain the lease before
// ctx was done.
var ErrNotAcquired = errors.New("lock: not acquired")

// Locker acquires exclusive, TTL-bounded leases on a named key. Acquire
// blocks, polling, until it obtains the lease or ctx is done.
type Locker interface {
	Acquire(ctx context.Context, key string, ttl time.Duration) (Lease, error)
}

// Lease is a held lock. Release gives it up before its TTL expires.
type Lease interface {
	Release(ctx context.Context) error
}

// Consumer is a one-shot, NON-BLOCKING atomic consume (F2 plan ruling R3).
// Unlike Locker, which waits by polling until ctx is done, Consume never
// waits: there is nothing to wait FOR once a key is already gone. It exists
// for single-use tokens (an OAuth state nonce, for example) where a replay
// must fail fast, not queue behind the request that already consumed it.
type Consumer interface {
	// Consume marks key as used for ttl and reports whether THIS call is
	// the one that marked it. found=true: first use — proceed. found=false:
	// the key was already consumed (or, for Memory, is still within a
	// previous consume's ttl) — the caller rejects immediately, no waiting.
	Consume(ctx context.Context, key string, ttl time.Duration) (found bool, err error)
}
