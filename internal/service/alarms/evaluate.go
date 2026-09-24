package alarms

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/alarm"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	perr "github.com/MErenTalan/ekokod-rewrite/internal/platform/errors"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// powerLookbackHours is R214: the power alarm compares the newest reading
// inside this window. ReadingRepository.Latest demands a TimeRange, and an
// unbounded "newest row" probes every chunk of the hypertable.
const powerLookbackHours int32 = 48

// maxBillsCompared is R217: the two newest analyzer bills, newest first.
const maxBillsCompared int32 = 2

// AnalyzerEvaluation is one meter's outcome for one rule.
type AnalyzerEvaluation struct {
	AnalyzerID         uuid.UUID
	InstallationNumber string
	Verdict            alarm.Verdict
	// Lines are Describe's rendered breach sentences, in the caller's locale.
	Lines []string
}

// Evaluation is what a dry run returns and what a real run summarises.
type Evaluation struct {
	EvaluatedAt       time.Time
	DryRun            bool
	Analyzers         []AnalyzerEvaluation
	NotificationsSent int
}

// Fired reports whether any analyzer breached anything.
func (e Evaluation) Fired() bool {
	for _, a := range e.Analyzers {
		if a.Verdict.Fired() {
			return true
		}
	}
	return false
}

// EvaluateDeps are the reads an evaluation needs beyond the CRUD deps. They
// are optional: the API process does dry runs only when they are present, and
// answers Unavailable when they are not, rather than panicking on a nil
// interface — the same nil-tolerant contract job.Register follows.
type EvaluateDeps struct {
	Readings store.ReadingRepository
	Bills    store.BillRepository
	// Locale is the company's outbound locale for rendered lines; "tr" unless set.
	Locale string
}

// WithEvaluate returns a Service that can evaluate rules. It is a separate
// step from New because the API process needs CRUD without the hypertable
// reads, and the worker needs both.
func (s *Service) WithEvaluate(d EvaluateDeps) *Service {
	clone := *s
	clone.ed = &d
	return &clone
}

var errEvaluateUnavailable = perr.New(perr.Unavailable.Code, perr.Unavailable.HTTPStatus, "errors.alarm.evaluateUnavailable")

// DryRun is R223: evaluate now and return the verdicts, writing nothing — no
// event, no bill claim, no notification. notify=true runs the real firing
// path, which Task 9 supplies.
func (s *Service) DryRun(ctx context.Context, sc store.Scope, id uuid.UUID, notify bool) (Evaluation, error) {
	rule, err := s.Get(ctx, sc, id) // R213 first
	if err != nil {
		return Evaluation{}, err
	}
	if notify {
		if s.rd == nil {
			// Honest: a silent dry run would tell the operator a notification
			// went out when none did.
			return Evaluation{}, perr.New(perr.Unavailable.Code, perr.Unavailable.HTTPStatus, "errors.alarm.notifyUnavailable")
		}
		return s.fire(ctx, sc, rule, s.d.Clock.Now())
	}
	return s.EvaluateRule(ctx, sc, rule, s.d.Clock.Now())
}

// EvaluateRule evaluates one rule against every analyzer it can see and
// returns the verdicts. It writes NOTHING: the invoice alarm in particular
// must not claim a bill here, because a dry run that claimed one would
// silence the real firing (R217 and R223 together).
func (s *Service) EvaluateRule(ctx context.Context, sc store.Scope, rule Rule, now time.Time) (Evaluation, error) {
	if s.ed == nil {
		return Evaluation{}, errEvaluateUnavailable
	}
	out := Evaluation{EvaluatedAt: now, DryRun: true, Analyzers: make([]AnalyzerEvaluation, 0, len(rule.Analyzers))}
	for _, ref := range rule.Analyzers {
		verdict, err := s.evaluateAnalyzer(ctx, sc, rule.Alarm, ref, now)
		if err != nil {
			return Evaluation{}, err
		}
		_, lines, _ := alarm.Describe(rule.Alarm, verdict, ref.InstallationNumber, s.locale())
		out.Analyzers = append(out.Analyzers, AnalyzerEvaluation{
			AnalyzerID: ref.ID, InstallationNumber: ref.InstallationNumber, Verdict: verdict, Lines: lines})
	}
	return out, nil
}

func (s *Service) locale() string {
	if s.ed != nil && s.ed.Locale != "" {
		return s.ed.Locale
	}
	return "tr"
}

