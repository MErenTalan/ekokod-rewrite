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
