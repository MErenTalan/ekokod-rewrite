package redis

import (
	"fmt"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/secret"
)

// scrubParseErr reports a redis URL parse failure without ever including the
// raw input: goredis.ParseURL forwards net/url's parse error unchanged, and
// url.Error.Error() embeds its whole argument — including any password —
// verbatim. We cannot reason about what a URL we cannot even parse might
// contain, so the underlying error text is dropped entirely rather than
// risk leaking a credential.
func scrubParseErr(op string) error {
	return fmt.Errorf("%s: redis url is not valid (details withheld)", op)
}

// scrubErr guarantees the returned error never contains any fragment of
// password. Use it for errors raised after the URL was parsed successfully
// (e.g. a ping or dial failure), where password is the credential actually
// in use by the client that produced err. See internal/platform/secret for
// the fragment-based redaction this relies on.
func scrubErr(password, op string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %s", op, secret.Redact(err.Error(), secret.Fragments(password)))
}
