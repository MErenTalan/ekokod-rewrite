package generation_test

import (
	"context"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/ingest/generation"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/lock"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// hookTestAnalyzerID is fixed for every fake-backed hook test in this file.
var hookTestAnalyzerID = uuid.MustParse("33333333-3333-4333-8333-333333333333")

// hookTestScope is fixed too: these tests exercise concurrency, not
// tenancy (that is generation_integration_test.go's
// TestGenerationIsScoped, against the real store), so one fixed scope for
// every fake call is enough.
var hookTestScope = store.SystemScope(uuid.MustParse("44444444-4444-4444-8444-444444444444"))

func hookTestAnalyzer() model.Analyzer {
	return model.Analyzer{ID: hookTestAnalyzerID, Provider: model.IntegrationProviderPM5340}
}

func hookTS(s string) time.Time {
	tm, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic("hook_test: bad timestamp literal " + s + ": " + err.Error())
	}
	return tm
}

func hookDecPtr(lit string) *decimal.Decimal {
	v := decimal.RequireFromString(lit)
	return &v
}

func hookReading(tsStr, intervalKwh string) model.MeterReading {
	return model.MeterReading{
		AnalyzerID: hookTestAnalyzerID, Ts: hookTS(tsStr), Kind: model.ReadingKindLoadProfile,
		IntervalGenerationKwh: hookDecPtr(intervalKwh),
		MultiplierApplied:     decimal.NewFromInt(1), SourceProvider: model.IntegrationProviderPM5340,
	}
}

func requireHookActiveExport(t *testing.T, rows map[time.Time]model.MeterReading, tsStr, want string) {
	t.Helper()
	r, ok := rows[hookTS(tsStr)]
	require.True(t, ok, "no row at %s", tsStr)
	require.NotNil(t, r.ActiveExport, "row at %s: active_export is nil", tsStr)
	require.True(t, decimal.RequireFromString(want).Equal(*r.ActiveExport),
		"row at %s: want active_export %s, got %s", tsStr, want, r.ActiveExport.String())
}

// hookFakeReadings is an in-memory store.ReadingRepository fake, guarded by
// a mutex so -race stays clean under real concurrent goroutines. Range has
// one controllable extra behaviour: the first call (across every goroutine
// sharing one instance) invokes onPause AFTER taking its snapshot but
// BEFORE returning it — the exact rendezvous point
// TestAfterPersistIsSerialisedPerAnalyzer needs to suspend one AfterPersist
// call mid-read while a second one, for the same analyzer, runs to
// completion.
//
// The "first call only" gate is a CAS on pauseWon, deliberately NOT
// sync.Once: Once.Do blocks every OTHER concurrent caller until the first
// caller's function returns, which would silently re-serialise the fake's
// two Range calls on its own — exactly the behaviour this test exists to
// prove only Accumulator's real lock provides. A losing CAS must return
// immediately, unblocked, so the test's own harness can never be mistaken
// for R52's lock (this was caught during implementation: the mutation
// proof below still passed with the real lock's Acquire/Release deleted,
// because sync.Once.Do was silently doing the serialising itself).
type hookFakeReadings struct {
	mu   sync.Mutex
	rows map[time.Time]model.MeterReading

	pauseWon atomic.Bool
	onPause  func()
}

func newHookFakeReadings() *hookFakeReadings {
	return &hookFakeReadings{rows: make(map[time.Time]model.MeterReading)}
}

func (f *hookFakeReadings) BulkInsert(_ context.Context, _ store.Scope, rows []model.MeterReading) (int, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var inserted, updated int
	for _, r := range rows {
		if _, ok := f.rows[r.Ts]; ok {
			updated++
		} else {
			inserted++
		}
		f.rows[r.Ts] = r
	}
	return inserted, updated, nil
}

func (f *hookFakeReadings) Range(_ context.Context, _ store.Scope, _ uuid.UUID, r store.TimeRange, _ model.ReadingKind) ([]model.MeterReading, error) {
	f.mu.Lock()
	var out []model.MeterReading
	for ts, row := range f.rows {
		if !ts.Before(r.From) && ts.Before(r.To) {
			out = append(out, row)
		}
	}
	f.mu.Unlock()

	sort.Slice(out, func(i, j int) bool { return out[i].Ts.Before(out[j].Ts) })

	// Fires once, ever, for the first caller across every goroutine — every
	// OTHER (losing) caller falls through immediately, unblocked. See the
	// type doc for why this is a CAS, not sync.Once.
	if f.onPause != nil && f.pauseWon.CompareAndSwap(false, true) {
		f.onPause()
	}

	return out, nil
}

