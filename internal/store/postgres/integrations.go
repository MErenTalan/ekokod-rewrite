package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/crypto"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/internal/pgerr"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/sqlcgen"
)

// IntegrationRepository implements store.IntegrationRepository.
//
// It seals and opens integration_credentials.secret_enc / extra_enc with
// cipher (internal/platform/crypto's AES-256-GCM helpers). Get and List never
// touch cipher at all: they return the ciphertext columns exactly as stored,
// which is what TestIntegrationCredentialsAreNeverReturnedInPlaintext pins.
// OpenSecret is the one method that calls cipher.Open.
type IntegrationRepository struct {
	q      *sqlcgen.Queries
	pool   *pgxpool.Pool
	cipher *crypto.Cipher
}

// NewIntegrationRepository builds an IntegrationRepository on pool, sealing
// and opening secrets with cipher. cipher must not be nil: a nil cipher
// would panic later, at the first Seal/Open call, with a confusing
// nil-pointer trace far from the real mistake — refusing it here, at
// construction, is a programmer error surfaced immediately and clearly
// (fix round 1, folded minor).
func NewIntegrationRepository(pool *pgxpool.Pool, cipher *crypto.Cipher) *IntegrationRepository {
	if cipher == nil {
		panic("postgres.NewIntegrationRepository: cipher must not be nil")
	}
	return &IntegrationRepository{q: sqlcgen.New(pool), pool: pool, cipher: cipher}
}

var _ store.IntegrationRepository = (*IntegrationRepository)(nil)

// integrationCredentialAAD binds a credential's ciphertext to its own (company,
// definition) pair, so a ciphertext copied onto another row fails to open.
func integrationCredentialAAD(companyID, definitionID uuid.UUID) []byte {
	return []byte(companyID.String() + ":" + definitionID.String())
}

// IntegrationCredentialAAD is the same binding, for the legacy migration, which
// re-seals credentials before they reach this repository (F14a Q-J3).
func IntegrationCredentialAAD(companyID, definitionID uuid.UUID) []byte {
	return integrationCredentialAAD(companyID, definitionID)
}

