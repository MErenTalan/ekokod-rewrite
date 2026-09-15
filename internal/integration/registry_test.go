package integration_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/stretchr/testify/require"
)

// fakeAdapter is a minimal Adapter used only to exercise Registry: it never
// does I/O and its Provider() is the only field the tests care about.
type fakeAdapter struct{ provider integration.Provider }

func (f fakeAdapter) Provider() integration.Provider { return f.provider }

func (f fakeAdapter) Verify(context.Context, integration.Credentials) error { return nil }

func (f fakeAdapter) DiscoverMeteringPoints(context.Context, integration.Credentials) ([]integration.MeteringPoint, error) {
	return nil, nil
}

func (f fakeAdapter) FetchReadings(context.Context, integration.Credentials, integration.FetchRequest) (integration.FetchResult, error) {
	return integration.FetchResult{}, nil
}

func (f fakeAdapter) Kinds(integration.Credentials) []model.ReadingKind { return nil }

func (f fakeAdapter) MaxWindow(model.ReadingKind) time.Duration { return 0 }

// TestRegistryRefusesDuplicateProviders proves NewRegistry rejects two
// adapters that report the same Provider() rather than silently letting
// slice order decide which one Source ever returns.
func TestRegistryRefusesDuplicateProviders(t *testing.T) {
	_, err := integration.NewRegistry(
		fakeAdapter{provider: integration.ProviderOSOS},
		fakeAdapter{provider: integration.ProviderOSOS},
	)
	require.Error(t, err)
}

// TestRegistryUnknownProviderIsNotFound proves Source's failure for an
// unregistered provider is classifiable with errors.Is against
// integration.ErrNotFound, the same sentinel every adapter call failure
// uses.
func TestRegistryUnknownProviderIsNotFound(t *testing.T) {
	reg, err := integration.NewRegistry(fakeAdapter{provider: integration.ProviderOSOS})
	require.NoError(t, err)

	_, err = reg.Source(integration.ProviderGridBox)
	require.Error(t, err)
	require.True(t, errors.Is(err, integration.ErrNotFound))
}
