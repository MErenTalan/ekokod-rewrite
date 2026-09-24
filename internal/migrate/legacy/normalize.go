package legacy

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/shopspring/decimal"
)

var (
	// ErrBadDate is any date value the migration will not interpret.
	ErrBadDate = errors.New("bad_date")
	// ErrAmbiguousDate is a value with more than one plausible reading (R401).
	ErrAmbiguousDate = fmt.Errorf("%w: ambiguous_date", ErrBadDate)
	// ErrBadNumber is a numeric value that does not parse unambiguously (R402).
	ErrBadNumber = errors.New("bad_number")
	// ErrUnknownEnum is a legacy enum value with no mapping (R403).
	ErrUnknownEnum = errors.New("unknown_enum")
)

var istanbul = mustZone("Europe/Istanbul")

func mustZone(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}

var months = map[string]time.Month{
	"oca": 1, "şub": 2, "sub": 2, "mar": 3, "nis": 4, "may": 5, "haz": 6, "tem": 7, "ağu": 8, "agu": 8, "eyl": 9, "eki": 10, "kas": 11, "ara": 12,
	"jan": 1, "feb": 2, "apr": 4, "jun": 6, "jul": 7, "aug": 8, "sep": 9, "oct": 10, "nov": 11, "dec": 12,
}

var (
	reYMD      = regexp.MustCompile(`^(\d{4})-(\d{2})-(\d{2})$`)
	reYM       = regexp.MustCompile(`^(\d{4})-(\d{2})$`)
	reDMY      = regexp.MustCompile(`^(\d{1,2})[-./](\d{1,2})[-./](\d{4})$`)
	reDMY2     = regexp.MustCompile(`^\d{1,2}[-./]\d{1,2}[-./]\d{2}$`)
	reDMonY    = regexp.MustCompile(`^(\d{1,2})[- ]([\p{L}]{3})[a-zğüşıöç]*[- ](\d{4})$`)
	reYMDClock = regexp.MustCompile(`^(\d{4})-(\d{2})-(\d{2})[ T](\d{2}):(\d{2})(?::(\d{2}))?$`)
)

func civil(y, m, d, hh, mm, ss int) (time.Time, error) {
	t := time.Date(y, time.Month(m), d, hh, mm, ss, 0, istanbul)
	if t.Year() != y || int(t.Month()) != m || t.Day() != d || t.Hour() != hh || t.Minute() != mm {
		return time.Time{}, ErrBadDate
	}
	return t.UTC(), nil
}

func atoi(s string) int { n, _ := strconv.Atoi(s); return n }

// ParseDate is R401: every layout the legacy data uses, naive values in
// Europe/Istanbul, the result in UTC. Legacy never wrote month-first dates, so
// d/m/y separators read day-first; two-digit years are ambiguous and rejected.
func ParseDate(raw string) (time.Time, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return time.Time{}, fmt.Errorf("%w: empty", ErrBadDate)
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.UTC(), nil
	}
	if m := reYMDClock.FindStringSubmatch(s); m != nil {
		return civil(atoi(m[1]), atoi(m[2]), atoi(m[3]), atoi(m[4]), atoi(m[5]), atoi(m[6]))
	}
	if m := reYMD.FindStringSubmatch(s); m != nil {
		return civil(atoi(m[1]), atoi(m[2]), atoi(m[3]), 0, 0, 0)
	}
	if m := reYM.FindStringSubmatch(s); m != nil {
		return civil(atoi(m[1]), atoi(m[2]), 1, 0, 0, 0)
	}
	if m := reDMY.FindStringSubmatch(s); m != nil {
		return civil(atoi(m[3]), atoi(m[2]), atoi(m[1]), 0, 0, 0)
	}
	if reDMY2.MatchString(s) {
		return time.Time{}, fmt.Errorf("%w: %q has a two-digit year", ErrAmbiguousDate, s)
	}
	if m := reDMonY.FindStringSubmatch(strings.ToLowerSpecial(unicode.TurkishCase, s)); m != nil {
		mon, ok := months[m[2]]
		if !ok {
			return time.Time{}, fmt.Errorf("%w: month %q", ErrBadDate, m[2])
		}
		return civil(atoi(m[3]), int(mon), atoi(m[1]), 0, 0, 0)
	}
	return time.Time{}, fmt.Errorf("%w: %q", ErrBadDate, s)
}

