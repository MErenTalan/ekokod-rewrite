// Package loadprofile resolves a company's calendar (weekend days and
// vacation periods) and its consumption_hourly buckets, and feeds the pure
// internal/domain/loadprofile package to produce 24-hour load profiles and
// their statistics — 02-domain-rules.md §10.3, backing the
// GET /companies/{id}/analyzers/{id}/load-profile endpoint (05 §5).
//
// This package sits in the service layer introduced by F3: it may import
// internal/domain/..., internal/store, internal/job and internal/platform,
// and must not import internal/api or internal/ingest (R76,
// internal/service/doc.go). Every method that touches tenant data takes ctx
// first and store.Scope second, validated before any I/O.
//
// The following rulings from
// docs/superpowers/plans/2026-09-16-f3-consumption-engine.md ("Spec gaps and
// rulings") govern this package and are recorded here so an implementer
// reading only the code can see why it looks the way it does:
//
//   - R68: the load profile is fed active-import consumption only. This
//     package never reads any other register; internal/domain/loadprofile
//     never names one either.
//   - R69: the pure classifier applies exactly the weekend-day set it is
//     given (no default there). This package resolves it:
//     store.CalendarRepository.WeekendDays when the company has configured
//     rows, otherwise the documented default {Saturday, Sunday}
//     (DefaultWeekendDays below), recording ConfigUsed.WeekendSource so a
//     caller can show which applied.
//   - R70: calendar_events never affect classification. This package never
//     calls CalendarRepository.Event/ListEvents. Only company_weekend_days
//     and company_vacations feed Classify. This holds regardless of Q4's
//     escalation to F6 — 09-implementation-plan.md §F6 line 434's wording
//     implies a calendar-event-driven split, but F6 must not build that
//     without a product-owner ruling, and F3 does not anticipate it.
//   - R87: hour h's load-profile value is
//     ActiveImportStart(h+1) − ActiveImportStart(h) across two CONSECUTIVE
//     consumption_hourly rows — never that hour's own active_consumption
//     column, which is structurally ZERO for a 1-hour meter (its one
//     reading is both first() and last()) and ~25% low for a 15-minute one.
//     This package therefore reads store.AnalyticsRepository directly
//     through the narrow HourlySource seam, bypassing
//     internal/service/consumption's Row/Values entirely — that type holds
//     the bucket's own active_consumption, the wrong figure here. The value
//     is nil when either bucket is missing or the difference is negative;
//     this analysis-side computation never writes an anomaly (only the
//     billing path does, R58).
//
// Q6 is a related, DELIBERATELY UNCORRECTED defect: the /analytics chart
// endpoint's own hourly aggregate (migration 00005's active_consumption,
// last-first inside the bucket) is still all zeros for a 1-hour meter. F3
// fixes the load profile (R87) but leaves the analytics chart as §4.3
// defines it, because acceptance criterion 4 requires that documented
// step-loss behaviour; the defect is real and has been raised with the
// product owner / F6, not silently left for a later phase to rediscover.
package loadprofile
