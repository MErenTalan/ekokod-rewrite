package invoicepdf

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
)

func TestLabelsCoverEveryLineCode(t *testing.T) {
	codes := append(append([]string{}, model.BillLineCodes...), model.BillLineExtraPrefix, model.BillLineTaxPrefix)
	for _, locale := range []string{"tr", "en"} {
		for _, code := range codes {
			_, ok := labels[locale][code]
			require.True(t, ok, "%s label missing for %q", locale, code)
		}
		for key := range labels["tr"] {
			_, ok := labels[locale][key]
			require.True(t, ok, "%s lacks key %q that tr has", locale, key)
		}
	}
	require.Equal(t, "Vergi/fon: BTV", lineLabel("tr", "tax:BTV"))
}

func TestDayRangesCollapsesRuns(t *testing.T) {
	got := DayRanges([]string{"2026-09-01", "2026-09-02", "2026-09-03", "2026-09-05", "2026-09-30", "2026-10-01"})
	require.Equal(t, []string{"01.09.2026 – 03.09.2026", "05.09.2026", "30.09.2026 – 01.10.2026"}, got)
	require.Empty(t, DayRanges(nil))
}

// The index table names registers in the invoice's language, in a fixed
// order, and leaves out the ones the meter never reported.
func TestIndexRowsNameAndSkipRegisters(t *testing.T) {
	start := []byte(`{"active_import":"16783.576","reactive_inductive_import":"5958.197","t1_import":null,"active_export":"2375.891","t2_export":null}`)
	end := []byte(`{"active_import":"18630.836","reactive_inductive_import":"6784.622","t1_import":null,"active_export":"3066.135","custom_reg":"7"}`)

	rows := IndexRows("tr", start, end)
	require.Equal(t, [][3]string{
		{"Aktif çekiş (kWh)", "16.783,576", "18.630,836"},
		{"Endüktif reaktif (kVArh)", "5.958,197", "6.784,622"},
		{"Aktif veriş (kWh)", "2.375,891", "3.066,135"},
		{"custom_reg", "—", "7,000"},
	}, rows)
	require.Empty(t, IndexRows("tr", nil, nil))
	require.Equal(t, "Active import (kWh)", IndexRows("en", start, end)[0][0])
}

func TestPercent(t *testing.T) {
	ratio := decimal.RequireFromString("0.4474")
	require.Equal(t, "%44,74", Percent("tr", &ratio))
	limit := decimal.RequireFromString("0.2")
	require.Equal(t, "%20,00", Percent("tr", &limit))
	require.Equal(t, "—", Percent("tr", nil))
}
