package postgres

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/sqlcgen"
)

// DB is the entry point every repository in this package is built on.
//
// The generated query set is a NAMED, UNEXPORTED field rather than an embedded
// one, and that is load-bearing. Embedding promoted every generated method onto
// the exported DB type, which made the entry point a public, unscoped,
// context-taking query surface — exactly the cross-tenant hole
// TestEveryStoreMethodIsScoped exists to catch, and it caught it. Repositories
// live in this package, so they reach the query set as db.q with nothing lost;
// callers outside the package get Pool() and whatever scoped repository methods
// a later task adds, and no way to run an unscoped query.
//
// Keep it unembedded. Restoring the embedded field turns that guard red again.
type DB struct {
	q    *sqlcgen.Queries
	pool *pgxpool.Pool
}

// New wraps a connection pool in the generated query set.
func New(pool *pgxpool.Pool) *DB {
	return &DB{q: sqlcgen.New(pool), pool: pool}
}

// Pool returns the underlying connection pool. It is what a caller needs to
// begin a transaction, which a repository then hands to sqlcgen.Queries.WithTx;
// every non-transactional statement should go through a scoped repository
// method instead.
func (db *DB) Pool() *pgxpool.Pool { return db.pool }
