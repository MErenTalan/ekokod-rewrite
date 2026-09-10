package id_test

import (
	"testing"

	"github.com/MErenTalan/ekokod-rewrite/internal/platform/id"
	"github.com/stretchr/testify/require"
)

func TestNewIsUniqueParsableAndTimeOrdered(t *testing.T) {
	seen := make(map[string]struct{}, 1000)
	previous := ""
	for i := 0; i < 1000; i++ {
		got := id.New()
		_, err := id.Parse(got)
		require.NoError(t, err)
		require.NotContains(t, seen, got)
		seen[got] = struct{}{}
		require.Greater(t, got, previous, "UUIDv7 must sort by creation time")
		previous = got
	}
}
