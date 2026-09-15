package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/crypto"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/internal/pgerr"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/sqlcgen"
)

// SMTPRepository implements store.SMTPRepository.
//
// It seals and opens smtp_settings.password_enc with cipher -- the same
// AES-256-GCM helper (internal/platform/crypto) IntegrationRepository uses
// for integration_credentials. Get never touches cipher at all: it returns
// the ciphertext column exactly as stored, which is what
// TestSMTPPasswordIsNeverReturnedInPlaintext pins. OpenPassword is the one
// method that calls cipher.Open.
//
// smtp_settings.company_id is the table's primary key and is NOT NULL
// (`primary key references companies(id) on delete cascade`), so -- unlike
// integration_credentials or emission_factors -- there is no platform-wide
// (company_id NULL) row here. R7's premise ("if smtp_settings.company_id is
// nullable") does not hold for this schema, so there is no platform row to
// serve and no AdminSMTP... method to add.
type SMTPRepository struct {
	q      *sqlcgen.Queries
	pool   *pgxpool.Pool
	cipher *crypto.Cipher
}

// NewSMTPRepository builds an SMTPRepository on pool, sealing and opening
// passwords with cipher.
func NewSMTPRepository(pool *pgxpool.Pool, cipher *crypto.Cipher) *SMTPRepository {
	return &SMTPRepository{q: sqlcgen.New(pool), pool: pool, cipher: cipher}
}

var _ store.SMTPRepository = (*SMTPRepository)(nil)

// smtpPasswordAAD binds one company's password ciphertext to its own row, so
// a ciphertext copied onto another company's row fails to open. Unlike
// integrationCredentialAAD there is no second key to bind: smtp_settings'
// primary key IS company_id, so the company id alone identifies the row.
func smtpPasswordAAD(companyID uuid.UUID) []byte {
	return []byte(companyID.String())
}

func smtpSettingsFromRow(row sqlcgen.SmtpSetting) model.SMTPSettings {
	return model.SMTPSettings{
		CompanyID: row.CompanyID, Host: row.Host, Port: row.Port, Secure: row.Secure,
		Username: row.Username, PasswordEnc: row.PasswordEnc, FromAddress: row.FromAddress,
		UpdatedAt: row.UpdatedAt.Time,
	}
}

// Get returns s.CompanyID's SMTP settings. PasswordEnc is ciphertext.
func (r *SMTPRepository) Get(ctx context.Context, s store.Scope) (model.SMTPSettings, error) {
	if !s.Valid() {
		return model.SMTPSettings{}, store.ErrInvalidScope
	}
	row, err := r.q.SMTPGet(ctx, s.CompanyID)
	if err != nil {
		return model.SMTPSettings{}, pgerr.Translate(r.pool, "get smtp settings", err)
	}
	return smtpSettingsFromRow(row), nil
}

// Upsert seals password with r.cipher and stores the ciphertext under one
// atomic INSERT ... ON CONFLICT statement (R5). password is a parameter,
// never a struct field, for the same reason as
// IntegrationRepository.UpsertCredential's secret/extra: it cannot be
// accidentally retained, logged or returned.
//
// As with IntegrationRepository.UpsertCredential, a nil or empty password on
// an UPDATE of an existing row (guarded here by the same `len(password) > 0`
// check) leaves the stored password_enc unchanged -- every other field still
// updates. Unlike that method, smtp_settings.password_enc is NOT NULL, so a
// nil/empty password on the FIRST Upsert for a company (no existing row to
// fall back to) is refused: SMTPUpsert's query falls back to the existing
// row's password_enc for both the INSERT target list and the ON CONFLICT
// UPDATE, entirely inside the one statement (see queries/smtp.sql), so a
// first insert with no fallback to fall back to leaves the candidate row's
// password_enc NULL and Postgres itself rejects it with a not-null-violation
// (SQLSTATE 23502) before any conflict is considered. That violation is not
// mapped to a store sentinel -- like 23503/23514 in pgerr, it is the caller
// passing data that violates a constraint it should have enforced first, not
// a conflict worth retrying -- so it comes back as a plain, scrubbed,
// non-sentinel error from pgerr.Translate. No row is written.
//
// settings.CompanyID must equal s.CompanyID: another company's id is refused
// with ErrNotFound before any database call.
func (r *SMTPRepository) Upsert(ctx context.Context, s store.Scope, settings model.SMTPSettings, password []byte) (model.SMTPSettings, error) {
	if !s.Valid() {
		return model.SMTPSettings{}, store.ErrInvalidScope
	}
	if settings.CompanyID != s.CompanyID {
		return model.SMTPSettings{}, store.ErrNotFound
	}
	var passwordEnc []byte
	if len(password) > 0 {
		token, err := r.cipher.Seal(password, smtpPasswordAAD(s.CompanyID))
		if err != nil {
			return model.SMTPSettings{}, fmt.Errorf("seal smtp password: %w", err)
		}
		passwordEnc = []byte(token)
	}
	row, err := r.q.SMTPUpsert(ctx, sqlcgen.SMTPUpsertParams{
		CompanyID: s.CompanyID, Host: settings.Host, Port: settings.Port, Secure: settings.Secure,
		Username: settings.Username, PasswordEnc: passwordEnc, FromAddress: settings.FromAddress,
	})
	if err != nil {
		return model.SMTPSettings{}, pgerr.Translate(r.pool, "upsert smtp settings", err)
	}
	return smtpSettingsFromRow(row), nil
}

// OpenPassword decrypts s.CompanyID's stored password. The only method that
// returns plaintext.
func (r *SMTPRepository) OpenPassword(ctx context.Context, s store.Scope) ([]byte, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	row, err := r.q.SMTPGet(ctx, s.CompanyID)
	if err != nil {
		return nil, pgerr.Translate(r.pool, "get smtp settings", err)
	}
	password, err := r.cipher.Open(string(row.PasswordEnc), smtpPasswordAAD(s.CompanyID))
	if err != nil {
		return nil, fmt.Errorf("open smtp password: %w", err)
	}
	return password, nil
}

// Delete removes s.CompanyID's SMTP settings.
func (r *SMTPRepository) Delete(ctx context.Context, s store.Scope) error {
	if !s.Valid() {
		return store.ErrInvalidScope
	}
	n, err := r.q.SMTPDelete(ctx, s.CompanyID)
	if err != nil {
		return pgerr.Translate(r.pool, "delete smtp settings", err)
	}
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}
