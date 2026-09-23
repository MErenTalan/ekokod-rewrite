// Package ingest will hold the ingestion pipeline that turns
// internal/integration adapter output into persisted readings (Task 10:
// fetch planning, deduplication and the store writes) plus its
// internal/ingest/generation and internal/ingest/backfill subpackages
// (Tasks 11 and 15; iSolar production moved to internal/service/solar in F9).
//
// This file is a placeholder — package clause and doc comment only — so
// that internal/arch's F2 float and purity guards (TestIntegrationTreesDoNotParseFloats,
// TestNoFloatFieldsInModelOrStore) have a non-empty package to load before
// Task 10 exists; loadTypedPackages/loadPackages require a non-empty match,
// or the guard would pass vacuously. Task 10 owns every other file directly
// in this package and leaves this one unchanged.
package ingest
