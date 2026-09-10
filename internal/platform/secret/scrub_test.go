package secret_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/secret"
	"github.com/stretchr/testify/require"
)

var errSentinel = errors.New("no rows in result set")

type dialError struct{ host string }

func (e *dialError) Error() string { return "dial tcp " + e.host + ": connection refused" }

// TestWrapKeepsErrorsIsWorkingThroughRedactedText is the first half of the
// carried-over defect: before this type existed, scrubErr built its result
// with fmt.Errorf("%s: %s", …), which flattened the driver error to a string
// and made errors.Is(err, pgx.ErrNoRows) impossible for every caller.
func TestWrapKeepsErrorsIsWorkingThroughRedactedText(t *testing.T) {
	cause := fmt.Errorf("query on host my@pass.example.com: %w", errSentinel)

	err := secret.Wrap("read schema version", secret.Fragments("my@pass"), cause)

	require.ErrorIs(t, err, errSentinel, "errors.Is must traverse a scrubbed error")
}

// TestWrapKeepsErrorsAsWorking pins the same property for errors.As, which
// callers need to reach a driver's structured error (e.g. *pgconn.PgError).
func TestWrapKeepsErrorsAsWorking(t *testing.T) {
	cause := &dialError{host: "my@pass.example.com"}

	err := secret.Wrap("ping redis", secret.Fragments("my@pass"), cause)

	var target *dialError
	require.ErrorAs(t, err, &target)
	require.Equal(t, "my@pass.example.com", target.host,
		"errors.As deliberately yields the unredacted cause; only Error() is scrubbed")
}

// TestWrapNeverPrintsThePassword is the second half, and the one that must
// never regress: restoring errors.Is by switching to %w would have exposed
// the unscrubbed cause through the wrapper's own text.
func TestWrapNeverPrintsThePassword(t *testing.T) {
	const password = "my@pass"
	cause := fmt.Errorf("dial tcp my.pass.example.com:5432 (auth for %s): %w", password, errSentinel)

	err := secret.Wrap("ping database", secret.Fragments(password), cause)

	msg := err.Error()
	require.NotContains(t, msg, password, "the whole password must never be printed")
	require.NotContains(t, msg, "my", "a password fragment must never be printed")
	require.NotContains(t, msg, "pass", "a password fragment must never be printed")
	require.Contains(t, msg, secret.Mask)
	require.True(t, strings.HasPrefix(msg, "ping database: "), "op must prefix the message, got %q", msg)

	// Verbs that fmt handles specially must not reach around Error().
	require.NotContains(t, fmt.Sprintf("%v", err), password)
	require.NotContains(t, fmt.Sprintf("%s", err), password)
	require.NotContains(t, fmt.Sprintf("%+v", err), password)

	// And a caller that wraps the scrubbed error again keeps the redaction.
	require.NotContains(t, fmt.Errorf("outer: %w", err).Error(), password)
}

// TestWrapOfNilIsNil keeps the helper safe to call unconditionally.
func TestWrapOfNilIsNil(t *testing.T) {
	require.NoError(t, secret.Wrap("op", secret.Fragments("pw"), nil))
}

// TestURLParseErrWithholdsEverything pins the deliberately information-free
// message: a URL that did not parse cannot be reasoned about, so none of its
// text is ever echoed.
func TestURLParseErrWithholdsEverything(t *testing.T) {
	err := secret.URLParseErr("parse redis url")
	require.EqualError(t, err, "parse redis url: redis url is not valid (details withheld)")
	require.NoError(t, errors.Unwrap(err), "there is no cause to expose")
}
