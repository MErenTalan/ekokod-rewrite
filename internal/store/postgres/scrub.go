package postgres

import (
	"fmt"
	"net/url"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/secret"
)

// scrubErr wraps err under op, guaranteeing the result never contains any
// fragment of dsn's password. dsn is parsed once with net/url; if it cannot
// be parsed at all, the underlying error text is dropped entirely rather
// than risk leaking a credential — we cannot reason about what a DSN we
// cannot even parse might contain, or how a different parser downstream
// might have split it. See internal/platform/secret for the fragment-based
// redaction this relies on.
func scrubErr(dsn, op string, err error) error {
	if err == nil {
		return nil
	}
	u, parseErr := url.Parse(dsn)
	if parseErr != nil {
		return fmt.Errorf("%s: database error (details withheld: dsn did not parse)", op)
	}
	var password string
	if u.User != nil {
		password, _ = u.User.Password()
	}
	return fmt.Errorf("%s: %s", op, secret.Redact(err.Error(), secret.Fragments(password)))
}

// scrubPoolErr is scrubErr's counterpart for callers that only hold a
// *pgxpool.Pool, not the original DSN string. It scrubs using the password
// pgx itself parsed out of the connection string, which is the credential
// actually in use by this pool.
func scrubPoolErr(password, op string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %s", op, secret.Redact(err.Error(), secret.Fragments(password)))
}
