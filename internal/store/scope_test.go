package store_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// TestEmptyBuildingSetGrantsNothing pins the fail-closed shape of the zero
// value. The whole reason AllBuildings is a separate flag is that a nil or
// empty BuildingIDs slice — the shape a forgotten assignment produces — must
// mean "no buildings", never "every building".
func TestEmptyBuildingSetGrantsNothing(t *testing.T) {
	s := store.Scope{CompanyID: uuid.New()}
	require.True(t, s.Valid())
	require.False(t, s.AllBuildings, "the zero value must not grant every building")
	require.Empty(t, s.BuildingIDs)
}

// TestZeroScopeIsInvalid pins the other half: a Scope that was never
// populated must be rejected before it can reach a query.
func TestZeroScopeIsInvalid(t *testing.T) {
	require.False(t, store.Scope{}.Valid(), "a scope with no company must never be usable")
}
