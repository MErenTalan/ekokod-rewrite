// Package tenancy manages companies, users and SMTP settings (05 §3,
// R172–R174).
package tenancy

import (
	"errors"

	"github.com/MErenTalan/ekokod-rewrite/internal/auth"
	"github.com/MErenTalan/ekokod-rewrite/internal/mail"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/clock"
	perr "github.com/MErenTalan/ekokod-rewrite/internal/platform/errors"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// Errors specific to tenancy.
var (
	ErrCompanyNameTaken    = perr.New("company_name_taken", 409, "errors.companies.nameTaken")
	ErrCannotDeleteOwn     = perr.New("cannot_delete_own_company", 409, "errors.companies.cannotDeleteOwn")
	ErrCannotModifySelf    = perr.New("cannot_modify_self", 409, "errors.users.cannotModifySelf")
	ErrEmailTaken          = perr.New("email_taken", 409, "errors.users.emailTaken")
	ErrSMTPTestFailed      = perr.New("smtp_test_failed", 422, "errors.smtp.testFailed")
	errRoleNotAssignable   = "not_assignable"
	errPasswordRequiredKey = "required"
)

// Deps is everything Service needs.
type Deps struct {
	Companies   store.CompanyRepository
	AdminTenant store.AdminTenantRepository
	Users       store.UserRepository
	Sessions    store.SessionRepository
	Analyzers   store.AnalyzerRepository
	SMTP        store.SMTPRepository
	Mail        mail.Sender
	Hasher      auth.Hasher
	Clock       clock.Clock
}

// Service implements tenancy management.
type Service struct{ d Deps }

// New validates deps.
func New(d Deps) (*Service, error) {
	if d.Companies == nil || d.AdminTenant == nil || d.Users == nil || d.Sessions == nil || d.Analyzers == nil ||
		d.SMTP == nil || d.Mail == nil || d.Clock == nil || len(d.Hasher.Pepper) == 0 {
		return nil, errors.New("tenancy: missing dependency")
	}
	return &Service{d: d}, nil
}

func validation(field string, codes ...string) error {
	return perr.Validation.WithParams(map[string]any{field: codes})
}
