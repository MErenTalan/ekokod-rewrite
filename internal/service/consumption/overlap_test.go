package consumption

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/energy"
)

func TestHourlyOverlapSumsSoundHoursOnly(t *testing.T) {
	t.Parallel()
	d := func(s string) *decimal.Decimal { v := decimal.RequireFromString(s); return &v }
	at := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	row := func(imp, exp *decimal.Decimal) Row {
		return Row{Values: map[energy.Register]*decimal.Decimal{energy.ActiveImport: imp, energy.ActiveExport: exp}}
	}
	absorbed := row(d("5"), d("5"))
	absorbed.SpanFrom, absorbed.SpanTo = at, at.Add(3*time.Hour)
	partial := row(d("5"), d("5"))
	partial.Partial = true
	suspect := row(d("5"), d("5"))
	suspect.Suspect = map[energy.Register]energy.Suspicion{energy.ActiveImport: {}}
	got := HourlyOverlap([]Row{row(d("2"), d("0.5")), row(d("0"), d("3")), row(d("1.5"), d("4")), row(d("1"), nil), absorbed, partial, suspect})
	require.Equal(t, "2", got.String(), "0.5 + 0 + 1.5; nil, absorbed, partial and suspect hours skipped")
	require.Nil(t, HourlyOverlap([]Row{row(d("1"), nil)}))
}
