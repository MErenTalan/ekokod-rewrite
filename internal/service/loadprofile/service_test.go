package loadprofile

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// TestDefaultWeekendDaysCopyIsNotAliased proves the mutation R69's ruling
// forbids: DefaultWeekendDays must not be mutated by a caller that receives
// a Config built from it. defaultWeekendDaysCopy is the ONLY place a domain
// Config's WeekendDays map is populated from the default, so pinning IT
// directly is what actually protects the package variable — mutating the
// derived []time.Weekday on a Result.Config would prove nothing, since a
// slice of weekdays cannot alias a map.
func TestDefaultWeekendDaysCopyIsNotAliased(t *testing.T) {
	original := map[time.Weekday]bool{time.Saturday: true, time.Sunday: true}
	require.Equal(t, original, DefaultWeekendDays, "precondition: the documented default is Saturday+Sunday")

	cp := defaultWeekendDaysCopy()
	cp[time.Monday] = true
	delete(cp, time.Saturday)

	require.Equal(t, original, DefaultWeekendDays, "mutating a copy must never change DefaultWeekendDays")
}

// TestAllProfileKeysAreExactlyTheTenProfileKeys pins the set Request.Keys is
// validated against: the two bare day types plus the eight season/day-type
// combinations, and nothing else.
func TestAllProfileKeysAreExactlyTheTenProfileKeys(t *testing.T) {
	want := map[string]bool{
		"weekday": true, "weekend": true,
		"winter_weekday": true, "winter_weekend": true,
		"spring_weekday": true, "spring_weekend": true,
		"summer_weekday": true, "summer_weekend": true,
		"autumn_weekday": true, "autumn_weekend": true,
	}
	require.Len(t, allProfileKeys, len(want))
	require.Len(t, validProfileKeys, len(want))
	for _, k := range allProfileKeys {
		require.True(t, want[string(k)], "unexpected profile key %q", k)
		require.True(t, validProfileKeys[k])
	}
}

// --- R99: span cap, validated before any I/O --------------------------------

// noCalendar panics on WeekendDays or Vacations — the only two
// CalendarRepository methods Profiles ever calls — so a validation check
// that (wrongly) runs after either call fails the test immediately, rather
// than quietly succeeding against the embedded nil interface's zero value.
// Every other CalendarRepository method is unreachable from this package;
// calling one of those instead panics on the nil embed, which still fails
// the test loudly.
type noCalendar struct {
	store.CalendarRepository
}

func (noCalendar) WeekendDays(context.Context, store.Scope) ([]model.CompanyWeekendDay, error) {
	panic("loadprofile: validation must run before any CalendarRepository.WeekendDays call")
}

func (noCalendar) Vacations(context.Context, store.Scope, *store.TimeRange) ([]model.CompanyVacation, error) {
	panic("loadprofile: validation must run before any CalendarRepository.Vacations call")
}

// noHourly panics on ConsumptionHourly, HourlySource's one method.
type noHourly struct{}

func (noHourly) ConsumptionHourly(context.Context, store.Scope, []uuid.UUID, store.TimeRange) ([]model.ConsumptionBucket, error) {
	panic("loadprofile: validation must run before any HourlySource.ConsumptionHourly call")
}

// fakeCalendar/fakeHourly are the positive-control counterparts: they
// return empty results rather than panicking, so a request that SHOULD be
// accepted can actually run to completion.
type fakeCalendar struct {
	store.CalendarRepository
}

func (fakeCalendar) WeekendDays(context.Context, store.Scope) ([]model.CompanyWeekendDay, error) {
	return nil, nil
}

func (fakeCalendar) Vacations(context.Context, store.Scope, *store.TimeRange) ([]model.CompanyVacation, error) {
	return nil, nil
}

type fakeHourly struct{}

func (fakeHourly) ConsumptionHourly(context.Context, store.Scope, []uuid.UUID, store.TimeRange) ([]model.ConsumptionBucket, error) {
	return nil, nil
}

func testIstanbul(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Istanbul")
	require.NoError(t, err)
	return loc
}

// TestProfilesValidatesBeforeAnyIO is R99's "before I/O" proof (final
// review A I-4): every one of these requests must be refused by
// ErrInvalidRequest/store.ErrInvalidRange without Profiles ever calling
// CalendarRepository or HourlySource — noCalendar/noHourly panic
// immediately if it does.
func TestProfilesValidatesBeforeAnyIO(t *testing.T) {
	svc, err := New(Deps{Calendar: noCalendar{}, Hourly: noHourly{}, Location: testIstanbul(t)})
	require.NoError(t, err)

	ctx := context.Background()
	validScope := store.Scope{CompanyID: uuid.New(), AllBuildings: true}
	from := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	validRange := store.TimeRange{From: from, To: from.Add(time.Hour)}
	oneID := []uuid.UUID{uuid.New()}

	cases := []struct {
		name  string
		scope store.Scope
		req   Request
	}{
		{"invalid scope", store.Scope{}, Request{AnalyzerIDs: oneID, Range: validRange}},
		{"invalid range", validScope, Request{AnalyzerIDs: oneID, Range: store.TimeRange{From: from, To: from}}},
		{"span over 400 days", validScope, Request{AnalyzerIDs: oneID, Range: store.TimeRange{From: from, To: from.Add(401 * 24 * time.Hour)}}},
		{"zero analyzer ids", validScope, Request{AnalyzerIDs: nil, Range: validRange}},
		{"two analyzer ids", validScope, Request{AnalyzerIDs: []uuid.UUID{uuid.New(), uuid.New()}, Range: validRange}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Profiles(ctx, tc.scope, tc.req)
			require.Error(t, err, "must be refused before any I/O: noCalendar/noHourly panic on any call")
		})
	}
}

// TestProfilesRejectsASpanOver400Days pins R99's exact boundary: 401 days is
// refused with loadprofile's own ErrInvalidRequest (not store.ErrInvalidRange
// — the range itself is well-formed, just too wide).
func TestProfilesRejectsASpanOver400Days(t *testing.T) {
	svc, err := New(Deps{Calendar: noCalendar{}, Hourly: noHourly{}, Location: testIstanbul(t)})
	require.NoError(t, err)

	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	_, err = svc.Profiles(context.Background(), store.Scope{CompanyID: uuid.New(), AllBuildings: true}, Request{
		AnalyzerIDs: []uuid.UUID{uuid.New()},
		Range:       store.TimeRange{From: from, To: from.Add(401 * 24 * time.Hour)},
	})
	require.ErrorIs(t, err, ErrInvalidRequest)
}

// TestProfilesAcceptsExactly400DaySpan is the positive control: exactly
// MaxRequestSpan must NOT be refused.
func TestProfilesAcceptsExactly400DaySpan(t *testing.T) {
	svc, err := New(Deps{Calendar: fakeCalendar{}, Hourly: fakeHourly{}, Location: testIstanbul(t)})
	require.NoError(t, err)

	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	_, err = svc.Profiles(context.Background(), store.Scope{CompanyID: uuid.New(), AllBuildings: true}, Request{
		AnalyzerIDs: []uuid.UUID{uuid.New()},
		Range:       store.TimeRange{From: from, To: from.Add(MaxRequestSpan)},
	})
	require.NoError(t, err)
}