func (f *hookFakeReadings) BoundaryReadings(context.Context, store.Scope, uuid.UUID, model.ReadingKind, time.Time, time.Time) (*model.MeterReading, *model.MeterReading, error) {
	panic("hookFakeReadings: BoundaryReadings is not exercised by these tests")
}

func (f *hookFakeReadings) Latest(_ context.Context, _ store.Scope, _ uuid.UUID, r store.TimeRange, _ model.ReadingKind) (*model.MeterReading, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var latest *model.MeterReading
	for ts, row := range f.rows {
		if !ts.Before(r.From) && ts.Before(r.To) {
			row := row
			if latest == nil || row.Ts.After(latest.Ts) {
				latest = &row
			}
		}
	}
	return latest, nil
}

var _ store.ReadingRepository = (*hookFakeReadings)(nil)

// snapshot is the test-only accessor the assertions below read the final
// state through — never called from Accumulator itself.
func (f *hookFakeReadings) snapshot() map[time.Time]model.MeterReading {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make(map[time.Time]model.MeterReading, len(f.rows))
	for k, v := range f.rows {
		out[k] = v
	}
	return out
}

// hookFakeAnchors is an in-memory store.GenerationRepository fake.
type hookFakeAnchors struct {
	mu     sync.Mutex
	anchor *model.GenerationAnchor
}

func (f *hookFakeAnchors) Anchor(_ context.Context, _ store.Scope, _ uuid.UUID) (model.GenerationAnchor, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.anchor == nil {
		return model.GenerationAnchor{}, store.ErrNotFound
	}
	return *f.anchor, nil
}

func (f *hookFakeAnchors) SetAnchor(_ context.Context, _ store.Scope, a model.GenerationAnchor) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := a
	f.anchor = &cp
	return nil
}

var _ store.GenerationRepository = (*hookFakeAnchors)(nil)

// hookAcquireSignal wraps a lock.Locker and closes fired exactly once, the
// Nth time (armOnCall) Acquire is called on it — used by the two
// serialisation tests below to detect "the second goroutine has reached
// (and, if the first still holds the lease, is now blocked inside) its own
// Acquire call" DETERMINISTICALLY (M4, final review B), replacing a fixed
// time.Sleep whose "long enough" margin depends on machine/Docker load
// rather than on the behaviour under test. Acquire itself is untouched —
// this only observes calls to it, so it changes nothing about who actually
// gets the lease or when.
type hookAcquireSignal struct {
	lock.Locker
	arm int

	mu    sync.Mutex
	calls int
	fired chan struct{}
}

func newHookAcquireSignal(inner lock.Locker, armOnCall int) *hookAcquireSignal {
	return &hookAcquireSignal{Locker: inner, arm: armOnCall, fired: make(chan struct{})}
}

func (l *hookAcquireSignal) Acquire(ctx context.Context, key string, ttl time.Duration) (lock.Lease, error) {
	l.mu.Lock()
	l.calls++
	n := l.calls
	l.mu.Unlock()
	if n == l.arm {
		close(l.fired)
	}
	return l.Locker.Acquire(ctx, key, ttl)
}

