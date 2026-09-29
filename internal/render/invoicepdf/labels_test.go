package invoicepdf

import (
	"testing"

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
