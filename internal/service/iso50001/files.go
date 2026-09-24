package iso50001

import (
	"context"
	"errors"
	"io"
	"os"

	"github.com/google/uuid"

	domain "github.com/MErenTalan/ekokod-rewrite/internal/domain/iso50001"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	perr "github.com/MErenTalan/ekokod-rewrite/internal/platform/errors"
	"github.com/MErenTalan/ekokod-rewrite/internal/storage"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// ErrFileTooLarge is R336's 413, naming the limit.
var ErrFileTooLarge = perr.New("file_too_large", 413, "errors.iso50001.fileTooLarge")

// Upload stores one evidence file under the building and sub-clause (R336, R337).
func (s *Service) Upload(ctx context.Context, sc store.Scope, userID, building uuid.UUID, clause, name string, r io.Reader) (model.StoredFile, error) {
	if _, ok := domain.SubByID(clause); !ok {
		return model.StoredFile{}, validation("clause", "unknown")
	}
	if _, err := s.d.Buildings.Get(ctx, sc, building); err != nil {
		return model.StoredFile{}, err
	}
	name = storage.CleanName(name)
	saved, err := s.d.Store.Save(ctx, sc.CompanyID, OwnerType, name, r)
	switch {
	case errors.Is(err, storage.ErrTooLarge):
		return model.StoredFile{}, ErrFileTooLarge.WithParams(map[string]any{"max_bytes": s.d.Store.Max})
	case errors.Is(err, storage.ErrTypeNotAllowed):
		return model.StoredFile{}, validation("file", "type_not_allowed")
	case err != nil:
		return model.StoredFile{}, err
	}
	b, c := building, clause
	return s.d.Files.Create(ctx, sc, model.StoredFile{CompanyID: sc.CompanyID, OwnerType: OwnerType, OwnerID: &b, ClauseID: &c,
		OriginalName: name, StoredPath: saved.Path, ContentType: saved.ContentType, SizeBytes: saved.Size, Checksum: saved.Checksum,
		UploadedBy: &userID, CreatedAt: s.d.Clock.Now()})
}

// Files lists a sub-clause's live files.
func (s *Service) Files(ctx context.Context, sc store.Scope, building uuid.UUID, clause string) ([]model.StoredFile, error) {
	if _, ok := domain.SubByID(clause); !ok {
		return nil, validation("clause", "unknown")
	}
	if _, err := s.d.Buildings.Get(ctx, sc, building); err != nil {
		return nil, err
	}
	return s.buildingFiles(ctx, sc, building, &clause)
}

// file resolves an id to this module's file in a visible building (R338).
func (s *Service) file(ctx context.Context, sc store.Scope, id uuid.UUID) (model.StoredFile, error) {
	f, err := s.d.Files.Get(ctx, sc, id)
	if err != nil {
		return model.StoredFile{}, err
	}
	if f.OwnerType != OwnerType || f.OwnerID == nil || f.DeletedAt != nil {
		return model.StoredFile{}, store.ErrNotFound
	}
	if _, err := s.d.Buildings.Get(ctx, sc, *f.OwnerID); err != nil {
		return model.StoredFile{}, err
	}
	return f, nil
}

// Download is the authorising handler's read: metadata and the open bytes.
func (s *Service) Download(ctx context.Context, sc store.Scope, id uuid.UUID) (model.StoredFile, *os.File, error) {
	f, err := s.file(ctx, sc, id)
	if err != nil {
		return model.StoredFile{}, nil, err
	}
	body, err := s.d.Store.Open(sc.CompanyID, OwnerType, f.StoredPath)
	if err != nil {
		return model.StoredFile{}, nil, store.ErrNotFound
	}
	return f, body, nil
}

// DeleteFile soft-deletes; the bytes stay recoverable (FileRepository contract).
func (s *Service) DeleteFile(ctx context.Context, sc store.Scope, id uuid.UUID) error {
	if _, err := s.file(ctx, sc, id); err != nil {
		return err
	}
	return s.d.Files.SoftDelete(ctx, sc, id, s.d.Clock.Now())
}
