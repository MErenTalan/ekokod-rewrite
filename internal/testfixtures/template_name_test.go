package testfixtures

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestTemplateNameForFingerprintChangesWithFingerprint is F4 Task 0's
// requirement 6c, at the level this package owns: the shared
// isolated-database template's name is isolatedTemplateNamePrefix plus a
// migrations fingerprint (see postgres.MigrationsFingerprint, and its own
// TestFingerprintFSChangesWithContent for the fingerprint half of this
// property), so two different fingerprints must never collide on the same
// template name. No database connection and no build tag: this is a pure
// function of a string.
func TestTemplateNameForFingerprintChangesWithFingerprint(t *testing.T) {
	a := templateNameForFingerprint("aaaaaaaaaaaaaaaa")
	b := templateNameForFingerprint("bbbbbbbbbbbbbbbb")

	require.NotEqual(t, a, b, "different fingerprints must produce different template names")
	require.Contains(t, a, "aaaaaaaaaaaaaaaa")
	require.Contains(t, b, "bbbbbbbbbbbbbbbb")
}