// TestAfterPersistIsSerialisedPerAnalyzer is R52's concurrency proof: two
// AfterPersist calls for the SAME analyzer, orchestrated so goroutine A's
// own Range read (inside recomputeForward) is suspended AFTER it has
// already taken a snapshot, and BEFORE goroutine B inserts a new row and
// runs its own full AfterPersist cycle to completion — the exact "an older
// window persisting after a newer window has read its rows" shape the
// reviewer's I3 finding describes (final-review-A-report.md).
//
// Setup: an operator anchor of 0 at 10:00. Rows 10:15 (1 kWh) and 10:45
// (1 kWh) exist before either goroutine starts; 10:30 does not exist yet.
// Goroutine A calls AfterPersist(from=10:45,to=10:45): it is the only
// goroutine running so far, so its own Latest lookup ([10:00,10:45), which
// excludes 10:45 itself) finds only 10:15 — with ActiveExport still nil
// (nothing has derived it yet) — so A's base stays the anchor itself,
// (0, 10:00), and its recomputeForward Range call is the FIRST Range call
// made at all, which is exactly the one the fake pauses AFTER taking its
// snapshot = [10:15, 10:45] (10:30 is not in the store yet).
//
// While A is paused, goroutine B inserts row 10:30 (1 kWh) and runs its own
// AfterPersist(from=10:30,to=10:30) to completion — B's own Latest lookup
// ([10:00,10:30)) likewise finds only 10:15 with ActiveExport nil, so B's
// base is ALSO (0, 10:00) at read time (10:15's active_export has not been
// written by A yet, since A is still paused).
//
// Hand-derived, serial-equivalent, correct final result (identical
// regardless of which of A/B the lock lets run first, since both start
// from the same base and the store converges through a fresh Range read
// whichever one runs second):
//
//	10:15: 0 + 1 = 1
//	10:30: 1 + 1 = 2
//	10:45: 2 + 1 = 3
//
// WITHOUT R52's lock: B (unimpeded) finishes first, writing the correct
// [1, 2, 3] over all three rows via its own fresh Range read (which DOES
// see B's own just-inserted 10:30, since B's Range call happens after the
// insert and is not the paused one). A then resumes holding its STALE
// two-row snapshot (missing 10:30, captured before B's insert) and writes
// Accumulate(0, [10:15=1, 10:45=1]) = [1, 2] — A's write of 2 at 10:45
// clobbers B's correct 3. See task-fix-Y-report.md for the recorded output
// of this exact scenario with withAnalyzerLock's Acquire call disabled.
//
// WITH the lock (this test, the real production code path): B's own
// Acquire blocks until A's afterPersistLocked returns and releases —
// lock.Memory polls every 5ms, so B never even reaches its own Latest/Range
// calls while A is paused. By the time B's cycle actually runs, A has
// already written 10:15=1 and (wrongly, in isolation) 10:45=2 — but B's own
// fresh Latest lookup now finds 10:15 WITH ActiveExport=1 and rebases
// itself there (base=(1, 10:15)), so its own recomputeForward only touches
// 10:30 and 10:45, computing [2, 3] and correcting A's stale 10:45 write
// back to 3. The two calls' read-then-write cycles never interleave, and
// the final state is the same correct [1, 2, 3] either way.
func TestAfterPersistIsSerialisedPerAnalyzer(t *testing.T) {
	readings := newHookFakeReadings()
	anchors := &hookFakeAnchors{}
	require.NoError(t, anchors.SetAnchor(context.Background(), hookTestScope, model.GenerationAnchor{
		AnalyzerID: hookTestAnalyzerID, AnchorTs: hookTS("2026-09-01T10:00:00Z"),
		ActiveExport: decimal.Zero, Source: "operator",
	}))
	_, _, err := readings.BulkInsert(context.Background(), hookTestScope, []model.MeterReading{
		hookReading("2026-09-01T10:15:00Z", "1"),
		hookReading("2026-09-01T10:45:00Z", "1"),
	})
	require.NoError(t, err)

	pauseCh := make(chan struct{})
	resumeCh := make(chan struct{})
	readings.onPause = func() {
		close(pauseCh)
		<-resumeCh
	}

	// M4: signal fires on the SECOND Acquire call ever made through this
	// locker — A's own (first, uncontended) Acquire happens before A even
	// reaches pauseCh, so the second call is unambiguously B's.
	signal := newHookAcquireSignal(lock.NewMemory(nil), 2)
	acc := generation.New(readings, anchors, clock.NewFake(hookTS("2026-09-05T00:00:00Z")), signal, 0)

	errA := make(chan error, 1)
	go func() {
		errA <- acc.AfterPersist(context.Background(), hookTestScope, hookTestAnalyzer(), model.ReadingKindLoadProfile,
			hookTS("2026-09-01T10:45:00Z"), hookTS("2026-09-01T10:45:00Z"))
	}()

	<-pauseCh // A has taken its snapshot and is now paused inside Range.

	errB := make(chan error, 1)
	go func() {
		_, _, ierr := readings.BulkInsert(context.Background(), hookTestScope, []model.MeterReading{
			hookReading("2026-09-01T10:30:00Z", "1"),
		})
		if ierr != nil {
			errB <- ierr
			return
		}
		errB <- acc.AfterPersist(context.Background(), hookTestScope, hookTestAnalyzer(), model.ReadingKindLoadProfile,
			hookTS("2026-09-01T10:30:00Z"), hookTS("2026-09-01T10:30:00Z"))
	}()

	// M4: wait for B to actually CALL Acquire (locked: it then blocks there,
	// since A still holds the lease) instead of sleeping a fixed, load-
	// dependent margin. If B finishes first without ever calling Acquire,
	// the lock was not reached at all — fail immediately rather than rely
	// on the final value assertions alone to notice.
	select {
	case <-signal.fired:
	case err := <-errB:
		t.Fatalf("B finished (err=%v) before ever calling Acquire: the per-analyzer lock was not reached", err)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for B to call Acquire")
	}
	close(resumeCh)

	require.NoError(t, <-errA)
	require.NoError(t, <-errB)

	rows := readings.snapshot()
	requireHookActiveExport(t, rows, "2026-09-01T10:15:00Z", "1")
	requireHookActiveExport(t, rows, "2026-09-01T10:30:00Z", "2")
	requireHookActiveExport(t, rows, "2026-09-01T10:45:00Z", "3")
}

