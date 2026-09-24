package iso50001

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/google/uuid"

	domain "github.com/MErenTalan/ekokod-rewrite/internal/domain/iso50001"
	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

var readme = map[string]string{
	"tr": "ISO 50001 klasörü — %s\nOluşturulma: %s\n\nTüm maddeler TS EN ISO 50001:2018 standardına dayanmaktadır.\nBu araç yalnızca rehberlik amaçlıdır.\n\nnotes/ her alt maddenin notlarını, files/ yüklenen kanıt dosyalarını içerir.\n",
	"en": "ISO 50001 folder — %s\nGenerated: %s\n\nAll clauses are based on TS EN ISO 50001:2018.\nThis tool is for guidance only.\n\nnotes/ holds each sub-clause's notes, files/ the uploaded evidence.\n",
}

// Export streams every note and file of the building as a zip (R339): the
// archive is written straight to w and file bytes are copied from disk.
func (s *Service) Export(ctx context.Context, sc store.Scope, building uuid.UUID, locale string, w io.Writer) error {
	if locale != "en" {
		locale = "tr"
	}
	b, err := s.d.Buildings.Get(ctx, sc, building)
	if err != nil {
		return err
	}
	var notes []model.ISO50001Note
	p, err := s.d.ISO.Project(ctx, sc, building)
	switch {
	case err == nil:
		if notes, err = s.d.ISO.Notes(ctx, sc, p.ID, nil); err != nil {
			return err
		}
	case !errors.Is(err, store.ErrNotFound):
		return err
	}
	files, err := s.buildingFiles(ctx, sc, building, nil)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(w)
	now := s.d.Clock.Now().In(istanbul)
	if err := add(zw, "README.txt", fmt.Sprintf(readme[locale], b.Name, now.Format("02.01.2006 15:04"))); err != nil {
		return err
	}
	byClause := map[string][]model.ISO50001Note{}
	for _, n := range notes {
		byClause[n.ClauseID] = append(byClause[n.ClauseID], n)
	}
	for _, id := range domain.SubIDs() {
		list := byClause[id]
		if len(list) == 0 {
			continue
		}
		sub, _ := domain.SubByID(id)
		var md strings.Builder
		fmt.Fprintf(&md, "# %s\n", sub.Title[locale])
		for _, n := range list {
			title := "Not"
			if locale == "en" {
				title = "Note"
			}
			if n.Title != nil {
				title = *n.Title
			}
			fmt.Fprintf(&md, "\n## %s — %s\n\n%s\n", title, n.CreatedAt.In(istanbul).Format("02.01.2006"), n.Body)
		}
		if err := add(zw, "notes/"+id+".md", md.String()); err != nil {
			return err
		}
	}
	used := map[string]int{}
	for _, f := range files {
		if f.ClauseID == nil {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		name := unique(used, "files/"+*f.ClauseID+"/"+f.OriginalName)
		if err := s.copyFile(zw, sc, f, name); err != nil {
			return err
		}
	}
	return zw.Close()
}

func add(zw *zip.Writer, name, content string) error {
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = io.WriteString(w, content)
	return err
}

func (s *Service) copyFile(zw *zip.Writer, sc store.Scope, f model.StoredFile, name string) error {
	body, err := s.d.Store.Open(sc.CompanyID, OwnerType, f.StoredPath)
	if err != nil {
		return fmt.Errorf("iso50001 export %s: %w", f.ID, err)
	}
	defer func() { _ = body.Close() }()
	w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: f.CreatedAt})
	if err != nil {
		return err
	}
	_, err = io.Copy(w, body)
	return err
}

// unique suffixes a repeated name " (2)", " (3)"… before its extension (Review Focus 5).
func unique(used map[string]int, name string) string {
	used[name]++
	if used[name] == 1 {
		return name
	}
	ext := path.Ext(name)
	return fmt.Sprintf("%s (%d)%s", strings.TrimSuffix(name, ext), used[name], ext)
}
