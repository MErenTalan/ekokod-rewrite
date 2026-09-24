//go:build integration

package cli_test

import (
	"bytes"
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/auth"
	"github.com/MErenTalan/ekokod-rewrite/internal/cli"
	"github.com/MErenTalan/ekokod-rewrite/internal/seed"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

// TestRecomputeThroughTheCLI regenerates derived data over the e2e fixtures
// (real tariffs and readings) and proves a rerun converges (08 §7, Q-K1).
func TestRecomputeThroughTheCLI(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	redisURL := os.Getenv("EKOKOD_TEST_REDIS_URL")
	if redisURL == "" {
		t.Skip("EKOKOD_TEST_REDIS_URL is not set")
	}
	now := time.Now()
	_, err := seed.Load(ctx, pool, testfixtures.DiscardLogger())
	require.NoError(t, err)
	f, err := seed.E2EFixtures(ctx, pool, auth.Hasher{Pepper: []byte("password-pepper-at-least-32-chars-long!!"), Cost: 4}, "Guvenli!Sifre-42", now)
	require.NoError(t, err)
	_, err = seed.E2EData(ctx, pool, f, now)
	require.NoError(t, err)
	setValidEnv(t, map[string]string{"EKOKOD_DB_URL": pool.Config().ConnString(), "EKOKOD_REDIS_URL": redisURL,
		"EKOKOD_REDIS_QUEUE_DB": "9", "EKOKOD_REDIS_CACHE_DB": "8", "EKOKOD_STORAGE_ROOT": t.TempDir()})

	month := now.AddDate(0, -2, 0).Format("2006-01")
	var out bytes.Buffer
	require.NoError(t, cli.Execute(ctx, []string{"recompute", "consumption", "--from", now.AddDate(0, -3, 0).Format(time.DateOnly)}, &out), out.String())
	require.Contains(t, out.String(), `"failed": 0`)

	count := func() (live, all int) {
		require.NoError(t, pool.QueryRow(ctx, `select count(*) filter (where status <> 'superseded'), count(*) from bills`).Scan(&live, &all))
		return live, all
	}
	out.Reset()
	err = cli.Execute(ctx, []string{"recompute", "bills", "--from", month}, &out)
	firstLive, firstAll := count()
	require.Positive(t, firstLive, "bills were generated: %s", out.String())
	out.Reset()
	err2 := cli.Execute(ctx, []string{"recompute", "bills", "--from", month}, &out)
	live, all := count()
	require.Equal(t, []int{firstLive, firstAll}, []int{live, all}, "without --force a rerun converges")
	require.Equal(t, err == nil, err2 == nil, "the same subjects fail both times")

	out.Reset()
	require.NoError(t, cli.Execute(ctx, []string{"recompute", "carbon", "--from", now.AddDate(0, 0, -3).Format(time.DateOnly)}, &out), out.String())
	require.Contains(t, out.String(), `"processed": 3`)
}
