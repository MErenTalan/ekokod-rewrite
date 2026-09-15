package postgres

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// tariffIstanbulLocation is the one Europe/Istanbul *time.Location every
// `date`-column conversion in this package uses (see tariffTimeToDate).
// "Europe/Istanbul" is a fixed IANA zone name, not user input, so a failure
// to load it means the runtime's tzdata is broken — a startup-time condition
// worth panicking on rather than silently falling back to UTC and mispricing
// every day-boundary decision in the system (02-domain-rules §1: day
// boundaries are Istanbul).
var tariffIstanbulLocation = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		panic("postgres: cannot load Europe/Istanbul: " + err.Error())
	}
	return loc
}()

// This file holds the small set of pgtype <-> time.Time conversions every
// file in this package reuses. It is named for the concern, not for the file
// that first needed it: tariffs.go, bills.go, reports.go and alarms.go all
// call into it, and no sibling Wave F task (9, 10) defines a pgtime.go of its
// own to collide with (wave-f-task-notes.md, Important Finding 6).
//
// The `tariff` prefix on every exported-within-package name is kept even
// though the functions no longer live in tariffs.go: renaming them package-
// wide, on top of moving the file, would have made this diff impossible to
// review against the original report's guard-failure proofs, which name
// these functions by their current identifiers.

// tariffDateToTime and tariffTimeToDate convert between a `date` column and
// time.Time for a NOT NULL date column. Only the calendar date carried by
// pgtype.Date is meaningful; the pair is deliberately not the numeric.go
// pair, which exists for `numeric` columns only.
//
// tariffTimeToDate takes t's calendar day in Europe/Istanbul, never in t's own
// Location and never in UTC: 02-domain-rules §1 fixes day boundaries to
// Istanbul, so the SAME instant given as 2026-03-01T00:30+03:00 and as its
// equal UTC instant 2026-02-28T21:30Z must resolve to the identical `date`
// row (2026-03-01) — the calendar day a person in Istanbul was living through
// when the instant occurred. Converting in t's own Location (or in UTC, which
// is what the pre-fix-round-1 version of this function did) makes an
// otherwise-identical write or query resolve to a different tariff/report
// row depending on the caller's incidental Location, which is the bug
// Important Finding 5 named.
func tariffDateToTime(d pgtype.Date) time.Time {
	if !d.Valid {
		return time.Time{}
	}
	return d.Time
}

func tariffTimeToDate(t time.Time) pgtype.Date {
	ist := t.In(tariffIstanbulLocation)
	y, m, day := ist.Date()
	return pgtype.Date{Time: time.Date(y, m, day, 0, 0, 0, 0, time.UTC), Valid: true}
}

// tariffDateToTimePtr and tariffTimePtrToDate are the nullable-date pair,
// used by BillRepository for bills.tariff_effective_from.
func tariffDateToTimePtr(d pgtype.Date) *time.Time {
	if !d.Valid {
		return nil
	}
	t := d.Time
	return &t
}

func tariffTimePtrToDate(t *time.Time) pgtype.Date {
	if t == nil {
		return pgtype.Date{}
	}
	return tariffTimeToDate(*t)
}

// tariffTimeToExclusiveEndDate takes the Istanbul `date` that ends a
// half-open instant window [from, to) — F1 final review pass A, Important
// Finding 2. A half-open window's `to` instant is itself NOT included, so if
// `to` falls exactly on an Istanbul midnight, the Istanbul day it starts is
// excluded and the exclusive-end date is that same calendar day (a `<
// to_date` SQL predicate already excludes it). Otherwise `to` falls partway
// through an Istanbul day, that day IS partially covered by the window, and
// the exclusive-end date must be the NEXT Istanbul day so a `< to_date`
// predicate still includes it.
//
// Used wherever a TimeRange (instants) is turned into a pair of `date`
// column bounds for an overlap query — see CalendarRepository.Vacations,
// which is what Important Finding 2 named: the pre-fix version took
// timeRange.To.UTC() instead of the Istanbul day, wrong at every Istanbul
// day boundary that does not also fall on a UTC day boundary.
func tariffTimeToExclusiveEndDate(to time.Time) pgtype.Date {
	ist := to.In(tariffIstanbulLocation)
	if h, m, s := ist.Clock(); h != 0 || m != 0 || s != 0 || ist.Nanosecond() != 0 {
		ist = ist.AddDate(0, 0, 1)
	}
	return tariffTimeToDate(ist)
}

// tariffNullTimestamptz reports a nullable timestamptz as *time.Time,
// reused by every file in this package that needs the same nullable
// conversion (DeletedAt, ProcessedAt, NotifiedAt, …).
func tariffNullTimestamptz(ts pgtype.Timestamptz) *time.Time {
	if !ts.Valid {
		return nil
	}
	t := ts.Time
	return &t
}

func tariffTimestamptz(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func tariffNullableTimestamptz(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}
