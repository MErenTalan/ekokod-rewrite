package worker_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/config"
	"github.com/MErenTalan/ekokod-rewrite/internal/worker"
)

// TestRedirectURIDerivedFromPublicURL pins worker.RedirectURI's three
// cases: an explicit EKOKOD_ISOLAR_REDIRECT_URL override always wins;
// otherwise it is PublicURL plus the fixed callback path, whether or not
// PublicURL itself already carries a trailing slash.
func TestRedirectURIDerivedFromPublicURL(t *testing.T) {
	t.Run("explicit override wins", func(t *testing.T) {
		cfg := &config.Config{
			HTTP:     config.HTTP{PublicURL: "https://app.example.com/"},
			External: config.External{ISolarRedirect: "https://override.example.com/cb"},
		}
		require.Equal(t, "https://override.example.com/cb", worker.RedirectURI(cfg))
	})

	t.Run("derived from public URL, trailing slash trimmed", func(t *testing.T) {
		cfg := &config.Config{HTTP: config.HTTP{PublicURL: "https://app.example.com/"}}
		require.Equal(t, "https://app.example.com/api/v1/integrations/isolar/callback", worker.RedirectURI(cfg))
	})

	t.Run("no trailing slash on public URL", func(t *testing.T) {
		cfg := &config.Config{HTTP: config.HTTP{PublicURL: "https://app.example.com"}}
		require.Equal(t, "https://app.example.com/api/v1/integrations/isolar/callback", worker.RedirectURI(cfg))
	})
}

// TestStateKeyIsDomainSeparated pins worker.StateKey: it must never equal
// the raw JWT signing key it is derived from, must be stable across calls
// for the same input, and must vary with the input.
func TestStateKeyIsDomainSeparated(t *testing.T) {
	jwtKey := []byte("jwt-signing-key-at-least-32-chars-long!!")

	key := worker.StateKey(jwtKey)
	require.NotEqual(t, jwtKey, key, "the derived state key must never equal the raw JWT signing key")
	require.NotEmpty(t, key)

	again := worker.StateKey(jwtKey)
	require.Equal(t, key, again, "StateKey must be stable across calls for the same input")

	other := worker.StateKey([]byte("a-completely-different-jwt-signing-key!"))
	require.NotEqual(t, key, other, "a different input must derive a different state key")
}
