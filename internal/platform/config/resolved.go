package config

import (
	"fmt"
	"net/url"
	"strings"
)

// Resolved is one configuration variable as the process actually sees it.
type Resolved struct {
	Name   string
	Value  string // already masked when Secret is true
	Secret bool
	Source string // "env" or "default"
}

const maskGlyph = "••••••••"

func maskSecret(raw string) string {
	if raw == "" {
		return maskGlyph + " (unset)"
	}
	return fmt.Sprintf("%s (len=%d)", maskGlyph, len(raw))
}

// dsnMaskGlyph is the redaction marker for DSN passwords: 4 "•" runes.
var dsnMaskGlyph = strings.Repeat("•", 4)

// dsnInvalid and dsnWithheld are the two phrases redactDSN falls back to
// when it cannot safely redact in place, matching the pattern the codebase
// already established for an unparseable DSN (requiredDSN's
// "(invalid — value withheld)" at load.go) and for a value this package
// cannot reason about at all (internal/job/scrub.go's "details withheld").
// dsnDisplay always prepends the mask glyph in front of whichever of these
// is returned, so the caller sees "•••••••• (...)" — a glyph followed by an
// explanation, never a glyph followed by the raw value.
const (
	dsnInvalid  = "(invalid — value withheld)"
	dsnWithheld = "(non-URL DSN — value withheld)"
)

// redactDSN renders raw for display with any password removed. It must
// never return raw unredacted on any path (task 9 review round 2,
// Critical): the two early exits below used to be a bare `return raw`,
// which — combined with dsnDisplay unconditionally prepending the mask
// glyph — made an un-redacted DSN look redacted while printing its
// password in clear. That was reachable for any DSN whose credential does
// not live in URL userinfo: pgx's keyword/value form
// ("host=... password=... dbname=..."), or a URL-shaped DSN that carries
// its credential in the query string (?password=... or ?sslpassword=...,
// both real libpq/pgx connection parameters) instead of, or in addition
// to, userinfo.
//
// Only a URL-shaped DSN — one url.Parse accepts AND that carries a
// non-empty Scheme — is redacted in place: the userinfo password is
// stripped, and the password/sslpassword query parameters are stripped
// too, keeping the rest (host, port, database, sslmode, ...) readable,
// since that is the useful signal this line exists to show.
//
// Anything else has its value withheld entirely rather than partially
// redacted:
//   - a parse error: matches requiredDSN's own existing behaviour.
//   - no scheme (pgx's keyword/value form, or any other non-URL text):
//     deliberately NOT tokenised on whitespace/'=' to hunt for
//     "password=...", because pgx's keyword/value grammar supports
//     quoting and backslash escapes that make such splitting fallible —
//     and a fallible redactor guarding a credential is worse than one
//     that withholds the value outright.
func redactDSN(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return dsnInvalid
	}
	if u.Scheme == "" {
		return dsnWithheld
	}

	if u.User != nil {
		if _, hasPassword := u.User.Password(); hasPassword {
			u.User = url.UserPassword(u.User.Username(), "")
		}
	}

	if q := u.Query(); q.Has("password") || q.Has("sslpassword") {
		q.Del("password")
		q.Del("sslpassword")
		u.RawQuery = q.Encode()
	}

	out := u.String()
	// url.String renders an empty password as "user:@host"; make it explicit.
	return strings.Replace(out, ":@", ":"+dsnMaskGlyph+"@", 1)
}
