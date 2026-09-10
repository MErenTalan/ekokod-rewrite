package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseRateLimit(t *testing.T) {
	cases := map[string]RateLimit{
		"120/min":   {Limit: 120, Window: time.Minute},
		"5/15min":   {Limit: 5, Window: 15 * time.Minute},
		"1000/hour": {Limit: 1000, Window: time.Hour},
		"10/s":      {Limit: 10, Window: time.Second},
	}
	for in, want := range cases {
		got, err := parseRateLimit(in)
		require.NoError(t, err, in)
		require.Equal(t, want, got, in)
	}

	for _, bad := range []string{"", "120", "abc/min", "120/", "-1/min", "0/min", "120/fortnight"} {
		_, err := parseRateLimit(bad)
		require.Error(t, err, bad)
	}
}

func TestRedactDSN(t *testing.T) {
	require.Equal(t,
		"postgres://user:••••@db:5432/ekokod?sslmode=require",
		redactDSN("postgres://user:s3cret@db:5432/ekokod?sslmode=require"))
	require.Equal(t, "redis://redis:6379/0", redactDSN("redis://redis:6379/0"))
	// "not a url" parses without error (url.Parse is extremely permissive —
	// it succeeds with an empty Scheme, treating the whole input as a
	// relative path) but carries no scheme, so it is not URL-shaped. Before
	// task 9's fix round 2, redactDSN's fallback for this case was a bare
	// `return raw`, which happened to be safe here only because this
	// particular string contains no credential — see
	// TestRedactDSNNeverLeaksThePasswordAcrossDSNShapes below for the
	// shapes where that same fallback leaked a real password in clear. The
	// fixed redactDSN withholds the value for any non-URL-shaped input
	// unconditionally, whether or not this specific input happens to be
	// safe, because a redactor that only fails closed on the inputs its
	// author thought to test is not fail-closed.
	require.Equal(t, "(non-URL DSN — value withheld)", redactDSN("not a url"))
}

// TestRedactDSNNeverLeaksThePasswordAcrossDSNShapes pins the task 9 review
// round 2 Critical: config.redactDSN's two `return raw` fallbacks (on a
// parse error, and when url.User is nil) failed OPEN — they printed the
// input completely unredacted — for any DSN shape whose credential does
// not live in URL userinfo. dsnDisplay (load.go) unconditionally prepends
// the mask glyph to whatever redactDSN returns, so the output *looked*
// redacted while the very next token was the cleartext password. All four
// non-userinfo shapes below are accepted by pgxpool.ParseConfig and by
// config's own requiredDSN (which only requires url.Parse to succeed), so
// this was reachable today through `ekokod config:check`, and task 9's fix
// round newly wired the identical exposure into `ekokod seed`.
func TestRedactDSNNeverLeaksThePasswordAcrossDSNShapes(t *testing.T) {
	const password = "s3cr3t"
	cases := map[string]string{
		"userinfo (safe today, must stay safe)":  "postgres://u:" + password + "@localhost:5432/ekokod?sslmode=disable",
		"pgx keyword/value":                      "host=db user=x password=" + password + " dbname=y",
		"unix socket + query credential":         "postgres:///ekokod?host=/var/run/postgresql&password=" + password,
		"query credential, no userinfo password": "postgres://u@localhost:5432/ekokod?password=" + password,
		"query credential, no userinfo at all":   "postgresql://localhost/ekokod?user=x&password=" + password,
	}
	for name, in := range cases {
		got := dsnDisplay(in)
		require.NotContains(t, got, password, "%s: input=%q rendered=%q", name, in, got)
	}
}
