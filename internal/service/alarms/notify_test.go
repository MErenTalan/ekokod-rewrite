package alarms_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/mail"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/alarms"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

const smtpPassword = "s3cr3t-pass"

type fakeSender struct {
	sent []mail.Message
	err  error
}

func (f *fakeSender) Send(_ context.Context, _ model.SMTPSettings, _ []byte, m mail.Message) error {
	if f.err != nil {
		return f.err
	}
	f.sent = append(f.sent, m)
	return nil
}

type fakeSMTP struct {
	store.SMTPRepository
	missing bool
}

func (f fakeSMTP) Get(_ context.Context, s store.Scope) (model.SMTPSettings, error) {
	if f.missing {
		return model.SMTPSettings{}, store.ErrNotFound
	}
	return model.SMTPSettings{CompanyID: s.CompanyID, Host: "smtp.example", Port: 587,
		Username: "user", FromAddress: "noreply@example.com"}, nil
}

func (f fakeSMTP) OpenPassword(_ context.Context, _ store.Scope) ([]byte, error) {
	if f.missing {
		return nil, store.ErrNotFound
	}
	return []byte(smtpPassword), nil
}

type fakeOps struct {
	store.OpsRepository
	messages []model.OperationalMessage
}

func (f *fakeOps) AppendMessage(_ context.Context, _ store.Scope, m model.OperationalMessage) (model.OperationalMessage, error) {
	f.messages = append(f.messages, m)
	return m, nil
}

// notifyFakes bundles what a notify test asserts on.
type notifyFakes struct {
	alarms *fakeAlarms
	sender *fakeSender
	ops    *fakeOps
	n      alarms.Notification
}

func (f *notifyFakes) statuses(status string) []model.OperationalMessage {
	var out []model.OperationalMessage
	for _, m := range f.ops.messages {
		if m.Status == status {
			out = append(out, m)
		}
	}
	return out
}

type notifyOption func(*fakeSMTP)

func withoutSMTP() notifyOption { return func(s *fakeSMTP) { s.missing = true } }

// newNotifyService builds a Service that can deliver, with one stored event.
func newNotifyService(t *testing.T, channels []model.AlarmChannel, opts ...notifyOption) (*alarms.Service, *notifyFakes) {
	t.Helper()
	repo := newFakeAlarms()
	repo.visible[analyzerA] = true
	sender := &fakeSender{}
	ops := &fakeOps{}
	smtp := fakeSMTP{}
	for _, o := range opts {
		o(&smtp)
	}
	svc, err := alarms.New(alarms.Deps{
		Alarms:    repo,
		Analyzers: fakeAnalyzers{known: map[uuid.UUID]bool{analyzerA: true}},
		Clock:     clock.NewFake(time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)),
	})
	require.NoError(t, err)
	svc = svc.WithNotify(alarms.NotifyDeps{SMTP: smtp, Mail: sender, Ops: ops})

	in := commsInput("İletişim", analyzerA)
	in.Channels = channels
	rule, err := svc.Create(context.Background(), companyScope, in)
	require.NoError(t, err)

	detail, err := json.Marshal(map[string]any{
		"analyzer": "INST-0000", "alarm_type": "data_communication",
		"lines": []string{"Analizör 6 saattir veri göndermiyor (eşik: 6 saat)"},
	})
	require.NoError(t, err)
	event := model.AlarmEvent{ID: uuid.New(), AlarmID: rule.Alarm.ID, AnalyzerID: &[]uuid.UUID{analyzerA}[0],
		TriggeredAt: time.Date(2026, 9, 18, 11, 0, 0, 0, time.UTC), Message: "İletişim — INST-0000", Detail: detail}
	repo.events[rule.Alarm.ID] = append(repo.events[rule.Alarm.ID], event)

	return svc, &notifyFakes{alarms: repo, sender: sender, ops: ops,
		n: alarms.Notification{CompanyID: companyID, AlarmID: rule.Alarm.ID, EventID: event.ID, AnalyzerID: &[]uuid.UUID{analyzerA}[0]}}
}

func email(target string) model.AlarmChannel {
	return model.AlarmChannel{Channel: model.NotifyChannelEmail, Target: target}
}

func sms(target string) model.AlarmChannel {
	return model.AlarmChannel{Channel: model.NotifyChannelSMS, Target: target}
}

func TestNotifySendsOneMessageToEveryRecipient(t *testing.T) {
	t.Parallel()
	svc, f := newNotifyService(t, []model.AlarmChannel{email("ops@example.com"), email("boss@example.com")})
	require.NoError(t, svc.Notify(context.Background(), f.n))

	require.Len(t, f.sender.sent, 1, "one message addressed to both, not one each")
	require.ElementsMatch(t, []string{"ops@example.com", "boss@example.com"}, f.sender.sent[0].To)
	require.Contains(t, f.sender.sent[0].Text, "6 saat")
	require.NotNil(t, f.alarms.notified[f.n.EventID])
	require.Nil(t, f.alarms.notifyErr[f.n.EventID])
}

