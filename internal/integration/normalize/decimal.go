// Package normalize converts raw provider data — numbers, timestamps and
// units — into the canonical types the rest of the system uses
// (decimal.Decimal, UTC time.Time). It has no dependency on any other F2
// task; it consumes only github.com/shopspring/decimal and the standard
// library.
package normalize

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"

	"github.com/shopspring/decimal"
)

// ErrEmpty is returned when a numeric string is empty (after trimming).
var ErrEmpty = errors.New("normalize: empty value")

// numberPattern matches an optionally-signed decimal number after locale
// normalisation: an integer part and an optional ".fraction" part.
var numberPattern = regexp.MustCompile(`^-?\d+(\.\d+)?$`)

// ProviderNumber parses a provider numeric string per R10 (06 §2 "numbers …
// sometimes with thousands separators"). It never goes through float64:
// after locale normalisation the string is handed to
// decimal.NewFromString, which parses digit-by-digit.
//
// Locale rules (R10):
//   - If both ',' and '.' are present, the right-most occurrence is the
//     decimal separator; the other symbol (every occurrence) is a
//     thousands separator and is stripped.
//   - If only one of ',' or '.' is present: a single occurrence is the
//     decimal separator; more than one occurrence means the symbol is a
//     thousands separator and every occurrence is stripped (no decimal
//     part).
//   - A space or apostrophe is always a thousands separator.
func ProviderNumber(raw string) (decimal.Decimal, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return decimal.Decimal{}, ErrEmpty
	}

	negative := false
	switch {
	case strings.HasPrefix(s, "-"):
		negative = true
		s = s[1:]
	case strings.HasPrefix(s, "+"):
		s = s[1:]
	}

	// Spaces and apostrophes are always thousands separators.
	s = strings.NewReplacer(" ", "", " ", "", "'", "").Replace(s)

	commaIdx := strings.LastIndexByte(s, ',')
	dotIdx := strings.LastIndexByte(s, '.')

	var normalised string
	switch {
	case commaIdx >= 0 && dotIdx >= 0:
		// Both present: the right-most symbol is the decimal separator.
		var decSep byte
		if commaIdx > dotIdx {
			decSep = ','
		} else {
			decSep = '.'
		}
		var b strings.Builder
		lastDecIdx := strings.LastIndexByte(s, decSep)
		for i := 0; i < len(s); i++ {
			c := s[i]
			if c == ',' || c == '.' {
				if i == lastDecIdx {
					b.WriteByte('.')
				}
				// any other occurrence (of either symbol) is a
				// thousands separator: dropped.
				continue
			}
			b.WriteByte(c)
		}
		normalised = b.String()

	case commaIdx >= 0: // only comma present
		if strings.Count(s, ",") == 1 {
			normalised = strings.Replace(s, ",", ".", 1)
		} else {
			normalised = strings.ReplaceAll(s, ",", "")
		}

	case dotIdx >= 0: // only dot present
		if strings.Count(s, ".") == 1 {
			normalised = s
		} else {
			normalised = strings.ReplaceAll(s, ".", "")
		}

	default:
		normalised = s
	}

	if !numberPattern.MatchString(normalised) {
		return decimal.Decimal{}, errors.New("normalize: invalid number " + raw)
	}
	if negative {
		normalised = "-" + normalised
	}

	return decimal.NewFromString(normalised)
}

// OptionalNumber parses a provider numeric string that may signal "no
// value": "", "null" and "-" all mean the register is unavailable and
// return (nil, nil) — never a zero decimal (removed-behaviour 21).
func OptionalNumber(raw string) (*decimal.Decimal, error) {
	s := strings.TrimSpace(raw)
	if s == "" || s == "null" || s == "-" {
		return nil, nil
	}
	v, err := ProviderNumber(s)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// JSONNumber converts a json.Number — produced by a decoder configured with
// UseNumber() — to a decimal.Decimal exactly. json.Number already holds the
// literal digits from the JSON text, so this never touches float64.
func JSONNumber(n json.Number) (decimal.Decimal, error) {
	s := strings.TrimSpace(n.String())
	if s == "" {
		return decimal.Decimal{}, ErrEmpty
	}
	return decimal.NewFromString(s)
}

// OptionalJSONNumber converts a possibly-nil json.Number, preserving nil
// (removed-behaviour 21: unavailable registers are nil, never zero).
func OptionalJSONNumber(n *json.Number) (*decimal.Decimal, error) {
	if n == nil {
		return nil, nil
	}
	v, err := JSONNumber(*n)
	if err != nil {
		return nil, err
	}
	return &v, nil
}
