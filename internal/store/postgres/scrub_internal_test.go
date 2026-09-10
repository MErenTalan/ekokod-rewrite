package postgres

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/secret"
	"github.com/stretchr/testify/require"
)

var errDriver = errors.New("driver sentinel")

// dsnFormsThatHideThePassword are the connection-string shapes pgx accepts
// and net/url parses successfully while yielding NO userinfo password.
// Every one of them used to reach scrubErr's redact branch with an empty
// fragment list, and Redact(msg, nil) is a no-op — so the driver error was
// printed VERBATIM. That is the fail-open class this whole task is about:
// the scrubber appeared to run and removed nothing.
//
// Both forms also pass config.requiredDSN's url.Parse validation, so
// neither is rejected before it ever reaches the store layer.
var dsnFormsThatHideThePassword = []struct {
	name string
	dsn  string
}{
	{
		// libpq keyword/value form. url.Parse does not reject the spaces;
		// it simply finds no userinfo, so u.User is nil.
		name: "keyword value form",
		dsn:  "host=localhost user=ekokod password=s3cret dbname=ekokod",
	},
	{
		// URL form carrying the credential as a query parameter instead of
		// in userinfo. u.User is non-nil but has no password.
		name: "password in query parameter",
		dsn:  "postgres://user@host:5432/db?password=s3cret",
	},
}

// TestScrubErrWithholdsWhenNoPasswordIsFound pins the fail-CLOSED rule: if
// the DSN parses but we cannot identify a credential in it, we do not know
// what to redact, so nothing from the driver error may be shown at all.
func TestScrubErrWithholdsWhenNoPasswordIsFound(t *testing.T) {
	for _, form := range dsnFormsThatHideThePassword {
		t.Run(form.name, func(t *testing.T) {
			cause := fmt.Errorf("failed to connect: password authentication failed using %q: %w", "s3cret", errDriver)

			err := scrubErr(form.dsn, "ping database", cause)

			require.Error(t, err)
			require.NotContains(t, err.Error(), "s3cret",
				"a DSN whose credential we could not locate must have its driver error withheld entirely, not passed through unredacted")
			require.True(t, strings.HasPrefix(err.Error(), "ping database: "),
				"the operation must still be named so the failure stays diagnosable, got %q", err.Error())
			require.ErrorIs(t, err, errDriver,
				"withholding the TEXT must not cost callers errors.Is on the cause")
		})
	}
}

// TestScrubErrStillRedactsWhenThePasswordIsFound guards the other direction:
// failing closed on an unlocatable credential must not turn every error into
// a content-free one when the credential IS locatable.
func TestScrubErrStillRedactsWhenThePasswordIsFound(t *testing.T) {
	cause := fmt.Errorf("dial tcp s3cret.example.com:5432: %w", errDriver)

	err := scrubErr("postgres://ekokod:s3cret@host:5432/db", "ping database", cause)

	msg := err.Error()
	require.NotContains(t, msg, "s3cret")
	require.Contains(t, msg, secret.Mask, "the driver error must still come through, redacted")
	require.Contains(t, msg, "dial tcp", "diagnostic detail unrelated to the credential is preserved")
	require.ErrorIs(t, err, errDriver)
}

// TestScrubPoolErrRedactsTheCredentialOnTheReadinessPath is the positive
// fingerprint for scrubPoolErr, which health.go:61 uses for MigrationsCheck.
// That is the sharpest path in the package: readyHandler serialises the
// error into the UNAUTHENTICATED /health/ready JSON body, which the web
// health page renders.
//
// The integration test cannot prove redaction here — it induces the failure
// with pool.Close(), whose driver text ("closed pool") never contained a
// credential in the first place, so a NotContains assertion there passes
// whether or not any redaction ran. Supplying the cause directly is what
// makes the assertion mean something.
func TestScrubPoolErrRedactsTheCredentialOnTheReadinessPath(t *testing.T) {
	cause := fmt.Errorf("FATAL: password authentication failed for user %q (tried %q): %w", "ekokod", "ekokod", errDriver)

	err := scrubPoolErr("ekokod", "read schema version", cause)

	msg := err.Error()
	require.NotContains(t, msg, "ekokod", "no credential may reach the readiness body")
	require.Contains(t, msg, secret.Mask, "the mask is the positive proof that redaction actually ran")
	require.Contains(t, msg, "read schema version")
	require.ErrorIs(t, err, errDriver, "errors.Is must traverse a scrubbed error")
	require.True(t, secret.IsScrubbed(err), "the readiness path must return a scrubbed error, not a plain wrap")
}

// TestScrubPoolErrWithholdsWhenThePoolHasNoPassword is the pool-side half of
// the fail-closed rule. pgx leaves ConnConfig.Password empty for a DSN that
// supplies the credential another way (PGPASSWORD, a .pgpass file, or a
// query parameter pgx understands but our net/url read does not), and an
// empty password used to mean "redact nothing".
func TestScrubPoolErrWithholdsWhenThePoolHasNoPassword(t *testing.T) {
	cause := fmt.Errorf("FATAL: password authentication failed using %q: %w", "s3cret", errDriver)

	err := scrubPoolErr("", "read schema version", cause)

	require.NotContains(t, err.Error(), "s3cret")
	require.Contains(t, err.Error(), "read schema version")
	require.ErrorIs(t, err, errDriver)
}

// TestScrubErrAttachesNoCauseWhenTheDSNDidNotParse pins a deliberate
// inconsistency, so that "fixing" it fails loudly.
//
// Every other branch in scrub.go keeps the cause reachable: that is the whole
// point of secret.Wrap, and it buys callers errors.Is against a real driver
// sentinel. This branch must NOT, because here the cause is *url.Error, whose
// Error() embeds its entire input verbatim — the raw DSN, password and all.
// That is the value behind Critical #1 of the previous phase. Attaching it
// would put the DSN back within reach of errors.Unwrap, and would buy
// nothing: nobody matches on a URL-parse sentinel.
//
// The uniformity argument for attaching it is reasonable, which is exactly
// why this test exists rather than a comment alone.
func TestScrubErrAttachesNoCauseWhenTheDSNDidNotParse(t *testing.T) {
	// url.Parse rejects this outright, and its error quotes the whole DSN.
	// Assembled at runtime rather than written as a literal because
	// staticcheck's SA1007 rejects a constant invalid URL passed to
	// url.Parse — invalidity being the entire point here. That the linter
	// objects at all is independent confirmation of the premise.
	dsn := fmt.Sprintf("postgres://ekokod:%s@host:5432/db%s", "s3cret", "%zz")

	_, parseErr := url.Parse(dsn)
	require.Error(t, parseErr, "the fixture must actually fail to parse")
	require.Contains(t, parseErr.Error(), "s3cret",
		"the premise of this test: url.Error embeds its input, so the cause carries the credential")

	err := scrubErr(dsn, "parse database url", parseErr)

	require.NotContains(t, err.Error(), "s3cret")
	require.Contains(t, err.Error(), "parse database url")
	require.NoError(t, errors.Unwrap(err),
		"this branch must attach NO cause: unwrapping it would expose the raw DSN that url.Error embedded")
	require.True(t, secret.IsScrubbed(err),
		"withholding still goes through the scrubber, so the value stays identifiable as scrubbed")
}
