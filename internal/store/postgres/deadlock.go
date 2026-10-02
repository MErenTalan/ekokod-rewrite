package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// deadlockDetected is SQLSTATE 40P01: Postgres chose this statement as the
// victim of a lock cycle and rolled it back.
const deadlockDetected = "40P01"

// deadlockAttempts and deadlockPause bound retryOnDeadlock.
const (
	deadlockAttempts = 5
	deadlockPause    = 40 * time.Millisecond
)

// retryOnDeadlock runs fn again when it failed as a deadlock victim (R508).
// One analyzer's backfill windows run side by side: an insert that opens a
// new chunk takes a table lock on analyzers (the chunk's foreign key) while
// it holds the analyzer's row lock, and an update of that row waiting behind
// it closes the cycle. The victim did nothing wrong and nothing of it was
// kept, so it simply goes again. fn must be safe to repeat from scratch.
func retryOnDeadlock(ctx context.Context, fn func() error) error {
	for attempt := 1; ; attempt++ {
		err := fn()
		var pgErr *pgconn.PgError
		if err == nil || attempt == deadlockAttempts || !errors.As(err, &pgErr) || pgErr.Code != deadlockDetected {
			return err
		}
		select {
		case <-ctx.Done():
			return err
		case <-time.After(time.Duration(attempt) * deadlockPause):
		}
	}
}
