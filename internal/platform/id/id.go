// Package id generates the platform's primary keys: time-ordered UUIDv7.
package id

import "github.com/google/uuid"

// New returns a new time-ordered identifier.
func New() string {
	return uuid.Must(uuid.NewV7()).String()
}

// Parse validates an identifier.
func Parse(s string) (uuid.UUID, error) { return uuid.Parse(s) }
