package tariff_test

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"

	tariffsvc "github.com/MErenTalan/ekokod-rewrite/internal/service/tariff"
)

// craftedWorkbook is a ~50 KB .xlsx whose A1 points at shared string -1 and whose
// shared-string table inflates past 16 MiB: the GO-2026-6452 panic in excelize
// (no fixed release), reachable through an icmal upload.
func craftedWorkbook(t *testing.T) []byte {
	t.Helper()
	f := excelize.NewFile()
	require.NoError(t, f.SetCellStr("Sheet1", "A1", "Tesisat No"))
	var buf bytes.Buffer
	require.NoError(t, f.Write(&buf))
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	require.NoError(t, err)
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	for _, file := range zr.File {
		rc, err := file.Open()
		require.NoError(t, err)
		body, err := io.ReadAll(rc)
		require.NoError(t, err)
		_ = rc.Close()
		if file.Name == "xl/sharedStrings.xml" {
			// Past excelize's 16 MiB in-memory limit the table is spilled to a temp
			// file, and that path indexes it without a bounds check.
			body = []byte(strings.Replace(string(body), "Tesisat No", strings.Repeat("A", 17<<20), 1))
		}
		if file.Name == "xl/worksheets/sheet1.xml" {
			s := string(body)
			i := strings.Index(s, `t="s"><v>`)
			require.Positive(t, i, "the fixture has a shared-string cell")
			j := i + len(`t="s"><v>`)
			k := strings.Index(s[j:], "</v>")
			body = []byte(s[:j] + "-1" + s[j+k:])
		}
		w, err := zw.CreateHeader(&zip.FileHeader{Name: file.Name, Method: zip.Deflate})
		require.NoError(t, err)
		_, err = w.Write(body)
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	return out.Bytes()
}

// R450: a crafted upload is refused, never a panic.
func TestACraftedWorkbookIsRefusedNotAPanic(t *testing.T) {
	var err error
	require.NotPanics(t, func() { _, err = tariffsvc.ReadSheet("icmal.xlsx", craftedWorkbook(t)) })
	require.ErrorIs(t, err, tariffsvc.ErrInvalidRequest)
	require.ErrorContains(t, err, "unreadable_workbook")
}
