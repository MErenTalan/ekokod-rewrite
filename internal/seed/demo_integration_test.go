//go:build integration

package seed_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/MErenTalan/ekokod-rewrite/internal/auth"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/seed"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

func TestSeedDemoIdempotentAndExtends(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	hasher := auth.Hasher{Pepper: []byte("pepper-0123456789abcdef-0123456789"), Cost: bcrypt.MinCost}
	at := time.Date(2026, 9, 17, 10, 20, 0, 0, time.UTC)

	pool := testfixtures.NewIsolatedDB(t)
	n, err := seed.ExtendDemo(ctx, pool, at)
	require.NoError(t, err)
	require.Zero(t, n, "no demo company: nothing to extend")
	_, err = seed.SeedDemo(ctx, pool, hasher, "", at)
	require.Error(t, err, "creating the demo user needs a password")
	_, err = seed.SeedDemo(ctx, pool, hasher, "demo1234", at)
	require.ErrorIs(t, err, seed.ErrWeakPassword)

	res, err := seed.SeedDemo(ctx, pool, hasher, "Guvenli!Sifre-42", at)
	require.NoError(t, err)
	require.True(t, res.Created)
	require.Equal(t, 2*(180*24+1), res.Readings)
	res, err = seed.SeedDemo(ctx, pool, hasher, "", at)
	require.NoError(t, err)
	require.False(t, res.Created)
	require.Zero(t, res.Readings, "a second run in the same hour adds nothing")

	later := at.Add(3 * time.Hour)
	n, err = seed.ExtendDemo(ctx, pool, later)
	require.NoError(t, err)
	require.Equal(t, 6, n)

	sc := store.SystemScope(seed.DemoCompanyID)
	readings := postgres.NewReadingRepository(pool)
	window := store.TimeRange{From: at.Add(-48 * time.Hour), To: later.Add(time.Hour)}
	extended, err := readings.Range(ctx, sc, seed.DemoAnalyzerIDs[0], window, model.ReadingKindLoadProfile)
	require.NoError(t, err)
	for i := 1; i < len(extended); i++ {
		require.Equal(t, time.Hour, extended[i].Ts.Sub(extended[i-1].Ts), "contiguous hourly readings")
		require.True(t, extended[i].ActiveImport.GreaterThan(*extended[i-1].ActiveImport))
	}

	fresh := testfixtures.NewIsolatedDB(t)
	_, err = seed.SeedDemo(ctx, fresh, hasher, "Guvenli!Sifre-42", later)
	require.NoError(t, err)
	regenerated, err := postgres.NewReadingRepository(fresh).Range(ctx, sc, seed.DemoAnalyzerIDs[0], window, model.ReadingKindLoadProfile)
	require.NoError(t, err)
	require.Len(t, regenerated, len(extended))
	steps := func(rows []model.MeterReading) []string {
		var out []string
		for i := 1; i < len(rows); i++ {
			out = append(out, rows[i].ActiveImport.Sub(*rows[i-1].ActiveImport).String())
		}
		return out
	}
	require.Equal(t, steps(extended), steps(regenerated), "extending and seeding from scratch produce the same hourly consumption")

	analyzer, err := postgres.NewAnalyzerRepository(pool).Get(ctx, sc, seed.DemoAnalyzerIDs[1])
	require.NoError(t, err)
	require.True(t, analyzer.LastReadingAt.Equal(later.Truncate(time.Hour)))
}
