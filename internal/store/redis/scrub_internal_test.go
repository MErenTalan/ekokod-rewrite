package redis

import (
	"errors"
	"fmt"
	"testing"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/secret"
	"github.com/stretchr/testify/require"
)

var errDriver = errors.New("driver sentinel")

// TestScrubErrRedactsThePassword is the ordinary path: the client knows its
// credential, so the driver error comes through redacted rather than lost.
func TestScrubErrRedactsThePassword(t *testing.T) {
	cause := fmt.Errorf("NOAUTH Authentication required (tried %q): %w", "s3cret", errDriver)

	err := scrubErr("s3cret", "ping redis", cause)

	require.NotContains(t, err.Error(), "s3cret")
	require.Contains(t, err.Error(), secret.Mask)
	require.ErrorIs(t, err, errDriver)
	require.True(t, secret.IsScrubbed(err))
}

// TestScrubErrWithholdsWhenTheClientHasNoPassword closes the redis half of
// the fail-open hole found in review. goredis leaves Options.Password empty
// whenever the URL carried no credential — including when one reached the
// server another way — and an empty password used to mean "redact nothing",
// so the driver error printed verbatim.
func TestScrubErrWithholdsWhenTheClientHasNoPassword(t *testing.T) {
	cause := fmt.Errorf("NOAUTH Authentication required (tried %q): %w", "s3cret", errDriver)

	err := scrubErr("", "ping redis", cause)

	require.NotContains(t, err.Error(), "s3cret",
		"an unlocatable credential must withhold the driver text, not pass it through")
	require.Contains(t, err.Error(), "ping redis", "the operation must stay diagnosable")
	require.ErrorIs(t, err, errDriver)
}

// TestScrubErrOfNilIsNil keeps the helper safe to call unconditionally.
func TestScrubErrOfNilIsNil(t *testing.T) {
	require.NoError(t, scrubErr("s3cret", "ping redis", nil))
}
