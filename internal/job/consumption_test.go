package job

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/require"
)

// consumptionWindow is a one-hour window at 10:00-11:00 UTC, used by
// several tests below as the shared "same window" fixture.
var (
	consumptionFrom = time.Date(2026, 3, 14, 10, 0, 0, 0, time.UTC)
	consumptionTo   = consumptionFrom.Add(time.Hour)
)

// TestConsumptionRefreshTaskIDIsDeterministic proves ConsumptionRefreshTaskID
// is a pure, repeatable function of (From, To) alone.
func TestConsumptionRefreshTaskIDIsDeterministic(t *testing.T) {
	a := ConsumptionRefreshTaskID(consumptionFrom, consumptionTo)
	b := ConsumptionRefreshTaskID(consumptionFrom, consumptionTo)
	require.Equal(t, a, b)
	require.NotEmpty(t, a)
}

// TestConsumptionRefreshTaskIDIgnoresAnalyzerAndCompany proves
// ConsumptionRefreshTaskID — the exact formula NewConsumptionRefreshTask
// feeds asynq.TaskID — takes only (From, To): it has no CompanyID or
// AnalyzerID parameter at all, so two payloads differing only in one of
// those fields necessarily compute the identical id (R71/R72). The
// end-to-end proof that the ACTUAL enqueued task collides — a second
// asynq.ErrTaskIDConflict for a different analyzer's payload over the same
// window — lives in consumption_integration_test.go, since asynq.Task
// exposes no accessor for the id it was built with outside a real broker
// round trip.
func TestConsumptionRefreshTaskIDIgnoresAnalyzerAndCompany(t *testing.T) {
	company1, company2 := uuid.New(), uuid.New()
	analyzer1, analyzer2 := uuid.New(), uuid.New()

	base := ConsumptionRefreshPayload{CompanyID: company1, AnalyzerID: analyzer1, From: consumptionFrom, To: consumptionTo}
	differentAnalyzer := base
	differentAnalyzer.AnalyzerID = analyzer2
	differentCompany := base
	differentCompany.CompanyID = company2

	want := ConsumptionRefreshTaskID(base.From, base.To)
	require.Equal(t, want, ConsumptionRefreshTaskID(differentAnalyzer.From, differentAnalyzer.To))
	require.Equal(t, want, ConsumptionRefreshTaskID(differentCompany.From, differentCompany.To))

	// And every one of these must still build successfully.
	for _, p := range []ConsumptionRefreshPayload{base, differentAnalyzer, differentCompany} {
		_, err := NewConsumptionRefreshTask(p, TaskOptions{})
		require.NoError(t, err)
	}
}

// TestConsumptionRefreshTaskIDDiffersByWindow proves different windows
// produce different ids.
func TestConsumptionRefreshTaskIDDiffersByWindow(t *testing.T) {
	a := ConsumptionRefreshTaskID(consumptionFrom, consumptionTo)
	b := ConsumptionRefreshTaskID(consumptionFrom, consumptionTo.Add(time.Hour))
	require.NotEqual(t, a, b)

	c := ConsumptionRefreshTaskID(consumptionFrom.Add(-time.Hour), consumptionTo)
	require.NotEqual(t, a, c)
}

// TestConsumptionRefreshTaskIDCollapsesWithinTheSameHour proves From values
// 10:15 and 10:45 (both inside the 10:00-11:00 hour) map to the same id,
// given the same To.
func TestConsumptionRefreshTaskIDCollapsesWithinTheSameHour(t *testing.T) {
	to := time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC)
	a := ConsumptionRefreshTaskID(time.Date(2026, 3, 14, 10, 15, 0, 0, time.UTC), to)
	b := ConsumptionRefreshTaskID(time.Date(2026, 3, 14, 10, 45, 0, 0, time.UTC), to)
	require.Equal(t, a, b)
}

// TestConsumptionRefreshTaskIDCeilsExactHourToOnLiteral proves hourCeil does
// NOT over-ceil a To that already sits exactly on an hour boundary — the
// mutation that unconditionally does floor.Add(time.Hour) (dropping the
// floor.Equal(t.UTC()) guard) must turn case "to exactly on the hour" red
// here, by asserting the literal formatted id string rather than mere
// equality-with-itself.
func TestConsumptionRefreshTaskIDCeilsExactHourToOnLiteral(t *testing.T) {
	from := time.Date(2026, 3, 14, 10, 15, 0, 0, time.UTC)

	t.Run("to exactly on the hour is not ceiled further", func(t *testing.T) {
		to := time.Date(2026, 3, 14, 11, 0, 0, 0, time.UTC)
		got := ConsumptionRefreshTaskID(from, to)
		require.Equal(t, "consumption.refresh:2026-03-14T10:00:00Z/2026-03-14T11:00:00Z", got)
	})

	t.Run("to one nanosecond past the hour ceils to the next hour", func(t *testing.T) {
		to := time.Date(2026, 3, 14, 11, 0, 0, 1, time.UTC)
		got := ConsumptionRefreshTaskID(from, to)
		require.Equal(t, "consumption.refresh:2026-03-14T10:00:00Z/2026-03-14T12:00:00Z", got)
	})
}

