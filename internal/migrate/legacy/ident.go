// Package legacy moves the legacy Mongo system's data into the rewrite
// (08-migration.md): inventory, extract, transform (F14a), load (F14b).
package legacy

import "github.com/google/uuid"

// namespace is fixed forever: changing it would give every migrated row a new id (R400).
var namespace = uuid.MustParse("6f1d7a52-3c8e-4b0a-9d5e-0e4c2b1a7f10")

// ID is the deterministic UUIDv5 of a legacy document (R400).
func ID(collection, objectID string) uuid.UUID {
	return uuid.NewSHA1(namespace, []byte(collection+":"+objectID))
}
