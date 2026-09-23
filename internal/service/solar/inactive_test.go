package solar

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/isolar"
	perr "github.com/MErenTalan/ekokod-rewrite/internal/platform/errors"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// An operator-deactivated credential stops every iSolar call (R280, X-M3):
// link refuses it, sync and the fault fetch end before the network.

type inactiveOpener struct{}

func (inactiveOpener) Open(context.Context, store.Scope, uuid.UUID) (integration.Credentials, error) {
	return integration.Credentials{Provider: integration.ProviderISolar, IsActive: false}, nil
}

// noCallAdapter fails the test on any network call.
type noCallAdapter struct {
	Adapter
	t *testing.T
}

func (a noCallAdapter) Plants(context.Context, integration.Credentials) ([]isolar.Plant, error) {
	a.t.Fatal("Plants called with an inactive credential")
	return nil, nil
}

func (a noCallAdapter) Devices(context.Context, integration.Credentials, string) ([]isolar.Device, error) {
	a.t.Fatal("Devices called with an inactive credential")
	return nil, nil
}

func (a noCallAdapter) Faults(context.Context, integration.Credentials, time.Time, time.Time) ([]isolar.Fault, error) {
	a.t.Fatal("Faults called with an inactive credential")
	return nil, nil
}

type linkedPlants struct {
	store.PlantRepository
	plant model.PowerPlant
	code  *string
}

func (p *linkedPlants) Get(context.Context, store.Scope, uuid.UUID) (model.PowerPlant, error) {
	return p.plant, nil
}

func (p *linkedPlants) SetSyncState(_ context.Context, _ store.Scope, _ uuid.UUID, _ time.Time, code *string) error {
	p.code = code
	return nil
}

func inactiveService(t *testing.T, plants store.PlantRepository) *Service {
	return New(Deps{Plants: plants, Creds: inactiveOpener{}, ISolar: noCallAdapter{t: t}})
}

func TestInactiveCredentialStopsSync(t *testing.T) {
	ps, cred := "ps-1", uuid.New()
	plants := &linkedPlants{plant: model.PowerPlant{ID: uuid.New(), IsolarPsID: &ps, IsolarCredentialID: &cred}}
	_, err := inactiveService(t, plants).SyncPlant(context.Background(), uuid.New(), plants.plant.ID, 0)
	require.Error(t, err)
	se := classify(err)
	require.Equal(t, CodeCredentialInactive, se.code)
	require.False(t, se.retry, "a deactivated credential is final, not retried")
	require.NotNil(t, plants.code)
	require.Equal(t, CodeCredentialInactive, *plants.code)
}

func TestInactiveCredentialCannotLink(t *testing.T) {
	_, err := inactiveService(t, nil).AccountPlants(context.Background(), store.SystemScope(uuid.New()), uuid.New())
	var pe *perr.Error
	require.True(t, errors.As(err, &pe), "want a validation error, got %v", err)
	require.Equal(t, []string{"not_isolar"}, pe.Params["credential_id"])
}

func TestInactiveCredentialFetchesNoFaults(t *testing.T) {
	ps := "ps-1"
	err := inactiveService(t, nil).storeCredentialFaults(context.Background(), store.SystemScope(uuid.New()), uuid.New(),
		[]model.PowerPlant{{ID: uuid.New(), IsolarPsID: &ps}}, time.Now())
	require.Equal(t, CodeCredentialInactive, classify(err).code)
}
