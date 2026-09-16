// Package admin holds the only unscoped query surface in the system
// (docs/rewrite/03-target-architecture.md §2.5: "The repository layer takes
// an explicit Scope value; there is no 'unscoped' query method outside the
// admin package."). internal/arch's TestEveryStoreMethodIsScoped enforces
// this by walking every exported, context-taking method under
// internal/store/postgres/... and requiring a store.Scope parameter — except
// here. Every method in this package is exempt from that requirement, and
// every one of them is a permanent, deliberate cross-tenant capability: it
// can read or write across every company in the database, forever, because
// its caller provably has no tenant to offer it.
//
// Nothing outside operator tooling (the CLI's admin commands, the platform
// ingestion jobs, and the authentication path before a Scope exists) may
// import this package. A product request handler that reaches for a
// convenient method here instead of its scoped sibling has created a
// cross-tenant hole that no test at the request layer will ever catch, because
// the method itself is exactly as capable as it looks: nothing narrows it.
//
// The seven interfaces, and why each one cannot take a Scope:
//
//   - AdminAuthRepository resolves the two credentials a request presents
//     BEFORE it has a Scope: a login carries only an email, and a refresh
//     carries only a token hash. A Scope is what a successful lookup
//     PRODUCES, not something the caller could supply to obtain it.
//
//   - AdminAuditRepository appends platform audit rows — an action taken by
//     the platform or an operator that belongs to no tenant (a catalogue
//     update, a market-data import). A Scope's CompanyID is never nil, so
//     there is no Scope that means "no company"; borrowing a tenant's would
//     misattribute a platform action to that tenant.
//
//   - AdminMarketDataRepository writes the platform-wide market series
//     (hourly prices, YEKDEM) every tenant's bills are priced from. Any
//     Scope that authorised the write would let one tenant's credentials
//     reprice every tenant's invoices.
//
//   - AdminCatalogueRepository writes the platform reference catalogues
//     (the national tariff schedule, platform emission factors and their
//     conversions, the integration provider catalogue) that every tenant
//     reads through a scoped sibling. The catalogues are shared and
//     maintained by the platform, not owned by any one company, so a Scope
//     would name no legitimate owner.
//
//   - AdminJournalRepository records platform jobs — work such as the market
//     price import that runs for no tenant — in the same job_runs and
//     operational_messages tables tenant jobs use, with company_id NULL. A
//     platform job made to borrow a tenant's Scope would file its failures on
//     that tenant's own Messages screen.
//
//   - AdminIngestionRepository lists the credentials the scheduled ingestion
//     dispatcher must fan out to, on every tick, across every tenant in one
//     pass. Its caller has no tenant to offer for exactly the same structural
//     reason AdminMarketDataRepository's and AdminJournalRepository's do: the
//     dispatcher is not acting on behalf of any one company, it is finding
//     the work every company has waiting. A Scope would have to name a
//     company before the dispatcher even knows which companies have active
//     credentials, which is backwards — this method is what tells it. It is
//     a genuine sixth member of this closed list, not an exception to it:
//     implemented by F2 Task 5.
//
//   - AdminAggregateRepository refreshes TimescaleDB continuous aggregates.
//     refresh_continuous_aggregate takes a time range and nothing else, so a
//     refresh necessarily covers every tenant's buckets in that range, and a
//     Scope parameter would be a lie about what the call actually does. It is
//     a genuine seventh member of this closed list: implemented by F3 Task 6.
//
// THE LIST OF METHODS IS CLOSED. A method is added here only when its caller
// cannot hold a Scope, never because holding one is inconvenient: see each
// interface's declaration in internal/store/repository.go for the closed list
// this package implements exactly, and internal/store/postgres/admin/*.go —
// one file per owning task — for the implementations.
package admin
