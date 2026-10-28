package demo_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/demo"
)

type loader struct {
	calls int
	demo  map[uuid.UUID]bool
	err   error
}

func (l *loader) load(_ context.Context, id uuid.UUID) (bool, error) {
	l.calls++
	return l.demo[id], l.err
}

func TestCheckerCachesAndExpires(t *testing.T) {
	id := uuid.New()
	l := &loader{demo: map[uuid.UUID]bool{id: true}}
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	c := demo.NewChecker(l.load, time.Minute, func() time.Time { return now })

	for range 2 {
		got, err := c.IsDemo(t.Context(), id)
		require.NoError(t, err)
		require.True(t, got)
	}
	require.Equal(t, 1, l.calls)

	now = now.Add(time.Minute + time.Second)
	_, err := c.IsDemo(t.Context(), id)
	require.NoError(t, err)
	require.Equal(t, 2, l.calls)
}

func TestCheckerDoesNotCacheErrors(t *testing.T) {
	id := uuid.New()
	l := &loader{err: errors.New("db down")}
	c := demo.NewChecker(l.load, time.Minute, time.Now)

	_, err := c.IsDemo(t.Context(), id)
	require.Error(t, err)
	l.err = nil
	got, err := c.IsDemo(t.Context(), id)
	require.NoError(t, err)
	require.False(t, got)
	require.Equal(t, 2, l.calls)
}

func TestCheckerUnknownCompanyIsNotDemo(t *testing.T) {
	c := demo.NewChecker(demo.FromCompanies(notFound{}), time.Minute, time.Now)
	got, err := c.IsDemo(t.Context(), uuid.New())
	require.NoError(t, err)
	require.False(t, got)
}
