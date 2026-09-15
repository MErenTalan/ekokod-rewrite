//go:build integration

package postgres_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

func smtpSettingsFixture(companyID uuid.UUID) model.SMTPSettings {
	return model.SMTPSettings{
		CompanyID: companyID, Host: "smtp.example.invalid", Port: 587, Secure: true,
		Username: "notifications", FromAddress: "notifications@example.invalid",
	}
}

func TestSMTPUpsertGetDelete(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenantA := testfixtures.NewTenant(t, ctx, pool, 8100)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 8101)
	repo := postgres.NewSMTPRepository(pool, integrationCipher(t))

	created, err := repo.Upsert(ctx, tenantA.Scope, smtpSettingsFixture(tenantA.Company.ID), []byte("s3cret-pw"))
	require.NoError(t, err)
	require.Equal(t, "smtp.example.invalid", created.Host)
	require.NotEmpty(t, created.PasswordEnc)

	got, err := repo.Get(ctx, tenantA.Scope)
	require.NoError(t, err)
	require.Equal(t, created.Host, got.Host)

	// Another company cannot read, open or delete it: it is
	// indistinguishable from a missing row.
	_, err = repo.Get(ctx, tenantB.Scope)
	require.ErrorIs(t, err, store.ErrNotFound)
	_, err = repo.OpenPassword(ctx, tenantB.Scope)
	require.ErrorIs(t, err, store.ErrNotFound)
	err = repo.Delete(ctx, tenantB.Scope)
	require.ErrorIs(t, err, store.ErrNotFound)

	// Upsert again (same company) updates in place: one row per company.
	changed := smtpSettingsFixture(tenantA.Company.ID)
	changed.Host = "smtp2.example.invalid"
	changed.Port = 25
	updated, err := repo.Upsert(ctx, tenantA.Scope, changed, []byte("new-pw"))
	require.NoError(t, err)
	require.Equal(t, "smtp2.example.invalid", updated.Host)
	require.Equal(t, int32(25), updated.Port)

	password, err := repo.OpenPassword(ctx, tenantA.Scope)
	require.NoError(t, err)
	require.Equal(t, "new-pw", string(password))

	require.NoError(t, repo.Delete(ctx, tenantA.Scope))
	_, err = repo.Get(ctx, tenantA.Scope)
	require.ErrorIs(t, err, store.ErrNotFound)
}

func TestSMTPUpsertRefusesMismatchedCompanyID(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenantA := testfixtures.NewTenant(t, ctx, pool, 8110)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 8111)
	repo := postgres.NewSMTPRepository(pool, integrationCipher(t))

	_, err := repo.Upsert(ctx, tenantA.Scope, smtpSettingsFixture(tenantB.Company.ID), []byte("pw"))
	require.ErrorIs(t, err, store.ErrNotFound)

	_, err = repo.Get(ctx, tenantA.Scope)
	require.ErrorIs(t, err, store.ErrNotFound, "the refused upsert must not have written anything")
}

// TestSMTPPasswordIsNeverReturnedInPlaintext proves the repository returns
// the password only through the explicit OpenPassword method, mirroring
// TestIntegrationCredentialsAreNeverReturnedInPlaintext for
// integration_credentials.
func TestSMTPPasswordIsNeverReturnedInPlaintext(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenantA := testfixtures.NewTenant(t, ctx, pool, 8120)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 8121)
	repo := postgres.NewSMTPRepository(pool, integrationCipher(t))

	const plainPassword = "super-secret-smtp-password"

	created, err := repo.Upsert(ctx, tenantA.Scope, smtpSettingsFixture(tenantA.Company.ID), []byte(plainPassword))
	require.NoError(t, err)

	assertNoPlaintextPassword := func(s model.SMTPSettings) {
		require.NotContains(t, string(s.PasswordEnc), plainPassword)
		require.NotEmpty(t, s.PasswordEnc, "the ciphertext column must actually be populated, or this test is vacuous")
	}
	assertNoPlaintextPassword(created)

	got, err := repo.Get(ctx, tenantA.Scope)
	require.NoError(t, err)
	assertNoPlaintextPassword(got)

	// OpenPassword is the ONLY method that returns plaintext, and it must
	// return exactly what was sealed.
	password, err := repo.OpenPassword(ctx, tenantA.Scope)
	require.NoError(t, err)
	require.Equal(t, plainPassword, string(password))

	// Tenant B has no row of its own: OpenPassword resolves purely by
	// s.CompanyID, so tenant A's ciphertext is never even looked at, let
	// alone opened, on tenant B's behalf.
	_, err = repo.OpenPassword(ctx, tenantB.Scope)
	require.ErrorIs(t, err, store.ErrNotFound)

	// Two companies sealing the same plaintext must not produce the same
	// ciphertext (a fresh nonce every time) — otherwise ciphertext equality
	// would itself leak whether two tenants share a password.
	createdOther, err := repo.Upsert(ctx, tenantB.Scope, smtpSettingsFixture(tenantB.Company.ID), []byte(plainPassword))
	require.NoError(t, err)
	require.NotEqual(t, string(created.PasswordEnc), string(createdOther.PasswordEnc))
}

// TestSMTPPasswordSealingBindsToItsOwnRow proves the AAD binding: a
// ciphertext sealed for one company does not open under another company's
// row, even with the same cipher.
func TestSMTPPasswordSealingBindsToItsOwnRow(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	cipher := integrationCipher(t)
	tenantA := testfixtures.NewTenant(t, ctx, pool, 8130)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 8131)
	repoA := postgres.NewSMTPRepository(pool, cipher)

	credA, err := repoA.Upsert(ctx, tenantA.Scope, smtpSettingsFixture(tenantA.Company.ID), []byte("secret-a"))
	require.NoError(t, err)

	// Insert a row for tenant B directly, carrying tenant A's ciphertext,
	// and confirm OpenPassword on THAT row fails to decrypt: the AAD binds
	// ciphertext to company_id.
	_, err = pool.Exec(ctx,
		`insert into smtp_settings (company_id, host, port, secure, username, password_enc, from_address)
		 values ($1, 'smtp.example.invalid', 587, true, 'n', $2, 'n@example.invalid')`,
		tenantB.Company.ID, credA.PasswordEnc)
	require.NoError(t, err)

	_, err = repoA.OpenPassword(ctx, tenantB.Scope)
	require.Error(t, err, "a ciphertext copied onto another company's row must not decrypt")
}

func TestSMTPRepositoryRejectsInvalidScope(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	repo := postgres.NewSMTPRepository(pool, integrationCipher(t))
	var invalid store.Scope

	_, err := repo.Get(ctx, invalid)
	require.ErrorIs(t, err, store.ErrInvalidScope)
	_, err = repo.Upsert(ctx, invalid, model.SMTPSettings{}, []byte("pw"))
	require.ErrorIs(t, err, store.ErrInvalidScope)
	_, err = repo.OpenPassword(ctx, invalid)
	require.ErrorIs(t, err, store.ErrInvalidScope)
	err = repo.Delete(ctx, invalid)
	require.ErrorIs(t, err, store.ErrInvalidScope)
}
