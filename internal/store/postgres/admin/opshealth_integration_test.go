//go:build integration

package admin_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/admin"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

// F15b R463: the newest successful and failed reading pull per provider, from job_runs.
func TestIntegrationRunsPerProvider(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	tenant := testfixtures.NewTenant(t, ctx, pool, 1951)
	a := tenant.Analyzers[0]
	ok, bad, old := time.Now().Add(-2*time.Hour).UTC().Truncate(time.Second), time.Now().Add(-time.Hour).UTC().Truncate(time.Second), time.Now().AddDate(0, 0, -30)
	for _, r := range []struct {
		status string
		at     time.Time
		typ    string
	}{{"success", ok, "integration.fetch_readings"}, {"failed", bad, "integration.fetch_readings"},
		{"success", old, "integration.fetch_readings"}, {"success", time.Now(), "billing.generate"}} {
		_, err := pool.Exec(ctx, `insert into job_runs (company_id, job_type, scope, started_at, finished_at, status) values ($1, $2, $3, $4, $4, $5)`,
			tenant.Company.ID, r.typ, `{"analyzer_id":"`+a.ID.String()+`"}`, r.at, r.status)
		require.NoError(t, err)
	}
	runs, err := admin.NewOpsHealthRepository(pool).IntegrationRuns(ctx)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	require.Equal(t, string(a.Provider), runs[0].Provider)
	require.True(t, runs[0].LastSuccess.Equal(ok), "the newest success of the last week")
	require.True(t, runs[0].LastError.Equal(bad))
}
