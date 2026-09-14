package pgerr_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/secret"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/internal/pgerr"
)

var errDriver = errors.New("driver sentinel")

// newUnreachablePool builds a *pgxpool.Pool carrying password in its parsed
// config, WITHOUT ever dialing anything.
//
// pgxpool.NewWithConfig only tries to reach the server in a background
// goroutine that targets max(MinConns, MinIdleConns) idle connections
// (pgxpool@v5.11.0/pool.go NewWithConfig); ParseConfig defaults both to 0, so
// that goroutine has nothing to do and no connection is ever attempted. The
// host below (203.0.113.1, TEST-NET-3, RFC 5737) is reserved for
// documentation and guaranteed unroutable, so if this assumption were ever
// wrong the test would hang or time out rather than silently pass — it does
// neither, which is the proof.
func newUnreachablePool(t *testing.T, password string) *pgxpool.Pool {
	t.Helper()

	cfg, err := pgxpool.ParseConfig(fmt.Sprintf("postgres://ekokod:%s@203.0.113.1:5432/ekokod", password))
	require.NoError(t, err)
	require.Zero(t, cfg.MinConns, "the premise of this helper: no idle target means no dial")

	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	require.NoError(t, err, "NewWithConfig must return without dialing the unroutable host")
	t.Cleanup(pool.Close)
	return pool
}

func TestTranslateOfNilIsNil(t *testing.T) {
	pool := newUnreachablePool(t, "s3cret")
	require.NoError(t, pgerr.Translate(pool, "op", nil))
}

func TestTranslateMapsErrNoRowsToStoreErrNotFound(t *testing.T) {
	pool := newUnreachablePool(t, "s3cret")

	for name, cause := range map[string]error{
		"bare":    pgx.ErrNoRows,
		"wrapped": fmt.Errorf("scan row: %w", pgx.ErrNoRows),
	} {
		t.Run(name, func(t *testing.T) {
			err := pgerr.Translate(pool, "get building", cause)

			require.ErrorIs(t, err, store.ErrNotFound)
			require.ErrorIs(t, err, pgx.ErrNoRows, "the driver sentinel must stay reachable")
			require.NotContains(t, err.Error(), "s3cret")
		})
	}
}

func TestTranslateMapsUniqueViolationToStoreErrConflict(t *testing.T) {
	pool := newUnreachablePool(t, "s3cret")

	cause := &pgconn.PgError{
		Code:           "23505",
		Message:        `duplicate key value violates unique constraint "users_email_key"`,
		ConstraintName: "users_email_key",
		// Detail echoes the offending VALUES, which must never reach a
		// caller. Present here so the assertion below is not vacuous.
		Detail: "Key (email)=(admin@example.com) already exists.",
	}

	err := pgerr.Translate(pool, "create user", cause)

	require.ErrorIs(t, err, store.ErrConflict)
	var pgErr *pgconn.PgError
	require.ErrorAs(t, err, &pgErr, "the driver's structured error must stay reachable")
	require.Equal(t, "users_email_key", pgErr.ConstraintName)

	msg := err.Error()
	require.NotContains(t, msg, "s3cret")
	require.Contains(t, msg, "users_email_key", "Error() may name the constraint")
	require.NotContains(t, msg, "admin@example.com", "Error() must never echo Detail's column values")
	require.NotContains(t, msg, "already exists.", "Error() must never render Detail at all")
}

// TestTranslateDoesNotMapForeignKeyOrCheckViolationsToConflict pins the
// scope limit stated in the brief: 23503 and 23504 are caller bugs or scope
// escapes, not a conflict a client can retry past, so they must fall through
// to the scrubbed default like any other error.
func TestTranslateDoesNotMapForeignKeyOrCheckViolationsToConflict(t *testing.T) {
	pool := newUnreachablePool(t, "s3cret")

	for name, code := range map[string]string{
		"foreign_key_violation": "23503",
		"check_violation":       "23514",
	} {
		t.Run(name, func(t *testing.T) {
			cause := &pgconn.PgError{Code: code, Message: "constraint violated"}

			err := pgerr.Translate(pool, "insert reading", cause)

			require.False(t, errors.Is(err, store.ErrConflict),
				"%s must not be mapped to ErrConflict", name)
			require.False(t, errors.Is(err, store.ErrNotFound))
			require.True(t, secret.IsScrubbed(err), "it must still go through the default scrubbed branch")
		})
	}
}

// TestTranslateScrubsEverythingElseUsingThePoolsPassword is the fourth
// outcome: an error that is neither ErrNoRows nor a unique violation is
// routed through secret.Wrap using the pool's own parsed password, exactly
// as scrubPoolErr does in package postgres.
func TestTranslateScrubsEverythingElseUsingThePoolsPassword(t *testing.T) {
	pool := newUnreachablePool(t, "s3cret")
	cause := fmt.Errorf("dial tcp: password authentication failed using %q: %w", "s3cret", errDriver)

	err := pgerr.Translate(pool, "list buildings", cause)

	require.NotContains(t, err.Error(), "s3cret")
	require.Contains(t, err.Error(), secret.Mask, "the mask is the positive proof that redaction ran")
	require.Contains(t, err.Error(), "list buildings")
	require.ErrorIs(t, err, errDriver, "errors.Is must still traverse the scrubbed error")
	require.True(t, secret.IsScrubbed(err))
}

// TestTranslateNeverLeaksThePasswordInAnyOutcome repeats the no-leak
// assertion across all four outcomes against the SAME pool and password, so
// that a future change to any one branch cannot silently start leaking a
// credential without a red test somewhere in this file.
func TestTranslateNeverLeaksThePasswordInAnyOutcome(t *testing.T) {
	const password = "s3cret"
	pool := newUnreachablePool(t, password)

	outcomes := map[string]error{
		"not found": pgx.ErrNoRows,
		"conflict":  &pgconn.PgError{Code: "23505", Message: "duplicate key value violates unique constraint \"x\""},
		"default":   fmt.Errorf("auth failed for %q: %w", password, errDriver),
	}
	for name, cause := range outcomes {
		t.Run(name, func(t *testing.T) {
			err := pgerr.Translate(pool, "op", cause)
			require.NotNil(t, err)
			require.NotContains(t, err.Error(), password)
		})
	}

	require.Nil(t, pgerr.Translate(pool, "op", nil), "the fourth outcome, nil, trivially never leaks anything")
}
