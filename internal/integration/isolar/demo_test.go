package isolar_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/isolar"
)

// spyUpstream counts every call; a demo company must reach none of them.
type spyUpstream struct{ calls int }

func (s *spyUpstream) hit() { s.calls++ }
func (s *spyUpstream) Plants(context.Context, integration.Credentials) ([]isolar.Plant, error) {
	s.hit()
	return []isolar.Plant{{PSID: "real"}}, nil
}
func (s *spyUpstream) Devices(context.Context, integration.Credentials, string) ([]isolar.Device, error) {
	s.hit()
	return nil, nil
}
func (s *spyUpstream) DeviceMinuteSeries(context.Context, integration.Credentials, []string, time.Time, time.Time) ([]isolar.YieldSample, error) {
	s.hit()
	return nil, nil
}
func (s *spyUpstream) PlantMinuteSeries(context.Context, integration.Credentials, string, time.Time, time.Time) ([]isolar.YieldSample, error) {
	s.hit()
	return nil, nil
}
func (s *spyUpstream) PlantDailySeries(context.Context, integration.Credentials, string, time.Time, time.Time) ([]isolar.DailyYield, error) {
	s.hit()
	return nil, nil
}
func (s *spyUpstream) DeviceRealtime(context.Context, integration.Credentials, int32, []string) ([]isolar.DeviceSnapshot, error) {
	s.hit()
	return nil, nil
}
func (s *spyUpstream) Faults(context.Context, integration.Credentials, time.Time, time.Time) ([]isolar.Fault, error) {
	s.hit()
	return nil, nil
}
func (s *spyUpstream) AuthorizeURL(integration.Credentials, string) (string, error) {
	s.hit()
	return "https://isolar.example/authorize", nil
}
func (s *spyUpstream) ExchangeCode(context.Context, integration.Credentials, string, string) (isolar.Token, error) {
	s.hit()
	return isolar.Token{}, nil
}
func (s *spyUpstream) Refresh(context.Context, integration.Credentials) (isolar.Token, error) {
	s.hit()
	return isolar.Token{}, nil
}
func (s *spyUpstream) Verify(context.Context, integration.Credentials) error {
	s.hit()
	return nil
}

func TestISolarGuardDemo(t *testing.T) {
	demoID := uuid.New()
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	spy := &spyUpstream{}
	g := isolar.GuardDemo(spy, func(_ context.Context, id uuid.UUID) (bool, error) { return id == demoID, nil },
		func() time.Time { return now })
	ctx := t.Context()
	demo := integration.Credentials{CompanyID: demoID, Provider: integration.ProviderISolar}

	require.NoError(t, g.Verify(ctx, demo))
	tok, err := g.Refresh(ctx, demo)
	require.NoError(t, err)
	require.False(t, tok.AccessToken.IsZero())
	require.Equal(t, now.Add(24*time.Hour), tok.ExpiresAt)
	tok, err = g.ExchangeCode(ctx, demo, "demo", "https://x/cb")
	require.NoError(t, err)
	require.False(t, tok.AccessToken.IsZero())
	plants, err := g.Plants(ctx, demo)
	require.NoError(t, err)
	require.Equal(t, isolar.DemoPlants, plants)
	u, err := g.AuthorizeURL(demo, "https://x/cb")
	require.NoError(t, err)
	require.Equal(t, "https://x/cb?code=demo", u)
	faults, err := g.Faults(ctx, demo, now.Add(-time.Hour), now)
	require.NoError(t, err)
	require.Empty(t, faults)
	daily, err := g.PlantDailySeries(ctx, demo, "x", now.Add(-time.Hour), now)
	require.NoError(t, err)
	require.Empty(t, daily)
	require.Equal(t, 0, spy.calls, "R545: no demo call reaches iSolarCloud")

	real := integration.Credentials{CompanyID: uuid.New(), Provider: integration.ProviderISolar}
	plants, err = g.Plants(ctx, real)
	require.NoError(t, err)
	require.Equal(t, "real", plants[0].PSID)
	_, err = g.AuthorizeURL(real, "https://x/cb")
	require.NoError(t, err)
	require.Equal(t, 2, spy.calls)
}
