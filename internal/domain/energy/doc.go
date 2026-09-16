// Package energy implements docs/rewrite/02-domain-rules.md §2 and §3: the
// meter-reading register contract, the Istanbul-local aggregation buckets,
// and the §3.1 core differencing operation.
//
// It is pure: no I/O, no clock, no package-level state. The current instant,
// the location and every reading are parameters — never read from the
// environment. Enforced by internal/arch/arch_test.go
// (TestDomainHasNoProjectImports, TestDomainHasNoIOImports).
//
// Spec-to-function map:
//
//   - Register, AllRegisters, Kind — 02 §2.1, §2.3 (the register set and the
//     reading classes; mirrors model.MeterReading in spirit without
//     importing it).
//   - Reading, Reading.Value — 02 §2.1: a nil register value means the meter
//     did not report it; it is never treated as zero (removed-behaviour 21).
//   - Level, Window, Bucket, Buckets — 02 §3.4: the four aggregation levels,
//     each bucketed on the Europe/Istanbul calendar through the tz database,
//     never a fixed offset.
//   - Suspicion, Reason, Derivation, Difference — 02 §3.1: the core
//     difference operation for one window with no reset evidence. Applying
//     reset evidence (02 §3.2) is Derive's job, added in Task 2.
package energy
