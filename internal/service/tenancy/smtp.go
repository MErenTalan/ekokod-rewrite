package tenancy

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/mail"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/secret"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// SMTPInput is PUT /smtp-settings; an empty Password keeps the stored one (R172).
type SMTPInput struct {
	Host        string
	Port        int32
	Secure      bool
	Username    string
	FromAddress string
	Password    string
}

// GetSMTP returns the scope company's SMTP settings (never the password).
func (s *Service) GetSMTP(ctx context.Context, sc store.Scope) (model.SMTPSettings, error) {
	return s.d.SMTP.Get(ctx, sc)
}

// PutSMTP upserts the settings; the first save needs a password.
func (s *Service) PutSMTP(ctx context.Context, sc store.Scope, in SMTPInput) (model.SMTPSettings, error) {
	if in.Password == "" {
		if _, err := s.d.SMTP.Get(ctx, sc); errors.Is(err, store.ErrNotFound) {
			return model.SMTPSettings{}, validation("password", errPasswordRequiredKey)
		} else if err != nil {
			return model.SMTPSettings{}, err
		}
	}
	var password []byte
	if in.Password != "" {
		password = []byte(in.Password)
	}
	return s.d.SMTP.Upsert(ctx, sc, model.SMTPSettings{
		CompanyID: sc.CompanyID, Host: strings.TrimSpace(in.Host), Port: in.Port, Secure: in.Secure,
		Username: in.Username, FromAddress: strings.TrimSpace(in.FromAddress), UpdatedAt: s.d.Clock.Now(),
	}, password)
}

const smtpTestTimeout = 20 * time.Second

// TestSMTP sends the fixed test message; a failure is 422 smtp_test_failed with
// the password redacted from the reason (R172).
func (s *Service) TestSMTP(ctx context.Context, sc store.Scope, to, locale string) error {
	settings, err := s.d.SMTP.Get(ctx, sc)
	if err != nil {
		return err
	}
	password, err := s.d.SMTP.OpenPassword(ctx, sc)
	if err != nil {
		return err
	}
	msg := mail.SMTPTest(locale)
	msg.To = []string{to}
	ctx, cancel := context.WithTimeout(ctx, smtpTestTimeout)
	defer cancel()
	if err := s.d.Mail.Send(ctx, settings, password, msg); err != nil {
		reason := secret.Redact(err.Error(), secret.Fragments(string(password)))
		return ErrSMTPTestFailed.WithParams(map[string]any{"reason": reason})
	}
	return nil
}
