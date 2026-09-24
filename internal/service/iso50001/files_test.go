package iso50001_test

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"

	perr "github.com/MErenTalan/ekokod-rewrite/internal/platform/errors"
	"github.com/MErenTalan/ekokod-rewrite/internal/service/iso50001"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

var pdf = []byte("%PDF-1.7\n%%EOF\n")

func TestUploadStoresUnderTheBuilding(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	user := uuid.New()
	f, err := w.svc().Upload(ctx, w.ba, user, w.b1, "5.1", `C:\evrak\Politika.pdf`, bytes.NewReader(pdf))
	require.NoError(t, err)
	require.Equal(t, iso50001.OwnerType, f.OwnerType)
	require.Equal(t, w.b1, *f.OwnerID)
	require.Equal(t, "5.1", *f.ClauseID)
	require.Equal(t, "Politika.pdf", f.OriginalName)
	require.Equal(t, "application/pdf", f.ContentType)
	require.Equal(t, user, *f.UploadedBy)
	st, err := w.svc().Project(ctx, w.ba, w.b1)
	require.NoError(t, err)
	require.Equal(t, 1, st.Counts["5.1"].Files)
	require.Equal(t, 5, st.Progress)

	list, err := w.svc().Files(ctx, w.ba, w.b1, "5.1")
	require.NoError(t, err)
	require.Len(t, list, 1)
}

func TestUploadRefusals(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	_, err := w.svc().Upload(ctx, w.admin, uuid.New(), w.b1, "5.9", "a.pdf", bytes.NewReader(pdf))
	requireValidation(t, err, "clause", "unknown")
	_, err = w.svc().Upload(ctx, w.admin, uuid.New(), w.b1, "5.1", "a.pdf", bytes.NewReader([]byte("MZ\x90\x00")))
	requireValidation(t, err, "file", "type_not_allowed")
	_, err = w.svc().Upload(ctx, w.ba, uuid.New(), w.b2, "5.1", "a.pdf", bytes.NewReader(pdf))
	require.ErrorIs(t, err, store.ErrNotFound)
	big := append(append([]byte{}, pdf...), bytes.Repeat([]byte("x"), 2<<20)...)
	_, err = w.svc().Upload(ctx, w.admin, uuid.New(), w.b1, "5.1", "a.pdf", bytes.NewReader(big))
	var pe *perr.Error
	require.True(t, errors.As(err, &pe))
	require.Equal(t, 413, pe.HTTPStatus)
	require.EqualValues(t, 1<<20, pe.Params["max_bytes"])
}

func TestDownloadIsAuthorised(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	s := w.svc()
	own, err := s.Upload(ctx, w.admin, uuid.New(), w.b1, "5.1", "a.pdf", bytes.NewReader(pdf))
	require.NoError(t, err)
	onB2, err := s.Upload(ctx, w.admin, uuid.New(), w.b2, "5.1", "b.pdf", bytes.NewReader(pdf))
	require.NoError(t, err)

	meta, body, err := s.Download(ctx, w.ba, own.ID)
	require.NoError(t, err)
	got, _ := io.ReadAll(body)
	_ = body.Close()
	require.Equal(t, pdf, got)
	require.Equal(t, "a.pdf", meta.OriginalName)

	_, _, err = s.Download(ctx, w.ba, onB2.ID)
	require.ErrorIs(t, err, store.ErrNotFound, "another building (Review Focus 3)")
	_, _, err = s.Download(ctx, w.oSc, own.ID)
	require.ErrorIs(t, err, store.ErrNotFound, "another company")
	require.ErrorIs(t, s.DeleteFile(ctx, w.ba, onB2.ID), store.ErrNotFound)
	require.NoError(t, s.DeleteFile(ctx, w.ba, own.ID))
	_, _, err = s.Download(ctx, w.ba, own.ID)
	require.ErrorIs(t, err, store.ErrNotFound, "deleted")

	other := w.files.rows[onB2.ID]
	other.OwnerType = "carbon"
	w.files.rows[onB2.ID] = other
	_, _, err = s.Download(ctx, w.admin, onB2.ID)
	require.ErrorIs(t, err, store.ErrNotFound, "another module's file")
}

func TestExportZipsEveryNoteAndFile(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	s := w.svc()
	title := "Politika"
	_, err := s.CreateNote(ctx, w.admin, uuid.New(), w.b1, "5.2", &title, "Enerji politikası yayımlandı.")
	require.NoError(t, err)
	_, err = s.CreateNote(ctx, w.admin, uuid.New(), w.b1, "5.2", nil, "İkinci not")
	require.NoError(t, err)
	for range 2 {
		_, err = s.Upload(ctx, w.admin, uuid.New(), w.b1, "5.1", "Kanıt.pdf", bytes.NewReader(pdf))
		require.NoError(t, err)
	}
	var buf bytes.Buffer
	require.NoError(t, s.Export(ctx, w.ba, w.b1, "tr", &buf))
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	require.NoError(t, err)
	var names []string
	content := map[string]string{}
	for _, f := range zr.File {
		names = append(names, f.Name)
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		_ = rc.Close()
		content[f.Name] = string(b)
	}
	sort.Strings(names)
	require.Equal(t, []string{"README.txt", "files/5.1/Kanıt (2).pdf", "files/5.1/Kanıt.pdf", "notes/5.2.md"}, names)
	require.Contains(t, content["notes/5.2.md"], "5.2 Enerji Politikası")
	require.Contains(t, content["notes/5.2.md"], "## Politika")
	require.Contains(t, content["notes/5.2.md"], "İkinci not")
	require.Equal(t, string(pdf), content["files/5.1/Kanıt.pdf"])
	require.Contains(t, content["README.txt"], "TS EN ISO 50001:2018")

	require.ErrorIs(t, s.Export(ctx, w.ba, w.b2, "tr", io.Discard), store.ErrNotFound)
}

func TestTemplatesAreRealFiles(t *testing.T) {
	s := newWorld(t).svc()
	list := s.Templates()
	require.Len(t, list, 3)
	for _, tpl := range list {
		body, name, contentType, err := s.Template(tpl.ID)
		require.NoError(t, err, tpl.ID)
		require.Equal(t, tpl.FileName, name)
		if strings.HasSuffix(name, ".xlsx") {
			f, err := excelize.OpenReader(bytes.NewReader(body))
			require.NoError(t, err, name)
			require.NotEmpty(t, f.GetSheetList())
			require.Contains(t, contentType, "spreadsheetml")
			continue
		}
		require.True(t, bytes.HasPrefix(body, []byte("%PDF-")), name)
	}
	_, _, _, err := s.Template("nope")
	require.ErrorIs(t, err, store.ErrNotFound)
}
