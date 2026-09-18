package alarms

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/alarm"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/job"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// runCategory is the operational_messages.category of an evaluation summary.
const runCategory = "alarm-evaluate"

// maxRulesPerRun bounds one company's evaluation. A tenant with more rules
// than this has a configuration problem, and silently evaluating a prefix
// would be worse than the run reporting what it covered.
const maxRulesPerRun int32 = 500

// RunDeps are what a real (writing) run needs beyond EvaluateDeps.
type RunDeps struct {
	Ops store.OpsRepository
	// Enqueue schedules one notification. It is a function rather than a
	// client so the service does not depend on asynq directly.
	Enqueue func(ctx context.Context, p job.AlarmNotifyPayload) error
}

// WithRun returns a Service that can fire rules: write events, claim invoices
// and enqueue notifications.
func (s *Service) WithRun(d RunDeps) *Service {
	clone := *s
	clone.rd = &d
	return &clone
}

// Run evaluates every enabled rule of one company (01 §8's job contract:
// idempotent per hour through R225's TaskID, isolated per rule, observable
// through job_runs plus one summary message).
func (s *Service) Run(ctx context.Context, companyID uuid.UUID, now time.Time) error {
	if s.rd == nil || s.ed == nil {
		return errors.New("alarms: Run needs RunDeps and EvaluateDeps")
	}
	sc := store.SystemScope(companyID)
	enabled := true
	rules, err := s.List(ctx, sc, store.AlarmFilter{IsEnabled: &enabled, Page: store.Page{Limit: maxRulesPerRun}})
	if err != nil {
		return err
	}

	scopeJSON, err := json.Marshal(map[string]any{"company_id": companyID, "rules": len(rules)})
	if err != nil {
		return err
	}
	run, err := s.rd.Ops.StartRun(ctx, sc, model.JobRun{
		CompanyID: &companyID, JobType: job.TypeAlarmEvaluate, Scope: scopeJSON, StartedAt: now, Status: "running",
	})
	if err != nil {
		return err
	}

	var processed, skipped, failed, notified int32
	for _, rule := range rules {
		ev, err := s.fire(ctx, sc, rule, now)
		if err != nil {
			// 01 §8: one failing rule never aborts the run.
			failed++
			s.ruleFailed(ctx, sc, rule, err)
			continue
		}
		processed++
		notified += int32(ev.NotificationsSent)
		if !ev.Fired() {
			skipped++
		}
	}

	status := "success"
	switch {
	case failed > 0 && processed == 0:
		status = "failed"
	case failed > 0:
		status = "partial"
	}
	detail, err := json.Marshal(map[string]any{"notified": notified})
	if err != nil {
		return err
	}
	if _, err := s.rd.Ops.FinishRun(ctx, sc, run.ID, status, processed, skipped, failed, nil, detail, s.d.Clock.Now()); err != nil {
		return err
	}
	// R219: ONE summary per run. A row per rule per hour would be millions a
	// month, and the alarm log already records every firing.
	return s.appendRunSummary(ctx, sc, status, processed, skipped, failed, notified)
}

func (s *Service) appendRunSummary(ctx context.Context, sc store.Scope, status string, processed, skipped, failed, notified int32) error {
	messageStatus := "success"
	switch {
	case failed > 0:
		messageStatus = "error"
	case notified > 0:
		messageStatus = "warning"
	}
	companyID := sc.CompanyID
	_, err := s.rd.Ops.AppendMessage(ctx, sc, model.OperationalMessage{
		CompanyID: &companyID, Kind: "job", Category: runCategory, Status: messageStatus,
		Message: fmt.Sprintf("Alarm değerlendirmesi tamamlandı (%s): %d kural işlendi, %d tetiklenmedi, %d hata, %d bildirim.",
			status, processed, skipped, failed, notified),
		CreatedAt: s.d.Clock.Now(),
	})
	return err
}

// ruleFailed records one rule's failure without aborting the run. The error
// text is operator-facing, so it says which rule and what went wrong, never a
// DSN or a provider payload.
func (s *Service) ruleFailed(ctx context.Context, sc store.Scope, rule Rule, cause error) {
	companyID := sc.CompanyID
	related, alarmID := "alarm", rule.Alarm.ID
	_, _ = s.rd.Ops.AppendMessage(ctx, sc, model.OperationalMessage{
		CompanyID: &companyID, Kind: "job", Category: runCategory, Status: "error",
		Message:     fmt.Sprintf("Alarm değerlendirilemedi: %s", rule.Alarm.Name),
		Detail:      strPtr(cause.Error()),
		RelatedType: &related, RelatedID: &alarmID, CreatedAt: s.d.Clock.Now(),
	})
}

func strPtr(s string) *string { return &s }

