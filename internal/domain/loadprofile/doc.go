// Package loadprofile implements docs/rewrite/02-domain-rules.md §10.3: load
// profile classification, per-hour averaging across matching days, and
// per-profile statistics.
//
// It is pure: the company's weekend days, vacation periods, and the
// *time.Location to evaluate calendar dates in are all parameters, resolved
// by internal/service/loadprofile before it calls in here. This package
// imports nothing from the project, does no I/O, and never calls time.Now().
//
// The following rulings from
// docs/superpowers/plans/2026-09-16-f3-consumption-engine.md ("Spec gaps and
// rulings") govern this package's shapes and arithmetic and are recorded
// here because an implementer reading only the code should be able to see
// why it looks the way it does:
//
//   - R67: Stats computes standard deviation over a profile's 24 hourly
//     means using the POPULATION divisor n, not the sample divisor n-1. The
//     24 hourly means are the whole curve §10.3 describes, not a sample
//     drawn from some larger population.
//   - R68: HourValue carries a bare decimal reading and this package never
//     names a register. §10.3's "the mean consumption across all matching
//     days" is resolved by the caller: internal/service/loadprofile feeds
//     Build active-import consumption only. A later phase can feed the same
//     function export or generation values without a change here.
//   - R69: Config.WeekendDays is applied exactly as given, with no default.
//     The documented fallback of {Saturday, Sunday} for a company that
//     configured no weekend days lives in internal/service/loadprofile
//     (loadprofile.DefaultWeekendDays there), not in this package.
//   - R70: calendar_events never affect classification. Config has no field
//     through which one could reach Classify — there is no possible input
//     to guard against, by construction.
//   - R81: Stats.LoadFactor is decimal.Zero when Max is zero, because §10.3
//     says so literally. This is deliberately UNLIKE the R54 consumption
//     ratios computed elsewhere in F3, which are nil on an undefined
//     denominator instead of zero — both rulings are cited together, here
//     and at the LoadFactor computation, so the asymmetry reads as
//     intentional rather than an inconsistency.
//   - R82: SeasonOf uses meteorological month boundaries (winter
//     December-February, spring March-May, summer June-August, autumn
//     September-November) rather than the legacy system's equinox-anchored
//     21 March / 21 June / 23 September / 21 December cutoffs. This is a
//     deliberate divergence from the legacy report: some days move between
//     seasonal profiles near the equinoxes.
//   - R83: hours are 0-23 everywhere in this package (HourValue.Hour,
//     Profile.Hours' index), matching time.Time.Hour() and the aggregates —
//     never the legacy system's 1-24 axis.
//   - R84: Statistics are computed over the 24 hourly means held in a
//     Profile — not over each hour's spread across the individual days that
//     fed it. The legacy per-hour spread is not reproduced.
//
// R79 ("intermediate values are never rounded") also holds throughout: no
// Round, Truncate or StringFixed call appears in this package's production
// code. Every division uses decimal.Decimal.DivRound at a scale that is
// precision (how many fractional digits are kept internally), never
// presentation (how a figure is displayed).
package loadprofile
