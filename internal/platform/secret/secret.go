// Package secret provides small, dependency-free primitives for redacting a
// credential out of error text before it can reach a log, stderr or an HTTP
// response.
//
// It exists because the same defect has now been found and fixed twice
// independently: a URL-shaped DSN's password can be split differently by
// different parsers (pgx's own conninfo parser splits userinfo from host at
// the first unescaped "@", while net/url, correctly per RFC 3986, splits at
// the last one), so a *fragment* of the password — not the whole password —
// can end up embedded in a downstream error, for example as a bogus
// hostname in a DNS-lookup failure. See internal/store/postgres/scrub.go
// and internal/store/redis/scrub.go for the call sites that use this.
package secret

import "strings"

// Mask replaces every password fragment Redact removes.
const Mask = "••••••••"

// URLDelimiters are the runes that are structurally significant inside a
// DSN or URL. Splitting a password on these before redacting closes the
// fragment-leak gap described in the package doc: even if a downstream
// parser splits the credential differently than the one that produced Mask,
// every piece it could have produced is still redacted.
const URLDelimiters = "@:/?#&="

// Fragments returns password itself plus every non-trivial fragment
// produced by splitting it on URLDelimiters. Fragments shorter than two
// characters are skipped: redacting single characters would make error text
// useless without meaningfully protecting the credential.
func Fragments(password string) []string {
	if password == "" {
		return nil
	}
	fragments := []string{password}
	for _, frag := range strings.FieldsFunc(password, func(r rune) bool {
		return strings.ContainsRune(URLDelimiters, r)
	}) {
		if len(frag) >= 2 {
			fragments = append(fragments, frag)
		}
	}
	return fragments
}

// Redact removes every occurrence of every fragment in fragments from msg,
// replacing each with Mask.
func Redact(msg string, fragments []string) string {
	for _, frag := range fragments {
		msg = strings.ReplaceAll(msg, frag, Mask)
	}
	return msg
}
