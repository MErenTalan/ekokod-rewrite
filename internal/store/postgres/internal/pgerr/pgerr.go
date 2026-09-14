// Package pgerr provides the one error translation every repository under
// internal/store/postgres calls on every database error.
//
// It exists so that Tasks 9, 10 and 11 — three parallel implementers building
// every repository in the project in the same package — do not each write
// their own mapping from a driver error to a store sentinel. Three divergent
// mappings (or three copies of the same one) would have been the merge
// result; this package is the one copy.
//
// It must be its own package, rather than an unexported function in package
// postgres, because it also has to be callable from
// internal/store/postgres/admin — the spec's only unscoped query surface,
// which runs queries and hits the same driver errors, but which
// internal/arch/arch_test.go and this package's own layering keep from
// importing package postgres itself. A helper package one level below both is
// the only shape that works for both callers.
package pgerr

import (
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/secret"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// uniqueViolation is Postgres SQLSTATE 23505. pgx exposes error codes as the
// raw five-character string on *pgconn.PgError with no named constant, so
// this package names the one it maps.
//
// 23503 (foreign_key_violation) and 23514 (check_violation) are deliberately
// NOT mapped here. Both are a caller passing data that violates a
// relationship or a check the caller should have enforced before writing —
// a bug or a scope escape, not a conflict a client can retry past — so they
// fall through to the default, scrubbed branch like any other error.
const uniqueViolation = "23505"

// sentinel makes an already-scrubbed error additionally answer errors.Is for
// one of store's sentinel values.
//
// It wraps a cause that has ALREADY BEEN THROUGH secret.Wrap — never the raw
// driver error — which is what makes this a thin wrapper around the one
// scrubbing style rather than a second one. Task 8c fix round 1 found the
// first version of this type built its cause as plain fmt.Errorf("%s: %w",
// op, err), on the theory that pgx.ErrNoRows and *pgconn.PgError never print
// a credential on their own — true in isolation, but err here is whatever a
// caller passed in, and a caller can (and, per the review's probe, did) wrap
// pgx.ErrNoRows or a *pgconn.PgError in text that names a DSN. Every non-nil
// outcome of Translate must be scrubbed, not just the branches that "happen
// not to need it".
//
// Error() and Unwrap() therefore both defer entirely to cause: Error() is
// already redacted (cause is secret.Wrap's own result), and Unwrap() keeps
// the FULL original chain reachable, because secret.Wrap's own type keeps
// its cause traversable too — errors.Is(result, pgx.ErrNoRows) and
// errors.As(result, &pgErr) both walk sentinel -> scrubbed -> the original
// err. Is is the one addition over secret.Wrap's own type, which never needs
// to answer for a synthetic sentinel like store.ErrNotFound — only for the
// driver's own values.
type sentinel struct {
	target error // store.ErrNotFound or store.ErrConflict
	cause  error // always the result of secret.Wrap — never a bare fmt.Errorf wrap
}

func (s *sentinel) Error() string        { return s.cause.Error() }
func (s *sentinel) Unwrap() error        { return s.cause }
func (s *sentinel) Is(target error) bool { return target == s.target }

// Translate maps a database error from a query run against pool to the
// sentinel every repository must return, per the mapping fixed by the F1
// repository conventions:
//
//   - nil -> nil, so callers can call Translate unconditionally.
//   - pgx.ErrNoRows, wrapped or not -> errors.Is(result, store.ErrNotFound).
//   - a unique_violation (SQLSTATE 23505) -> errors.Is(result, store.ErrConflict).
//   - everything else -> scrubbed the same way, with no synthetic sentinel.
//
// ALL FOUR non-nil outcomes go through scrub, which is secret.Wrap using the
// password pgx itself parsed out of pool's own connection string — exactly
// as scrubPoolErr does in package postgres. (scrubPoolErr is one line of
// secret.Wrap; this calls secret.Wrap directly rather than importing package
// postgres, which pgerr is underneath and must not import back into.) A
// unique violation's Detail, which echoes the offending column VALUES ("Key
// (email)=(x@y) already exists"), still never reaches a caller either way:
// *pgconn.PgError's own Error() renders Severity + Message + SQLSTATE only,
// never Detail, so scrubbing the driver error's text can never surface it.
//
// op is a short description of the failed operation, folded into the
// result's Error() text the same way every other error in this package does.
func Translate(pool *pgxpool.Pool, op string, err error) error {
	if err == nil {
		return nil
	}

	scrubbed := scrub(pool, op, err)

	if errors.Is(err, pgx.ErrNoRows) {
		return &sentinel{target: store.ErrNotFound, cause: scrubbed}
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
		return &sentinel{target: store.ErrConflict, cause: scrubbed}
	}

	return scrubbed
}

// scrub is the ONE scrubbing call every non-nil outcome of Translate goes
// through, so that no branch can be added later that forgets to.
func scrub(pool *pgxpool.Pool, op string, err error) error {
	return secret.Wrap(op, secret.Fragments(poolPassword(pool)), err)
}

// poolPassword returns the password pgx parsed out of pool's own connection
// string, or "" if pool or its config is nil. secret.Wrap treats an empty
// (or nil) fragment list as "withhold everything" rather than "redact
// nothing", so a nil pool fails closed here rather than passing the
// underlying error through unscrubbed.
func poolPassword(pool *pgxpool.Pool) string {
	if pool == nil {
		return ""
	}
	cfg := pool.Config()
	if cfg == nil || cfg.ConnConfig == nil {
		return ""
	}
	return cfg.ConnConfig.Password
}
