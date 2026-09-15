// Package credentials will hold credential storage, encryption and
// dispatch scheduling for tenant integration credentials (Task 14).
//
// This file is a placeholder — package clause and doc comment only — so
// that internal/arch's F2 float and purity guards
// (TestIntegrationTreesDoNotParseFloats, TestNoFloatFieldsInModelOrStore)
// have a non-empty package to load before Task 14 exists; loadTypedPackages
// requires a non-empty match, or the guard would pass vacuously. Task 14
// owns every other file in this package and leaves this one unchanged.
package credentials
