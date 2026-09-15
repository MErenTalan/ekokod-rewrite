package normalize

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	_ "time/tzdata" // Europe/Istanbul must exist in a scratch image
)

// Istanbul is the Europe/Istanbul location. All offset-less provider
// timestamps are interpreted in this zone (removed-behaviour 20); loading
// it from time/tzdata (imported above) means it exists even in a scratch
// container with no system tzdata.
var Istanbul *time.Location

func init() {
	loc, err := time.LoadLocation("Europe/Istanbul")
	if err != nil {
		panic(fmt.Errorf("normalize: Europe/Istanbul unavailable: %w", err))
	}
	Istanbul = loc
}

// isoOffsetless are the layouts tried, in order, for an ISO 8601-shaped
// string that carries no explicit offset. The trailing nines make the
// fractional-second component optional.
var isoOffsetless = []string{
	"2006-01-02T15:04:05.999999999",
	"2006-01-02 15:04:05.999999999",
}

// LocalLayout parses raw using layout in Europe/Istanbul and returns the
// equivalent UTC instant.
func LocalLayout(layout, raw string) (time.Time, error) {
	t, err := time.ParseInLocation(layout, raw, Istanbul)
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}

// ISO8601 parses an ISO 8601 timestamp. When raw carries an explicit
// offset (or "Z") that offset is honoured. When it does not, the value is
// Europe/Istanbul local time, never UTC (removed-behaviour 20) — this is
// what lets historical data before Turkey's 2016 DST freeze convert
// correctly (UTC+2 in winter, UTC+3 in summer), which a fixed offset
// cannot do.
func ISO8601(raw string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return t.UTC(), nil
	}
	var lastErr error
	for _, layout := range isoOffsetless {
		t, err := time.ParseInLocation(layout, raw, Istanbul)
		if err == nil {
			return t.UTC(), nil
		}
		lastErr = err
	}
	return time.Time{}, fmt.Errorf("normalize: invalid ISO 8601 timestamp %q: %w", raw, lastErr)
}

const ososLayout = "02/01/2006 15:04:05"

// OSOSDate parses "DD/MM/YYYY HH:mm:ss" as Europe/Istanbul local time and
// returns the UTC instant.
func OSOSDate(raw string) (time.Time, error) {
	return LocalLayout(ososLayout, raw)
}

// FormatOSOSDate renders a UTC instant as Istanbul-local "DD/MM/YYYY
// HH:mm:ss", the inverse of OSOSDate.
func FormatOSOSDate(t time.Time) string {
	return t.In(Istanbul).Format(ososLayout)
}

const arilLayout = "20060102150405"

// ARILProfileDate parses the 14-digit yyyyMMddHHmmss number ARIL uses, as
// Europe/Istanbul local time.
func ARILProfileDate(n int64) (time.Time, error) {
	if n < 0 {
		return time.Time{}, fmt.Errorf("normalize: invalid ARIL profile date %d", n)
	}
	s := strconv.FormatInt(n, 10)
	if len(s) != 14 {
		return time.Time{}, fmt.Errorf("normalize: ARIL profile date %d must have 14 digits, got %d", n, len(s))
	}
	return LocalLayout(arilLayout, s)
}

var pm5340Layouts = []string{
	"02/01/2006 15:04:05",
	"02/01/2006 15:04",
}

// PM5340Date accepts ISO 8601 (offset or not) or "DD/MM/YYYY HH:mm[:ss]".
func PM5340Date(raw string) (time.Time, error) {
	if t, err := ISO8601(raw); err == nil {
		return t, nil
	}
	lastErr := errors.New("normalize: empty PM5340 date")
	for _, layout := range pm5340Layouts {
		t, err := time.ParseInLocation(layout, raw, Istanbul)
		if err == nil {
			return t.UTC(), nil
		}
		lastErr = err
	}
	return time.Time{}, fmt.Errorf("normalize: invalid PM5340 date %q: %w", raw, lastErr)
}
