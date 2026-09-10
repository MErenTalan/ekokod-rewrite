package postgres

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/sqlcgen"
)

// DB is the entry point every repository is built on. It embeds the sqlc
// generated Queries so a repository can call a generated method directly, and
// keeps the pool so a repository that needs a transaction can begin one and
// hand the resulting pgx.Tx to Queries.WithTx.
type DB struct {
	*sqlcgen.Queries

	pool *pgxpool.Pool
}

// New wraps a connection pool in the generated query set.
func New(pool *pgxpool.Pool) *DB {
	return &DB{Queries: sqlcgen.New(pool), pool: pool}
}

// Pool returns the underlying connection pool. It is what a caller needs to
// begin a transaction; every non-transactional statement should go through an
// embedded generated method instead.
func (db *DB) Pool() *pgxpool.Pool { return db.pool }
