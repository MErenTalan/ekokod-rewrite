package gridbox_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/gridbox"
)

func TestMapStoredRowPrefersTheProviderMultipliedRegister(t *testing.T) {
	id := uuid.New()
	with := json.RawMessage(`{"ProfileDateTime":"2026-09-24T13:00:00+03:00","ActiveEndex":100,"ActiveEndexWithMultiplier":4000,"Multiplier":40}`)
	r, field, err := gridbox.MapStoredRow(with, id, model.ReadingKindLoadProfile, decimal.NewFromInt(40))
	require.NoError(t, err, field)
	require.True(t, decimal.NewFromInt(4000).Equal(*r.ActiveImport))
	require.Equal(t, model.IntegrationProviderGridbox, r.SourceProvider)

	rawOnly := json.RawMessage(`{"EndexDate":"2026-09-24T00:00:00+03:00","ActiveEndex":100}`)
	r, _, err = gridbox.MapStoredRow(rawOnly, id, model.ReadingKindDaily, decimal.NewFromInt(40))
	require.NoError(t, err)
	require.True(t, decimal.NewFromInt(4000).Equal(*r.ActiveImport), "raw × multiplier when no multiplied register exists")

	_, field, err = gridbox.MapStoredRow(json.RawMessage(`{"ActiveEndex":1}`), id, model.ReadingKindDaily, decimal.NewFromInt(1))
	require.Error(t, err)
	require.Equal(t, "timestamp", field)
}
