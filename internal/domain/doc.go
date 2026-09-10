// Package domain holds the platform's business rules as pure functions:
// no database, no HTTP, no clock, no filesystem. Sub-packages arrive with the
// phases that need them — energy (F3), tariff, billing, reactive (F4),
// alarm (F7), carbon (F10), loadprofile and report.
//
// The dependency rule is enforced by internal/arch/arch_test.go.
package domain
