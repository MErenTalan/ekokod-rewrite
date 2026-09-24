package osos_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/osos"
)

func TestMapStoredRowReusesTheIngestionMapping(t *testing.T) {
	id := uuid.New()
	raw := json.RawMessage(`{"meter_date":"24/09/2026 13:00:00","meter_serial_no":"S1","t_top_kWh":"1234,5","t_ri_kVarh":"10","t_rc_kVarh":"2","t_t1_kWh":"1","t_t2_kWh":"2","t_t3_kWh":"3","t_p_kW":"7","u_top_kWh":"0","u_ri_kVarh":"","u_rc_kVarh":"","u_u1_kWh":"","u_u2_kWh":"","u_u3_kWh":"","u_p_kW":""}`)
	r, field, ok := osos.MapStoredRow(raw, id, model.ReadingKindLoadProfile, decimal.NewFromInt(2))
	require.True(t, ok, field)
	require.Equal(t, id, r.AnalyzerID)
	require.Equal(t, time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC), r.Ts.UTC())
	require.True(t, decimal.RequireFromString("2469").Equal(*r.ActiveImport), "multiplied once")
	require.True(t, decimal.NewFromInt(2).Equal(r.MultiplierApplied))

	_, field, ok = osos.MapStoredRow(json.RawMessage(`{"meter_date":"2026-13-40"}`), id, model.ReadingKindLoadProfile, decimal.NewFromInt(1))
	require.False(t, ok)
	require.Equal(t, "meter_date", field)
}
