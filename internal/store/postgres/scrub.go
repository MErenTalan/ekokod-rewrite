package postgres

import (
	"net/url"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/secret"
)

// scrubErr wraps err under op, guaranteeing the result never contains any
// fragment of dsn's password. dsn is parsed once with net/url; if it cannot
// be parsed at all, OR if it parses but yields no password, the underlying
// error text is dropped entirely rather than risk leaking a credential — we
// cannot reason about what a DSN we cannot read might contain, or how a
// different parser downstream might have split it.
//
// This deliberately does NOT share secret.URLParseErr with the redis and job
// packages: the two have genuinely diverged. Those withhold everything the
// moment goredis.ParseURL fails, whereas this one withholds everything only
// on a net/url failure and otherwise redacts the parsed password out of a
// real driver error.
//
// The success path goes through secret.Wrap, so callers keep
// errors.Is/errors.As on the driver's error while Error() stays redacted.
func scrubErr(dsn, op string, err error) error {
	if err == nil {
		return nil
	}
	u, parseErr := url.Parse(dsn)
	if parseErr != nil {
		return secret.Withhold(op, "dsn did not parse", err)
	}
	var password string
	if u.User != nil {
		password, _ = u.User.Password()
	}
	// An empty password here does NOT mean "there is no credential to
	// protect": it means net/url found none in the userinfo, which is also
	// what it reports for `host=… password=…` keyword/value form and for
	// `postgres://user@host/db?password=…`. Both are accepted by pgx and
	// both pass config.requiredDSN's url.Parse check, so both arrive here.
	// secret.Wrap treats the resulting empty fragment list as a reason to
	// withhold the driver text entirely rather than pass it through
	// unredacted; that is deliberate and is asserted in
	// scrub_internal_test.go.
	return secret.Wrap(op, secret.Fragments(password), err)
}

// scrubPoolErr is scrubErr's counterpart for callers that only hold a
// *pgxpool.Pool, not the original DSN string. It scrubs using the password
// pgx itself parsed out of the connection string, which is the credential
// actually in use by this pool.
//
// See secret.Wrap for why the redacted text and the traversable cause are
// split across Error and Unwrap, and for the deliberate consequence that
// errors.Unwrap(err).Error() still yields unredacted text. Callers on a
// logging or HTTP-response path must print err.Error() and nothing else.
func scrubPoolErr(password, op string, err error) error {
	return secret.Wrap(op, secret.Fragments(password), err)
}
