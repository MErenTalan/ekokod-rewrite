//go:build integration

package admin_test

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/admin"
	"github.com/MErenTalan/ekokod-rewrite/internal/testfixtures"
)

func TestActiveCredentialsSpansTenantsAndCarriesNoSecret(t *testing.T) {
	ctx := context.Background()
	pool := testfixtures.NewIsolatedDB(t)
	repo := admin.NewIngestionRepository(pool)

	// Two live companies and one soft-deleted one.
	var companyA, companyB, companyC uuid.UUID
	require.NoError(t, pool.QueryRow(ctx,
		`insert into companies (name) values ($1) returning id`, "ingest tenant A "+uuid.NewString()).Scan(&companyA))
	require.NoError(t, pool.QueryRow(ctx,
		`insert into companies (name) values ($1) returning id`, "ingest tenant B "+uuid.NewString()).Scan(&companyB))
	require.NoError(t, pool.QueryRow(ctx,
		`insert into companies (name, deleted_at) values ($1, now()) returning id`,
		"ingest tenant C (deleted) "+uuid.NewString()).Scan(&companyC))

	// Two definitions, so each tenant can hold one active and one inactive
	// credential without colliding on integration_credentials(company_id,
	// definition_id).
	var defActive, defInactive uuid.UUID
	require.NoError(t, pool.QueryRow(ctx,
		`insert into integration_definitions (provider, subtype) values ('osos', $1) returning id`,
		"IngestSuffix-"+uuid.NewString()).Scan(&defActive))
	require.NoError(t, pool.QueryRow(ctx,
		`insert into integration_definitions (provider, subtype) values ('gridbox', $1) returning id`,
		"IngestSuffix-"+uuid.NewString()).Scan(&defInactive))

	var credA, credB uuid.UUID
	require.NoError(t, pool.QueryRow(ctx,
		`insert into integration_credentials (company_id, definition_id, is_active) values ($1, $2, true) returning id`,
		companyA, defActive).Scan(&credA))
	_, err := pool.Exec(ctx,
		`insert into integration_credentials (company_id, definition_id, is_active) values ($1, $2, false)`,
		companyA, defInactive)
	require.NoError(t, err)

	require.NoError(t, pool.QueryRow(ctx,
		`insert into integration_credentials (company_id, definition_id, is_active) values ($1, $2, true) returning id`,
		companyB, defActive).Scan(&credB))
	_, err = pool.Exec(ctx,
		`insert into integration_credentials (company_id, definition_id, is_active) values ($1, $2, false)`,
		companyB, defInactive)
	require.NoError(t, err)

	// An active credential belonging to the SOFT-DELETED company — must
	// never appear, whatever its own is_active value.
	_, err = pool.Exec(ctx,
		`insert into integration_credentials (company_id, definition_id, is_active) values ($1, $2, true)`,
		companyC, defActive)
	require.NoError(t, err)

	got, err := repo.ActiveCredentials(ctx)
	require.NoError(t, err)
	require.Len(t, got, 2, "exactly the two active credentials of the two LIVE companies")

	// Ordered by (company_id, credential id): since UUID string form
	// preserves the same byte-order Postgres orders uuid columns by, this is
	// a faithful proxy for the SQL's own ORDER BY.
	require.True(t, got[0].CompanyID.String() < got[1].CompanyID.String(),
		"results must be ordered by company_id ascending")

	byCompany := map[uuid.UUID]model.CredentialRef{got[0].CompanyID: got[0], got[1].CompanyID: got[1]}

	refA, ok := byCompany[companyA]
	require.True(t, ok, "tenant A's active credential must be present")
	require.Equal(t, credA, refA.CredentialID)
	require.Equal(t, defActive, refA.DefinitionID)
	require.Equal(t, model.IntegrationProviderOSOS, refA.Provider)

	refB, ok := byCompany[companyB]
	require.True(t, ok, "tenant B's active credential must be present")
	require.Equal(t, credB, refB.CredentialID)
	require.Equal(t, model.IntegrationProviderOSOS, refB.Provider)

	// No secret-shaped field exists on model.CredentialRef at all — this is
	// a static proof about the TYPE, not just this call's values, so a
	// future field added to the struct trips it immediately.
	adminIngestRequireNoSecretFields(t, reflect.TypeOf(model.CredentialRef{}))
}

// adminIngestRequireNoSecretFields fails if typ has any field whose name
// contains Enc, Secret, Token or Password — the shapes a ciphertext or
// plaintext credential field would take, per repository.go's
// AdminIngestionRepository.ActiveCredentials doc ("No secret column is
// selected").
func adminIngestRequireNoSecretFields(t *testing.T, typ reflect.Type) {
	t.Helper()
	for i := range typ.NumField() {
		name := typ.Field(i).Name
		for _, bad := range []string{"Enc", "Secret", "Token", "Password"} {
			require.False(t, strings.Contains(name, bad),
				"model.CredentialRef.%s looks secret-shaped (contains %q); this type must carry NO ciphertext", name, bad)
		}
	}
}
