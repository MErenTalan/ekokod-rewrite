package alarms

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/mail"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/secret"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// messageCategory is the operational_messages.category every alarm row carries,
// so the Messages screen can filter them as one family.
const messageCategory = "alarm-trigger"

// Notification names one event to deliver.
type Notification struct {
	CompanyID  uuid.UUID
	AlarmID    uuid.UUID
	EventID    uuid.UUID
	AnalyzerID *uuid.UUID
}

// NotifyDeps are what delivery needs beyond the CRUD deps.
type NotifyDeps struct {
	SMTP store.SMTPRepository
	Mail mail.Sender
	Ops  store.OpsRepository
	// Locale is the company's outbound locale; "tr" unless set.
	Locale string
}

// WithNotify returns a Service that can deliver notifications. Like
// WithEvaluate, it is a separate step: the API never sends mail.
func (s *Service) WithNotify(d NotifyDeps) *Service {
	clone := *s
	clone.nd = &d
	return &clone
}

// Notify delivers one alarm event and records the outcome.
//
// It returns nil for a DELIVERY failure (R222: "it does not fail the job") and
// an error only when the event itself cannot be read or marked — that is worth
// a retry, because nothing was recorded either way.
func (s *Service) Notify(ctx context.Context, n Notification) error {
	if s.nd == nil {
		return errors.New("alarms: Notify needs NotifyDeps")
	}
	sc := store.SystemScope(n.CompanyID)
	rule, err := s.Get(ctx, sc, n.AlarmID)
	if err != nil {
		return err // an unreadable rule is worth a retry
	}

	var emails []string
	smsCount := 0
	for _, c := range rule.Channels {
		switch c.Channel {
		case model.NotifyChannelEmail:
			emails = append(emails, c.Target)
		case model.NotifyChannelSMS:
			smsCount++
		}
	}

	label := analyzerLabel(rule, n.AnalyzerID)
	lines, err := s.eventLines(ctx, sc, n)
	if err != nil {
		return err
	}
	if lines == nil {
		return nil // the event is gone; there is nothing to deliver
	}

	// R514: every Messages row of a firing says what fired; the delivery
	// outcome follows it.
	fired := fmt.Sprintf("Alarm tetiklendi: %s (%s) — %s.", rule.Alarm.Name, label, strings.Join(lines, "; "))

	// R211: say it once, with a COUNT and never a number — Messages rows are
	// exported, and a phone number in one is the tenant's data leaving the app.
	if smsCount > 0 {
		s.appendMessage(ctx, sc, n, "warning", fmt.Sprintf(
			"SMS bildirimi henüz etkin değil: %s kuralı için %d numaraya ulaşılmadı.", rule.Alarm.Name, smsCount))
	}

	if len(emails) == 0 {
		// A rule nobody subscribed to fired as designed: a warning, not an error.
		reason := "bildirim kanalı tanımlı değil"
		if smsCount > 0 {
			reason = "yalnızca SMS numarası tanımlı, SMS henüz gönderilmiyor"
		}
		return s.markUnsent(ctx, sc, n, "warning", fired, reason, nil)
	}

	settings, err := s.nd.SMTP.Get(ctx, sc)
	if errors.Is(err, store.ErrNotFound) {
		// Not configured is the whole story; the driver's text adds nothing.
		return s.markUnsent(ctx, sc, n, "error", fired, "şirketin SMTP ayarları yok", nil)
	}
	if err != nil {
		return s.markUnsent(ctx, sc, n, "error", fired, "SMTP ayarları okunamadı", err)
	}
	password, err := s.nd.SMTP.OpenPassword(ctx, sc)
	if err != nil {
		return s.markUnsent(ctx, sc, n, "error", fired, "SMTP parolası okunamadı", err)
	}

	now := s.d.Clock.Now()
	msg := mail.AlarmFired(s.notifyLocale(), rule.Alarm.Name, label, lines, now.In(s.loc))
	msg.To = emails
	if err := s.nd.Mail.Send(ctx, settings, password, msg); err != nil {
		// The SMTP server's reply can quote what it rejected, so the password's
		// own fragments are removed before the text reaches Messages.
		return s.markUnsent(ctx, sc, n, "error", fired, "e-posta gönderilemedi", err, secret.Fragments(string(password))...)
	}
	if err := s.d.Alarms.MarkNotified(ctx, sc, n.EventID, now, nil); err != nil {
		return err // recorded nothing: retry
	}
	s.appendMessage(ctx, sc, n, "info", fmt.Sprintf("%s Bildirim %d alıcıya gönderildi.", fired, len(emails)))
	return nil
}

