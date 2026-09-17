package tariff

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/xuri/excelize/v2"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/tariff/icmal"
)

// ReadSheet reads an icmal upload: .csv (',' or ';', R129) or the first
// sheet of an .xlsx. The header is the first row naming ≥ 3 known columns.
func ReadSheet(fileName string, content []byte) (icmal.Table, error) {
	var rows [][]string
	switch strings.ToLower(filepath.Ext(fileName)) {
	case ".csv":
		content = bytes.TrimPrefix(content, []byte("\xef\xbb\xbf"))
		r := csv.NewReader(bytes.NewReader(content))
		r.Comma = delimiter(content)
		r.FieldsPerRecord = -1
		r.LazyQuotes = true
		var err error
		if rows, err = r.ReadAll(); err != nil {
			return icmal.Table{}, fmt.Errorf("%w: csv: %w", ErrInvalidRequest, err)
		}
	case ".xlsx":
		f, err := excelize.OpenReader(bytes.NewReader(content))
		if err != nil {
			return icmal.Table{}, fmt.Errorf("%w: xlsx: %w", ErrInvalidRequest, err)
		}
		defer func() { _ = f.Close() }()
		sheets := f.GetSheetList()
		if len(sheets) == 0 {
			return icmal.Table{}, fmt.Errorf("%w: xlsx has no sheet", ErrInvalidRequest)
		}
		if rows, err = f.GetRows(sheets[0]); err != nil {
			return icmal.Table{}, fmt.Errorf("%w: xlsx: %w", ErrInvalidRequest, err)
		}
	default:
		return icmal.Table{}, fmt.Errorf("%w: unsupported file type %q", ErrInvalidRequest, fileName)
	}
	for i, row := range rows {
		if icmal.HeaderScore(row) >= 3 {
			return icmal.Table{Header: row, Rows: rows[i+1:]}, nil
		}
	}
	return icmal.Table{}, fmt.Errorf("%w: no header row with known icmal columns", ErrInvalidRequest)
}

// delimiter picks ',' or ';' by which appears more in the first line outside quotes.
func delimiter(content []byte) rune {
	line, _, _ := bytes.Cut(content, []byte("\n"))
	commas, semis, quoted := 0, 0, false
	for _, c := range string(line) {
		switch {
		case c == '"':
			quoted = !quoted
		case quoted:
		case c == ',':
			commas++
		case c == ';':
			semis++
		}
	}
	if semis > commas {
		return ';'
	}
	return ','
}
