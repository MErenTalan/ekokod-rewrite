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
	require.Equal(t, "not a url", redactDSN("not a url"))
}
