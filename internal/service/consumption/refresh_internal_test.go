package consumption

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// migrationPolicyIntervalPattern matches one
// add_continuous_aggregate_policy(...) call's view name and its
// start_offset interval literal, tolerant of the exact whitespace migration
// 00005_continuous_aggregates.sql uses.
var migrationPolicyIntervalPattern = regexp.MustCompile(
	`add_continuous_aggregate_policy\('(\w+)',\s*\n\s*start_offset\s*=>\s*interval '([^']+)'`)

// parseIntervalDuration converts a Postgres interval literal of the exact
// shape migration 00005 uses ("30 days", "1 year", "5 years") into a
// time.Duration, using the same day/year conversion this package's policy
// constants do (365-day year, no leap adjustment) — so a comparison against
// those constants is meaningful.
func parseIntervalDuration(t *testing.T, s string) time.Duration {
	t.Helper()
	fields := strings.Fields(s)
	require.Lenf(t, fields, 2, "unexpected interval literal %q", s)
	n, err := strconv.Atoi(fields[0])
	require.NoError(t, err)
	unit := strings.TrimSuffix(fields[1], "s")
	switch unit {
	case "day":
		return time.Duration(n) * 24 * time.Hour
	case "year":
		return time.Duration(n) * 365 * 24 * time.Hour
	default:
		t.Fatalf("unsupported interval unit %q in %q", fields[1], s)
		return 0
	}
}

// TestPolicyStartOffsetsMatchMigration is R100(3)'s drift guard: it parses
// internal/store/postgres/migrations/00005_continuous_aggregates.sql's
// add_continuous_aggregate_policy calls directly and fails the moment this
// package's hourly/daily/monthly/yearlyPolicyStartOffset constants disagree
// with what the migration actually declares. A mutation that changes either
// side without the other must turn this red.
func TestPolicyStartOffsetsMatchMigration(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller failed to resolve this test file's path")
	migrationPath := filepath.Join(filepath.Dir(thisFile), "..", "..", "store", "postgres", "migrations", "00005_continuous_aggregates.sql")

	data, err := os.ReadFile(migrationPath)
	require.NoError(t, err, "read migration 00005 at %s", migrationPath)
	content := string(data)

	matches := migrationPolicyIntervalPattern.FindAllStringSubmatch(content, -1)
	found := map[string]time.Duration{}
	for _, m := range matches {
		found[m[1]] = parseIntervalDuration(t, m[2])
	}

	cases := []struct {
		view string
		want time.Duration
	}{
		{"consumption_hourly", hourlyPolicyStartOffset},
		{"consumption_daily", dailyPolicyStartOffset},
		{"consumption_monthly", monthlyPolicyStartOffset},
		{"consumption_yearly", yearlyPolicyStartOffset},
	}

	for _, c := range cases {
		t.Run(c.view, func(t *testing.T) {
			got, ok := found[c.view]
			require.True(t, ok, "migration 00005 has no add_continuous_aggregate_policy(%q, ...) with a start_offset this regex could parse", c.view)
			require.Equal(t, c.want, got,
				"consumption package's policy start_offset constant for %s has drifted from migration 00005's actual start_offset", c.view)
		})
	}
}

// TestPolicyStartOffsetForViewUnmappedViewErrors proves an unmapped view is
// reported as an error rather than silently defaulting to some other
// view's policy window.
func TestPolicyStartOffsetForViewUnmappedViewErrors(t *testing.T) {
	_, err := policyStartOffsetForView("not_a_real_view")
	require.Error(t, err)
}