// fire evaluates one rule and, for every analyzer that breached, writes an
// event and decides whether to notify (R218). It is the writing counterpart of
// EvaluateRule, which writes nothing.
func (s *Service) fire(ctx context.Context, sc store.Scope, rule Rule, now time.Time) (Evaluation, error) {
	if s.rd == nil {
		return Evaluation{}, errNotifyUnavailable
	}
	out := Evaluation{EvaluatedAt: now, DryRun: false, Analyzers: make([]AnalyzerEvaluation, 0, len(rule.Analyzers))}
	for _, ref := range rule.Analyzers {
		verdict, err := s.evaluateAnalyzer(ctx, sc, rule.Alarm, ref, now)
		if err != nil {
			return Evaluation{}, err
		}
		summary, lines, detail := alarm.Describe(rule.Alarm, verdict, ref.InstallationNumber, s.locale())
		out.Analyzers = append(out.Analyzers, AnalyzerEvaluation{
			AnalyzerID: ref.ID, InstallationNumber: ref.InstallationNumber, Verdict: verdict, Lines: lines})

		if !verdict.Fired() {
			continue // R219: a non-firing evaluation writes no event
		}
		// R217: claim the invoice BEFORE anything is written, so a crash
		// between claim and send loses one notification rather than sending
		// forever. claimed=false means this bill already fired this rule.
		if rule.Alarm.Type == model.AlarmTypeInvoiceIncrease {
			claimed, err := s.claimBill(ctx, sc, rule.Alarm.ID, verdict)
			if err != nil {
				return Evaluation{}, err
			}
			if !claimed {
				continue
			}
		}
		sent, err := s.recordAndNotify(ctx, sc, rule, ref, verdict, summary, detail, now)
		if err != nil {
			return Evaluation{}, err
		}
		out.NotificationsSent += sent
	}
	return out, nil
}

var errNotifyUnavailable = errors.New("alarms: fire needs RunDeps")

// claimBill claims (alarm, bill) once, from the breach's own recorded bill id.
func (s *Service) claimBill(ctx context.Context, sc store.Scope, alarmID uuid.UUID, v alarm.Verdict) (bool, error) {
	for _, b := range v.Breaches {
		raw, ok := b.Extra["bill_id"]
		if !ok {
			continue
		}
		billID, err := uuid.Parse(raw)
		if err != nil {
			return false, err
		}
		return s.d.Alarms.MarkBillFired(ctx, sc, alarmID, billID)
	}
	return true, nil // no bill named: nothing to deduplicate against
}

// recordAndNotify writes the event, then decides delivery under the frequency
// window (R218). The EVENT is always written; only the notification is
// suppressed, so the log stays honest about what happened.
func (s *Service) recordAndNotify(ctx context.Context, sc store.Scope, rule Rule, ref AnalyzerRef,
	v alarm.Verdict, summary string, detail map[string]any, now time.Time,
) (int, error) {
	lastNotified, err := s.lastNotifiedAt(ctx, sc, rule.Alarm.ID, ref.ID)
	if err != nil {
		return 0, err
	}
	suppressed := alarm.Suppressed(rule.Alarm, lastNotified, now)
	if suppressed {
		detail["notification_suppressed"] = true
	}
	raw, err := json.Marshal(detail)
	if err != nil {
		return 0, err
	}
	analyzerID := ref.ID
	event, err := s.d.Alarms.CreateEvent(ctx, sc, model.AlarmEvent{
		AlarmID: rule.Alarm.ID, AnalyzerID: &analyzerID, TriggeredAt: now, Message: summary, Detail: raw,
	})
	if err != nil {
		return 0, err
	}
	if suppressed {
		return 0, nil
	}
	if err := s.rd.Enqueue(ctx, job.AlarmNotifyPayload{
		CompanyID: sc.CompanyID, AlarmID: rule.Alarm.ID, EventID: event.ID, AnalyzerID: &analyzerID,
	}); err != nil {
		return 0, err
	}
	return 1, nil
}

// lastNotifiedAt is the newest DELIVERED event for this (rule, analyzer) pair,
// which is what the frequency window is measured from (R218).
func (s *Service) lastNotifiedAt(ctx context.Context, sc store.Scope, alarmID, analyzerID uuid.UUID) (*time.Time, error) {
	if rule := s.d.Alarms; rule == nil {
		return nil, nil
	}
	yes := true
	events, err := s.d.Alarms.ListEvents(ctx, sc, store.AlarmEventFilter{
		AlarmID: &alarmID, AnalyzerID: &analyzerID, Notified: &yes, Page: store.Page{Limit: 1}})
	if err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return nil, nil
	}
	return events[0].NotifiedAt, nil
}
