package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
)

type spyAdapter struct{ calls int }

func (s *spyAdapter) Provider() integration.Provider { return integration.ProviderOSOS }
func (s *spyAdapter) Verify(context.Context, integration.Credentials) error {
	s.calls++
	return nil
}
func (s *spyAdapter) DiscoverMeteringPoints(context.Context, integration.Credentials) ([]integration.MeteringPoint, error) {
	s.calls++
	return []integration.MeteringPoint{{InstallationNumber: "REAL"}}, nil
}
func (s *spyAdapter) FetchReadings(context.Context, integration.Credentials, integration.FetchRequest) (integration.FetchResult, error) {
	s.calls++
	return integration.FetchResult{Readings: []model.MeterReading{{}}}, nil
}
func (s *spyAdapter) Kinds(integration.Credentials) []model.ReadingKind {
	return []model.ReadingKind{model.ReadingKindLoadProfile}
}
func (s *spyAdapter) MaxWindow(model.ReadingKind) time.Duration { return 7 * 24 * time.Hour }

func TestGuardDemoAdapterNeverCallsInnerForDemo(t *testing.T) {
	demoID := uuid.New()
	isDemo := func(_ context.Context, id uuid.UUID) (bool, error) { return id == demoID, nil }
	spy := &spyAdapter{}
	a := integration.GuardDemo(spy, isDemo)
	ctx := t.Context()
	demo := integration.Credentials{CompanyID: demoID, Provider: integration.ProviderOSOS}

	require.NoError(t, a.Verify(ctx, demo))
	points, err := a.DiscoverMeteringPoints(ctx, demo)
	require.NoError(t, err)
	require.Empty(t, points)
	res, err := a.FetchReadings(ctx, demo, integration.FetchRequest{})
	require.NoError(t, err)
	require.Empty(t, res.Readings)
	require.Nil(t, res.NextCursor)
	require.Equal(t, 0, spy.calls, "R545: no demo call reaches the provider")

	real := integration.Credentials{CompanyID: uuid.New(), Provider: integration.ProviderOSOS}
	require.NoError(t, a.Verify(ctx, real))
	points, err = a.DiscoverMeteringPoints(ctx, real)
	require.NoError(t, err)
	require.Len(t, points, 1)
	res, err = a.FetchReadings(ctx, real, integration.FetchRequest{})
	require.NoError(t, err)
	require.Len(t, res.Readings, 1)
	require.Equal(t, 3, spy.calls)

	require.Equal(t, integration.ProviderOSOS, a.Provider())
	require.Equal(t, []model.ReadingKind{model.ReadingKindLoadProfile}, a.Kinds(demo))
	require.Equal(t, 7*24*time.Hour, a.MaxWindow(model.ReadingKindLoadProfile))
}
