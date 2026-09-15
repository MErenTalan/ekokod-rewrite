package job

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/require"
)

// integTestLogger is a Handlers.Log that never touches stdout/stderr: every
// test in this file that builds a Handlers needs SOMETHING non-nil there,
// since Register unconditionally wires h.Noop, which dereferences it.
func integTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// TestIntegrationPayloadsRoundTrip proves every constructor's payload comes
// back byte-for-byte equal from its Decode counterpart.
func TestIntegrationPayloadsRoundTrip(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)

	t.Run("SyncAnalyzers", func(t *testing.T) {
		want := SyncAnalyzersPayload{CompanyID: uuid.New(), CredentialID: uuid.New()}
		task, err := NewSyncAnalyzersTask(want, TaskOptions{})
		require.NoError(t, err)
		got, err := DecodeSyncAnalyzers(task)
		require.NoError(t, err)
		require.Equal(t, want, got)
	})

	t.Run("FetchReadings cursor-driven", func(t *testing.T) {
		want := FetchReadingsPayload{
			CompanyID:    uuid.New(),
			CredentialID: uuid.New(),
			AnalyzerID:   uuid.New(),
			Kind:         model.ReadingKindLoadProfile,
		}
		task, err := NewFetchReadingsTask(want, TaskOptions{})
		require.NoError(t, err)
		got, err := DecodeFetchReadings(task)
		require.NoError(t, err)
		require.Equal(t, want, got)
	})

	t.Run("FetchReadings windowed", func(t *testing.T) {
		want := FetchReadingsPayload{
			CompanyID:    uuid.New(),
			CredentialID: uuid.New(),
			AnalyzerID:   uuid.New(),
			Kind:         model.ReadingKindBilling,
			Window:       &Window{From: from, To: to},
		}
		task, err := NewFetchReadingsTask(want, TaskOptions{})
		require.NoError(t, err)
		got, err := DecodeFetchReadings(task)
		require.NoError(t, err)
		require.Equal(t, want.CompanyID, got.CompanyID)
		require.Equal(t, want.CredentialID, got.CredentialID)
		require.Equal(t, want.AnalyzerID, got.AnalyzerID)
		require.Equal(t, want.Kind, got.Kind)
		require.NotNil(t, got.Window)
		require.True(t, want.Window.From.Equal(got.Window.From))
		require.True(t, want.Window.To.Equal(got.Window.To))
	})

	t.Run("Backfill", func(t *testing.T) {
		want := BackfillPayload{
			CompanyID:    uuid.New(),
			CredentialID: uuid.New(),
			AnalyzerIDs:  []uuid.UUID{uuid.New(), uuid.New()},
			Kinds:        []model.ReadingKind{model.ReadingKindDaily, model.ReadingKindBilling},
			From:         from,
			To:           to,
		}
		task, err := NewBackfillTask(want, TaskOptions{})
		require.NoError(t, err)
		got, err := DecodeBackfill(task)
		require.NoError(t, err)
		require.Equal(t, want.CompanyID, got.CompanyID)
		require.Equal(t, want.CredentialID, got.CredentialID)
		require.Equal(t, want.AnalyzerIDs, got.AnalyzerIDs)
		require.Equal(t, want.Kinds, got.Kinds)
		require.True(t, want.From.Equal(got.From))
		require.True(t, want.To.Equal(got.To))
	})

	t.Run("SyncPrices", func(t *testing.T) {
		want := SyncPricesPayload{Window: &Window{From: from, To: to}}
		task, err := NewSyncPricesTask(want, TaskOptions{})
		require.NoError(t, err)
		got, err := DecodeSyncPrices(task)
		require.NoError(t, err)
		require.NotNil(t, got.Window)
		require.True(t, want.Window.From.Equal(got.Window.From))
		require.True(t, want.Window.To.Equal(got.Window.To))
	})

	t.Run("SyncPrices nil window", func(t *testing.T) {
		want := SyncPricesPayload{}
		task, err := NewSyncPricesTask(want, TaskOptions{})
		require.NoError(t, err)
		got, err := DecodeSyncPrices(task)
		require.NoError(t, err)
		require.Nil(t, got.Window)
	})
}

