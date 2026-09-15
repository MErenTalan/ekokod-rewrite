//go:build integration

package postgres_test

import (
	"context"
	"crypto/rand"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/platform/crypto"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

func integrationCipher(t *testing.T) *crypto.Cipher {
	t.Helper()
	key := make([]byte, 32)
	_, err := rand.Read(key)
	require.NoError(t, err)
	c, err := crypto.NewCipher(key)
	require.NoError(t, err)
	return c
}

func integrationInsertDefinition(t *testing.T, ctx context.Context, pool *pgxpool.Pool, provider, subtype string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	require.NoError(t, pool.QueryRow(ctx,
		`insert into integration_definitions (provider, subtype) values ($1, $2) returning id`,
		provider, subtype).Scan(&id))
	return id
}

func TestIntegrationDefinitionsArePlatformWideButScopeValidated(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenant := testfixtures.NewTenant(t, ctx, pool, 7001)
	repo := postgres.NewIntegrationRepository(pool, integrationCipher(t))

	definitionID := integrationInsertDefinition(t, ctx, pool, "osos", "default")

	defs, err := repo.Definitions(ctx, tenant.Scope)
	require.NoError(t, err)
	require.Len(t, defs, 1)
	require.Equal(t, definitionID, defs[0].ID)

	got, err := repo.Definition(ctx, tenant.Scope, model.IntegrationProviderOSOS, "default")
	require.NoError(t, err)
	require.Equal(t, definitionID, got.ID)

	// The Scope is still validated even though it narrows nothing.
	var invalid store.Scope
	_, err = repo.Definitions(ctx, invalid)
	require.ErrorIs(t, err, store.ErrInvalidScope)
	_, err = repo.Definition(ctx, invalid, model.IntegrationProviderOSOS, "default")
	require.ErrorIs(t, err, store.ErrInvalidScope)
}

// TestIntegrationCredentialsAreNeverReturnedInPlaintext proves the repository
// returns ciphertext or a decrypted value only through the explicit
// OpenSecret method, and that Get/List never carry the secret.
func TestIntegrationCredentialsAreNeverReturnedInPlaintext(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenant := testfixtures.NewTenant(t, ctx, pool, 7010)
	repo := postgres.NewIntegrationRepository(pool, integrationCipher(t))

	definitionID := integrationInsertDefinition(t, ctx, pool, "gridbox", "default")

	const plainSecret = "super-secret-api-key"
	const plainExtra = "extra-secret-token"

	created, err := repo.UpsertCredential(ctx, tenant.Scope, model.IntegrationCredential{
		CompanyID: tenant.Company.ID, DefinitionID: definitionID, IsActive: true,
	}, []byte(plainSecret), []byte(plainExtra))
	require.NoError(t, err)

	// The ordinary Get/List path never carries plaintext: SecretEnc/ExtraEnc
	// are ciphertext, and the model has no plaintext field at all to check
	// against — the only thing this test CAN assert is that the plaintext
	// string never appears anywhere in the ciphertext bytes or in the
	// struct's other fields.
	assertNoPlaintext := func(c model.IntegrationCredential) {
		require.NotContains(t, string(c.SecretEnc), plainSecret)
		require.NotContains(t, string(c.ExtraEnc), plainExtra)
		require.NotEmpty(t, c.SecretEnc, "the ciphertext column must actually be populated, or this test is vacuous")
		require.NotEmpty(t, c.ExtraEnc)
	}
	assertNoPlaintext(created)

	got, err := repo.Credential(ctx, tenant.Scope, definitionID)
	require.NoError(t, err)
	assertNoPlaintext(got)

	list, err := repo.ListCredentials(ctx, tenant.Scope)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assertNoPlaintext(list[0])

	// OpenSecret is the ONLY method that returns plaintext, and it must
	// return exactly what was sealed.
	secret, extra, err := repo.OpenSecret(ctx, tenant.Scope, created.ID)
	require.NoError(t, err)
	require.Equal(t, plainSecret, string(secret))
	require.Equal(t, plainExtra, string(extra))

	// Two credentials sealed with the same plaintext must not produce the
	// same ciphertext (a fresh nonce every time) — otherwise ciphertext
	// equality would itself leak whether two tenants share a password.
	other := testfixtures.NewTenant(t, ctx, pool, 7011)
	createdOther, err := repo.UpsertCredential(ctx, other.Scope, model.IntegrationCredential{
		CompanyID: other.Company.ID, DefinitionID: definitionID, IsActive: true,
	}, []byte(plainSecret), nil)
	require.NoError(t, err)
	require.NotEqual(t, string(created.SecretEnc), string(createdOther.SecretEnc))

	// Another tenant cannot open tenant A's secret through OpenSecret
	// either: the credential is not visible to it at all.
	_, _, err = repo.OpenSecret(ctx, other.Scope, created.ID)
	require.ErrorIs(t, err, store.ErrNotFound)
}

// TestIntegrationCredentialSealingBindsToItsOwnRow proves the AAD binding:
// a ciphertext sealed for one (company, definition) pair does not open under
// another company's key material, even with the same cipher.
func TestIntegrationCredentialSealingBindsToItsOwnRow(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	cipher := integrationCipher(t)
	tenantA := testfixtures.NewTenant(t, ctx, pool, 7020)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 7021)
	repoA := postgres.NewIntegrationRepository(pool, cipher)

	definitionID := integrationInsertDefinition(t, ctx, pool, "aril", "default")

	credA, err := repoA.UpsertCredential(ctx, tenantA.Scope, model.IntegrationCredential{
		CompanyID: tenantA.Company.ID, DefinitionID: definitionID, IsActive: true,
	}, []byte("secret-a"), nil)
	require.NoError(t, err)

	// Insert a row for tenant B directly, carrying tenant A's ciphertext,
	// and confirm OpenSecret on THAT row fails to decrypt: the AAD binds
	// ciphertext to (company_id, definition_id).
	var credBID uuid.UUID
	require.NoError(t, pool.QueryRow(ctx,
		`insert into integration_credentials (company_id, definition_id, secret_enc)
		 values ($1, $2, $3) returning id`,
		tenantB.Company.ID, definitionID, credA.SecretEnc).Scan(&credBID))

	_, _, err = repoA.OpenSecret(ctx, tenantB.Scope, credBID)
	require.Error(t, err, "a ciphertext copied onto another company's row must not decrypt")
}

