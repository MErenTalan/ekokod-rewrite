package energy_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
)

// I-17: SelectBoundary is "the last reading with ts <= bound", over a slice
// already sorted ascending and already filtered to the kind(s) the caller
// wants. These tests probe the three offsets around a shared bound plus the
// two empty-result shapes.
func TestSelectBoundaryOneNanosecondBeforeTheBound(t *testing.T) {
	readings := []energy.Reading{
		*readingAt(t0, "10"),
		*readingAt(t0.Add(time.Hour), "20"),
	}
	got := energy.SelectBoundary(readings, t0.Add(time.Hour).Add(-time.Nanosecond))
	require.NotNil(t, got)
	require.Equal(t, "10", got.Value(energy.ActiveImport).String())
}

func TestSelectBoundaryExactlyAtTheBound(t *testing.T) {
	readings := []energy.Reading{
		*readingAt(t0, "10"),
		*readingAt(t0.Add(time.Hour), "20"),
	}
	got := energy.SelectBoundary(readings, t0.Add(time.Hour))
	require.NotNil(t, got)
	require.Equal(t, "20", got.Value(energy.ActiveImport).String(), "the reading AT the bound must be selected, not skipped")
}

func TestSelectBoundaryOneNanosecondAfterTheBound(t *testing.T) {
	readings := []energy.Reading{
		*readingAt(t0, "10"),
		*readingAt(t0.Add(time.Hour), "20"),
		*readingAt(t0.Add(2*time.Hour), "30"),
	}
	got := energy.SelectBoundary(readings, t0.Add(time.Hour).Add(time.Nanosecond))
	require.NotNil(t, got)
	require.Equal(t, "20", got.Value(energy.ActiveImport).String(), "the next reading must not be pulled in one ns early")
}

func TestSelectBoundaryOnEmptyInput(t *testing.T) {
	require.Nil(t, energy.SelectBoundary(nil, t0))
}

func TestSelectBoundaryWhenEveryReadingIsAfterTheBound(t *testing.T) {
	readings := []energy.Reading{*readingAt(t0.Add(time.Hour), "10"), *readingAt(t0.Add(2*time.Hour), "20")}
	require.Nil(t, energy.SelectBoundary(readings, t0))
}

func TestSelectBoundaryDoesNotMutateOrReorderInput(t *testing.T) {
	readings := []energy.Reading{*readingAt(t0, "10"), *readingAt(t0.Add(time.Hour), "20"), *readingAt(t0.Add(2*time.Hour), "30")}
	cp := append([]energy.Reading(nil), readings...)

	energy.SelectBoundary(readings, t0.Add(time.Hour))

	require.Equal(t, cp, readings)
}
