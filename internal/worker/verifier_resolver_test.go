package worker

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/httpx"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/isolar"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/osos"
)

// TestVerifierResolverRoutesProviders is the fix-round-1 I2 test: it pins
// verifierResolver.Verifier's three branches, which stayed green even with
// the isolar branch disabled entirely (the reviewer's proof (d), reproduced
// below). It needs no database or redis — httpx.NewPool, osos.New and
// isolar.New all construct without dialing anything, so this is a true
// unit test, unlike the rest of this package's graph tests.
func TestVerifierResolverRoutesProviders(t *testing.T) {
	pool, err := httpx.NewPool(httpx.PoolOptions{})
	require.NoError(t, err)

	ososAdapter := osos.New(pool, osos.Options{})
	registry, err := integration.NewRegistry(ososAdapter)
	require.NoError(t, err)

	isolarClient := isolar.New(pool, isolar.Options{})

	resolver := verifierResolver{registry: registry, isolar: isolarClient}

	t.Run("isolar routes to the isolar client, not the registry", func(t *testing.T) {
		v, err := resolver.Verifier(integration.ProviderISolar)
		require.NoError(t, err)
		require.Same(t, isolarClient, v, "ProviderISolar must resolve to the *isolar.Client, not registry.Source")
	})

	t.Run("a registered meter provider routes to the registry's adapter", func(t *testing.T) {
		v, err := resolver.Verifier(integration.ProviderOSOS)
		require.NoError(t, err)
		require.Same(t, ososAdapter, v)
	})

	t.Run("an unregistered provider wraps ErrNotFound", func(t *testing.T) {
		_, err := resolver.Verifier(integration.Provider("does-not-exist"))
		require.Error(t, err)
		require.True(t, errors.Is(err, integration.ErrNotFound))
	})
}
