package hourlyxlsx_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/render/hourlyxlsx"
)

func TestXLSXHasOneRowPerHourAndTotals(t *testing.T) {
	h0 := time.Date(2026, 1, 14, 21, 0, 0, 0, time.UTC)
	var rows []model.BillHourlyDetail
	for i := range 3 {
		rows = append(rows, model.BillHourlyDetail{Ts: h0.Add(time.Duration(i) * time.Hour), Consumption: decimal.NewFromInt(int64(10 + i)),
			PTF: decimal.RequireFromString("2000.1234"), Yekdem: decimal.NewFromInt(500), Kbk: decimal.RequireFromString("1.1"),
			UnitPrice: decimal.RequireFromString("2.750136"), Cost: decimal.RequireFromString("27.50136")})
	}
	out, err := hourlyxlsx.Render(model.Bill{PeriodKey: "2026-01"}, rows, "tr")
	require.NoError(t, err)
	f, err := excelize.OpenReader(bytes.NewReader(out))
	require.NoError(t, err)
	got, err := f.GetRows("2026-01")
	require.NoError(t, err)
	require.Len(t, got, 5, "header + 3 hours + totals")
	require.Equal(t, "Saat (İstanbul)", got[0][0])
	require.Equal(t, "2026-01-15 00:00", got[1][0], "Istanbul local hour")
	require.Equal(t, "2000.1234", got[1][2])
	require.Equal(t, "Toplam", got[4][0])
	require.Equal(t, "33", got[4][1])
	require.Equal(t, "82.5041", got[4][6])
}
