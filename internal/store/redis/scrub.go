package redis

import (
	"fmt"
	"strings"
)

// redactionMask replaces every password fragment this package scrubs.
const redactionMask = "••••••••"

// urlDelimiters are the runes that are structurally significant inside a
// redis URL. A password containing one of them unescaped can end up split
// differently by different parsers, so a fragment of the password — not
// just the whole password — can leak into a downstream error (for example a
// bogus host in a DNS-lookup failure). Splitting the password on these
// delimiters and redacting every resulting fragment, in addition to the
// whole password, closes that gap. See internal/store/postgres/scrub.go for
// the DSN counterpart of this same defect class.
const urlDelimiters = "@:/?#&="

// passwordFragmentsOf returns password itself plus every non-trivial
// fragment produced by splitting it on urlDelimiters. Fragments shorter than
// two characters are skipped: redacting single characters would make error
// text useless without meaningfully protecting the credential.
func passwordFragmentsOf(password string) []string {
	if password == "" {
		return nil
	}
	fragments := []string{password}
	for _, frag := range strings.FieldsFunc(password, func(r rune) bool {
		return strings.ContainsRune(urlDelimiters, r)
	}) {
		if len(frag) >= 2 {
			fragments = append(fragments, frag)
		}
	}
	return fragments
}

// redact removes every occurrence of every fragment in fragments from msg.
func redact(msg string, fragments []string) string {
	for _, frag := range fragments {
		msg = strings.ReplaceAll(msg, frag, redactionMask)
	}
	return msg
}

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
// in use by the client that produced err.
func scrubErr(password, op string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %s", op, redact(err.Error(), passwordFragmentsOf(password)))
}
