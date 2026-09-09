package health_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/health"
	"github.com/stretchr/testify/require"
)

func TestAllChecksPass(t *testing.T) {
	rep := health.Run(context.Background(), time.Second,
		health.Check{Name: "database", Fn: func(context.Context) error { return nil }},
		health.Check{Name: "redis", Fn: func(context.Context) error { return nil }},
	)
	require.Equal(t, "ok", rep.Status)
	require.Len(t, rep.Checks, 2)
	require.Equal(t, "ok", rep.Checks[0].Status)
}

func TestFailingCheckDegradesTheReport(t *testing.T) {
	rep := health.Run(context.Background(), time.Second,
		health.Check{Name: "database", Fn: func(context.Context) error { return nil }},
		health.Check{Name: "redis", Fn: func(context.Context) error { return errors.New("connection refused") }},
	)
	require.Equal(t, "degraded", rep.Status)
	require.Equal(t, "failed", rep.Checks[1].Status)
	require.Contains(t, rep.Checks[1].Error, "connection refused")
}

func TestSlowCheckTimesOutWithoutBlockingTheReport(t *testing.T) {
	rep := health.Run(context.Background(), 50*time.Millisecond,
		health.Check{Name: "slow", Fn: func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		}},
	)
	require.Equal(t, "degraded", rep.Status)
	require.Equal(t, "failed", rep.Checks[0].Status)
}

func TestResultsKeepCheckOrder(t *testing.T) {
	rep := health.Run(context.Background(), time.Second,
		health.Check{Name: "a", Fn: func(context.Context) error { return nil }},
		health.Check{Name: "b", Fn: func(context.Context) error { return nil }},
		health.Check{Name: "c", Fn: func(context.Context) error { return nil }},
	)
	require.Equal(t, []string{"a", "b", "c"},
		[]string{rep.Checks[0].Name, rep.Checks[1].Name, rep.Checks[2].Name})
}
