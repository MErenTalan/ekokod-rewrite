//go:build integration

package solar_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/integration/isolar"
	"github.com/MErenTalan/ekokod-rewrite/internal/mail"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/solar"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/admin"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

const smtpPassword = "s3cret-smtp-pass"

type fakeSMTP struct {
	store.SMTPRepository
	missing bool
}

func (f fakeSMTP) Get(context.Context, store.Scope) (model.SMTPSettings, error) {
	if f.missing {
		return model.SMTPSettings{}, store.ErrNotFound
	}
	return model.SMTPSettings{Host: "smtp.test", Port: 587, Username: "u", FromAddress: "noreply@test"}, nil
}
func (f fakeSMTP) OpenPassword(context.Context, store.Scope) ([]byte, error) {
	return []byte(smtpPassword), nil
}

type recordingSender struct {
	sent []mail.Message
	fail error
}

func (r *recordingSender) Send(_ context.Context, _ model.SMTPSettings, _ []byte, m mail.Message) error {
	if r.fail != nil {
		return r.fail
	}
	r.sent = append(r.sent, m)
	return nil
}

func faultHarness(t *testing.T, sender *recordingSender, smtp fakeSMTP, recipients ...string) (*harness, *fakeISolar) {
	t.Helper()
	fake := inverterFake()
	h := newHarness(t, fake, okOpener())
	psID := *h.plant.IsolarPsID
	level, typ := int32(1), int32(1)
	dev := "INV-1"
	fake.faults = []isolar.Fault{
		{Ref: psID + "||10|20260310090000", PSID: psID, Code: "10", Message: "电网掉电", OccurredAt: at(10, 9), Level: &level, Type: &typ, DeviceName: &dev},
		{Ref: psID + "||501|20260310100000", PSID: psID, Code: "501", Message: "Unknown thing", OccurredAt: at(10, 10)},
		{Ref: "elsewhere||10|20260310100000", PSID: "elsewhere", Code: "10", Message: "电网掉电", OccurredAt: at(10, 10)},
	}
	if len(recipients) > 0 {
		_, err := postgres.NewPlantRepository(h.pool).ReplaceAlarmRecipients(context.Background(), h.sc, h.plant.ID, recipients)
		require.NoError(t, err)
	}
	h.svc = solar.New(solar.Deps{
		Plants: postgres.NewPlantRepository(h.pool), Production: postgres.NewProductionRepository(h.pool),
		Totals: postgres.NewProductionTotalsRepository(h.pool), Faults: postgres.NewFaultRepository(h.pool),
		Ops: postgres.NewOpsRepository(h.pool), AdminSolar: admin.NewSolarRepository(h.pool),
		SMTP: smtp, Mail: sender, Creds: okOpener(), ISolar: fake, Clock: clock.NewFake(syncNow), Enqueuer: h.enq, Inspector: noInspector{},
	})
	return h, fake
}

func (h *harness) messages(t *testing.T) []model.OperationalMessage {
	t.Helper()
	msgs, err := postgres.NewOpsRepository(h.pool).ListMessages(context.Background(), h.sc, store.MessageFilter{Page: store.Page{Limit: 100}})
	require.NoError(t, err)
	return msgs
}

func TestForwardOnceAcrossRuns(t *testing.T) {
	t.Parallel()
	sender := &recordingSender{}
	h, _ := faultHarness(t, sender, fakeSMTP{}, "ops@example.test")
	ctx := context.Background()
	require.NoError(t, h.svc.FetchAlarms(ctx))
	require.NoError(t, h.svc.FetchAlarms(ctx))

	require.Len(t, sender.sent, 1, "R287: each fault is mailed once across repeated runs")
	m := sender.sent[0]
	require.Equal(t, []string{"ops@example.test"}, m.To)
	require.Contains(t, m.Subject, "2 yeni alarm")
	require.Contains(t, m.Text, "[Arıza - Kritik] INV-1 cihazında şebeke kesintisi hatası oluştu.")
	require.Contains(t, m.Text, "Unknown thing", "an untranslated fault keeps its text")
	require.Contains(t, m.Text, "10.03.2026 09:00", "Istanbul time")

	faults, total, err := postgres.NewFaultRepository(h.pool).List(ctx, h.sc, h.plant.ID, store.Page{Limit: 10})
	require.NoError(t, err)
	require.Equal(t, 2, total, "the fault of an unknown plant is not stored")
	require.Equal(t, "501", faults[0].Code, "newest first")
}