// TestDecodeRefusesUnknownFields proves every Decode<Name> — all four
// decoders the brief lists, not just two of them (M5) — rejects a payload
// carrying a field it does not know about, rather than silently discarding
// it — a payload a newer worker wrote and an older one must not
// half-understand.
func TestDecodeRefusesUnknownFields(t *testing.T) {
	raw, err := json.Marshal(map[string]any{
		"CompanyID":    uuid.New(),
		"CredentialID": uuid.New(),
		"bogus_field":  "should not be allowed",
	})
	require.NoError(t, err)
	_, err = DecodeSyncAnalyzers(asynq.NewTask(TypeIntegrationSyncAnalyzers, raw))
	require.Error(t, err)

	rawFetch, err := json.Marshal(map[string]any{
		"CompanyID":    uuid.New(),
		"CredentialID": uuid.New(),
		"AnalyzerID":   uuid.New(),
		"Kind":         model.ReadingKindLoadProfile,
		"extra":        "nope",
	})
	require.NoError(t, err)
	_, err = DecodeFetchReadings(asynq.NewTask(TypeIntegrationFetchReadings, rawFetch))
	require.Error(t, err)

	rawBackfill, err := json.Marshal(map[string]any{
		"CompanyID":    uuid.New(),
		"CredentialID": uuid.New(),
		"bogus_field":  "should not be allowed",
	})
	require.NoError(t, err)
	_, err = DecodeBackfill(asynq.NewTask(TypeIntegrationBackfill, rawBackfill))
	require.Error(t, err)

	rawPrices, err := json.Marshal(map[string]any{
		"bogus_field": "should not be allowed",
	})
	require.NoError(t, err)
	_, err = DecodeSyncPrices(asynq.NewTask(TypeEPIASSyncPrices, rawPrices))
	require.Error(t, err)
}

// TestDecodeRefusesTrailingData proves integDecode rejects bytes left over
// after the one JSON object it decodes (M5): asynq's Task.Payload is a bare
// byte slice, not a framed single value, so a stray second JSON value (or
// any trailing garbage) appended after the object must be noticed rather
// than silently ignored.
func TestDecodeRefusesTrailingData(t *testing.T) {
	valid, err := json.Marshal(SyncAnalyzersPayload{CompanyID: uuid.New(), CredentialID: uuid.New()})
	require.NoError(t, err)

	trailing := append(append([]byte{}, valid...), []byte(`{}`)...)
	_, err = DecodeSyncAnalyzers(asynq.NewTask(TypeIntegrationSyncAnalyzers, trailing))
	require.Error(t, err)

	trailingGarbage := append(append([]byte{}, valid...), []byte(`garbage`)...)
	_, err = DecodeSyncAnalyzers(asynq.NewTask(TypeIntegrationSyncAnalyzers, trailingGarbage))
	require.Error(t, err)
}

// TestIntegMaxRetryOptionsAlwaysExplicit proves TaskOptions.MaxRetry is
// always passed to asynq as an explicit asynq.MaxRetry option, including
// zero (I5): a TaskOptions{} — "no retries", a legitimate configuration per
// Global Constraints — must produce asynq.MaxRetry(0), never silently fall
// back to asynq's own built-in default of 25 by omitting the option.
func TestIntegMaxRetryOptionsAlwaysExplicit(t *testing.T) {
	for _, n := range []int{0, 5} {
		opts := integMaxRetryOptions(TaskOptions{MaxRetry: n})
		require.Len(t, opts, 1, "MaxRetry=%d must always produce exactly one explicit option", n)
		require.Equal(t, asynq.MaxRetryOpt, opts[0].Type())
		require.Equal(t, n, opts[0].Value())
	}
}

// TestFetchTaskWithWindowHasDeterministicTaskID proves
// integFetchReadingsTaskID — what NewFetchReadingsTask feeds asynq.TaskID
// for a windowed payload — is a pure function of the payload: the same
// analyzer, kind and window always produce the same ID, and changing the
// window changes it.
func TestFetchTaskWithWindowHasDeterministicTaskID(t *testing.T) {
	analyzer := uuid.New()
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)

	pA := FetchReadingsPayload{AnalyzerID: analyzer, Kind: model.ReadingKindLoadProfile, Window: &Window{From: from, To: to}}
	pB := FetchReadingsPayload{AnalyzerID: analyzer, Kind: model.ReadingKindLoadProfile, Window: &Window{From: from, To: to}}
	require.Equal(t, integFetchReadingsTaskID(pA), integFetchReadingsTaskID(pB),
		"the same payload must produce the same task ID")

	pDifferentWindow := FetchReadingsPayload{
		AnalyzerID: analyzer,
		Kind:       model.ReadingKindLoadProfile,
		Window:     &Window{From: from, To: to.Add(24 * time.Hour)},
	}
	require.NotEqual(t, integFetchReadingsTaskID(pA), integFetchReadingsTaskID(pDifferentWindow),
		"a different window must produce a different task ID")
}

