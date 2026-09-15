package ingest_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/ingest"
)

func TestDetectNegativeDeltas(t *testing.T) {
	prev := reading("2026-09-01T00:00:00Z", map[string]string{"active_import": "1000", "t1_import": "400"})
	rows := []model.MeterReading{
		reading("2026-09-01T01:00:00Z", map[string]string{"active_import": "1010", "t1_import": "390"}), // t1 decreases
		reading("2026-09-01T02:00:00Z", map[string]string{"active_import": "5", "t1_import": "391"}),    // active resets
	}
	got := ingest.DetectNegativeDeltas(&prev, rows, nil)
	require.ElementsMatch(t, []ingest.NegativeDelta{
		{Register: "t1_import", PrevTs: ts("2026-09-01T00:00:00Z"), CurTs: ts("2026-09-01T01:00:00Z")},
		{Register: "active_import", PrevTs: ts("2026-09-01T01:00:00Z"), CurTs: ts("2026-09-01T02:00:00Z")},
	}, got)

	// A reset reading at 01:30 suppresses the active_import delta only: it
	// falls inside (01:00, 02:00] — the pair the active_import delta came
	// from — but not inside (00:00, 01:00], the t1_import pair.
	resets := []model.MeterReading{resetAt("2026-09-01T01:30:00Z")}
	got2 := ingest.DetectNegativeDeltas(&prev, rows, resets)
	require.Len(t, got2, 1)
	require.Equal(t, "t1_import", got2[0].Register)
}

func TestDetectNegativeDeltasNilRegistersNeverProduceADelta(t *testing.T) {
	prev := reading("2026-09-01T00:00:00Z", map[string]string{"active_import": "1000"}) // t1_import nil
	cur := reading("2026-09-01T01:00:00Z", map[string]string{"t1_import": "5"})         // active_import nil here

	got := ingest.DetectNegativeDeltas(&prev, []model.MeterReading{cur}, nil)
	require.Empty(t, got, "a register missing on either side of the pair must never produce a delta")
}

func TestDetectNegativeDeltasIgnoresPairsOfDifferentKind(t *testing.T) {
	prev := reading("2026-09-01T00:00:00Z", map[string]string{"active_import": "1000"})
	dailySnapshot := readingKind("2026-09-01T01:00:00Z", model.ReadingKindDaily, map[string]string{"active_import": "5"})

	got := ingest.DetectNegativeDeltas(&prev, []model.MeterReading{dailySnapshot}, nil)
	require.Empty(t, got, "R14 compares consecutive readings of the SAME kind only")
}

func TestDetectNegativeDeltasWithNoPrevStartsFromTheFirstRow(t *testing.T) {
	rows := []model.MeterReading{
		reading("2026-09-01T00:00:00Z", map[string]string{"active_import": "1000"}),
		reading("2026-09-01T01:00:00Z", map[string]string{"active_import": "5"}),
	}
	got := ingest.DetectNegativeDeltas(nil, rows, nil)
	require.Equal(t, []ingest.NegativeDelta{
		{Register: "active_import", PrevTs: ts("2026-09-01T00:00:00Z"), CurTs: ts("2026-09-01T01:00:00Z")},
	}, got)
}
