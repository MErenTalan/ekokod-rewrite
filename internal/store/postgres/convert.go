package postgres

// Shared, package-wide conversion helpers.
//
// MERGE-HYGIENE NOTE (Task 9 fix round 1). This package is shared with Tasks
// 10 and 11 at merge time, and every name below is generic enough that an
// independently-written repository file for readings, tariffs, alarms or
// bills could plausibly declare an identically-named helper of its own. Per
// wave-f-context.md's merge-hygiene instruction, Task 9 is moving its
// non-test package-wide helpers into this one clearly-named file — rather
// than renaming each with an awkward task-specific prefix — so a merge-time
// collision with an identically-named helper from Task 10 or 11 is a single,
// easy-to-find place to resolve (keep one implementation, delete the other;
// every one of these is a pure, stateless conversion, so the two are
// expected to be behaviourally interchangeable). See the task report's
// "package-wide unexported helpers" list for the full inventory.
import (
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// ts converts a NOT NULL timestamp to the pgtype the generated code writes.
func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

// tsPtr converts a NULLABLE pgtype.Timestamptz read back from the database
// into a *time.Time, nil for SQL NULL.
func tsPtr(v pgtype.Timestamptz) *time.Time {
	if !v.Valid {
		return nil
	}
	t := v.Time
	return &t
}

// tsPtrOrZero is ts for a NULLABLE timestamptz column written from a
// *time.Time: a nil pointer becomes SQL NULL rather than the zero instant.
func tsPtrOrZero(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return ts(*t)
}

// tsOrNow is ts for a NOT NULL created_at/updated_at column written from a
// plain time.Time value: a ZERO time.Time (a caller that left CreatedAt
// unset) becomes SQL NULL, which every Create query in this package pairs
// with `coalesce(sqlc.narg(at), now())`, so the row gets the database's own
// now() instead of literally storing 0001-01-01. A caller that DID set a
// real instant gets that instant written verbatim, unchanged.
func tsOrNow(t time.Time) pgtype.Timestamptz {
	if t.IsZero() {
		return pgtype.Timestamptz{}
	}
	return ts(t)
}

// pageLimit resolves store.Page.Limit against a repository's own default and
// cap: zero or negative means "the repository's default", never "no limit",
// and anything above the cap is clamped rather than passed through — see the
// doc on store.Page.
func pageLimit(p store.Page, def, max int32) int32 {
	switch {
	case p.Limit <= 0:
		return def
	case p.Limit > max:
		return max
	default:
		return p.Limit
	}
}

// uuidsOrEmpty normalises a nil []uuid.UUID to a non-nil, empty one.
//
// This is not cosmetic. pgx encodes a nil slice as SQL NULL, and every
// optional "narrow by these ids" filter in this task's queries reads as
// `cardinality(sqlc.arg(x)::uuid[]) = 0 or id = any(sqlc.arg(x)::uuid[])`.
// Postgres's cardinality(NULL) is NULL, not 0, so a nil filter makes the
// whole OR — and everything AND-ed with it — evaluate to NULL, which
// excludes every row: a caller that left an ids filter unset would see an
// empty list instead of an unfiltered one. An empty, non-nil slice encodes
// as '{}', whose cardinality is a real 0, which is what "not filtering by
// this" is supposed to mean. Every FilterIds-shaped parameter in this
// package's List methods must be passed through this first.
func uuidsOrEmpty(ids []uuid.UUID) []uuid.UUID {
	if ids == nil {
		return []uuid.UUID{}
	}
	return ids
}
