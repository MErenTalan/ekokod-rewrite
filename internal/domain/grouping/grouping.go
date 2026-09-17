// Package grouping buckets daily consumption for the Consumption screen's
// "detailed graphs" (01 §7.3, R193). Day types come from the company calendar
// exactly as the load profile computes them (R137) and seasons are
// meteorological (R82), so the two screens never disagree.
package grouping

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/loadprofile"
)

// By is a grouping mode.
type By string

// The five grouping modes (R193).
const (
	Daily         By = "daily"
	Week          By = "week"
	DayType       By = "day_type"
	Season        By = "season"
	SeasonDayType By = "season_day_type"
)

// Range is an inclusive vacation period.
type Range struct{ Start, End time.Time }

// Day is one day's consumption. A nil register means "no value", never zero.
type Day struct {
	Date                          time.Time
	Active, Inductive, Capacitive *decimal.Decimal
	// Partial marks a day whose own figures were incomplete or suspect.
	Partial bool
}

// Bucket is one group.
type Bucket struct {
	Key                           string
	Days                          int
	Active, Inductive, Capacitive *decimal.Decimal
	Partial                       bool
}

// Extreme names the peak or valley group.
type Extreme struct {
	Key   string
	Value decimal.Decimal
}

// Statistics summarises the groups' active consumption.
type Statistics struct {
	Total, Average *decimal.Decimal
	Peak, Valley   *Extreme
}

// Valid reports whether by is a grouping mode.
func Valid(by By) bool {
	switch by {
	case Daily, Week, DayType, Season, SeasonDayType:
		return true
	default:
		return false
	}
}

// Group buckets days by mode. Buckets come back in key order for the calendar
// modes and weekday-before-weekend for the day-type ones, which is the order
// the charts draw.
func Group(days []Day, by By, cfg loadprofile.Config) []Bucket {
	index := map[string]*Bucket{}
	var order []string
	for _, day := range days {
		key := keyOf(day.Date, by, cfg)
		bucket, ok := index[key]
		if !ok {
			bucket = &Bucket{Key: key}
			index[key] = bucket
			order = append(order, key)
		}
		bucket.Days++
		bucket.Active = add(bucket.Active, day.Active)
		bucket.Inductive = add(bucket.Inductive, day.Inductive)
		bucket.Capacitive = add(bucket.Capacitive, day.Capacitive)
		if day.Partial || day.Active == nil || day.Inductive == nil || day.Capacitive == nil {
			bucket.Partial = true
		}
	}
	sort.Slice(order, func(i, j int) bool { return less(order[i], order[j], by) })
	out := make([]Bucket, 0, len(order))
	for _, key := range order {
		out = append(out, *index[key])
	}
	return out
}

// Stats is the "total, average, peak, valley" row under the chart (01 §7.3),
// over the groups' active consumption.
func Stats(buckets []Bucket) Statistics {
	var stats Statistics
	total := decimal.Zero
	counted := 0
	for _, b := range buckets {
		if b.Active == nil {
			continue
		}
		total = total.Add(*b.Active)
		counted++
		if stats.Peak == nil || b.Active.GreaterThan(stats.Peak.Value) {
			stats.Peak = &Extreme{Key: b.Key, Value: *b.Active}
		}
		if stats.Valley == nil || b.Active.LessThan(stats.Valley.Value) {
			stats.Valley = &Extreme{Key: b.Key, Value: *b.Active}
		}
	}
	if counted == 0 {
		return Statistics{}
	}
	average := total.Div(decimal.NewFromInt(int64(counted))).RoundBank(3)
	stats.Total, stats.Average = &total, &average
	return stats
}

func keyOf(day time.Time, by By, cfg loadprofile.Config) string {
	loc := cfg.Location
	if loc == nil {
		loc = time.UTC
	}
	local := day.In(loc)
	switch by {
	case Week:
		// Weeks start Monday (plan D19); the key is the Monday's date.
		offset := (int(local.Weekday()) + 6) % 7
		return local.AddDate(0, 0, -offset).Format(time.DateOnly)
	case DayType:
		return string(loadprofile.Classify(day, cfg))
	case Season:
		return seasonKey(local, cfg)
	case SeasonDayType:
		return seasonKey(local, cfg) + "-" + string(loadprofile.Classify(day, cfg))
	default:
		return local.Format(time.DateOnly)
	}
}

// seasonKey names the season and the year it started in, so a December and the
// January after it fall in the same winter (R193).
func seasonKey(local time.Time, cfg loadprofile.Config) string {
	season := loadprofile.SeasonOf(local, cfg.Location)
	year := local.Year()
	if season == loadprofile.Winter && local.Month() != time.December {
		year--
	}
	return fmt.Sprintf("%s-%d", season, year)
}

var dayTypeOrder = map[string]int{string(loadprofile.Weekday): 0, string(loadprofile.Weekend): 1}

var seasonOrder = map[loadprofile.Season]int{
	loadprofile.Winter: 0, loadprofile.Spring: 1, loadprofile.Summer: 2, loadprofile.Autumn: 3,
}

// less orders keys the way the chart draws them: dates and weeks
// chronologically, day types weekday-first, seasons by the season they name.
func less(a, b string, by By) bool {
	switch by {
	case DayType:
		return dayTypeOrder[a] < dayTypeOrder[b]
	case Season, SeasonDayType:
		return seasonLess(a, b)
	default:
		return a < b
	}
}

func seasonLess(a, b string) bool {
	seasonA, yearA, typeA := splitSeasonKey(a)
	seasonB, yearB, typeB := splitSeasonKey(b)
	switch {
	case yearA != yearB:
		return yearA < yearB
	case seasonA != seasonB:
		return seasonOrder[seasonA] < seasonOrder[seasonB]
	default:
		return dayTypeOrder[typeA] < dayTypeOrder[typeB]
	}
}

// splitSeasonKey reads "<season>-<year>[-<day type>]" back apart for ordering.
func splitSeasonKey(key string) (loadprofile.Season, int, string) {
	parts := strings.Split(key, "-")
	var season loadprofile.Season
	var year int
	var dayType string
	if len(parts) > 0 {
		season = loadprofile.Season(parts[0])
	}
	if len(parts) > 1 {
		year, _ = strconv.Atoi(parts[1])
	}
	if len(parts) > 2 {
		dayType = parts[2]
	}
	return season, year, dayType
}

func add(sum, value *decimal.Decimal) *decimal.Decimal {
	if value == nil {
		return sum
	}
	if sum == nil {
		v := *value
		return &v
	}
	v := sum.Add(*value)
	return &v
}