func (s *Service) notifyLocale() string {
	if s.nd != nil && s.nd.Locale != "" {
		return s.nd.Locale
	}
	return "tr"
}

// eventLines finds the event and renders its breach sentences. A nil slice
// with a nil error means the event no longer exists.
func (s *Service) eventLines(ctx context.Context, sc store.Scope, n Notification) ([]string, error) {
	events, err := s.d.Alarms.ListEvents(ctx, sc, store.AlarmEventFilter{
		AlarmID: &n.AlarmID, AnalyzerID: n.AnalyzerID, Page: store.Page{Limit: eventLookupLimit}})
	if err != nil {
		return nil, err
	}
	for _, e := range events {
		if e.ID != n.EventID {
			continue
		}
		lines := linesFromDetail(e.Detail)
		if len(lines) == 0 {
			// An event with no rendered breaches still deserves a body: the
			// message column always carries the summary.
			lines = []string{e.Message}
		}
		return lines, nil
	}
	return nil, nil
}

// linesFromDetail pulls the rendered sentences out of an event's curated
// detail (R224). The evaluation stored them under "lines" precisely so the
// notifier does not have to re-derive them from thresholds it no longer has
// the readings for — the mail must say what the event said, not what a second
// evaluation would say now.
func linesFromDetail(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var d struct {
		Lines []string `json:"lines"`
	}
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil // a detail we cannot read is not worth failing delivery over
	}
	return d.Lines
}

// eventLookupLimit bounds the scan for one event id. Events are ordered newest
// first, and a notification is enqueued the moment its event is written, so
// the one being delivered is always near the front.
const eventLookupLimit int32 = 200

// analyzerLabel is the installation number when the event names an analyzer,
// and the rule's own name when it does not.
func analyzerLabel(rule Rule, analyzerID *uuid.UUID) string {
	if analyzerID != nil {
		for _, a := range rule.Analyzers {
			if a.ID == *analyzerID {
				return a.InstallationNumber
			}
		}
	}
	return rule.Alarm.Name
}

// markUnsent records the undelivered event on itself and in Messages, then
// returns nil (R222). The cause is scrubbed: an SMTP reply can quote the credentials
// it rejected, and both sinks are operator-facing and exported.
func (s *Service) markUnsent(ctx context.Context, sc store.Scope, n Notification, status, fired, reason string, cause error, hide ...string) error {
	text := reason
	if cause != nil {
		text = reason + ": " + secret.Redact(cause.Error(), hide)
	}
	// A zero notified_at with a non-nil error is the "fired and nobody was
	// told" state model.AlarmEvent documents.
	if err := s.d.Alarms.MarkNotified(ctx, sc, n.EventID, time.Time{}, &text); err != nil {
		return err // could not even record it: retry
	}
	s.appendMessage(ctx, sc, n, status, fired+" Bildirim gönderilmedi: "+text+".")
	return nil
}

// appendMessage writes one Messages row. A failure here is logged into the
// returned error of nothing: the notification itself already happened, and
// failing the task would re-send the mail.
func (s *Service) appendMessage(ctx context.Context, sc store.Scope, n Notification, status, text string) {
	companyID := n.CompanyID
	related := "alarm"
	alarmID := n.AlarmID
	_, _ = s.nd.Ops.AppendMessage(ctx, sc, model.OperationalMessage{
		CompanyID: &companyID, Kind: "alarm", Category: messageCategory, Status: status,
		Message: text, RelatedType: &related, RelatedID: &alarmID, CreatedAt: s.d.Clock.Now(),
	})
}
