package alarms_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/alarms"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

var runNow = time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)

type fakeReadings struct {
	store.ReadingRepository
	rows      []model.MeterReading
	latest    *model.MeterReading
	latestErr error
}

func (f *fakeReadings) Range(_ context.Context, _ store.Scope, _ uuid.UUID, _ store.TimeRange, _ model.ReadingKind) ([]model.MeterReading, error) {
	return f.rows, nil
}

func (f *fakeReadings) Latest(_ context.Context, _ store.Scope, _ uuid.UUID, _ store.TimeRange, _ model.ReadingKind) (*model.MeterReading, error) {
	return f.latest, f.latestErr
}

type fakeBills struct {
	store.BillRepository
	list []model.Bill
}

func (f fakeBills) List(_ context.Context, _ store.Scope, _ store.BillFilter) ([]model.Bill, error) {
	return f.list, nil
}

type fakeRunOps struct {
	store.OpsRepository
	runs     map[uuid.UUID]model.JobRun
	order    []uuid.UUID
	messages []model.OperationalMessage
}

func newRunOps() *fakeRunOps { return &fakeRunOps{runs: map[uuid.UUID]model.JobRun{}} }

func (f *fakeRunOps) StartRun(_ context.Context, _ store.Scope, r model.JobRun) (model.JobRun, error) {
	r.ID = uuid.New()
	f.runs[r.ID] = r
	f.order = append(f.order, r.ID)
	return r, nil
}

func (f *fakeRunOps) FinishRun(_ context.Context, _ store.Scope, id uuid.UUID, status string,
	processed, skipped, failed int32, errText *string, detail []byte, at time.Time,
) (model.JobRun, error) {
	r := f.runs[id]
	r.Status, r.Processed, r.Skipped, r.Failed = status, processed, skipped, failed
	r.Error, r.Detail, r.FinishedAt = errText, detail, &at
	f.runs[id] = r
	return r, nil
}

func (f *fakeRunOps) AppendMessage(_ context.Context, _ store.Scope, m model.OperationalMessage) (model.OperationalMessage, error) {
	f.messages = append(f.messages, m)
	return m, nil
}

func (f *fakeRunOps) only() model.JobRun {
	return f.runs[f.order[0]]
}

func (f *fakeRunOps) byCategory(category string) []model.OperationalMessage {
	var out []model.OperationalMessage
	for _, m := range f.messages {
		if m.Category == category {
			out = append(out, m)
		}
	}
	return out
}

// runFixture is one company with one rule and whatever data the type needs.
type runFixture struct {
	svc      *alarms.Service
	alarms   *fakeAlarms
	readings *fakeReadings
	ops      *fakeRunOps
	enqueued []job.AlarmNotifyPayload
	clock    *clock.Fake
	ruleID   uuid.UUID
}

func (f *runFixture) events() []model.AlarmEvent { return f.alarms.events[f.ruleID] }

// runConfig is one fixture's mutable setup. It is a struct rather than a set
// of package-level vars because these tests run in parallel, and a shared
// global made one test's reset flip another's meter mid-run.
type runConfig struct {
	readings   *fakeReadings
	bills      *fakeBills
	in         *alarms.Input
	staleHours int
}

type runOption func(*runConfig)

// staleMeter makes the analyzer's last reading old enough to breach a
// one-hour communication threshold.
func staleMeter(hours int) runOption {
	return func(c *runConfig) { c.staleHours = hours }
}

func withFrequency(value int32, u model.PeriodUnit) runOption {
	return func(c *runConfig) {
		c.in.Alarm.NotificationFrequencyValue = i32(value)
		c.in.Alarm.NotificationFrequencyUnit = unit(u)
	}
}