func TestIntegrationRecordVerificationAndDeleteAreScoped(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenantA := testfixtures.NewTenant(t, ctx, pool, 7030)
	tenantB := testfixtures.NewTenant(t, ctx, pool, 7031)
	repo := postgres.NewIntegrationRepository(pool, integrationCipher(t))

	definitionID := integrationInsertDefinition(t, ctx, pool, "pm5340", "default")
	cred, err := repo.UpsertCredential(ctx, tenantA.Scope, model.IntegrationCredential{
		CompanyID: tenantA.Company.ID, DefinitionID: definitionID, IsActive: true,
	}, []byte("s"), nil)
	require.NoError(t, err)

	_, err = repo.Credential(ctx, tenantB.Scope, definitionID)
	require.ErrorIs(t, err, store.ErrNotFound)

	err = repo.RecordVerification(ctx, tenantB.Scope, cred.ID, time.Now().UTC(), nil)
	require.ErrorIs(t, err, store.ErrNotFound)

	require.NoError(t, repo.RecordVerification(ctx, tenantA.Scope, cred.ID, time.Now().UTC(), nil))

	err = repo.DeleteCredential(ctx, tenantB.Scope, cred.ID)
	require.ErrorIs(t, err, store.ErrNotFound)

	require.NoError(t, repo.DeleteCredential(ctx, tenantA.Scope, cred.ID))
	_, err = repo.Credential(ctx, tenantA.Scope, definitionID)
	require.ErrorIs(t, err, store.ErrNotFound)
}

// TestIntegrationUpsertCredentialRefusesANonexistentDefinition proves
// definition_id, a stored foreign key, must name a real
// integration_definitions row before the write is allowed.
func TestIntegrationUpsertCredentialRefusesANonexistentDefinition(t *testing.T) {
	pool := testfixtures.NewIsolatedDB(t)
	ctx := context.Background()
	tenant := testfixtures.NewTenant(t, ctx, pool, 7040)
	repo := postgres.NewIntegrationRepository(pool, integrationCipher(t))

	_, err := repo.UpsertCredential(ctx, tenant.Scope, model.IntegrationCredential{
		CompanyID: tenant.Company.ID, DefinitionID: uuid.New(), IsActive: true,
	}, []byte("s"), nil)
	require.ErrorIs(t, err, store.ErrNotFound)

	list, err := repo.ListCredentials(ctx, tenant.Scope)
	require.NoError(t, err)
	require.Empty(t, list, "the refused upsert must not have written anything")
}