func TestNotifyRecordsSMSAsNotSent(t *testing.T) {
	t.Parallel()
	// R211: the numbers are stored, nothing is sent, and Messages says so.
	svc, f := newNotifyService(t, []model.AlarmChannel{email("ops@example.com"), sms("+905551234567")})
	require.NoError(t, svc.Notify(context.Background(), f.n))

	warnings := f.statuses("warning")
	require.Len(t, warnings, 1)
	require.Contains(t, warnings[0].Message, "SMS")
	require.Contains(t, warnings[0].Message, "1")
	// Never the number itself: Messages rows are exported.
	for _, m := range f.ops.messages {
		require.NotContains(t, m.Message, "905551234567")
	}
	// The e-mail still went out; SMS being inactive does not block delivery.
	require.Len(t, f.sender.sent, 1)
	require.NotNil(t, f.alarms.notified[f.n.EventID])
}

func TestNotifyWithOnlySMSTargetsSendsNothingAndSaysSo(t *testing.T) {
	t.Parallel()
	svc, f := newNotifyService(t, []model.AlarmChannel{sms("+905551234567")})
	require.NoError(t, svc.Notify(context.Background(), f.n))

	require.Empty(t, f.sender.sent)
	// The event is NOT delivered: nobody was actually told, and recording it as
	// notified would be the silent lie R211 exists to prevent.
	require.Nil(t, f.alarms.notified[f.n.EventID])
	require.NotNil(t, f.alarms.notifyErr[f.n.EventID])
	require.Contains(t, *f.alarms.notifyErr[f.n.EventID], "SMS")
}

func TestNotifyFailedSendIsRecordedAndDoesNotFailTheJob(t *testing.T) {
	t.Parallel()
	// 09 §F7 acceptance: recorded on the event, surfaced in Messages, job survives.
	svc, f := newNotifyService(t, []model.AlarmChannel{email("ops@example.com")})
	f.sender.err = errors.New("535 authentication failed")

	require.NoError(t, svc.Notify(context.Background(), f.n), "R222")
	require.Nil(t, f.alarms.notified[f.n.EventID])
	recorded := f.alarms.notifyErr[f.n.EventID]
	require.NotNil(t, recorded)
	require.Contains(t, *recorded, "535")
	require.Len(t, f.statuses("error"), 1)
}

func TestNotifyMissingSMTPSettingsIsRecordedNotRetried(t *testing.T) {
	t.Parallel()
	svc, f := newNotifyService(t, []model.AlarmChannel{email("ops@example.com")}, withoutSMTP())
	require.NoError(t, svc.Notify(context.Background(), f.n))
	require.NotNil(t, f.alarms.notifyErr[f.n.EventID])
	require.Contains(t, *f.alarms.notifyErr[f.n.EventID], "SMTP")
	require.Len(t, f.statuses("error"), 1)
}

func TestNotifyNeverLeaksTheSMTPPassword(t *testing.T) {
	t.Parallel()
	svc, f := newNotifyService(t, []model.AlarmChannel{email("ops@example.com")})
	// An SMTP server can quote back what it rejected.
	f.sender.err = errors.New("535 rejected credentials for " + smtpPassword)

	require.NoError(t, svc.Notify(context.Background(), f.n))
	require.NotContains(t, *f.alarms.notifyErr[f.n.EventID], smtpPassword)
	for _, m := range f.ops.messages {
		require.NotContains(t, m.Message, smtpPassword)
	}
}

func TestNotifyRepeatsWhatTheEventSaid(t *testing.T) {
	t.Parallel()
	// The body comes from the event's stored lines, not from a fresh
	// evaluation: the mail must describe what fired, not what is true now.
	svc, f := newNotifyService(t, []model.AlarmChannel{email("ops@example.com")})
	require.NoError(t, svc.Notify(context.Background(), f.n))
	require.Contains(t, f.sender.sent[0].Text, "Analizör 6 saattir veri göndermiyor (eşik: 6 saat)")
}

func TestNotifyIsANoOpWhenTheEventIsGone(t *testing.T) {
	t.Parallel()
	svc, f := newNotifyService(t, []model.AlarmChannel{email("ops@example.com")})
	f.n.EventID = uuid.New()
	require.NoError(t, svc.Notify(context.Background(), f.n))
	require.Empty(t, f.sender.sent)
}

func TestNotifyWithoutDepsIsAnError(t *testing.T) {
	t.Parallel()
	svc, _ := newTestService(t)
	require.Error(t, svc.Notify(context.Background(), alarms.Notification{CompanyID: companyID}))
}
