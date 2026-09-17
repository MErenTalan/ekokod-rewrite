package table_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"

	"github.com/MErenTalan/ekokod-rewrite/internal/render/table"
)

func TestCSVNeutralisesFormulasKeepsNumbers(t *testing.T) {
	out, err := table.CSV([]string{"a", "b"}, [][]string{{"=cmd|' /C calc'!A0", "-12.50"}, {"@SUM(A1)", "Işık Şube"}, {"+1", ""}})
	require.NoError(t, err)
	text := string(out)
	require.True(t, strings.HasPrefix(text, "\xef\xbb\xbfa,b\n"))
	require.Contains(t, text, `'=cmd|' /C calc'!A0`)
	require.Contains(t, text, ",-12.50\n", "a negative number is data, not a formula")
	require.Contains(t, text, "'@SUM(A1),Işık Şube")
	require.Contains(t, text, "+1,\n", "+1 is a number")
}

func TestXLSXWritesStrings(t *testing.T) {
	out, err := table.XLSX("Tüketim", []string{"period", "active_import"}, [][]string{{"2026-09-01", "123.456789"}, {"=HYPERLINK(\"x\")", "1"}})
	require.NoError(t, err)
	f, err := excelize.OpenReader(bytes.NewReader(out))
	require.NoError(t, err)
	v, err := f.GetCellValue("Tüketim", "B2")
	require.NoError(t, err)
	require.Equal(t, "123.456789", v, "decimals are written exactly")
	v, err = f.GetCellValue("Tüketim", "A3")
	require.NoError(t, err)
	require.Equal(t, `'=HYPERLINK("x")`, v)
}
