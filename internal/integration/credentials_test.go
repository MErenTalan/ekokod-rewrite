package integration_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"testing"

	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/stretchr/testify/require"
)

// unexportedSecretHolder and unexportedCredentialsHolder hold a Secret/
// Credentials in an UNEXPORTED field of some other struct — the I1 leak
// shape. A struct field that is itself unexported cannot be reached with
// reflect.Value.Interface() (Go forbids calling that on a value obtained
// from an unexported field), so fmt's default recursive printer cannot even
// ask whether the field's type implements Formatter/Stringer/GoStringer —
// it falls back to printing the field's raw underlying representation via
// reflection directly. Before the I1 fix (Secret's plaintext held as a
// value []byte field), that fallback path read the bytes straight out and
// printed them; the fix (holding the plaintext behind a pointer) means the
// fallback can only ever print the pointer's own address, since fmt does
// not follow a pointer held in a struct FIELD at nesting depth > 0 (only a
// value passed directly as the Print/Sprintf argument gets that
// expansion).
type unexportedSecretHolder struct{ s integration.Secret }
type unexportedCredentialsHolder struct{ c integration.Credentials }

// hideStaticType launders v's static type to `any`, on purpose: go vet's
// printf checker statically rejects %s/%x on a struct argument whose OWN
// type has no Stringer/Formatter (it cannot see through to whether an
// unexported field's type does), which is exactly the shape this test must
// exercise. The check is about a caller's likely mistake with a concrete
// struct type; laundering through `any` here is a deliberate, reviewed
// exception to reach the runtime behaviour under test, not a workaround for
// a real footgun.
func hideStaticType(v any) any { return v }

// TestCredentialsNeverFormatSecrets pins 03 §7 "never logged" for the one
// type that carries plaintext. Every rendering path a developer might use by
// accident is exercised; a new path that leaks must be added here.
func TestCredentialsNeverFormatSecrets(t *testing.T) {
	const pw = "FIXTURE-PLAINTEXT-7f3a"
	c := integration.Credentials{
		Username: "fixture-user",
		Secret:   integration.NewSecret([]byte(pw)),
		Extra:    map[string]integration.Secret{"access_token": integration.NewSecret([]byte(pw + "-tok"))},
	}
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, nil)) // bare handler on purpose: Secret must not rely on redaction by key
	logger.Info("creds", slog.Any("c", c), slog.Any("s", c.Secret))
	js, err := json.Marshal(c)
	require.NoError(t, err)

	// I1: the same secret, reached through an unexported field of a
	// wrapper struct fmt cannot call Interface() on — see the type doc
	// comments above. These prove the pointer-indirection fix, not merely
	// Secret's own Format method (already proven below).
	secretHolder := unexportedSecretHolder{s: c.Secret}
	credsHolder := unexportedCredentialsHolder{c: c}

	var textBuf, jsonBuf bytes.Buffer
	slog.New(slog.NewTextHandler(&textBuf, nil)).Info("holders",
		slog.Any("secretHolder", secretHolder), slog.Any("credsHolder", credsHolder))
	slog.New(slog.NewJSONHandler(&jsonBuf, nil)).Info("holders",
		slog.Any("secretHolder", secretHolder), slog.Any("credsHolder", credsHolder))

	for name, rendered := range map[string]string{
		"%v": fmt.Sprintf("%v", c), "%+v": fmt.Sprintf("%+v", c), "%#v": fmt.Sprintf("%#v", c),
		"%s": fmt.Sprintf("%s", c.Secret), "%q": fmt.Sprintf("%q", c.Secret), "%x": fmt.Sprintf("%x", c.Secret),
		"json": string(js), "slog": logBuf.String(),
		"error": fmt.Errorf("wrap: %v", c).Error(),
		// GoString() called directly: since Secret also implements
		// fmt.Formatter, fmt itself never reaches GoStringer (Formatter is
		// checked first, for every verb, including %#v — see the fmt package
		// doc's ordering), so this is the only path that actually exercises
		// GoString's own text. It is a real path a developer could still take
		// by calling GoString() directly instead of going through fmt.
		"GoString()": c.Secret.GoString(),

		// I1 unexported-holder cases, both types, every verb the brief names.
		"unexported holder Secret %v":       fmt.Sprintf("%v", secretHolder),
		"unexported holder Secret %+v":      fmt.Sprintf("%+v", secretHolder),
		"unexported holder Secret %#v":      fmt.Sprintf("%#v", secretHolder),
		"unexported holder Secret %s":       fmt.Sprintf("%s", hideStaticType(secretHolder)),
		"unexported holder Secret %x":       fmt.Sprintf("%x", hideStaticType(secretHolder)),
		"unexported holder Credentials %v":  fmt.Sprintf("%v", credsHolder),
		"unexported holder Credentials %+v": fmt.Sprintf("%+v", credsHolder),
		"unexported holder Credentials %#v": fmt.Sprintf("%#v", credsHolder),
		"unexported holder Credentials %s":  fmt.Sprintf("%s", hideStaticType(credsHolder)),
		"unexported holder Credentials %x":  fmt.Sprintf("%x", hideStaticType(credsHolder)),
		"unexported holder slog text":       textBuf.String(),
		"unexported holder slog json":       jsonBuf.String(),
	} {
		require.NotContains(t, rendered, pw, "secret leaked through %s", name)
	}
	require.Equal(t, pw, c.Secret.Reveal())
}

// TestCredentialsFragmentsDedupedAndSortedLongestFirst proves Fragments
// (M2) sorts longest-first and dedupes: a redactor that walks the list in
// the wrong order can replace a short secret that happens to be a prefix of
// a longer one, leaving the longer secret's suffix exposed — e.g. Secret
// "pw" and an Extra token "pw-tok" redacting "pw" first would turn
// "pw-tok" into "••••••••-tok" instead of a clean full match.
func TestCredentialsFragmentsDedupedAndSortedLongestFirst(t *testing.T) {
	c := integration.Credentials{
		Secret: integration.NewSecret([]byte("pw")),
		Extra: map[string]integration.Secret{
			"access_token":  integration.NewSecret([]byte("pw-tok")),
			"refresh_token": integration.NewSecret([]byte("pw-tok")), // duplicate value: must appear once
			"app_key":       integration.NewSecret([]byte("zz")),     // same length as "pw": tiebreak by string order
		},
	}
	got := c.Fragments()
	require.Equal(t, []string{"pw-tok", "pw", "zz"}, got, "longest first, duplicates removed, ties broken lexically")
}
