package icmal

import (
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/shopspring/decimal"
)

// NumberFormat is a file's numeric convention (R129).
type NumberFormat int

// The number formats.
const (
	FormatTurkish NumberFormat = iota
	FormatUS
)

// ErrMixedNumberFormats is returned when one file uses both conventions.
var ErrMixedNumberFormats = errors.New("icmal: file mixes Turkish and US number formats")

// DetectNumberFormat scans every numeric cell: `1.234,56` is Turkish,
// `1,234.56` is US, an integer-only cell decides nothing and the default is
// Turkish (R129).
func DetectNumberFormat(t Table) (NumberFormat, error) {
	cols := columnIndex(t.Header)
	turkish, us := false, false
	for _, a := range aliases {
		i, ok := cols[a.field]
		if !a.numeric || !ok {
			continue
		}
		for _, row := range t.Rows {
			if i >= len(row) {
				continue
			}
			cell := trimCell(row[i])
			turkish = turkish || grouped(cell, '.', ',')
			us = us || grouped(cell, ',', '.')
		}
	}
	switch {
	case turkish && us:
		return 0, ErrMixedNumberFormats
	case us:
		return FormatUS, nil
	}
	return FormatTurkish, nil
}

// grouped matches ^-?\d{1,3}(<group>\d{3})*<dec>\d+$ without regexp.
func grouped(s string, group, dec rune) bool {
	s = strings.TrimPrefix(s, "-")
	intPart, frac, ok := strings.Cut(s, string(dec))
	if !ok || frac == "" || !digits(frac) {
		return false
	}
	parts := strings.Split(intPart, string(group))
	if len(parts[0]) < 1 || len(parts[0]) > 3 || !digits(parts[0]) {
		return false
	}
	for _, p := range parts[1:] {
		if len(p) != 3 || !digits(p) {
			return false
		}
	}
	return true
}

func digits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

func trimCell(s string) string {
	return strings.Trim(strings.TrimSpace(s), `"`)
}

// ParseNumber parses one cell; "", "-" and "BOŞ" are nil (not known).
func ParseNumber(s string, f NumberFormat) (*decimal.Decimal, error) {
	cell := trimCell(s)
	if cell == "" || cell == "-" || strings.ToUpperSpecial(unicode.TurkishCase, cell) == "BOŞ" {
		return nil, nil
	}
	group, dec := ".", ","
	if f == FormatUS {
		group, dec = ",", "."
	}
	neg := strings.HasPrefix(cell, "-")
	body := strings.ReplaceAll(strings.TrimPrefix(cell, "-"), group, "")
	intPart, frac, hasDec := strings.Cut(body, dec)
	if !digits(intPart) || (hasDec && !digits(frac)) {
		return nil, fmt.Errorf("icmal: not a number: %q", s)
	}
	text := intPart
	if hasDec {
		text += "." + frac
	}
	if neg {
		text = "-" + text
	}
	v, err := decimal.NewFromString(text)
	if err != nil {
		return nil, fmt.Errorf("icmal: not a number: %q", s)
	}
	return &v, nil
}