// TestConsumptionRefreshPayloadRoundTrips proves encode/decode is
// byte-for-byte faithful.
func TestConsumptionRefreshPayloadRoundTrips(t *testing.T) {
	want := ConsumptionRefreshPayload{
		CompanyID:  uuid.New(),
		AnalyzerID: uuid.New(),
		From:       consumptionFrom,
		To:         consumptionTo,
	}
	task, err := NewConsumptionRefreshTask(want, TaskOptions{})
	require.NoError(t, err)
	got, err := DecodeConsumptionRefresh(task)
	require.NoError(t, err)
	require.Equal(t, want.CompanyID, got.CompanyID)
	require.Equal(t, want.AnalyzerID, got.AnalyzerID)
	require.True(t, want.From.Equal(got.From))
	require.True(t, want.To.Equal(got.To))
}

// TestNewConsumptionRefreshTaskRejectsInvalidPayloads proves every invalid
// shape is rejected before encoding, never silently accepted.
func TestNewConsumptionRefreshTaskRejectsInvalidPayloads(t *testing.T) {
	valid := ConsumptionRefreshPayload{CompanyID: uuid.New(), From: consumptionFrom, To: consumptionTo}

	t.Run("nil company", func(t *testing.T) {
		p := valid
		p.CompanyID = uuid.Nil
		_, err := NewConsumptionRefreshTask(p, TaskOptions{})
		require.Error(t, err)
	})

	t.Run("zero From", func(t *testing.T) {
		p := valid
		p.From = time.Time{}
		_, err := NewConsumptionRefreshTask(p, TaskOptions{})
		require.Error(t, err)
	})

	t.Run("zero To", func(t *testing.T) {
		p := valid
		p.To = time.Time{}
		_, err := NewConsumptionRefreshTask(p, TaskOptions{})
		require.Error(t, err)
	})

	t.Run("From equal To", func(t *testing.T) {
		p := valid
		p.To = p.From
		_, err := NewConsumptionRefreshTask(p, TaskOptions{})
		require.Error(t, err)
	})

	t.Run("From after To", func(t *testing.T) {
		p := valid
		p.From, p.To = p.To, p.From
		_, err := NewConsumptionRefreshTask(p, TaskOptions{})
		require.Error(t, err)
	})
}

// fakeConsumptionRefresher records every call it receives and returns
// whatever err was configured, so the routing test below can assert
// Register wired the decoded payload through to it.
type fakeConsumptionRefresher struct {
	calls []ConsumptionRefreshPayload
	err   error
}

func (f *fakeConsumptionRefresher) RefreshConsumption(_ context.Context, p ConsumptionRefreshPayload) error {
	f.calls = append(f.calls, p)
	return f.err
}

// TestRegisterRoutesConsumptionRefresh proves a task built by
// NewConsumptionRefreshTask and dispatched through Register's mux reaches
// Handlers.ConsumptionRefresh.RefreshConsumption with the payload decoded
// intact.
func TestRegisterRoutesConsumptionRefresh(t *testing.T) {
	fake := &fakeConsumptionRefresher{}
	mux := asynq.NewServeMux()
	Register(mux, &Handlers{Log: slog.New(slog.NewTextHandler(io.Discard, nil)), ConsumptionRefresh: fake})

	want := ConsumptionRefreshPayload{CompanyID: uuid.New(), AnalyzerID: uuid.New(), From: consumptionFrom, To: consumptionTo}
	task, err := NewConsumptionRefreshTask(want, TaskOptions{})
	require.NoError(t, err)

	handler, pattern := mux.Handler(task)
	require.Equal(t, TypeConsumptionRefresh, pattern)
	require.NoError(t, handler.ProcessTask(context.Background(), task))

	require.Len(t, fake.calls, 1)
	require.Equal(t, want.CompanyID, fake.calls[0].CompanyID)
	require.Equal(t, want.AnalyzerID, fake.calls[0].AnalyzerID)
	require.True(t, want.From.Equal(fake.calls[0].From))
	require.True(t, want.To.Equal(fake.calls[0].To))
}

// TestRegisterSkipsNilConsumptionRefresh proves a Handlers with no
// ConsumptionRefresh registers no route for consumption.refresh.
func TestRegisterSkipsNilConsumptionRefresh(t *testing.T) {
	mux := asynq.NewServeMux()
	Register(mux, &Handlers{Log: slog.New(slog.NewTextHandler(io.Discard, nil))})

	_, pattern := mux.Handler(asynq.NewTask(TypeConsumptionRefresh, nil))
	require.Empty(t, pattern, "no handler should be registered for %s", TypeConsumptionRefresh)
}

// TestConsumptionRefreshHandlerSkipsRetryOnDecodeFailure proves
// integHandleConsumptionRefresh wraps a payload decode failure with
// asynq.SkipRetry, exactly like every other integHandle* adapter.
func TestConsumptionRefreshHandlerSkipsRetryOnDecodeFailure(t *testing.T) {
	mux := asynq.NewServeMux()
	Register(mux, &Handlers{Log: slog.New(slog.NewTextHandler(io.Discard, nil)), ConsumptionRefresh: &fakeConsumptionRefresher{}})

	task := asynq.NewTask(TypeConsumptionRefresh, []byte("not valid json"))
	handler, pattern := mux.Handler(task)
	require.Equal(t, TypeConsumptionRefresh, pattern)

	err := handler.ProcessTask(context.Background(), task)
	require.Error(t, err)
	require.ErrorIs(t, err, asynq.SkipRetry)
}
