package postgres

import (
	"fmt"
	"net/url"
	"strings"
)

// redactionMask replaces every password fragment this package scrubs.
const redactionMask = "••••••••"

// urlDelimiters are the runes that are structurally significant inside a DSN
// (or any URL). A password containing one of them unescaped can be split
// differently by different parsers — pgx's own conninfo parser splits
// userinfo from host at the *first* unescaped "@", while net/url (correctly,
// per RFC 3986) splits at the *last* one. When the two disagree, a fragment
// of the password — not the whole password — can end up embedded in a
// downstream error (for example as a bogus hostname in a DNS-lookup
// failure). Splitting the password on these delimiters and redacting every
// resulting fragment, in addition to the whole password, closes that gap.
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

// scrubErr wraps err under op, guaranteeing the result never contains any
// fragment of dsn's password. dsn is parsed once with net/url; if it cannot
// be parsed at all, the underlying error text is dropped entirely rather
// than risk leaking a credential — we cannot reason about what a DSN we
// cannot even parse might contain, or how a different parser downstream
// might have split it.
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
	return fmt.Errorf("%s: %s", op, redact(err.Error(), passwordFragmentsOf(password)))
}

// scrubPoolErr is scrubErr's counterpart for callers that only hold a
// *pgxpool.Pool, not the original DSN string. It scrubs using the password
// pgx itself parsed out of the connection string, which is the credential
// actually in use by this pool.
func scrubPoolErr(password, op string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %s", op, redact(err.Error(), passwordFragmentsOf(password)))
}
