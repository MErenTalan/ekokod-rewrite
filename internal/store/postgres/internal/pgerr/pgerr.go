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
	"fmt"

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

// sentinel makes a wrapped error answer errors.Is for exactly one of store's
// sentinel values, while keeping the original driver error — pgx.ErrNoRows,
// or the *pgconn.PgError itself — reachable through errors.Is/errors.As for a
// caller that wants the driver's own value.
//
// This is the same Error()/Unwrap() split scrub.go's `scrubbed` type uses in
// package postgres, reused rather than reinvented: Error() prints only text
// that was already safe to print (see Translate's two comments below on why
// that is true for both branches that construct one of these), and Unwrap()
// keeps the cause traversable. The one addition over `scrubbed` is Is, which
// scrubbed has no need of because it never has to make an error answer for a
// synthetic sentinel — only for the driver's own value.
type sentinel struct {
	target error // store.ErrNotFound or store.ErrConflict
	cause  error // op-wrapped, so cause.Error() already names the operation
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
//   - everything else -> scrubbed through secret.Wrap using the password pgx
//     itself parsed out of pool's own connection string, exactly as
//     scrubPoolErr does in package postgres. (scrubPoolErr is one line of
//     secret.Wrap; this calls secret.Wrap directly rather than importing
//     package postgres, which pgerr is underneath and must not import back
//     into.)
//
// op is a short description of the failed operation, folded into the
// result's Error() text the same way every other error in this package does.
//
// Neither of the two sentinel branches can leak a credential: pgx.ErrNoRows's
// Error() is the fixed string "no rows in result set", and *pgconn.PgError's
// Error() is Severity + Message + SQLSTATE only — Detail, which is where a
// unique violation echoes the offending column VALUES ("Key (email)=(x@y)
// already exists"), is a separate field that Error() never renders. So the
// conflict branch below can wrap the driver error as-is: its Error() already
// names the constraint (Message) and never the value (Detail stays
// unreached), which is the default this task's brief asks for.
func Translate(pool *pgxpool.Pool, op string, err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, pgx.ErrNoRows) {
		return &sentinel{target: store.ErrNotFound, cause: fmt.Errorf("%s: %w", op, err)}
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
		return &sentinel{target: store.ErrConflict, cause: fmt.Errorf("%s: %w", op, err)}
	}

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