// TestRegisterSkipsNilIntegrationHandlers proves a Handlers with no
// Ingestion/Backfiller/PriceSyncer registers no route for the task types
// they would have served: the mux falls through to its not-found handler,
// signalled here by Handler returning an empty pattern.
func TestRegisterSkipsNilIntegrationHandlers(t *testing.T) {
	mux := asynq.NewServeMux()
	Register(mux, &Handlers{Log: integTestLogger()})

	_, pattern := mux.Handler(asynq.NewTask(TypeIntegrationFetchReadings, nil))
	require.Empty(t, pattern, "no handler should be registered for %s", TypeIntegrationFetchReadings)
}

// integFakeIngestion records every FetchReadings call it receives so the
// routing test can assert Register wired the decoded payload through to
// it, not just that no error occurred.
type integFakeIngestion struct {
	fetchCalls []FetchReadingsPayload
}

func (f *integFakeIngestion) Dispatch(context.Context) error { return nil }

func (f *integFakeIngestion) SyncAnalyzers(context.Context, SyncAnalyzersPayload) error { return nil }

func (f *integFakeIngestion) FetchReadings(_ context.Context, p FetchReadingsPayload) error {
	f.fetchCalls = append(f.fetchCalls, p)
	return nil
}

// TestRegisterRoutesFetchReadingsToIngestion proves a task built by
// NewFetchReadingsTask and dispatched through Register's mux reaches
// Handlers.Ingestion.FetchReadings with the payload decoded intact.
func TestRegisterRoutesFetchReadingsToIngestion(t *testing.T) {
	fake := &integFakeIngestion{}
	mux := asynq.NewServeMux()
	Register(mux, &Handlers{Log: integTestLogger(), Ingestion: fake})

	want := FetchReadingsPayload{
		CompanyID:    uuid.New(),
		CredentialID: uuid.New(),
		AnalyzerID:   uuid.New(),
		Kind:         model.ReadingKindCurrentIndex,
	}
	task, err := NewFetchReadingsTask(want, TaskOptions{})
	require.NoError(t, err)

	handler, pattern := mux.Handler(task)
	require.Equal(t, TypeIntegrationFetchReadings, pattern)
	require.NoError(t, handler.ProcessTask(context.Background(), task))

	require.Len(t, fake.fetchCalls, 1)
	require.Equal(t, want, fake.fetchCalls[0])
}

// integFakeBackfiller and integFakePriceSyncer are minimal Backfiller/
// PriceSyncer implementations, used only so TestIntegrationHandlersSkipRetryOnDecodeFailure
// can register every integration task type and reach every integHandle*
// adapter's decode-failure path — none of these fakes' methods are ever
// meant to run, since a bad payload must be skipped before the call.
type integFakeBackfiller struct{}

func (integFakeBackfiller) Backfill(context.Context, BackfillPayload) error { return nil }

type integFakePriceSyncer struct{}

func (integFakePriceSyncer) SyncPrices(context.Context, SyncPricesPayload) error { return nil }

// TestIntegrationHandlersSkipRetryOnDecodeFailure proves every
// integHandle* adapter wraps a payload decode failure with asynq.SkipRetry
// (M6): a payload that cannot be decoded will fail identically on every
// retry, so asynq must not spend its retry budget re-attempting it.
func TestIntegrationHandlersSkipRetryOnDecodeFailure(t *testing.T) {
	mux := asynq.NewServeMux()
	Register(mux, &Handlers{
		Log:       integTestLogger(),
		Ingestion: &integFakeIngestion{},
		Backfill:  integFakeBackfiller{},
		Prices:    integFakePriceSyncer{},
	})

	for _, taskType := range []string{
		TypeIntegrationSyncAnalyzers,
		TypeIntegrationFetchReadings,
		TypeIntegrationBackfill,
		TypeEPIASSyncPrices,
	} {
		t.Run(taskType, func(t *testing.T) {
			task := asynq.NewTask(taskType, []byte("not valid json"))
			handler, pattern := mux.Handler(task)
			require.Equal(t, taskType, pattern)

			err := handler.ProcessTask(context.Background(), task)
			require.Error(t, err)
			require.ErrorIs(t, err, asynq.SkipRetry,
				"a payload decode failure must be classified non-retryable, not left to burn the retry budget")
		})
	}
}
