package redis

import (
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/secret"
)

// scrubErr guarantees the returned error never contains any fragment of
// password, while keeping the underlying error reachable by errors.Is and
// errors.As. Use it for errors raised after the URL was parsed successfully
// (e.g. a ping or dial failure), where password is the credential actually
// in use by the client that produced err.
//
// See secret.Wrap for why the redacted text and the traversable cause are
// split across Error and Unwrap, and for the deliberate consequence that
// errors.Unwrap(err).Error() still yields unredacted text.
func scrubErr(password, op string, err error) error {
	return secret.Wrap(op, secret.Fragments(password), err)
}