func withBills(latest, previous string) runOption {
	return func(c *runConfig) {
		c.in.Type = model.AlarmTypeInvoiceIncrease
		c.in.Alarm = model.Alarm{InvoiceThresholdPct: dec("20")}
		c.bills.list = []model.Bill{
			{ID: uuid.New(), Scope: model.BillScopeAnalyzer, PeriodKey: "2026-08", TotalCost: decimal.RequireFromString(latest)},
			{ID: uuid.New(), Scope: model.BillScopeAnalyzer, PeriodKey: "2026-07", TotalCost: decimal.RequireFromString(previous)},
		}
	}
}

func withPower(kw string) runOption {
	return func(c *runConfig) {
		c.in.Type = model.AlarmTypeCurrentVoltagePower
		c.in.Alarm = model.Alarm{PowerMax: dec("100")}
		d := decimal.RequireFromString(kw)
		c.readings.latest = &model.MeterReading{Ts: runNow, Kind: model.ReadingKindLoadProfile, MaxDemandKw: &d}
	}
}

func withReactive(activeStart, activeEnd, inductiveStart, inductiveEnd string) runOption {
	return func(c *runConfig) {
		c.in.Type = model.AlarmTypeReactiveLimit
		c.in.Alarm = model.Alarm{InductiveRatioThreshold: dec("20"), InductivePeriodValue: i32(24),
			InductivePeriodUnit: unit(model.PeriodUnitHours)}
		mk := func(ts time.Time, active, inductive string) model.MeterReading {
			a := decimal.RequireFromString(active)
			i := decimal.RequireFromString(inductive)
			zero := decimal.Zero
			return model.MeterReading{Ts: ts, Kind: model.ReadingKindLoadProfile,
				ActiveImport: &a, ReactiveInductiveImport: &i, ReactiveCapacitiveImport: &zero}
		}
		c.readings.rows = []model.MeterReading{
			mk(runNow.Add(-24*time.Hour), activeStart, inductiveStart),
			mk(runNow, activeEnd, inductiveEnd),
		}
	}
}

func newRunFixture(t *testing.T, opts ...runOption) *runFixture {
	t.Helper()
	repo := newFakeAlarms()
	repo.visible[analyzerA] = true
	in := commsInput("Kural", analyzerA)
	in.Alarm.CommunicationThresholdHours = i32(1)
	cfg := &runConfig{readings: &fakeReadings{}, bills: &fakeBills{}, in: &in}
	for _, o := range opts {
		o(cfg)
	}
	readings, bills, ops := cfg.readings, cfg.bills, newRunOps()

	fx := &runFixture{alarms: repo, readings: readings, ops: ops, clock: clock.NewFake(runNow)}
	svc, err := alarms.New(alarms.Deps{
		Alarms: repo, Analyzers: staleAnalyzers{hours: cfg.staleHours}, Clock: fx.clock,
	})
	require.NoError(t, err)
	svc = svc.WithEvaluate(alarms.EvaluateDeps{Readings: readings, Bills: bills}).
		WithRun(alarms.RunDeps{Ops: ops, Enqueue: func(_ context.Context, p job.AlarmNotifyPayload) error {
			fx.enqueued = append(fx.enqueued, p)
			return nil
		}})
	fx.svc = svc

	rule, err := svc.Create(context.Background(), companyScope, in)
	require.NoError(t, err)
	fx.ruleID = rule.Alarm.ID
	return fx
}

// staleAnalyzers reports a last reading staleHours ago, or none at all when
// staleHours is 0 — which is a no-verdict, not a breach (R216).
type staleAnalyzers struct {
	store.AnalyzerRepository
	hours int
}

func (f staleAnalyzers) List(_ context.Context, s store.Scope, fl store.AnalyzerFilter) ([]model.Analyzer, error) {
	var out []model.Analyzer
	for _, id := range fl.IDs {
		if !scopeAllows(s, id) {
			continue
		}
		b := analyzerBuilding[id]
		a := model.Analyzer{ID: id, CompanyID: s.CompanyID, BuildingID: &b, InstallationNumber: "INST-A1"}
		if f.hours > 0 {
			last := runNow.Add(-time.Duration(f.hours) * time.Hour)
			a.LastReadingAt = &last
		}
		out = append(out, a)
	}
	return out, nil
}

