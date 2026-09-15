package lock

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// memoryPollInterval bounds how often a blocked Acquire re-checks a busy
// key. It is deliberately small: Memory is for unit tests and single-process
// use, where the whole point of polling is to make Acquire respect ctx
// promptly, not to spare CPU the way a distributed Redis client would.
const memoryPollInterval = 5 * time.Millisecond

// memoryTokenSeq hands out unique lease tokens within one process. An
// atomic counter is enough here — Memory is single-process by definition,
// so cross-process token collision (the reason Redis uses a random one)
// cannot happen.
var memoryTokenSeq atomic.Uint64

func newMemoryToken() string {
	return strconv.FormatUint(memoryTokenSeq.Add(1), 36)
}

// memoryEntry is one held lease.
type memoryEntry struct {
	token  string
	expiry time.Time
}

// Memory is a Locker and Consumer for unit tests and single-process use.
type Memory struct {
	now func() time.Time

	mu       sync.Mutex
	held     map[string]*memoryEntry
	consumed map[string]time.Time
}

var (
	_ Locker   = (*Memory)(nil)
	_ Consumer = (*Memory)(nil)
	_ Lease    = (*memoryLease)(nil)
)

// NewMemory returns a Memory locker. now is injectable for TTL tests; a nil
// now uses time.Now.
func NewMemory(now func() time.Time) *Memory {
	if now == nil {
		now = time.Now
	}
	return &Memory{
		now:      now,
		held:     make(map[string]*memoryEntry),
		consumed: make(map[string]time.Time),
	}
}

// Acquire blocks, polling every memoryPollInterval, until key is free or ctx
// is done.
func (m *Memory) Acquire(ctx context.Context, key string, ttl time.Duration) (Lease, error) {
	if ttl <= 0 {
		return nil, ErrInvalidTTL
	}

	token := newMemoryToken()
	for {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrNotAcquired, err)
		}

		if lease, ok := m.tryAcquire(key, ttl, token); ok {
			return lease, nil
		}

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("%w: %w", ErrNotAcquired, ctx.Err())
		case <-time.After(memoryPollInterval):
		}
	}
}

func (m *Memory) tryAcquire(key string, ttl time.Duration, token string) (Lease, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := m.now()
	if entry, held := m.held[key]; held && now.Before(entry.expiry) {
		return nil, false
	}
	m.held[key] = &memoryEntry{token: token, expiry: now.Add(ttl)}
	return &memoryLease{m: m, key: key, token: token}, true
}

// Consume is a single lookup-and-set under one mutex critical section — no
// loop, no wait. See Consumer.
func (m *Memory) Consume(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	if ttl <= 0 {
		return false, ErrInvalidTTL
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	now := m.now()
	if expiry, ok := m.consumed[key]; ok && now.Before(expiry) {
		return false, nil
	}
	m.consumed[key] = now.Add(ttl)
	return true, nil
}

// memoryLease is a lease held against a Memory locker. Release only clears
// the key if this lease's token is still the one recorded as holding it —
// the same owner check Redis enforces via its compare-and-delete script.
// Without this check, a lease whose TTL already expired (and was reacquired
// by someone else) would delete the NEW holder's entry on a stale Release.
type memoryLease struct {
	m     *Memory
	key   string
	token string
}

func (l *memoryLease) Release(_ context.Context) error {
	l.m.mu.Lock()
	defer l.m.mu.Unlock()

	if entry, ok := l.m.held[l.key]; ok && entry.token == l.token {
		delete(l.m.held, l.key)
	}
	return nil
}
