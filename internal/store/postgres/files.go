package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/internal/pgerr"
	"github.com/MErenTalan/ekokod-rewrite/internal/store/postgres/sqlcgen"
)

// FileRepository implements store.FileRepository.
//
// stored_files carries company_id directly and has no building_id, so Scope
// narrows it to company_id only (like PlantRepository, which has the same
// shape).
type FileRepository struct {
	q    *sqlcgen.Queries
	pool *pgxpool.Pool
}

// NewFileRepository builds a FileRepository on pool.
func NewFileRepository(pool *pgxpool.Pool) *FileRepository {
	return &FileRepository{q: sqlcgen.New(pool), pool: pool}
}

var _ store.FileRepository = (*FileRepository)(nil)

const (
	fileListDefaultLimit = 100
	fileListMaxLimit     = 1000
)

func filePageBounds(p store.Page) (limit, offset int32) {
	limit = p.Limit
	if limit <= 0 {
		limit = fileListDefaultLimit
	}
	if limit > fileListMaxLimit {
		limit = fileListMaxLimit
	}
	offset = p.Offset
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func storedFileFromRow(row sqlcgen.StoredFile) model.StoredFile {
	var deletedAt *time.Time
	if row.DeletedAt.Valid {
		t := row.DeletedAt.Time
		deletedAt = &t
	}
	return model.StoredFile{
		ID: row.ID, CompanyID: row.CompanyID,
		OwnerType: row.OwnerType, OwnerID: row.OwnerID, ClauseID: row.ClauseID,
		OriginalName: row.OriginalName, StoredPath: row.StoredPath, ContentType: row.ContentType,
		SizeBytes: row.SizeBytes, Checksum: row.Checksum, UploadedBy: row.UploadedBy,
		CreatedAt: row.CreatedAt.Time, DeletedAt: deletedAt,
	}
}

// Get returns one stored file's metadata by id, scoped to s.
func (r *FileRepository) Get(ctx context.Context, s store.Scope, id uuid.UUID) (model.StoredFile, error) {
	if !s.Valid() {
		return model.StoredFile{}, store.ErrInvalidScope
	}
	row, err := r.q.FileGet(ctx, sqlcgen.FileGetParams{ID: id, CompanyID: s.CompanyID})
	if err != nil {
		return model.StoredFile{}, pgerr.Translate(r.pool, "get stored file", err)
	}
	return storedFileFromRow(row), nil
}

// List returns stored file metadata matching f, scoped to s.
func (r *FileRepository) List(ctx context.Context, s store.Scope, f store.FileFilter) ([]model.StoredFile, error) {
	if !s.Valid() {
		return nil, store.ErrInvalidScope
	}
	limit, offset := filePageBounds(f.Page)
	rows, err := r.q.FileList(ctx, sqlcgen.FileListParams{
		CompanyID: s.CompanyID, IncludeDeleted: f.IncludeDeleted,
		OwnerType: f.OwnerType, OwnerID: f.OwnerID, ClauseID: f.ClauseID,
		OffsetVal: offset, LimitVal: limit,
	})
	if err != nil {
		return nil, pgerr.Translate(r.pool, "list stored files", err)
	}
	out := make([]model.StoredFile, 0, len(rows))
	for _, row := range rows {
		out = append(out, storedFileFromRow(row))
	}
	return out, nil
}

// Create stores one file's metadata; the bytes live in object storage.
func (r *FileRepository) Create(ctx context.Context, s store.Scope, f model.StoredFile) (model.StoredFile, error) {
	if !s.Valid() {
		return model.StoredFile{}, store.ErrInvalidScope
	}
	if f.CompanyID != s.CompanyID {
		return model.StoredFile{}, store.ErrNotFound
	}
	row, err := r.q.FileCreate(ctx, sqlcgen.FileCreateParams{
		ID: f.ID, CompanyID: s.CompanyID, OwnerType: f.OwnerType, OwnerID: f.OwnerID, ClauseID: f.ClauseID,
		OriginalName: f.OriginalName, StoredPath: f.StoredPath, ContentType: f.ContentType,
		SizeBytes: f.SizeBytes, Checksum: f.Checksum, UploadedBy: f.UploadedBy,
	})
	if err != nil {
		return model.StoredFile{}, pgerr.Translate(r.pool, "create stored file", err)
	}
	return storedFileFromRow(row), nil
}

// SoftDelete marks the metadata deleted.
func (r *FileRepository) SoftDelete(ctx context.Context, s store.Scope, id uuid.UUID, at time.Time) error {
	if !s.Valid() {
		return store.ErrInvalidScope
	}
	n, err := r.q.FileSoftDelete(ctx, sqlcgen.FileSoftDeleteParams{
		At: pgtype.Timestamptz{Time: at, Valid: true}, ID: id, CompanyID: s.CompanyID,
	})
	if err != nil {
		return pgerr.Translate(r.pool, "soft delete stored file", err)
	}
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}