func TestForwardFailureReleasesClaims(t *testing.T) {
	t.Parallel()
	sender := &recordingSender{fail: errors.New("dial smtp.test: " + smtpPassword + " refused")}
	h, _ := faultHarness(t, sender, fakeSMTP{}, "ops@example.test")
	ctx := context.Background()
	require.NoError(t, h.svc.FetchAlarms(ctx), "a failed send is recorded, not retried by asynq")
	var failed *model.OperationalMessage
	for _, m := range h.messages(t) {
		if m.Category == "isolar-alarm" && m.Status == "error" {
			failed = &m
		}
	}
	require.NotNil(t, failed, "the failure is visible on Messages")
	require.NotContains(t, failed.Message, smtpPassword, "never leaks the SMTP password")

	sender.fail = nil
	require.NoError(t, h.svc.FetchAlarms(ctx))
	require.Len(t, sender.sent, 1, "released claims are retried by the next run")
}

func TestForwardNoRecipientsClaimsNothing(t *testing.T) {
	t.Parallel()
	sender := &recordingSender{}
	h, _ := faultHarness(t, sender, fakeSMTP{})
	ctx := context.Background()
	require.NoError(t, h.svc.FetchAlarms(ctx))
	require.Empty(t, sender.sent)

	_, err := postgres.NewPlantRepository(h.pool).ReplaceAlarmRecipients(ctx, h.sc, h.plant.ID, []string{"late@example.test"})
	require.NoError(t, err)
	require.NoError(t, h.svc.FetchAlarms(ctx))
	require.Len(t, sender.sent, 1, "nothing was claimed while there was nobody to tell")
}

func TestForwardWithoutSMTPIsRecordedAndRetried(t *testing.T) {
	t.Parallel()
	sender := &recordingSender{}
	h, _ := faultHarness(t, sender, fakeSMTP{missing: true}, "ops@example.test")
	require.NoError(t, h.svc.FetchAlarms(context.Background()))
	require.Empty(t, sender.sent)
	found := false
	for _, m := range h.messages(t) {
		found = found || (m.Category == "isolar-alarm" && strings.Contains(m.Message, "SMTP"))
	}
	require.True(t, found)
	unforwarded, err := postgres.NewFaultRepository(h.pool).Unforwarded(context.Background(), h.sc, h.plant.ID, at(1, 0))
	require.NoError(t, err)
	require.Len(t, unforwarded, 2, "claims were released")
}

func TestFaultForAnotherCompanysPlantIsIgnored(t *testing.T) {
	t.Parallel()
	sender := &recordingSender{}
	h, fake := faultHarness(t, sender, fakeSMTP{}, "ops@example.test")
	other := testfixtures.NewTenant(t, context.Background(), h.pool, 2)
	otherPs := *other.Plants[0].IsolarPsID
	fake.faults = []isolar.Fault{{Ref: otherPs + "||1|x", PSID: otherPs, Code: "1", Message: "x", OccurredAt: at(10, 9)}}
	require.NoError(t, h.svc.FetchAlarms(context.Background()))
	_, total, err := postgres.NewFaultRepository(h.pool).List(context.Background(), other.AdminScope, other.Plants[0].ID, store.Page{Limit: 10})
	require.NoError(t, err)
	require.Zero(t, total, "a credential's answer never lands on another company's plant")
	require.Empty(t, sender.sent)
}

var _ = time.Second