var (
	reDigits   = regexp.MustCompile(`^-?\d+$`)
	reGrouped3 = regexp.MustCompile(`^-?\d{1,3}(X\d{3})+$`)
)

// ParseNumber is R402: decimal comma or point, thousands grouping only when it
// cannot be mistaken for a decimal part. Empty is nil.
func ParseNumber(raw string) (*decimal.Decimal, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return nil, nil //nolint:nilnil // an empty legacy value is a null, not an error
	}
	dots, commas := strings.Count(s, "."), strings.Count(s, ",")
	var plain string
	switch {
	case dots > 0 && commas > 0:
		dec, thou := ",", "."
		if strings.LastIndex(s, ".") > strings.LastIndex(s, ",") {
			dec, thou = ".", ","
		}
		if strings.Count(s, dec) != 1 {
			return nil, fmt.Errorf("%w: %q", ErrBadNumber, s)
		}
		whole, frac, _ := strings.Cut(s, dec)
		if !reGrouped3.MatchString(strings.ReplaceAll(whole, thou, "X")) || !reDigits.MatchString(frac) {
			return nil, fmt.Errorf("%w: %q", ErrBadNumber, s)
		}
		plain = strings.ReplaceAll(whole, thou, "") + "." + frac
	case dots+commas == 1:
		sep := "."
		if commas == 1 {
			sep = ","
		}
		whole, frac, _ := strings.Cut(s, sep)
		if len(frac) == 3 && len(strings.TrimPrefix(whole, "-")) <= 3 {
			return nil, fmt.Errorf("%w: %q could be a thousands separator", ErrBadNumber, s)
		}
		plain = whole + "." + frac
	case dots+commas > 1:
		sep := "."
		if commas > 0 {
			sep = ","
		}
		if !reGrouped3.MatchString(strings.ReplaceAll(s, sep, "X")) {
			return nil, fmt.Errorf("%w: %q", ErrBadNumber, s)
		}
		plain = strings.ReplaceAll(s, sep, "")
	default:
		plain = s
	}
	d, err := decimal.NewFromString(plain)
	if err != nil {
		return nil, fmt.Errorf("%w: %q", ErrBadNumber, s)
	}
	return &d, nil
}

// fold lower-cases in Turkish and drops diacritics, so "TİCARETHANE" and "ticarethane" meet.
func fold(s string) string {
	s = strings.ToLowerSpecial(unicode.TurkishCase, strings.TrimSpace(s))
	return strings.NewReplacer("ı", "i", "ç", "c", "ğ", "g", "ö", "o", "ş", "s", "ü", "u", "  ", " ").Replace(s)
}

func enum(kind, raw string, table map[string]string) (string, error) {
	if v, ok := table[fold(raw)]; ok {
		return v, nil
	}
	return "", fmt.Errorf("%w: %s %q", ErrUnknownEnum, kind, raw)
}

// SubscriberGroup is R403's subscriber group mapping.
func SubscriberGroup(raw string) (string, error) {
	return enum("subscriber group", raw, map[string]string{
		"mesken": "residential", "residential": "residential",
		"ticarethane": "commercial", "ticari": "commercial", "commercial": "commercial",
		"sanayi": "industrial", "industrial": "industrial",
		"tarim": "agricultural", "tarimsal": "agricultural", "tarimsal sulama": "agricultural", "agricultural": "agricultural",
		"aydinlatma": "lighting", "lighting": "lighting",
	})
}

// Voltage is R403's AG/OG mapping.
func Voltage(raw string) (string, error) {
	return enum("voltage", raw, map[string]string{"ag": "lv", "alcak gerilim": "lv", "lv": "lv", "og": "mv", "orta gerilim": "mv", "mv": "mv"})
}

// Term is R403's single/double term mapping.
func Term(raw string) (string, error) {
	return enum("term", raw, map[string]string{"tek terim": "monomial", "tek terimli": "monomial", "monomial": "monomial",
		"cift terim": "binomial", "cift terimli": "binomial", "binomial": "binomial"})
}

// MultiTime is R403's single/multi-time tariff mapping.
func MultiTime(raw string) (bool, error) {
	v, err := enum("tariff type", raw, map[string]string{"tek zamanli": "single", "single": "single", "cok zamanli": "multi", "multi": "multi"})
	return v == "multi", err
}
