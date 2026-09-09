package secret_test

import (
	"testing"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/secret"
	"github.com/stretchr/testify/require"
)

func TestFragmentsOfEmptyPasswordIsNil(t *testing.T) {
	require.Nil(t, secret.Fragments(""))
}

func TestFragmentsIncludesTheWholePasswordAndItsPieces(t *testing.T) {
	got := secret.Fragments("my@pass")
	require.Contains(t, got, "my@pass")
	require.Contains(t, got, "my")
	require.Contains(t, got, "pass")
}

func TestFragmentsSkipsSingleCharacterPieces(t *testing.T) {
	got := secret.Fragments("a@b")
	require.Contains(t, got, "a@b")
	require.NotContains(t, got, "a")
	require.NotContains(t, got, "b")
}

func TestRedactReplacesEveryFragment(t *testing.T) {
	msg := `dial tcp: lookup my.pass.example.com: no such host`
	got := secret.Redact(msg, secret.Fragments("my@pass"))
	require.NotContains(t, got, "my")
	require.NotContains(t, got, "pass")
	require.Contains(t, got, secret.Mask)
}

func TestRedactWithNoFragmentsLeavesMessageUnchanged(t *testing.T) {
	require.Equal(t, "connection refused", secret.Redact("connection refused", nil))
}
