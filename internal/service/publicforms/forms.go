// Package publicforms delivers the public site's contact and demo-request
// forms to the operator by e-mail (05 §17, F12a R353–R356). Nothing is stored.
package publicforms

import (
	"context"
	"fmt"
	"net/mail"
	"strings"

	"github.com/google/uuid"

	outmail "github.com/MErenTalan/ekokod-rewrite/internal/mail"
	perr "github.com/MErenTalan/ekokod-rewrite/internal/platform/errors"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

var (
	// ErrNotConfigured is an install without an operator mailbox (Q-H6).
	ErrNotConfigured = perr.New("forms_not_configured", 503, "errors.public.formsNotConfigured")
	// ErrDeliveryFailed is an SMTP failure; the visitor is told to write or call instead.
	ErrDeliveryFailed = perr.New("delivery_failed", 502, "errors.public.deliveryFailed")
)

// minElapsedMs is the fastest a person fills a form; a bot is quicker (R353).
const minElapsedMs = 2000

// Deps names the operator company whose SMTP settings send the mail, and the recipient.
type Deps struct {
	SMTP      store.SMTPRepository
	Mail      outmail.Sender
	CompanyID uuid.UUID
	To        string
}

// Service is the forms module.
type Service struct{ d Deps }

// New builds the service.
func New(d Deps) *Service { return &Service{d: d} }

// Contact is R353's contact form.
type Contact struct {
	Name, Email, Phone, Subject, Message string
	Website                              string // honeypot
	ElapsedMs                            int
}

// Demo is R353's demo-request form.
type Demo struct {
	Name, Email, Phone, Company, Role, Message string
	Website                                    string
	ElapsedMs                                  int
}

type check struct {
	field, value string
	min, max     int
}

func validate(checks []check, email string) error {
	for _, c := range checks {
		n := len([]rune(strings.TrimSpace(c.value)))
		if n < c.min || n > c.max || strings.ContainsAny(c.value, "\r\n") && c.field != "message" {
			return perr.Validation.WithParams(map[string]any{c.field: []string{"invalid"}})
		}
	}
	addr, err := mail.ParseAddress(strings.TrimSpace(email))
	if err != nil || addr.Address != strings.TrimSpace(email) || strings.ContainsAny(email, "\r\n") {
		return perr.Validation.WithParams(map[string]any{"email": []string{"email"}})
	}
	return nil
}

func spam(website string, elapsed int) bool {
	return strings.TrimSpace(website) != "" || elapsed < minElapsedMs
}

// SubmitContact is R353/R354 for the contact form.
func (s *Service) SubmitContact(ctx context.Context, c Contact) error {
	if err := validate([]check{{"name", c.Name, 1, 120}, {"phone", c.Phone, 0, 40}, {"subject", c.Subject, 0, 200},
		{"message", c.Message, 10, 5000}}, c.Email); err != nil {
		return err
	}
	if spam(c.Website, c.ElapsedMs) {
		return nil
	}
	body := fmt.Sprintf("Web sitesinden yeni bir iletişim mesajı geldi.\n\nAd soyad: %s\nE-posta: %s\nTelefon: %s\nKonu: %s\n\n%s\n",
		strings.TrimSpace(c.Name), strings.TrimSpace(c.Email), orDash(c.Phone), orDash(c.Subject), strings.TrimSpace(c.Message))
	return s.send(ctx, "Web sitesi iletişim formu — "+strings.TrimSpace(c.Name), body)
}

// SubmitDemo is R353/R354 for the demo-request form.
func (s *Service) SubmitDemo(ctx context.Context, d Demo) error {
	if err := validate([]check{{"name", d.Name, 1, 120}, {"phone", d.Phone, 1, 40}, {"company", d.Company, 1, 200},
		{"role", d.Role, 0, 120}, {"message", d.Message, 0, 2000}}, d.Email); err != nil {
		return err
	}
	if spam(d.Website, d.ElapsedMs) {
		return nil
	}
	body := fmt.Sprintf("Web sitesinden yeni bir demo talebi geldi.\n\nAd soyad: %s\nE-posta: %s\nTelefon: %s\nŞirket: %s\nGörev: %s\n\n%s\n",
		strings.TrimSpace(d.Name), strings.TrimSpace(d.Email), strings.TrimSpace(d.Phone), strings.TrimSpace(d.Company), orDash(d.Role), orDash(d.Message))
	return s.send(ctx, "Demo talebi — "+strings.TrimSpace(d.Company), body)
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return strings.TrimSpace(s)
}

func (s *Service) send(ctx context.Context, subject, body string) error {
	if s.d.SMTP == nil || s.d.Mail == nil || s.d.CompanyID == uuid.Nil || s.d.To == "" {
		return ErrNotConfigured
	}
	sc := store.SystemScope(s.d.CompanyID)
	settings, err := s.d.SMTP.Get(ctx, sc)
	if err != nil {
		return ErrNotConfigured
	}
	password, err := s.d.SMTP.OpenPassword(ctx, sc)
	if err != nil {
		return ErrNotConfigured
	}
	if err := s.d.Mail.Send(ctx, settings, password, outmail.Message{To: []string{s.d.To}, Subject: subject, Text: body}); err != nil {
		return ErrDeliveryFailed
	}
	return nil
}
