package ingest

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/job"
)

func TestRefreshModeTextIsNotTheWireValue(t *testing.T) {
	t.Parallel()
	require.Equal(t, "saatlik değer", refreshModeText(job.RefreshModeHourly))
	require.Equal(t, "enerji (endeks) değeri", refreshModeText(job.RefreshModeEnergy))
}
