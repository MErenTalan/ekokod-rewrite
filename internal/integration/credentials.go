package integration

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"time"

	"github.com/google/uuid"
)

// redacted is what every rendering path of Secret prints instead of its
// plaintext. It is a constant so every method below is provably consistent
// with the others.
const redacted = "[REDACTED]"

// Secret holds one piece of plaintext credential material — a password, an
// API key, a token — that must never reach a log line, an error message, a
// JSON response or a debugger's default %v. 03 §7 states this is never
// logged; this type is the only place in the codebase plaintext is allowed
// to exist, and every formatting path it might be pushed through by
// accident renders "[REDACTED]" instead. See credentials_test.go for the
// exhaustive proof.
type Secret struct{ b []byte }

// NewSecret copies b so the caller's own slice can be zeroed or reused
// without aliasing the Secret's contents.
func NewSecret(b []byte) Secret {
	cp := make([]byte, len(b))
	copy(cp, b)
	return Secret{b: cp}
}

// Reveal returns the plaintext. It is the ONLY method on this type that
// does; every caller of Reveal is a deliberate, reviewable decision to
// handle real credential material (building an Authorization header, an
// encrypted column, secret.Redact's fragment list).
func (s Secret) Reveal() string { return string(s.b) }

// IsZero reports whether the secret carries no bytes at all.
func (s Secret) IsZero() bool { return len(s.b) == 0 }

// Format implements fmt.Formatter. It ignores the verb entirely — %v, %+v,
// %#v, %s, %q, %x and any other verb all print "[REDACTED]" — because a
// Formatter is consulted by the fmt package before Stringer, GoStringer or
// the default struct/verb-specific printer, at every level of a recursive
// print (including a Secret field nested inside a Credentials value), so
// this one method is what keeps every one of those paths safe.
func (s Secret) Format(f fmt.State, verb rune) { _, _ = io.WriteString(f, redacted) }

// String implements fmt.Stringer as a second line of defence: any code path
// that calls String() directly instead of going through fmt still gets
// "[REDACTED]".
func (s Secret) String() string { return redacted }

// GoString implements fmt.GoStringer as a second line of defence for %#v
// call sites that bypass fmt.Formatter (there are none in the standard
// library, but future code should not be able to reintroduce a leak by
// calling GoString() directly).
func (s Secret) GoString() string { return "integration.Secret(" + redacted + ")" }

// LogValue implements slog.LogValuer so slog.Any(key, secret) — and a
// Secret field on any struct passed to slog with structured logging that
// resolves LogValuer — never renders plaintext.
func (s Secret) LogValue() slog.Value { return slog.StringValue(redacted) }

// MarshalJSON implements json.Marshaler so encoding/json never serialises
// the plaintext, including when Secret is nested inside another struct or
// held as a map value.
func (s Secret) MarshalJSON() ([]byte, error) { return json.Marshal(redacted) }

// MarshalText implements encoding.TextMarshaler for the encoders (CSV,
// some template engines, some config writers) that prefer it over
// json.Marshaler.
func (s Secret) MarshalText() ([]byte, error) { return []byte(redacted), nil }

// Settings are the small set of per-credential behavioural toggles that are
// not themselves secret.
type Settings struct {
	UseBillingIndexes bool `json:"use_billing_indexes"`
}

// Credentials is everything one adapter call needs to authenticate and
// address a provider: identity (CredentialID, CompanyID, Provider,
// Subtype), addressing (Endpoints, BaseURL, Region) and secret material
// (Username is not secret; Secret and Extra are).
type Credentials struct {
	CredentialID, CompanyID uuid.UUID
	Provider                Provider
	Subtype                 string
	Endpoints               map[string]string // integration_definitions.endpoints (templates)
	Username                string
	Secret                  Secret            // password / secret key
	Extra                   map[string]Secret // e.g. "app_key", "secret_key", "app_id", "access_token", "refresh_token"
	Settings                Settings
	BaseURL                 string // pm5340_url
	Region                  string // isolar_region
	TokenExpiresAt          *time.Time
}

// Fragments returns every secret's plaintext held by c, for
// secret.Redact(msg, c.Fragments()) to scrub out of operator-facing error
// text. The Extra map is walked in sorted key order so the result is
// deterministic across calls. Never log the result of this call — it exists
// to be fed into a redactor, not printed.
func (c Credentials) Fragments() []string {
	var frags []string
	if !c.Secret.IsZero() {
		frags = append(frags, c.Secret.Reveal())
	}
	keys := make([]string, 0, len(c.Extra))
	for k := range c.Extra {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if s := c.Extra[k]; !s.IsZero() {
			frags = append(frags, s.Reveal())
		}
	}
	return frags
}
