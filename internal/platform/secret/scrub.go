package secret

import (
	"errors"
	"fmt"
)

// scrubbed is an error whose printed text has already been through Redact
// and whose cause is still reachable by errors.Is and errors.As.
//
// It exists because the two halves of the requirement pull in opposite
// directions. Building the error with fmt.Errorf("%s: %s", op, redacted)
// — what every scrubErr used to do — protects the credential but flattens
// the cause to a string, so no caller can ever ask
// errors.Is(err, pgx.ErrNoRows). Switching that verb to %w restores the
// traversal but makes the wrapper's own Error() render the *unredacted*
// cause, which is the leak the scrubber exists to prevent. Splitting the
// two responsibilities across Error and Unwrap satisfies both.
//
// DELIBERATE, CALLER-VISIBLE CONSEQUENCE: errors.Unwrap(err).Error() still
// yields the original, UNREDACTED text. That is the point of keeping the
// cause at all — a caller that reaches past Error() to inspect the driver's
// own error has taken an explicit step to do so, and owns what it does with
// the result. Nothing may take that step on a path that logs, renders an
// HTTP body, or writes to stderr: those must print err.Error() (or use the
// error with %v/%s/%w, all of which route through Error) and nothing else.
type scrubbed struct {
	msg   string // already redacted; the ONLY text ever printed
	cause error
}

func (e *scrubbed) Error() string { return e.msg }

// Unwrap exposes the unredacted cause to errors.Is/errors.As only. fmt does
// not follow Unwrap when formatting, so no verb can reach this text.
func (e *scrubbed) Unwrap() error { return e.cause }

// Wrap returns err reported under op, with every fragment removed from the
// message, while leaving err reachable by errors.Is and errors.As. Pass
// Fragments(password) as fragments. A nil err returns nil so callers may
// call Wrap unconditionally.
//
// AN EMPTY fragments LIST FAILS CLOSED. Redact(msg, nil) is a no-op, so an
// empty list would mean "print the underlying error verbatim" — the scrubber
// appearing to run while removing nothing. That is not a hypothetical: a
// caller reaches this state whenever it could not locate the credential in
// its connection string, which is exactly when the underlying error is most
// likely to contain one. Two connection-string shapes pgx accepts do this
// (libpq keyword/value form, and a password carried as a URL query
// parameter), as does any pool whose credential came from PGPASSWORD or a
// .pgpass file. "I do not know what to redact" therefore means "show
// nothing", never "show everything", and the decision lives here at the one
// choke point rather than in each caller, so no future call site can
// reintroduce it.
func Wrap(op string, fragments []string, err error) error {
	if err == nil {
		return nil
	}
	if len(fragments) == 0 {
		return Withhold(op, "no credential could be identified, so nothing in the underlying error can be shown safely", err)
	}
	return &scrubbed{
		msg:   fmt.Sprintf("%s: %s", op, Redact(err.Error(), fragments)),
		cause: err,
	}
}

// Withhold reports that op failed and why nothing more can be shown, without
// including ANY of cause's own text. reason must describe the situation, not
// the error: it is printed verbatim, so it must never be derived from a
// value that could carry a credential.
//
// cause may be nil, and choosing that is a real decision rather than a
// convenience. Attaching it is usually right: withholding is a statement
// about what may be PRINTED, not about what a caller may inspect, fmt never
// follows Unwrap, and errors.Is/errors.As keep working against a real driver
// sentinel on a path where the text had to be dropped. The caveat documented
// on the scrubbed type applies with full force — errors.Unwrap(err).Error()
// yields the original text, which on this path is precisely the text we
// decided we could not show.
//
// Pass nil when the cause IS the dangerous value and no caller would ever
// match on it. internal/store/postgres/scrub.go's dsn-did-not-parse branch is
// the case in point: *url.Error embeds its whole input, so the cause there is
// the raw DSN, and no one writes errors.Is against a URL-parse sentinel. A
// reachable cause would cost containment and buy nothing.
func Withhold(op, reason string, cause error) error {
	return &scrubbed{
		msg:   fmt.Sprintf("%s: details withheld (%s)", op, reason),
		cause: cause,
	}
}

// IsScrubbed reports whether err, or anything it wraps, was produced by this
// package — i.e. whether its text has actually been through the scrubber.
//
// It exists to be a POSITIVE fingerprint. Before scrubbed errors became
// traversable, "errors.Unwrap(err) == nil" was the only way a test could
// tell a scrubbed error from a plain fmt.Errorf("%w") wrap, and that
// distinguishing property is gone now that the cause is deliberately
// reachable. Without a replacement, a call site on a credential-bearing path
// could be reverted to a bare %w wrap and every assertion about it would
// stay green, because the errors would be indistinguishable. Assertions of
// the form "no credential appears in the text" cannot cover that gap on
// their own: they are vacuous whenever the induced failure produced a driver
// message that never contained the credential to begin with.
func IsScrubbed(err error) bool {
	var target *scrubbed
	return errors.As(err, &target)
}

// URLParseErr reports a redis URL parse failure without ever including the
// raw input: goredis.ParseURL forwards net/url's parse error unchanged, and
// url.Error.Error() embeds its whole argument — including any password —
// verbatim. We cannot reason about what a URL we cannot even parse might
// contain, so the underlying error text is dropped entirely rather than
// risk leaking a credential.
//
// Unlike Wrap, this returns an error with NO cause: the cause is precisely
// the value that cannot be shown, so there is nothing safe to keep. It lives
// here rather than in either caller because internal/job and
// internal/store/redis both need it and internal/job must not import
// internal/store. internal/store/postgres/scrub.go keeps its own DSN
// variant, which has genuinely diverged: it parses the DSN with net/url and
// only withholds everything when that parse fails.
func URLParseErr(op string) error {
	return fmt.Errorf("%s: redis url is not valid (details withheld)", op)
}
