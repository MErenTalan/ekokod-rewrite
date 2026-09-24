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

// TestAllowsBuildingOnAnEmptySetAllowsNothing is the case the type exists to
// make unwriteable by hand. The obvious repository line
//
//	if len(s.BuildingIDs) > 0 { … AND building_id = ANY($1) … }
//
// is FAIL-OPEN: an empty slice silently drops the predicate and the query
// returns the whole company. AllowsBuilding and BuildingFilter are the
// branch nobody should be writing themselves.
func TestAllowsBuildingOnAnEmptySetAllowsNothing(t *testing.T) {
	s := store.Scope{CompanyID: uuid.New()}
	require.True(t, s.Valid())
	require.False(t, s.AllowsBuilding(uuid.New()), "an empty building set must allow nothing")

	ids, all := s.BuildingFilter()
	require.False(t, all, "an empty building set is not 'all buildings'")
	require.Empty(t, ids, "the filter must stay empty so the predicate matches no rows")
}

func TestAllowsBuildingOnlyTheListedBuildings(t *testing.T) {
	allowed, other := uuid.New(), uuid.New()
	s := store.Scope{CompanyID: uuid.New(), BuildingIDs: []uuid.UUID{allowed}}

	require.True(t, s.AllowsBuilding(allowed))
	require.False(t, s.AllowsBuilding(other))

	ids, all := s.BuildingFilter()
	require.False(t, all)
	require.Equal(t, []uuid.UUID{allowed}, ids)
}

func TestAllBuildingsGrantsTheWholeCompany(t *testing.T) {
	s := store.Scope{CompanyID: uuid.New(), AllBuildings: true}

	require.True(t, s.AllowsBuilding(uuid.New()), "the explicit flag is the only way to widen")

	ids, all := s.BuildingFilter()
	require.True(t, all, "callers must branch on this flag, never on len(ids)")
	require.Empty(t, ids, "no filter is applied when the whole company is granted")
}

// TestAnInvalidScopeAllowsNothing keeps Valid() from being the only line of
// defence: a scope that was never populated must deny even if a caller
// forgets to check it.
func TestAnInvalidScopeAllowsNothing(t *testing.T) {
	var s store.Scope
	require.False(t, s.AllowsBuilding(uuid.New()))

	ids, all := s.BuildingFilter()
	require.False(t, all)
	require.Empty(t, ids)

	// Even one that lists buildings but names no company.
	s = store.Scope{BuildingIDs: []uuid.UUID{uuid.New()}, AllBuildings: true}
	require.False(t, s.AllowsBuilding(s.BuildingIDs[0]), "no company means no access, whatever else is set")
	_, all = s.BuildingFilter()
	require.False(t, all)
}

// TestTheNilBuildingIsNeverAllowed stops a zero-value uuid from matching a
// zero-value entry in the slice.
func TestTheNilBuildingIsNeverAllowed(t *testing.T) {
	s := store.Scope{CompanyID: uuid.New(), BuildingIDs: []uuid.UUID{uuid.Nil}}
	require.False(t, s.AllowsBuilding(uuid.Nil))

	s.AllBuildings = true
	require.False(t, s.AllowsBuilding(uuid.Nil), "not even 'all buildings' makes a zero id a building")
}

// TestSystemScopeGrantsWholeCompany pins SystemScope's explicit-widening
// shape: a background job that has no single building grant still gets a
// valid, whole-company scope through the one constructor F2 adds, not a
// hand-built Scope{AllBuildings: true}.
func TestSystemScopeGrantsWholeCompany(t *testing.T) {
	companyID := uuid.New()
	s := store.SystemScope(companyID)

	require.True(t, s.Valid())
	require.Equal(t, companyID, s.CompanyID)
	require.True(t, s.AllBuildings)
	require.True(t, s.AllowsBuilding(uuid.New()), "SystemScope must grant any building in the company")

	ids, all := s.BuildingFilter()
	require.True(t, all)
	require.Empty(t, ids)
}

// TestSystemScopeOfNilCompanyIsInvalid pins the fail-closed half: a caller
// that forgets to supply a company id gets a Scope every repository rejects
// before any I/O, never one that silently reaches for "every company".
func TestSystemScopeOfNilCompanyIsInvalid(t *testing.T) {
	s := store.SystemScope(uuid.Nil)

	require.False(t, s.Valid())
	require.False(t, s.AllowsBuilding(uuid.New()), "an invalid scope must allow nothing even though AllBuildings is set")

	ids, all := s.BuildingFilter()
	require.False(t, all)
	require.Empty(t, ids)
}