// TestAfterPersistPreAnchorDerivationIsSerialisedPerAnalyzer is M4's
// (final review B) second gap: TestAfterPersistIsSerialisedPerAnalyzer
// above only pauses inside recomputeForward's OWN Range call, so it cannot
// tell a lock that covers the whole call (current code) apart from one
// narrowed to cover only recomputeForward (the reviewer's named mutation:
// "anchor read and deriveBeforeAnchor outside the lock") — both shapes
// serialise the one Range call this test's sibling pauses. This test
// instead pauses inside deriveBeforeAnchor's OWN Range call (the "older
// rows" probe, reached only when a call's `from` is at-or-before the
// current anchor — a pre-anchor/backfill call), which sits BEFORE
// recomputeForward and is NOT covered by a lock narrowed to recomputeForward
// alone.
//
// Setup: an "initial" (synthetic, value-0) anchor at 09:00. Row Y (1 kWh)
// already exists at 08:45 — CLOSER to the anchor. Row X (1 kWh) does not
// exist yet and is INSERTED BY B, further back at 08:00.
//
// Goroutine A calls AfterPersist(from=08:45, to=08:45) — a pre-anchor call
// (anchor 09:00 is not before 08:45) — so its deriveBeforeAnchor probe reads
// [08:45, 09:00] and finds only Y, the FIRST Range call made at all, which
// is exactly the one the fake pauses on.
//
// While A is paused, goroutine B inserts X at 08:00 and calls
// AfterPersist(from=08:00, to=08:00) — also pre-anchor (anchor is still the
// original 09:00 until A writes). B's own deriveBeforeAnchor probe reads
// [08:00, 09:00] and finds BOTH X and Y (Y already existed; X is B's own
// insert), so B moves the anchor to (0, X.Ts-15m=07:45) — the correct,
// widest position — and its own recomputeForward(0, 07:45, now) covers both
// rows: X=1, Y=2.
//
// Hand-derived, serial-equivalent, correct final result (identical
// regardless of execution order — see the two full traces in this task's
// final-fix-B-report.md):
//
//	X (08:00): 0 + 1 = 1
//	Y (08:45): 1 + 1 = 2
//	anchor:    (0, 07:45)
//
// WITH the lock covering the WHOLE call (this test, the real production
// code path): B's own Acquire blocks entirely until A releases, so B never
// even reaches its own deriveBeforeAnchor while A is paused. A resumes
// holding its STALE one-row ([Y]) snapshot, moves the anchor to (0, 08:30)
// and writes Y=1 (not yet knowing about X) — locally wrong, but B, running
// AFTER A fully releases, reads the CURRENT anchor fresh, discovers X, moves
// the anchor further back to (0, 07:45), and its own full recomputeForward
// from there corrects Y back to 2. Final state: X=1, Y=2, anchor=(0,07:45)
// — correct.
//
// WITHOUT the lock covering deriveBeforeAnchor (mutation: only
// recomputeForward is wrapped in withAnalyzerLock): B is never blocked from
// running its ENTIRE cycle (deriveBeforeAnchor AND recomputeForward) while A
// is merely paused, unprotected. B correctly computes X=1, Y=2,
// anchor=(0,07:45) — using the full, current picture, since nothing stopped
// it from reading a state that already reflects both rows. But A then
// resumes holding its OWN stale, PRE-B snapshot ([Y] only, captured before B
// ran), and — with no lock preventing it — OVERWRITES B's correct anchor
// with its own narrower (0, 08:30), then recomputes ONLY forward from there:
// its own recomputeForward(0, 08:30, now) does not reach back to X (08:00,
// before 08:30), so it writes Y = 0 + 1 = 1, clobbering B's correct 2 — and
// nothing runs afterward to fix it, since A finishes last. Final state:
// Y=1 (wrong; the assertion below requires 2).
func TestAfterPersistPreAnchorDerivationIsSerialisedPerAnalyzer(t *testing.T) {
	readings := newHookFakeReadings()
	anchors := &hookFakeAnchors{}
	require.NoError(t, anchors.SetAnchor(context.Background(), hookTestScope, model.GenerationAnchor{
		AnalyzerID: hookTestAnalyzerID, AnchorTs: hookTS("2026-09-01T09:00:00Z"),
		ActiveExport: decimal.Zero, Source: "initial",
	}))
	_, _, err := readings.BulkInsert(context.Background(), hookTestScope, []model.MeterReading{
		hookReading("2026-09-01T08:45:00Z", "1"), // Y
	})
	require.NoError(t, err)

	pauseCh := make(chan struct{})
	resumeCh := make(chan struct{})
	readings.onPause = func() {
		close(pauseCh)
		<-resumeCh
	}

	// A's own Acquire is the first call ever made through this locker; B's
	// is the second.
	signal := newHookAcquireSignal(lock.NewMemory(nil), 2)
	acc := generation.New(readings, anchors, clock.NewFake(hookTS("2026-09-05T00:00:00Z")), signal, 0)

	errA := make(chan error, 1)
	go func() {
		errA <- acc.AfterPersist(context.Background(), hookTestScope, hookTestAnalyzer(), model.ReadingKindLoadProfile,
			hookTS("2026-09-01T08:45:00Z"), hookTS("2026-09-01T08:45:00Z"))
	}()

	<-pauseCh // A has taken its snapshot ([Y]) and is now paused inside deriveBeforeAnchor's Range call.

	errB := make(chan error, 1)
	go func() {
		_, _, ierr := readings.BulkInsert(context.Background(), hookTestScope, []model.MeterReading{
			hookReading("2026-09-01T08:00:00Z", "1"), // X
		})
		if ierr != nil {
			errB <- ierr
			return
		}
		errB <- acc.AfterPersist(context.Background(), hookTestScope, hookTestAnalyzer(), model.ReadingKindLoadProfile,
			hookTS("2026-09-01T08:00:00Z"), hookTS("2026-09-01T08:00:00Z"))
	}()

	select {
	case <-signal.fired:
	case err := <-errB:
		t.Fatalf("B finished (err=%v) before ever calling Acquire: the per-analyzer lock was not reached", err)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for B to call Acquire")
	}
	close(resumeCh)

	require.NoError(t, <-errA)
	require.NoError(t, <-errB)

	rows := readings.snapshot()
	requireHookActiveExport(t, rows, "2026-09-01T08:00:00Z", "1")
	requireHookActiveExport(t, rows, "2026-09-01T08:45:00Z", "2")

	anchor, aerr := anchors.Anchor(context.Background(), hookTestScope, hookTestAnalyzerID)
	require.NoError(t, aerr)
	require.True(t, hookTS("2026-09-01T07:45:00Z").Equal(anchor.AnchorTs),
		"the anchor must reflect the EARLIEST pre-anchor row seen across both calls, not whichever call wrote last")
}
