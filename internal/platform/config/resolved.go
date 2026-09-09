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

// redactDSN removes the password from a URL-shaped DSN, leaving it readable.
func redactDSN(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		return raw
	}
	if _, hasPassword := u.User.Password(); !hasPassword {
		return raw
	}
	u.User = url.UserPassword(u.User.Username(), "")
	out := u.String()
	// url.String renders an empty password as "user:@host"; make it explicit.
	return strings.Replace(out, ":@", ":"+dsnMaskGlyph+"@", 1)
}
