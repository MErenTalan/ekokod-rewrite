package postgres_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/stretchr/testify/require"
)

// TestErrorsNeverContainTheDSNPassword locks in the fix for the leak
// described in the review of task 5: pgx's own conninfo parser splits DSN
// userinfo from host at the *first* unescaped "@", while net/url (correctly,
// per RFC 3986) splits at the *last* one. A password containing an unescaped
// "@" therefore gets split differently by the two parsers, and a fragment of
// it — not the whole password — used to end up embedded in the resulting
// error (for example as a bogus hostname in a DNS-lookup failure), which
// then reached stderr and the /health/ready JSON body.
//
// This is a plain unit test: it never reaches a real database. The host is
// unreachable, so NewPool is guaranteed to fail, and every code path that
// can return an error (parse, dial, ping) routes through scrubErr.
func TestErrorsNeverContainTheDSNPassword(t *testing.T) {
	const password = "my@pass"
	dsn := "postgres://ekokod_user:my@pass@127.0.0.1:59999/ekokod?sslmode=disable"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	_, err := postgres.NewPool(ctx, config.DB{
		URL:              dsn,
		MaxConns:         1,
		MinConns:         0,
		MaxConnLifetime:  time.Hour,
		StatementTimeout: time.Second,
	}, log)

	require.Error(t, err, "an unreachable host must fail to produce a pool")

	msg := err.Error()
	require.NotContains(t, msg, password, "error must not contain the whole password")
	require.NotContains(t, msg, "my", "error must not contain a password fragment")
	require.NotContains(t, msg, "pass", "error must not contain a password fragment")
}
