// Package store holds the contracts every repository implementation shares:
// the tenant boundary (Scope) and the sentinel errors repositories return.
// It deliberately depends on nothing but the standard library and uuid, so
// that both the postgres implementations and their callers can import it
// without pulling in a driver.
package store

import "github.com/google/uuid"

// Scope is the tenant boundary every repository read and write is
// parameterised by. It is a value, not a context key, so that a missing
// scope is a compile error rather than a runtime nil.
type Scope struct {
	CompanyID uuid.UUID

	// BuildingIDs is the set of buildings the principal may see. An EMPTY
	// slice means NO buildings — never "all". Widening must be explicit.
	BuildingIDs []uuid.UUID

	// AllBuildings grants the whole company. It is a separate flag precisely
	// so that "all buildings" can never be expressed accidentally by an
	// empty or nil BuildingIDs slice, which is the fail-open shape this
	// type exists to prevent.
	AllBuildings bool
}

// Valid reports whether the scope can be used. A zero CompanyID is never
// valid; every repository method rejects an invalid scope before touching
// the database.
func (s Scope) Valid() bool { return s.CompanyID != uuid.Nil }

// SystemScope is the scope a background job uses when it acts for a whole
// company — ingestion, backfill, an OAuth callback after state verification —
// rather than for one authenticated principal's grant. It is explicit
// widening, spelled out at the one call site that needs it, so that no F2
// code builds a Scope{AllBuildings: true} by hand: this is the only scope
// constructor F2 adds.
//
// A zero companyID yields an invalid scope (Valid() is false, since
// CompanyID is uuid.Nil), which every repository rejects with
// ErrInvalidScope before any I/O — SystemScope has no way to fail open.
func SystemScope(companyID uuid.UUID) Scope {
	return Scope{CompanyID: companyID, AllBuildings: true}
}

// AllowsBuilding reports whether the principal may see building id.
//
// This is the authorisation branch in one place, so that no repository has to
// write it. The version everyone reaches for by hand,
//
//	if len(s.BuildingIDs) > 0 { … AND building_id = ANY($1) … }
//
// is FAIL-OPEN: an empty slice drops the predicate and the query silently
// returns every building in the company — the precise failure Scope exists to
// prevent, arrived at through the most natural-looking line of Go in the
// package. Valid() is necessary but not sufficient on its own; this and
// BuildingFilter are what make the safe path also the easy one.
//
// Everything about it fails closed: an invalid scope allows nothing whatever
// else is set, the zero uuid is never a building, and an empty set is never
// widened into "all".
//
// THIS IS AN IN-MEMORY GRANT CHECK, NOT AN EXISTENCE CHECK. It answers "does
// the grant this Scope carries cover id", never "does id belong to this
// Scope's company" — it has no way to know that, because it never touches the
// database. A Scope with AllBuildings set returns true here for ANY id
// whatsoever, including a uuid that belongs to a different company entirely,
// or no company at all. Calling this to authorise STORING id as a foreign key
// (a tariff's or bill's building_id, a report's building_id, …) is therefore
// the bug Critical Finding 1 of the task-11a fix-round-1 review named: an
// AllBuildings Scope could store another tenant's building id, because
// AllowsBuilding said yes to every id it was asked about. Every stored
// foreign-key id MUST be validated in SQL instead — company_id = the Scope's
// company, deleted_at is null, and the Scope's building branch — inside the
// write statement or by a companion …Visible query in the same transaction.
// AllowsBuilding remains correct and sufficient for narrowing a READ (Get,
// List, WHERE), where the predicate is applied against rows the query itself
// already restricts to the right company.
func (s Scope) AllowsBuilding(id uuid.UUID) bool {
	if !s.Valid() || id == uuid.Nil {
		return false
	}
	if s.AllBuildings {
		return true
	}
	for _, allowed := range s.BuildingIDs {
		if allowed == id {
			return true
		}
	}
	return false
}

// BuildingFilter is what repositories consume when building a query. It
// returns the building ids to filter on and whether the whole company is
// granted.
//
// Callers MUST branch on all, never on len(ids):
//
//	ids, all := scope.BuildingFilter()
//	if !all {
//	    where = append(where, "building_id = ANY($n)") // ids may be empty:
//	}                                                  // that matches no rows,
//	                                                   // which is correct.
//
// An empty ids with all false is a real and meaningful state — a principal
// with no buildings — and the predicate must still be applied so the query
// returns nothing. Dropping it because the slice is empty is the fail-open
// bug. An invalid scope reports (nil, false) for the same reason: deny.
//
// The returned slice aliases the Scope's own; callers must not mutate it.
func (s Scope) BuildingFilter() (ids []uuid.UUID, all bool) {
	if !s.Valid() {
		return nil, false
	}
	if s.AllBuildings {
		return nil, true
	}
	return s.BuildingIDs, false
}
