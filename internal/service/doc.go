// Package service is the root of the service layer F3 introduces above
// internal/domain: internal/service/consumption and
// internal/service/loadprofile. It sits between internal/api and
// internal/domain (03 §2.1, §2.2; R75, R76):
//
//	api ──▶ service ──▶ domain
//
// A service package may import internal/domain/..., internal/store,
// internal/job and internal/platform/.... It must NOT import internal/api or
// internal/ingest, directly or transitively — enforced by
// internal/arch/arch_test.go's TestServiceLayerImportBoundaries.
// internal/store/... and internal/integration/... must not import
// internal/service/... — enforced by
// TestStoreAndIntegrationDoNotImportAPIOrService.
//
// Every service method that touches tenant data takes ctx first and
// store.Scope second, validated with Scope.Valid() before any I/O — the same
// rule internal/store/postgres already enforces one layer down.
//
// This file documents the layer's import boundary; the concrete rules and
// their rationale live in internal/service/consumption/doc.go and
// internal/service/loadprofile/doc.go.
package service
