// Package service is the root of the service layer F3 introduces above
// internal/domain: internal/service/consumption (Tasks 7-9, 11) and
// internal/service/loadprofile (Task 10). It sits between internal/api and
// internal/domain (03 §2.1, §2.2; R75, R76):
//
//	api ──▶ service ──▶ domain
//
// A service package may import internal/domain/..., internal/store,
// internal/job and internal/platform/.... It must NOT import internal/api or
// internal/ingest. internal/store/... and internal/integration/... must not
// import internal/service/... — enforced by
// internal/arch/arch_test.go's TestServiceLayerImportBoundaries.
//
// Every service method that touches tenant data takes ctx first and
// store.Scope second, validated with Scope.Valid() before any I/O — the same
// rule internal/store/postgres already enforces one layer down.
//
// This file exists so the tree is a non-empty Go package the moment the
// layer's guards land (Task 1), before any service package has real code —
// the same reason internal/integration/doc.go and its siblings were added
// ahead of F2's adapters.
package service
