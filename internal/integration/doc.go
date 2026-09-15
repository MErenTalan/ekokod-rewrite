// Package integration defines the provider-agnostic contract every meter
// data source, market-data source and credential source implements: the
// Adapter interface (06-integrations.md §1, verbatim), the request/result
// shapes that cross it, credential handling that never lets plaintext
// escape (Secret), and the typed errors that drive retry policy (Error,
// the sentinels, Retryable, RetryAfter).
//
// This package is PURE (06 §1 rule 2): it imports nothing under
// internal/store, internal/ingest, internal/credentials or
// internal/marketdata, and nothing from github.com/jackc/pgx. Concrete
// providers (internal/integration/osos, .../gridbox, .../aril, .../pm5340,
// .../isolar, .../epias) and the ingestion pipeline (internal/ingest)
// depend on this package; it never depends back on them. This is enforced
// by internal/arch's TestAdaptersDoNotImportTheStore.
package integration
