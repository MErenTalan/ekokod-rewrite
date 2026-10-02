package postgres

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

// R508: a deadlock victim goes again; anything else is returned as it came.
func TestRetryOnDeadlock(t *testing.T) {
	t.Parallel()
	deadlock := fmt.Errorf("reading bulk insert: %w", &pgconn.PgError{Code: "40P01"})
	other := errors.New("boom")

	calls := 0
	err := retryOnDeadlock(context.Background(), func() error {
		calls++
		if calls < 3 {
			return deadlock
		}
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 3, calls, "two victims, then through")

	calls = 0
	err = retryOnDeadlock(context.Background(), func() error { calls++; return other })
	require.ErrorIs(t, err, other)
	require.Equal(t, 1, calls, "only a deadlock is retried")

	calls = 0
	err = retryOnDeadlock(context.Background(), func() error { calls++; return deadlock })
	require.ErrorIs(t, err, deadlock)
	require.Equal(t, deadlockAttempts, calls, "bounded")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls = 0
	err = retryOnDeadlock(ctx, func() error { calls++; return deadlock })
	require.ErrorIs(t, err, deadlock)
	require.Equal(t, 1, calls, "a done context is not waited out")
}