// 09 §F7: each alarm type fires on a fixture that should trigger it and stays
// silent on one that should not — one case per type, per direction.
func TestRunFiresPerTypeAndStaysSilent(t *testing.T) {
	for _, tc := range []struct {
		name     string
		opts     []runOption
		wantFire bool
	}{
		{"comms fires", []runOption{staleMeter(9)}, true},
		{"comms silent", []runOption{staleMeter(1)}, false},
		{"power fires", []runOption{withPower("120")}, true},
		{"power silent", []runOption{withPower("50")}, false},
		{"invoice fires", []runOption{withBills("1300", "1000")}, true},
		{"invoice silent", []runOption{withBills("1100", "1000")}, false},
		{"reactive fires", []runOption{withReactive("1000", "1100", "100", "125")}, true},
		{"reactive silent", []runOption{withReactive("1000", "1100", "100", "115")}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newRunFixture(t, tc.opts...)
			require.NoError(t, f.svc.Run(context.Background(), companyID, runNow))
			if tc.wantFire {
				require.Len(t, f.events(), 1)
				require.Len(t, f.enqueued, 1)
			} else {
				// R219: a non-firing evaluation writes no event at all.
				require.Empty(t, f.events())
				require.Empty(t, f.enqueued)
			}
		})
	}
}

func TestRunFrequencySuppressesThenAllows(t *testing.T) {
	t.Parallel()
	f := newRunFixture(t, staleMeter(9), withFrequency(6, model.PeriodUnitHours))
	ctx := context.Background()

	require.NoError(t, f.svc.Run(ctx, companyID, runNow))
	require.Len(t, f.enqueued, 1, "the first firing notifies")

	// Mark it delivered, as the notify task would.
	require.NoError(t, f.alarms.MarkNotified(ctx, companyScope, f.events()[0].ID, runNow, nil))

	// R218: a second firing inside the window records the event and sends nothing.
	require.NoError(t, f.svc.Run(ctx, companyID, runNow.Add(time.Hour)))
	require.Len(t, f.enqueued, 1)
	require.Len(t, f.events(), 2)
	require.Contains(t, string(f.events()[0].Detail), "notification_suppressed")

	// Past the window it notifies again.
	require.NoError(t, f.svc.Run(ctx, companyID, runNow.Add(7*time.Hour)))
	require.Len(t, f.enqueued, 2)
	require.Len(t, f.events(), 3)
}

func TestRunWithoutFrequencyNotifiesEveryFiring(t *testing.T) {
	t.Parallel()
	f := newRunFixture(t, staleMeter(9))
	ctx := context.Background()
	require.NoError(t, f.svc.Run(ctx, companyID, runNow))
	require.NoError(t, f.alarms.MarkNotified(ctx, companyScope, f.events()[0].ID, runNow, nil))
	require.NoError(t, f.svc.Run(ctx, companyID, runNow.Add(time.Minute)))
	require.Len(t, f.enqueued, 2, "a rule with no frequency keeps legacy's behaviour")
}

func TestRunInvoiceFiresOnceAcrossRepeatedRuns(t *testing.T) {
	t.Parallel()
	// R217: MarkBillFired claims the pair once, so three evaluations fire once.
	f := newRunFixture(t, withBills("1300", "1000"))
	ctx := context.Background()
	for i := range 3 {
		require.NoError(t, f.svc.Run(ctx, companyID, runNow.Add(time.Duration(i)*time.Hour)))
	}
	require.Len(t, f.events(), 1)
	require.Len(t, f.enqueued, 1)
}

