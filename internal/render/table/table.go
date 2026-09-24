// Package table renders a header plus string rows as CSV or XLSX for the
// analysis exports (05 §5). Cells stay strings end to end: no float path.
package table

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"regexp"

	"github.com/xuri/excelize/v2"
)

var numeric = regexp.MustCompile(`^-?[0-9]+(\.[0-9]+)?$`)

// safeCell neutralises spreadsheet formulas: text starting with = + - @ tab or
// CR gets a leading apostrophe; plain numbers (including negatives) are kept.
func safeCell(v string) string {
	if v == "" || numeric.MatchString(v) {
		return v
	}
	switch v[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + v
	}
	return v
}

// CSV renders columns and rows with a UTF-8 BOM so Excel opens Turkish text correctly.
func CSV(columns []string, rows [][]string) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString("\xef\xbb\xbf")
	w := csv.NewWriter(&buf)
	if err := w.Write(columns); err != nil {
		return nil, err
	}
	for _, row := range rows {
		safe := make([]string, len(row))
		for i, v := range row {
			safe[i] = safeCell(v)
		}
		if err := w.Write(safe); err != nil {
			return nil, err
		}
	}
	w.Flush()
	return buf.Bytes(), w.Error()
}

// XLSX renders one sheet.
func XLSX(sheet string, columns []string, rows [][]string) ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	if err := f.SetSheetName("Sheet1", sheet); err != nil {
		return nil, fmt.Errorf("table: %w", err)
	}
	sw, err := f.NewStreamWriter(sheet)
	if err != nil {
		return nil, fmt.Errorf("table: %w", err)
	}
	write := func(r int, values []string) error {
		cells := make([]any, len(values))
		for i, v := range values {
			cells[i] = safeCell(v)
		}
		cell, err := excelize.CoordinatesToCellName(1, r)
		if err != nil {
			return err
		}
		return sw.SetRow(cell, cells)
	}
	if err := write(1, columns); err != nil {
		return nil, fmt.Errorf("table: %w", err)
	}
	for i, row := range rows {
		if err := write(i+2, row); err != nil {
			return nil, fmt.Errorf("table: %w", err)
		}
	}
	if err := sw.Flush(); err != nil {
		return nil, fmt.Errorf("table: %w", err)
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, fmt.Errorf("table: %w", err)
	}
	return buf.Bytes(), nil
}
