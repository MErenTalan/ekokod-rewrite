package loadprofile

import (
	"context"
	"errors"
	"log/slog"
	"sort"
	"time"

	"github.com/google/uuid"

	domainlp "github.com/MErenTalan/ekokod-rewrite/internal/domain/loadprofile"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// ErrInvalidRequest is returned before any I/O when a Request cannot be
// served: not exactly one AnalyzerIDs entry, a Keys entry that is not one of
// the ten profile keys internal/domain/loadprofile produces, or a Range
// wider than MaxRequestSpan (R99).
var ErrInvalidRequest = errors.New("loadprofile: invalid request")

// MaxRequestSpan bounds Request.Range's raw wall-clock span (R99, final
// review A I-4: service.go:157-165 had no range cap at all, so a Profiles
// call could load an unbounded number of consumption_hourly buckets). A
// Range wider than this fails ErrInvalidRequest before any I/O, mirroring
// internal/service/consumption's own MaxRequestSpan.
const MaxRequestSpan = 400 * 24 * time.Hour

// DefaultWeekendDays is applied when a company has configured no
// company_weekend_days rows (R69; 01-project-context.md's stated default).
//
// Callers never receive this map itself: resolveWeekendDays always hands the
// domain Config a fresh copy (defaultWeekendDaysCopy), so nothing reachable
// through a Result can mutate this package variable.
var DefaultWeekendDays = map[time.Weekday]bool{time.Saturday: true, time.Sunday: true}

// HourlySource is the narrow interface over store.AnalyticsRepository this
// package needs (R87): raw consumption_hourly buckets, so this package can
// read each bucket's OWN ActiveImportStart rather than internal/service/
// consumption's Row.Values, which holds the bucket's active_consumption
// (last-first inside the bucket) — structurally ZERO for a meter that
// reports once per hour and the wrong figure even for a 15-minute meter.
type HourlySource interface {
	ConsumptionHourly(ctx context.Context, sc store.Scope, analyzerIDs []uuid.UUID, r store.TimeRange) ([]model.ConsumptionBucket, error)
}

// Deps are Service's dependencies.
type Deps struct {
	Calendar store.CalendarRepository
	Hourly   HourlySource
	Location *time.Location
	Log      *slog.Logger
}

// Request is one load-profile request. AnalyzerIDs must hold exactly one id
// (05-api-contract.md §5's /load-profile takes a single analyzer_id); an
// empty or non-empty-but-not-singular slice is ErrInvalidRequest. Keys is
// the subset of the ten profile keys to compute; empty means every profile.
type Request struct {
	AnalyzerIDs []uuid.UUID
	Range       store.TimeRange
	Keys        []domainlp.Key
}

// ConfigUsed is what Profiles resolved from the company's calendar, so a
// caller can show it alongside the result.
type ConfigUsed struct {
	WeekendDays   []time.Weekday
	WeekendSource string // "company" or "default"
	Vacations     int
}

// Result is Profiles' return value. When req.Keys is non-empty, Profiles and
// Statistics contain exactly those keys — a requested key with no data is
// present with an empty Profile and the zero-value Statistics (every field
// nil, exactly what internal/domain/loadprofile.Stats returns for a profile
// with no hours). When req.Keys is empty, every one of the ten profile keys
// is present, on the same terms.
type Result struct {
	Profiles   map[domainlp.Key]domainlp.Profile
	Statistics map[domainlp.Key]domainlp.Statistics
	Config     ConfigUsed
}

// Service resolves a company's calendar and consumption_hourly buckets into
// load profiles.
type Service struct {
	deps Deps
}

// New builds a Service. It fails if a required dependency is missing.
func New(d Deps) (*Service, error) {
	if d.Calendar == nil {
		return nil, errors.New("loadprofile: Deps.Calendar is required")
	}
	if d.Hourly == nil {
		return nil, errors.New("loadprofile: Deps.Hourly is required")
	}
	if d.Location == nil {
		return nil, errors.New("loadprofile: Deps.Location is required")
	}
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	return &Service{deps: d}, nil
}

// allProfileKeys is the ten profile keys, in a deterministic order, derived
// from the domain package's own constants rather than duplicated as string
// literals here.
var allProfileKeys = buildAllProfileKeys()

// validProfileKeys is allProfileKeys as a set, for O(1) Request.Keys
// validation.
var validProfileKeys = buildValidProfileKeys()

func buildAllProfileKeys() []domainlp.Key {
	dayTypes := []domainlp.DayType{domainlp.Weekday, domainlp.Weekend}
	seasons := []domainlp.Season{domainlp.Winter, domainlp.Spring, domainlp.Summer, domainlp.Autumn}

	seen := map[domainlp.Key]bool{}
	var keys []domainlp.Key
	add := func(k domainlp.Key) {
		if !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}
	for _, dt := range dayTypes {
		add(domainlp.Key(dt))
	}
	for _, s := range seasons {
		for _, dt := range dayTypes {
			for _, k := range domainlp.KeysFor(dt, s) {
				add(k)
			}
		}
	}
	return keys
}

func buildValidProfileKeys() map[domainlp.Key]bool {
	m := make(map[domainlp.Key]bool, len(allProfileKeys))
	for _, k := range allProfileKeys {
		m[k] = true
	}
	return m
}

// defaultWeekendDaysCopy returns a fresh copy of DefaultWeekendDays, so a
// caller that mutates the map it feeds into a domain Config can never reach
// the package variable itself.
func defaultWeekendDaysCopy() map[time.Weekday]bool {
	out := make(map[time.Weekday]bool, len(DefaultWeekendDays))
	for k, v := range DefaultWeekendDays {
		out[k] = v
	}
	return out
}

// Profiles resolves sc's company calendar and req's analyzer's hourly
// consumption into 24-hour load profiles and their statistics.
func (s *Service) Profiles(ctx context.Context, sc store.Scope, req Request) (Result, error) {
	if !sc.Valid() {
		return Result{}, store.ErrInvalidScope
	}
	if !req.Range.Valid() {
		return Result{}, store.ErrInvalidRange
	}
	if req.Range.To.Sub(req.Range.From) > MaxRequestSpan {
		return Result{}, ErrInvalidRequest
	}
	if len(req.AnalyzerIDs) != 1 {
		return Result{}, ErrInvalidRequest
	}

	keys := req.Keys
	if len(keys) == 0 {
		keys = allProfileKeys
	} else {
		for _, k := range keys {
			if !validProfileKeys[k] {
				return Result{}, ErrInvalidRequest
			}
		}
	}

	weekendDays, weekendSource, err := s.resolveWeekendDays(ctx, sc)
	if err != nil {
		return Result{}, err
	}

	vacations, err := s.resolveVacations(ctx, sc, req.Range)
	if err != nil {
		return Result{}, err
	}

	cfg := domainlp.Config{
		WeekendDays: weekendDays,
		Vacations:   vacations,
		Location:    s.deps.Location,
	}

	values, err := s.hourValues(ctx, sc, req.AnalyzerIDs[0], req.Range)
	if err != nil {
		return Result{}, err
	}

	built := domainlp.Build(values, cfg)

	profiles := make(map[domainlp.Key]domainlp.Profile, len(keys))
	statistics := make(map[domainlp.Key]domainlp.Statistics, len(keys))
	for _, k := range keys {
		p, ok := built[k]
		if !ok {
			p = domainlp.Profile{Key: k}
		}
		profiles[k] = p
		statistics[k] = domainlp.Stats(p)
	}

	weekendList := make([]time.Weekday, 0, len(weekendDays))
	for d := time.Sunday; d <= time.Saturday; d++ {
		if weekendDays[d] {
			weekendList = append(weekendList, d)
		}
	}

	return Result{
		Profiles:   profiles,
		Statistics: statistics,
		Config: ConfigUsed{
			WeekendDays:   weekendList,
			WeekendSource: weekendSource,
			Vacations:     len(vacations),
		},
	}, nil
}

// resolveWeekendDays reads sc's company weekend days (R69), falling back to
// DefaultWeekendDays — a fresh copy, never the package variable itself —
// when the company configured none.
func (s *Service) resolveWeekendDays(ctx context.Context, sc store.Scope) (map[time.Weekday]bool, string, error) {
	rows, err := s.deps.Calendar.WeekendDays(ctx, sc)
	if err != nil {
		return nil, "", err
	}
	if len(rows) == 0 {
		return defaultWeekendDaysCopy(), "default", nil
	}
	days := make(map[time.Weekday]bool, len(rows))
	for _, r := range rows {
		days[time.Weekday(r.DayOfWeek)] = true
	}
	return days, "company", nil
}

// resolveVacations reads sc's company vacations (R70: never calendar_events)
// over r widened to whole Istanbul days.
func (s *Service) resolveVacations(ctx context.Context, sc store.Scope, r store.TimeRange) ([]domainlp.DateRange, error) {
	widened := istanbulWholeDayRange(r, s.deps.Location)
	rows, err := s.deps.Calendar.Vacations(ctx, sc, &widened)
	if err != nil {
		return nil, err
	}
	out := make([]domainlp.DateRange, 0, len(rows))
	for _, v := range rows {
		out = append(out, domainlp.DateRange{Start: v.StartDate, End: v.EndDate})
	}
	return out, nil
}

// istanbulWholeDayRange widens r to [startOfDay(r.From), startOfDay(r.To -
// 1ns) + 24h) in loc, so a vacation query never misses a day r's instants
// only partially cover.
func istanbulWholeDayRange(r store.TimeRange, loc *time.Location) store.TimeRange {
	from := istanbulStartOfDay(r.From, loc)
	lastInstant := r.To.Add(-time.Nanosecond)
	to := istanbulStartOfDay(lastInstant, loc).Add(24 * time.Hour)
	return store.TimeRange{From: from, To: to}
}

func istanbulStartOfDay(t time.Time, loc *time.Location) time.Time {
	y, m, d := t.In(loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, loc)
}

// hourValues fetches consumption_hourly buckets for r plus one trailing hour
// (R87) and differences each pair of consecutive buckets: hour h's value is
// ActiveImportStart(h+1) - ActiveImportStart(h). Day/Hour are taken from the
// bucket's own START instant, in s.deps.Location. A missing successor
// bucket, a missing ActiveImportStart on either side, or a negative
// difference contributes nothing — never a zero hour.
func (s *Service) hourValues(ctx context.Context, sc store.Scope, analyzerID uuid.UUID, r store.TimeRange) ([]domainlp.HourValue, error) {
	fetchRange := store.TimeRange{From: r.From, To: r.To.Add(time.Hour)}
	buckets, err := s.deps.Hourly.ConsumptionHourly(ctx, sc, []uuid.UUID{analyzerID}, fetchRange)
	if err != nil {
		return nil, err
	}

	byBucket := make(map[time.Time]model.ConsumptionBucket, len(buckets))
	for _, b := range buckets {
		byBucket[b.Bucket] = b
	}

	sort.Slice(buckets, func(i, j int) bool { return buckets[i].Bucket.Before(buckets[j].Bucket) })

	var values []domainlp.HourValue
	for _, cur := range buckets {
		if !cur.Bucket.Before(r.To) {
			// This is the trailing successor-only bucket (or beyond it):
			// it closes the previous hour's difference but is never itself
			// a requested hour.
			continue
		}
		if cur.ActiveImportStart == nil {
			continue
		}
		next, ok := byBucket[cur.Bucket.Add(time.Hour)]
		if !ok || next.ActiveImportStart == nil {
			continue
		}
		diff := next.ActiveImportStart.Sub(*cur.ActiveImportStart)
		if diff.IsNegative() {
			continue
		}
		values = append(values, domainlp.HourValue{
			Day:   cur.Bucket,
			Hour:  cur.Bucket.In(s.deps.Location).Hour(),
			Value: diff,
		})
	}
	return values, nil
}