func TestRunRecordsOneJobRunAndOneSummaryMessage(t *testing.T) {
	t.Parallel()
	f := newRunFixture(t, staleMeter(9))
	require.NoError(t, f.svc.Run(context.Background(), companyID, runNow))

	run := f.ops.only()
	require.Equal(t, job.TypeAlarmEvaluate, run.JobType)
	require.Equal(t, "success", run.Status)
	require.EqualValues(t, 1, run.Processed)
	require.EqualValues(t, 0, run.Failed)
	require.NotNil(t, run.FinishedAt)
	require.Contains(t, string(run.Scope), companyID.String())

	// R219: one summary, not one row per rule.
	summaries := f.ops.byCategory("alarm-evaluate")
	require.Len(t, summaries, 1)
	require.Contains(t, summaries[0].Message, "1 kural işlendi")
}

func TestRunCountsANonFiringRuleAsSkipped(t *testing.T) {
	t.Parallel()
	f := newRunFixture(t, staleMeter(1))
	require.NoError(t, f.svc.Run(context.Background(), companyID, runNow))
	run := f.ops.only()
	require.EqualValues(t, 1, run.Processed)
	require.EqualValues(t, 1, run.Skipped, "processed but nothing fired")
}

func TestRunOneFailingRuleDoesNotAbortTheRun(t *testing.T) {
	t.Parallel()
	// 01 §8: isolated failure. The second rule's reads fail; the first still fires.
	f := newRunFixture(t, staleMeter(9))
	ctx := context.Background()

	// The second rule is a POWER rule, the only type here that reads the
	// hypertable; making that read fail breaks it and leaves the comms rule
	// (which reads only LastReadingAt) untouched.
	broken := commsInput("Bozuk", analyzerA)
	broken.Type = model.AlarmTypeCurrentVoltagePower
	broken.Alarm = model.Alarm{PowerMax: dec("100")}
	_, err := f.svc.Create(ctx, companyScope, broken)
	require.NoError(t, err)
	f.readings.latestErr = errors.New("hypertable unavailable")

	require.NoError(t, f.svc.Run(ctx, companyID, runNow))
	run := f.ops.only()
	require.Equal(t, "partial", run.Status)
	require.EqualValues(t, 1, run.Processed)
	require.EqualValues(t, 1, run.Failed)
	require.Len(t, f.events(), 1, "the healthy rule still fired")
	require.NotEmpty(t, f.ops.byCategory("alarm-evaluate"))
}

func TestRunSkipsDisabledRules(t *testing.T) {
	t.Parallel()
	f := newRunFixture(t, staleMeter(9))
	ctx := context.Background()
	stored := f.alarms.stored[f.ruleID]
	stored.IsEnabled = false
	f.alarms.stored[f.ruleID] = stored

	require.NoError(t, f.svc.Run(ctx, companyID, runNow))
	require.Empty(t, f.events())
	require.EqualValues(t, 0, f.ops.only().Processed)
}

func TestRunNeedsItsDeps(t *testing.T) {
	t.Parallel()
	// A CRUD-only Service (the API's) must refuse to run rather than silently
	// evaluating without being able to record or notify.
	svc, _ := newTestService(t)
	require.Error(t, svc.Run(context.Background(), companyID, runNow))
}

type fakeAdminAlarms struct {
	companies []uuid.UUID
	err       error
}

func (f fakeAdminAlarms) CompaniesWithEnabledAlarms(_ context.Context) ([]uuid.UUID, error) {
	return f.companies, f.err
}

type fakeJournal struct {
	store.AdminJournalRepository
	runs map[uuid.UUID]model.JobRun
}

func (f *fakeJournal) StartPlatformRun(_ context.Context, r model.JobRun) (model.JobRun, error) {
	if f.runs == nil {
		f.runs = map[uuid.UUID]model.JobRun{}
	}
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
	}
	f.runs[r.ID] = r
	return r, nil
}

