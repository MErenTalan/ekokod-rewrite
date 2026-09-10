package secret

import "fmt"

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
func Wrap(op string, fragments []string, err error) error {
	if err == nil {
		return nil
	}
	return &scrubbed{
		msg:   fmt.Sprintf("%s: %s", op, Redact(err.Error(), fragments)),
		cause: err,
	}
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
