package ingest_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/ingest"
)

func TestDedupeKeepsIdenticalDuplicatesOnce(t *testing.T) {
	a := reading("2026-09-01T00:00:00Z", map[string]string{"active_import": "1000.0000"})
	b := reading("2026-09-01T00:00:00Z", map[string]string{"active_import": "1000.0000"})
	other := reading("2026-09-01T01:00:00Z", map[string]string{"active_import": "1010.0000"})

	kept, rejected := ingest.Dedupe([]model.MeterReading{a, b, other})

	require.Empty(t, rejected)
	require.Len(t, kept, 2)
	require.True(t, kept[0].Ts.Equal(a.Ts))
	require.True(t, kept[0].ActiveImport.Equal(*a.ActiveImport))
	require.True(t, kept[1].Ts.Equal(other.Ts))
}

func TestDedupeRejectsConflictingDuplicatesEntirely(t *testing.T) {
	a := reading("2026-09-01T00:00:00Z", map[string]string{"active_import": "1000.0000"})
	b := reading("2026-09-01T00:00:00Z", map[string]string{"active_import": "1001.0000"}) // same key, different value
	clean := reading("2026-09-01T01:00:00Z", map[string]string{"active_import": "1010.0000"})

	kept, rejected := ingest.Dedupe([]model.MeterReading{a, b, clean})

	require.Len(t, kept, 1, "only the unambiguous row survives")
	require.True(t, kept[0].Ts.Equal(clean.Ts))

	require.Len(t, rejected, 2, "both conflicting rows are rejected, not just one")
	for _, r := range rejected {
		require.Equal(t, ingest.RejectConflictingDuplicate, r.Reason)
		require.True(t, r.Ts.Equal(a.Ts))
	}
}

func TestDedupeIsPerAnalyzerAndKind(t *testing.T) {
	load := readingKind("2026-09-01T00:00:00Z", model.ReadingKindLoadProfile, map[string]string{"active_import": "1000.0000"})
	daily := readingKind("2026-09-01T00:00:00Z", model.ReadingKindDaily, map[string]string{"active_import": "2000.0000"})

	kept, rejected := ingest.Dedupe([]model.MeterReading{load, daily})

	require.Empty(t, rejected, "same Ts but different Kind is not the same key")
	require.Len(t, kept, 2)
}
