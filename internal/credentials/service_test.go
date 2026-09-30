package credentials_test

import (
	"context"
	"testing"
	"time"

	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/credentials"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/isolar"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/lock"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// TestNewRejectsShortStateKey is the "New enforces a minimum StateKey
// length (32 bytes)" folded minor: a StateKey shorter than 32 bytes must be
// refused up front, not silently accepted into a weaker-than-intended HMAC.
func TestNewRejectsShortStateKey(t *testing.T) {
	deps := credentials.Deps{
		Integrations: fakeMinimalIntegrations{},
		Analyzers:    fakeMinimalAnalyzers{},
		Buildings:    fakeMinimalBuildings{},
		Verifiers:    fakeMinimalVerifiers{},
		ISolar:       fakeMinimalISolar{},
		Enqueuer:     fakeMinimalEnqueuer{},
		Locker:       lock.NewMemory(func() time.Time { return time.Time{} }),
		Nonces:       lock.NewMemory(func() time.Time { return time.Time{} }),
		Clock:        clock.NewFake(time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)),
		RedirectURI:  "https://app.example.invalid/integrations/isolar/callback",
		MaxRetry:     3,
	}

	deps.StateKey = make([]byte, 31)
	_, err := credentials.New(deps)
	require.Error(t, err, "31 bytes must be rejected")

	deps.StateKey = make([]byte, 32)
	_, err = credentials.New(deps)
	require.NoError(t, err, "32 bytes must be accepted")
}

// --- minimal fakes for TestNewRejectsShortStateKey only --------------------

type fakeMinimalIntegrations struct{ store.IntegrationRepository }
type fakeMinimalAnalyzers struct{ store.AnalyzerRepository }
type fakeMinimalBuildings struct{ store.BuildingRepository }
type fakeMinimalVerifiers struct{}

func (fakeMinimalVerifiers) Verifier(integration.Provider) (credentials.Verifier, error) {
	return nil, nil
}

type fakeMinimalISolar struct{}

func (fakeMinimalISolar) AuthorizeURL(integration.Credentials, string) (string, error) {
	return "", nil
}
func (fakeMinimalISolar) ExchangeCode(context.Context, integration.Credentials, string, string) (isolar.Token, error) {
	return isolar.Token{}, nil
}
func (fakeMinimalISolar) Refresh(context.Context, integration.Credentials) (isolar.Token, error) {
	return isolar.Token{}, nil
}

type fakeMinimalEnqueuer struct{}

func (fakeMinimalEnqueuer) Enqueue(context.Context, *asynq.Task, ...asynq.Option) (*asynq.TaskInfo, error) {
	return nil, nil
}

// A bad backfill range used to be enqueued and fail silently in the worker;
// it must be refused at the API, and a `to` past now is clamped to now.
func TestBackfillRangeIsCheckedBeforeEnqueue(t *testing.T) {
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	svc, err := credentials.New(credentials.Deps{
		Integrations: fakeMinimalIntegrations{}, Analyzers: fakeMinimalAnalyzers{}, Buildings: fakeMinimalBuildings{},
		Verifiers: fakeMinimalVerifiers{}, ISolar: fakeMinimalISolar{}, Enqueuer: fakeMinimalEnqueuer{},
		Locker: lock.NewMemory(func() time.Time { return now }), Nonces: lock.NewMemory(func() time.Time { return now }),
		Clock: clock.NewFake(now), RedirectURI: "https://app.example.invalid/cb", MaxRetry: 3, StateKey: make([]byte, 32),
	})
	require.NoError(t, err)
	sc := store.Scope{CompanyID: [16]byte{1}, AllBuildings: true}

	_, err = svc.Backfill(context.Background(), sc, [16]byte{2}, credentials.BackfillInput{From: now.Add(-time.Hour), To: now.Add(-time.Hour)})
	require.ErrorIs(t, err, credentials.ErrInvalidBackfillRange)
	_, err = svc.Backfill(context.Background(), sc, [16]byte{2}, credentials.BackfillInput{From: now.Add(time.Hour), To: now.Add(48 * time.Hour)})
	require.ErrorIs(t, err, credentials.ErrInvalidBackfillRange, "from in the future is empty once to is clamped to now")
}