// evaluateAnalyzer loads exactly what this rule's type needs and calls the
// pure evaluator. Nothing else is read: the comms alarm in particular never
// touches the hypertable (R216).
func (s *Service) evaluateAnalyzer(ctx context.Context, sc store.Scope, a model.Alarm, ref AnalyzerRef, now time.Time) (alarm.Verdict, error) {
	switch a.Type {
	case model.AlarmTypeDataCommunication:
		return alarm.EvaluateComms(a, ref.ID, ref.LastReadingAt, now), nil

	case model.AlarmTypeCurrentVoltagePower:
		w := alarm.NewWindow(now, powerLookbackHours, model.PeriodUnitHours)
		latest, err := s.ed.Readings.Latest(ctx, sc, ref.ID,
			store.TimeRange{From: w.From, To: w.To}, model.ReadingKindLoadProfile)
		if err != nil {
			return alarm.Verdict{}, err
		}
		return alarm.EvaluatePower(a, ref.ID, latest, w), nil

	case model.AlarmTypeReactiveLimit:
		in, err := s.reactiveInput(ctx, sc, a, ref, now)
		if err != nil {
			return alarm.Verdict{}, err
		}
		return alarm.EvaluateReactive(a, in), nil

	case model.AlarmTypeInvoiceIncrease:
		latest, previous, err := s.twoNewestBills(ctx, sc, ref.ID)
		if err != nil {
			return alarm.Verdict{}, err
		}
		return alarm.EvaluateInvoice(a, ref.ID, latest, previous), nil
	}
	return alarm.Verdict{AnalyzerID: ref.ID}, nil
}

// reactiveGroups pairs each configured group with the period it is measured
// over, so one loop serves the four windows.
func reactiveGroups(a model.Alarm) map[string]struct {
	value *int32
	unit  *model.PeriodUnit
} {
	type period = struct {
		value *int32
		unit  *model.PeriodUnit
	}
	return map[string]period{
		alarm.FieldInductive:  {a.InductivePeriodValue, a.InductivePeriodUnit},
		alarm.FieldCapacitive: {a.CapacitivePeriodValue, a.CapacitivePeriodUnit},
		alarm.FieldActiveMax:  {a.ActiveConsumptionMaxPeriodValue, a.ActiveConsumptionMaxPeriodUnit},
		alarm.FieldActiveMin:  {a.ActiveConsumptionMinPeriodValue, a.ActiveConsumptionMinPeriodUnit},
	}
}

// reactiveInput reads one window per configured group — at most four — so a
// rule that configures one group costs one hypertable read, not four.
func (s *Service) reactiveInput(ctx context.Context, sc store.Scope, a model.Alarm, ref AnalyzerRef, now time.Time) (alarm.ReactiveInput, error) {
	in := alarm.ReactiveInput{AnalyzerID: ref.ID, Windows: map[string]alarm.Window{}}
	rows := map[string][]model.MeterReading{}

	for field, p := range reactiveGroups(a) {
		if p.value == nil || p.unit == nil {
			continue
		}
		w := alarm.NewWindow(now, *p.value, *p.unit)
		got, err := s.ed.Readings.Range(ctx, sc, ref.ID,
			store.TimeRange{From: w.From, To: w.To}, model.ReadingKindLoadProfile)
		if err != nil {
			return alarm.ReactiveInput{}, err
		}
		in.Windows[field] = w
		rows[field] = got
	}
	in.Inductive, in.Capacitive = rows[alarm.FieldInductive], rows[alarm.FieldCapacitive]
	in.ActiveMax, in.ActiveMin = rows[alarm.FieldActiveMax], rows[alarm.FieldActiveMin]
	return in, nil
}

// twoNewestBills returns the analyzer's two newest non-superseded bills,
// newest first. Either may be nil, which the evaluator reads as a no-verdict.
func (s *Service) twoNewestBills(ctx context.Context, sc store.Scope, analyzerID uuid.UUID) (*model.Bill, *model.Bill, error) {
	scope := model.BillScopeAnalyzer
	list, err := s.ed.Bills.List(ctx, sc, store.BillFilter{
		AnalyzerID: &analyzerID, BillScope: &scope, Page: store.Page{Limit: maxBillsCompared}})
	if err != nil {
		return nil, nil, err
	}
	var latest, previous *model.Bill
	if len(list) > 0 {
		latest = &list[0]
	}
	if len(list) > 1 {
		previous = &list[1]
	}
	return latest, previous, nil
}