func (f *fakeJournal) FinishPlatformRun(_ context.Context, id uuid.UUID, status string,
	processed, skipped, failed int32, errText *string, detail []byte, at time.Time,
) (model.JobRun, error) {
	r := f.runs[id]
	r.Status, r.Processed, r.Skipped, r.Failed = status, processed, skipped, failed
	r.Error, r.Detail, r.FinishedAt = errText, detail, &at
	f.runs[id] = r
	return r, nil
}

func (f *fakeJournal) only() model.JobRun {
	for _, r := range f.runs {
		return r
	}
	return model.JobRun{}
}

func newDispatchService(t *testing.T, companies []uuid.UUID) (*alarms.Service, *[]job.AlarmEvaluatePayload, *fakeJournal) {
	t.Helper()
	svc, _ := newTestService(t)
	journal := &fakeJournal{}
	var enqueued []job.AlarmEvaluatePayload
	svc = svc.WithDispatch(alarms.DispatchDeps{
		Companies: fakeAdminAlarms{companies: companies}, Journal: journal,
		Enqueue: func(_ context.Context, p job.AlarmEvaluatePayload) error {
			enqueued = append(enqueued, p)
			return nil
		},
	})
	return svc, &enqueued, journal
}

func TestDispatchEnqueuesOneEvaluationPerCompanyOnTheSameHour(t *testing.T) {
	t.Parallel()
	a, b := uuid.New(), uuid.New()
	svc, enqueued, journal := newDispatchService(t, []uuid.UUID{a, b})
	require.NoError(t, svc.Dispatch(context.Background()))

	require.Len(t, *enqueued, 2)
	// R225: every company in one tick carries the SAME hour, so a manual
	// trigger inside it is de-duplicated against the tick.
	require.Equal(t, (*enqueued)[0].Hour, (*enqueued)[1].Hour)
	require.Equal(t, (*enqueued)[0].Hour, (*enqueued)[0].Hour.Truncate(time.Hour))

	run := journal.only()
	require.Equal(t, job.TypeAlarmDispatch, run.JobType)
	require.Equal(t, "success", run.Status)
	require.EqualValues(t, 2, run.Processed)
	require.NotNil(t, run.FinishedAt)
	require.Nil(t, run.CompanyID, "a platform run belongs to no tenant")
}

func TestDispatchSkipsCompaniesWithoutEnabledAlarms(t *testing.T) {
	t.Parallel()
	// R219 at scale: a company with no rules must not get an empty run and a
	// summary message every hour.
	svc, enqueued, journal := newDispatchService(t, nil)
	require.NoError(t, svc.Dispatch(context.Background()))
	require.Empty(t, *enqueued)
	require.Equal(t, "success", journal.only().Status)
	require.EqualValues(t, 0, journal.only().Processed)
}

func TestDispatchOneFailingEnqueueDoesNotStopTheRest(t *testing.T) {
	t.Parallel()
	a, b := uuid.New(), uuid.New()
	svc, _ := newTestService(t)
	journal := &fakeJournal{}
	var enqueued []job.AlarmEvaluatePayload
	svc = svc.WithDispatch(alarms.DispatchDeps{
		Companies: fakeAdminAlarms{companies: []uuid.UUID{a, b}}, Journal: journal,
		Enqueue: func(_ context.Context, p job.AlarmEvaluatePayload) error {
			if p.CompanyID == a {
				return errors.New("queue unavailable")
			}
			enqueued = append(enqueued, p)
			return nil
		},
	})
	require.NoError(t, svc.Dispatch(context.Background()))
	require.Len(t, enqueued, 1, "the second tenant is still dispatched")
	require.Equal(t, "partial", journal.only().Status)
	require.EqualValues(t, 1, journal.only().Failed)
}

func TestDispatchNeedsItsDeps(t *testing.T) {
	t.Parallel()
	svc, _ := newTestService(t)
	require.Error(t, svc.Dispatch(context.Background()))
}
