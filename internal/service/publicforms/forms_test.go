package publicforms_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/mail"
	perr "github.com/MErenTalan/ekokod-rewrite/internal/platform/errors"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/publicforms"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

type fakeSMTP struct {
	store.SMTPRepository
	company uuid.UUID
}

func (f fakeSMTP) Get(_ context.Context, sc store.Scope) (model.SMTPSettings, error) {
	if sc.CompanyID != f.company {
		return model.SMTPSettings{}, store.ErrNotFound
	}
	return model.SMTPSettings{Host: "smtp.ornek.test", Port: 587}, nil
}

func (f fakeSMTP) OpenPassword(context.Context, store.Scope) ([]byte, error) {
	return []byte("secret"), nil
}

type fakeMail struct {
	sent []mail.Message
	err  error
}

func (f *fakeMail) Send(_ context.Context, _ model.SMTPSettings, _ []byte, m mail.Message) error {
	if f.err != nil {
		return f.err
	}
	f.sent = append(f.sent, m)
	return nil
}

var company = uuid.New()

func svc(m *fakeMail, to string) *publicforms.Service {
	return publicforms.New(publicforms.Deps{SMTP: fakeSMTP{company: company}, Mail: m, CompanyID: company, To: to})
}

func contact() publicforms.Contact {
	return publicforms.Contact{Name: "Ayşe Kaya", Email: "ayse@ornek.com.tr", Phone: "0532 000 00 00", Subject: "Teklif",
		Message: "Fabrikamız için enerji izleme hakkında bilgi almak istiyoruz.", ElapsedMs: 8000}
}

func requireField(t *testing.T, err error, field string) {
	t.Helper()
	var pe *perr.Error
	require.True(t, errors.As(err, &pe), "want validation on %s, got %v", field, err)
	require.Contains(t, pe.Params, field)
}

func TestContactIsMailedToTheOperator(t *testing.T) {
	m := &fakeMail{}
	require.NoError(t, svc(m, "satis@ekokod.test").SubmitContact(context.Background(), contact()))
	require.Len(t, m.sent, 1)
	msg := m.sent[0]
	require.Equal(t, []string{"satis@ekokod.test"}, msg.To)
	require.Equal(t, "Web sitesi iletişim formu — Ayşe Kaya", msg.Subject)
	require.Contains(t, msg.Text, "ayse@ornek.com.tr")
	require.Contains(t, msg.Text, "enerji izleme")
}

func TestDemoRequestIsMailed(t *testing.T) {
	m := &fakeMail{}
	err := svc(m, "satis@ekokod.test").SubmitDemo(context.Background(), publicforms.Demo{Name: "Can Demir", Email: "can@fabrika.test",
		Phone: "0212 000 00 00", Company: "Demir Tekstil", Role: "Enerji yöneticisi", ElapsedMs: 5000})
	require.NoError(t, err)
	require.Equal(t, "Demo talebi — Demir Tekstil", m.sent[0].Subject)
}

func TestBotsSeeSuccessButNothingIsSent(t *testing.T) {
	m := &fakeMail{}
	honeypot := contact()
	honeypot.Website = "http://spam.test"
	require.NoError(t, svc(m, "satis@ekokod.test").SubmitContact(context.Background(), honeypot))
	fast := contact()
	fast.ElapsedMs = 400
	require.NoError(t, svc(m, "satis@ekokod.test").SubmitContact(context.Background(), fast))
	require.Empty(t, m.sent, "R353: spam answers like success and sends nothing")
}

func TestContactValidation(t *testing.T) {
	m := &fakeMail{}
	s := svc(m, "satis@ekokod.test")
	for field, mutate := range map[string]func(*publicforms.Contact){
		"name":    func(c *publicforms.Contact) { c.Name = " " },
		"email":   func(c *publicforms.Contact) { c.Email = "not-an-email" },
		"message": func(c *publicforms.Contact) { c.Message = "kısa" },
		"subject": func(c *publicforms.Contact) { c.Subject = strings.Repeat("a", 201) },
		"phone":   func(c *publicforms.Contact) { c.Phone = strings.Repeat("1", 41) },
	} {
		c := contact()
		mutate(&c)
		requireField(t, s.SubmitContact(context.Background(), c), field)
	}
	c := contact()
	c.Email = "a@b.test\r\nBcc: x@y.test"
	requireField(t, s.SubmitContact(context.Background(), c), "email")
	require.Empty(t, m.sent)
}

func TestNotConfiguredAndDeliveryFailure(t *testing.T) {
	err := publicforms.New(publicforms.Deps{}).SubmitContact(context.Background(), contact())
	require.ErrorIs(t, err, publicforms.ErrNotConfigured)
	m := &fakeMail{err: errors.New("dial tcp: refused")}
	err = svc(m, "satis@ekokod.test").SubmitContact(context.Background(), contact())
	require.ErrorIs(t, err, publicforms.ErrDeliveryFailed)
	require.NotContains(t, err.Error(), "secret")
}
