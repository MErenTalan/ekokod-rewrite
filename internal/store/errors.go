package store

import "errors"

// The sentinel errors every repository returns. Callers match them with
// errors.Is, never by string comparison, so that implementations remain
// free to wrap them with context.
var (
	// ErrNotFound is returned when no row matches the request *within the
	// caller's Scope*. A row that exists in another tenant is indistinguishable
	// from a row that does not exist at all, which is the point.
	ErrNotFound = errors.New("not found")

	// ErrConflict is returned when a write would violate a uniqueness or
	// optimistic-concurrency constraint.
	ErrConflict = errors.New("conflict")

	// ErrInvalidScope is returned when a repository method is handed a Scope
	// whose Valid method reports false. It is returned before any database
	// round trip.
	ErrInvalidScope = errors.New("invalid scope")
)
