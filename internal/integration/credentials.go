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

// secretBytes is the plaintext, held one indirection away from Secret
// itself. This indirection is the fix for a real leak: when a Secret is
// held in an unexported field of some OTHER struct (e.g. `struct{ s
// Secret }`), fmt's reflection-based printer cannot call Interface() on
// that field (Go forbids it for values obtained from an unexported field),
// so it cannot even discover that Secret implements fmt.Formatter — it
// falls back to printing the field's raw underlying representation
// directly via reflection, which previously meant walking straight into
// Secret's own (also unexported) []byte field and dumping its bytes. fmt's
// fallback printer prints a nested pointer FIELD as a bare hex address
// (0xc0001…) rather than dereferencing it — only a value passed directly
// as a Print/Sprintf argument gets that one-level "&{...}" expansion — so
// holding the plaintext behind a pointer here means that fallback path can
// only ever reach the pointer's address, never the bytes it points to. See
// credentials_test.go's unexported-holder cases for the proof.
type secretBytes struct{ b []byte }

// Secret holds one piece of plaintext credential material — a password, an
// API key, a token — that must never reach a log line, an error message, a
// JSON response or a debugger's default %v. 03 §7 states this is never
// logged; this type is the only place in the codebase plaintext is allowed
// to exist, and every formatting path it might be pushed through by
// accident renders "[REDACTED]" instead. See credentials_test.go for the
// exhaustive proof.
type Secret struct{ p *secretBytes }

// NewSecret copies b so the caller's own slice can be zeroed or reused
// without aliasing the Secret's contents.
func NewSecret(b []byte) Secret {
	cp := make([]byte, len(b))
	copy(cp, b)
	return Secret{p: &secretBytes{b: cp}}
}

// Reveal returns the plaintext. It is the ONLY method on this type that
// does; every caller of Reveal is a deliberate, reviewable decision to
// handle real credential material (building an Authorization header, an
// encrypted column, secret.Redact's fragment list). A zero-value Secret
// (p == nil) reveals "".
func (s Secret) Reveal() string {
	if s.p == nil {
		return ""
	}
	return string(s.p.b)
}

// IsZero reports whether the secret carries no bytes at all.
func (s Secret) IsZero() bool { return s.p == nil || len(s.p.b) == 0 }

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
//
// Do not require.Equal/assert.Equal two Credentials values in a test.
// testify renders a failure diff with go-spew, which follows pointers
// (including Secret's, since NewSecret fix I1) and would print the
// plaintext it holds straight into the test log. Compare individual
// non-secret fields, or compare Secret.Reveal() values explicitly when a
// test genuinely needs to assert on the plaintext.
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

// Fragments returns every secret's plaintext held by c, deduplicated and
// sorted longest-first, for secret.Redact(msg, c.Fragments()) to scrub out
// of operator-facing error text. Longest-first matters: if one secret's
// plaintext is a prefix of another's (e.g. Secret "pw" and an Extra token
// "pw-tok"), a redactor that walks the list in the wrong order replaces the
// short match first and leaves the longer secret's suffix exposed
// ("••••••••-tok" instead of a clean redaction) — sorting longest-first
// makes the redactor always replace the longest, most specific match at
// each position before any of its own prefixes get a chance to. Dedup
// avoids redacting (and therefore scanning for) the same plaintext twice
// when, for example, Secret and an Extra entry happen to hold the same
// value. The result's order is otherwise deterministic (ties break by
// string comparison), since it is built by walking Extra in sorted key
// order before the final sort. Never log the result of this call — it
// exists to be fed into a redactor, not printed.
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

	seen := make(map[string]bool, len(frags))
	deduped := frags[:0]
	for _, f := range frags {
		if seen[f] {
			continue
		}
		seen[f] = true
		deduped = append(deduped, f)
	}

	sort.Slice(deduped, func(i, j int) bool {
		if len(deduped[i]) != len(deduped[j]) {
			return len(deduped[i]) > len(deduped[j])
		}
		return deduped[i] < deduped[j]
	})
	return deduped
}
