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
