package admin

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/internal/pgerr"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/sqlcgen"
)

// IngestionRepository implements store.AdminIngestionRepository: the list
// of credentials the scheduled dispatcher fans out to. See doc.go's sixth
// paragraph for why this cannot take a Scope.
type IngestionRepository struct {
	q    *sqlcgen.Queries
	pool *pgxpool.Pool
}

// NewIngestionRepository builds an IngestionRepository on pool.
func NewIngestionRepository(pool *pgxpool.Pool) *IngestionRepository {
	return &IngestionRepository{q: sqlcgen.New(pool), pool: pool}
}

var _ store.AdminIngestionRepository = (*IngestionRepository)(nil)

// ActiveCredentials implements store.AdminIngestionRepository.
// ActiveCredentials. AdminIngestionActiveCredentials's own SELECT list is
// what keeps this call from ever carrying a secret column: it names
// credential_id, company_id, definition_id, provider and subtype only,
// never secret_enc or extra_enc.
func (r *IngestionRepository) ActiveCredentials(ctx context.Context) ([]model.CredentialRef, error) {
	rows, err := r.q.AdminIngestionActiveCredentials(ctx)
	if err != nil {
		return nil, pgerr.Translate(r.pool, "admin ingestion active credentials", err)
	}
	out := make([]model.CredentialRef, len(rows))
	for i, row := range rows {
		out[i] = model.CredentialRef{
			CredentialID: row.CredentialID,
			CompanyID:    row.CompanyID,
			DefinitionID: row.DefinitionID,
			Provider:     model.IntegrationProvider(row.Provider),
			Subtype:      row.Subtype,
		}
	}
	return out, nil
}
