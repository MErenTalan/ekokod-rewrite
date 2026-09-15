// Package model holds one plain struct per aggregate in the schema, with the
// field types the business logic is entitled to assume: decimal.Decimal for
// every money and energy column, uuid.UUID for identifiers, time.Time for
// timestamps, and a pointer for every nullable column.
//
// It is the boundary between the database's representation and the rest of
// the system. The generated sqlc types (internal/store/postgres/sqlcgen) speak
// pgtype.Numeric and pgtype.Timestamptz; nothing outside internal/store should
// ever see those. Repositories convert, in both directions, through the single
// audited pair in internal/store/postgres (numericToDecimal/decimalToNumeric).
//
// Three rules hold for everything in this package, and each is enforced
// mechanically rather than by convention:
//
//   - It imports nothing from this project and performs no I/O
//     (internal/arch's TestDomainHasNoProjectImports and
//     TestDomainHasNoIOImports).
//   - No field is ever a float32 or a float64, through any number of
//     pointers, slices, maps or arrays (internal/arch's
//     TestNoFloatFieldsInModelOrStore). Money and energy are numeric in SQL
//     and decimal.Decimal here; a float would silently round exactly the
//     values this phase exists to keep exact.
//   - Every time.Time is a UTC-equivalent instant, because every timestamp
//     column is timestamptz: two time.Time values naming the same instant
//     are interchangeable regardless of Location, and code must compare and
//     store instants, never a Location. The Location on a value decoded from
//     the database is NOT guaranteed to be UTC (pgx decodes timestamptz in
//     time.Local) — callers that need UTC call .UTC() themselves. Business
//     rules that need calendar boundaries convert the instant to
//     Europe/Istanbul at the point of use, never by storing local time.
//
// The structs mirror the migrations in internal/store/postgres/migrations,
// which together with docs/rewrite/04-data-model.md are authoritative for
// names, types and nullability.
package model