func integrationDefinitionFromRow(row sqlcgen.IntegrationDefinition) model.IntegrationDefinition {
	return model.IntegrationDefinition{
		ID: row.ID, Provider: model.IntegrationProvider(row.Provider), Subtype: row.Subtype,
		Endpoints: row.Endpoints, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
}

func integrationCredentialFromRow(row sqlcgen.IntegrationCredential) model.IntegrationCredential {
	var tokenExpiresAt, lastVerifiedAt *time.Time
	if row.TokenExpiresAt.Valid {
		t := row.TokenExpiresAt.Time
		tokenExpiresAt = &t
	}
	if row.LastVerifiedAt.Valid {
		t := row.LastVerifiedAt.Time
		lastVerifiedAt = &t
	}
	return model.IntegrationCredential{
		ID: row.ID, CompanyID: row.CompanyID, DefinitionID: row.DefinitionID,
		Username: row.Username, SecretEnc: row.SecretEnc, ExtraEnc: row.ExtraEnc, Settings: row.Settings,
		Pm5340URL: row.Pm5340Url, IsolarRegion: row.IsolarRegion,
		TokenExpiresAt: tokenExpiresAt, LastVerifiedAt: lastVerifiedAt, IsActive: row.IsActive,
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
}

// Definitions is a tenant's read of the platform catalogue: no company_id, so
// the Scope narrows nothing but is still validated.
func (r *IntegrationRepository) Definitions(ctx context.Context, s store.Scope) ([]model.IntegrationDefinition, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	rows, err := r.q.IntegrationListDefinitions(ctx)
	if err != nil {
		return nil, pgerr.Translate(r.pool, "list integration definitions", err)
	}
	out := make([]model.IntegrationDefinition, 0, len(rows))
	for _, row := range rows {
		out = append(out, integrationDefinitionFromRow(row))
	}
	return out, nil
}

// Definition is a tenant's read of one platform catalogue entry.
func (r *IntegrationRepository) Definition(ctx context.Context, s store.Scope, provider model.IntegrationProvider, subtype string) (model.IntegrationDefinition, error) {
	if !s.Valid() {
		return model.IntegrationDefinition{}, store.ErrInvalidScope
	}
	row, err := r.q.IntegrationGetDefinition(ctx, sqlcgen.IntegrationGetDefinitionParams{
		Provider: sqlcgen.IntegrationProvider(provider), Subtype: subtype,
	})
	if err != nil {
		return model.IntegrationDefinition{}, pgerr.Translate(r.pool, "get integration definition", err)
	}
	return integrationDefinitionFromRow(row), nil
}

// Credential returns s.CompanyID's one credential for definitionID:
// integration_credentials(company_id, definition_id) is unique, so this is a
// lookup by definition, not by the credential's own id (that is
// DeleteCredential/OpenSecret's credentialID).
func (r *IntegrationRepository) Credential(ctx context.Context, s store.Scope, definitionID uuid.UUID) (model.IntegrationCredential, error) {
	if !s.Valid() {
		return model.IntegrationCredential{}, store.ErrInvalidScope
	}
	row, err := r.q.IntegrationGetCredentialByDefinition(ctx, sqlcgen.IntegrationGetCredentialByDefinitionParams{
		DefinitionID: definitionID, CompanyID: s.CompanyID,
	})
	if err != nil {
		return model.IntegrationCredential{}, pgerr.Translate(r.pool, "get integration credential", err)
	}
	return integrationCredentialFromRow(row), nil
}

// ListCredentials returns every credential ciphertext row for s.CompanyID.
func (r *IntegrationRepository) ListCredentials(ctx context.Context, s store.Scope) ([]model.IntegrationCredential, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	rows, err := r.q.IntegrationListCredentials(ctx, s.CompanyID)
	if err != nil {
		return nil, pgerr.Translate(r.pool, "list integration credentials", err)
	}
	out := make([]model.IntegrationCredential, 0, len(rows))
	for _, row := range rows {
		out = append(out, integrationCredentialFromRow(row))
	}
	return out, nil
}

// UpsertCredential seals secret and extra with r.cipher and stores the
// ciphertext. secret and extra are parameters, never struct fields, so they
// cannot be accidentally retained, logged or returned.
//
// A nil (empty) secret or extra on an UPDATE (an existing row for this
// (company_id, definition_id)) leaves that column's stored ciphertext
// UNCHANGED — this is how a caller updates username/settings/is_active alone
// without re-supplying the secret. The SQL's own
// `coalesce(excluded.x, x)` makes this atomic with the write (fix round 1,
// folded minor); to actually clear a secret, DeleteCredential and
// UpsertCredential again.
func (r *IntegrationRepository) UpsertCredential(ctx context.Context, s store.Scope, c model.IntegrationCredential, secret, extra []byte) (model.IntegrationCredential, error) {
	if !s.Valid() {
		return model.IntegrationCredential{}, store.ErrInvalidScope
	}
	if c.CompanyID != s.CompanyID {
		return model.IntegrationCredential{}, store.ErrNotFound
	}
	aad := integrationCredentialAAD(s.CompanyID, c.DefinitionID)
	var secretEnc, extraEnc []byte
	if len(secret) > 0 {
		token, err := r.cipher.Seal(secret, aad)
		if err != nil {
			return model.IntegrationCredential{}, fmt.Errorf("seal integration secret: %w", err)
		}
		secretEnc = []byte(token)
	}
	if len(extra) > 0 {
		token, err := r.cipher.Seal(extra, aad)
		if err != nil {
			return model.IntegrationCredential{}, fmt.Errorf("seal integration extra: %w", err)
		}
		extraEnc = []byte(token)
	}
	settings := c.Settings
	if settings == nil {
		settings = []byte("{}")
	}
	row, err := r.q.IntegrationUpsertCredential(ctx, sqlcgen.IntegrationUpsertCredentialParams{
		ID: c.ID, CompanyID: s.CompanyID, DefinitionID: c.DefinitionID, Username: c.Username,
		SecretEnc: secretEnc, ExtraEnc: extraEnc, Settings: settings,
		Pm5340Url: c.Pm5340URL, IsolarRegion: c.IsolarRegion, IsActive: c.IsActive,
	})
	if err != nil {
		return model.IntegrationCredential{}, pgerr.Translate(r.pool, "upsert integration credential", err)
	}
	return integrationCredentialFromRow(row), nil
}

// OpenSecret decrypts one credential's secrets. It is the ONLY method that
// returns plaintext.
func (r *IntegrationRepository) OpenSecret(ctx context.Context, s store.Scope, credentialID uuid.UUID) (secret, extra []byte, err error) {
	if !s.Valid() {
		return nil, nil, store.ErrInvalidScope
	}
	row, err := r.q.IntegrationGetCredential(ctx, sqlcgen.IntegrationGetCredentialParams{ID: credentialID, CompanyID: s.CompanyID})
	if err != nil {
		return nil, nil, pgerr.Translate(r.pool, "get integration credential", err)
	}
	aad := integrationCredentialAAD(s.CompanyID, row.DefinitionID)
	if len(row.SecretEnc) > 0 {
		secret, err = r.cipher.Open(string(row.SecretEnc), aad)
		if err != nil {
			return nil, nil, fmt.Errorf("open integration secret: %w", err)
		}
	}
	if len(row.ExtraEnc) > 0 {
		extra, err = r.cipher.Open(string(row.ExtraEnc), aad)
		if err != nil {
			return nil, nil, fmt.Errorf("open integration extra: %w", err)
		}
	}
	return secret, extra, nil
}

// DeleteCredential removes one credential by id, scoped to s.
func (r *IntegrationRepository) DeleteCredential(ctx context.Context, s store.Scope, credentialID uuid.UUID) error {
	if !s.Valid() {
		return store.ErrInvalidScope
	}
	n, err := r.q.IntegrationDeleteCredential(ctx, sqlcgen.IntegrationDeleteCredentialParams{ID: credentialID, CompanyID: s.CompanyID})
	if err != nil {
		return pgerr.Translate(r.pool, "delete integration credential", err)
	}
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

// RecordVerification stamps last_verified_at and token_expires_at after a
// successful provider handshake.
func (r *IntegrationRepository) RecordVerification(ctx context.Context, s store.Scope, credentialID uuid.UUID, verifiedAt time.Time, tokenExpiresAt *time.Time) error {
	if !s.Valid() {
		return store.ErrInvalidScope
	}
	n, err := r.q.IntegrationRecordVerification(ctx, sqlcgen.IntegrationRecordVerificationParams{
		VerifiedAt: pgtype.Timestamptz{Time: verifiedAt, Valid: true}, TokenExpiresAt: integrationTimestamptzParam(tokenExpiresAt),
		ID: credentialID, CompanyID: s.CompanyID,
	})
	if err != nil {
		return pgerr.Translate(r.pool, "record integration verification", err)
	}
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

func integrationTimestamptzParam(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}
