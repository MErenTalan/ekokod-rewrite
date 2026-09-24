package storage_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/storage"
)

var allowed = []string{"application/pdf", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document", "application/vnd.ms-excel", "text/csv", "image/png", "image/jpeg"}

func store(t *testing.T, max int64) storage.Store {
	return storage.Store{Root: t.TempDir(), Max: max, Allowed: allowed}
}

var pdf = []byte("%PDF-1.7\n1 0 obj << >> endobj\ntrailer\n%%EOF\n")

func TestSaveStreamsWithChecksumOutsideAnyNameInput(t *testing.T) {
	s := store(t, 1<<20)
	company := uuid.New()
	saved, err := s.Save(context.Background(), company, "iso50001", "../../etc/Politika.pdf", bytes.NewReader(pdf))
	require.NoError(t, err)
	require.Equal(t, "application/pdf", saved.ContentType)
	require.EqualValues(t, len(pdf), saved.Size)
	sum := sha256.Sum256(pdf)
	require.Equal(t, hex.EncodeToString(sum[:]), saved.Checksum)
	require.NotContains(t, saved.Path, "etc", "the name never reaches the path (R337)")
	require.NotContains(t, saved.Path, "..")
	f, err := s.Open(company, "iso50001", saved.Path)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	got, _ := os.ReadFile(f.Name())
	require.Equal(t, pdf, got)
}

func TestPathTraversal(t *testing.T) {
	s := store(t, 1<<20)
	company, other := uuid.New(), uuid.New()
	saved, err := s.Save(context.Background(), other, "iso50001", "a.pdf", bytes.NewReader(pdf))
	require.NoError(t, err)
	for name, p := range map[string]string{
		"parent":         "../../../../etc/passwd",
		"absolute":       "/etc/passwd",
		"other company":  filepath.Join(s.Root, "iso50001", other.String(), filepath.Base(saved.Path)),
		"other via dots": "../" + other.String() + "/" + filepath.Base(saved.Path),
		"other area":     "../../reports/" + company.String() + "/x.pdf",
		"empty":          "",
	} {
		_, err := s.Open(company, "iso50001", p)
		require.ErrorIs(t, err, storage.ErrOutsideRoot, name)
	}
	// A symlink inside the company directory that points out is refused too.
	dir := filepath.Join(s.Root, "iso50001", company.String())
	require.NoError(t, os.MkdirAll(dir, 0o700))
	require.NoError(t, os.Symlink("/etc/passwd", filepath.Join(dir, "link")))
	_, err = s.Open(company, "iso50001", "link")
	require.ErrorIs(t, err, storage.ErrOutsideRoot, "symlink out")
}

func TestTooLargeLeavesNothing(t *testing.T) {
	s := store(t, 100)
	company := uuid.New()
	_, err := s.Save(context.Background(), company, "iso50001", "big.pdf", bytes.NewReader(append(pdf, bytes.Repeat([]byte("x"), 200)...)))
	require.ErrorIs(t, err, storage.ErrTooLarge)
	entries, _ := os.ReadDir(filepath.Join(s.Root, "iso50001", company.String()))
	require.Empty(t, entries, "no partial or temp file stays behind")
}

func TestMagicBytesDecideNotTheExtension(t *testing.T) {
	s := store(t, 1<<20)
	company := uuid.New()
	zipHeader := append([]byte("PK\x03\x04"), bytes.Repeat([]byte{0}, 60)...)
	ole := append([]byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}, bytes.Repeat([]byte{0}, 60)...)
	png := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0}, 60)...)
	jpeg := append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, bytes.Repeat([]byte{0}, 60)...)
	for name, c := range map[string]struct {
		file    string
		content []byte
		want    string
	}{
		"pdf":          {"a.pdf", pdf, "application/pdf"},
		"xlsx":         {"a.xlsx", zipHeader, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
		"docx":         {"a.DOCX", zipHeader, "application/vnd.openxmlformats-officedocument.wordprocessingml.document"},
		"xls":          {"a.xls", ole, "application/vnd.ms-excel"},
		"png":          {"a.png", png, "image/png"},
		"jpeg":         {"a.jpg", jpeg, "image/jpeg"},
		"csv":          {"a.csv", []byte("tarih;tüketim\n2026-01-01;12,5\n"), "text/csv"},
		"exe as pdf":   {"a.pdf", append([]byte("MZ"), bytes.Repeat([]byte{0x90}, 60)...), ""},
		"pdf as xlsx":  {"a.xlsx", pdf, ""},
		"zip as pdf":   {"a.pdf", zipHeader, ""},
		"csv not utf8": {"a.csv", []byte{0xff, 0xfe, 0x00, 0x41}, ""},
		"html":         {"a.html", []byte("<html><script>alert(1)</script></html>"), ""},
		"no extension": {"README", pdf, ""},
	} {
		saved, err := s.Save(context.Background(), company, "iso50001", c.file, bytes.NewReader(c.content))
		if c.want == "" {
			require.ErrorIs(t, err, storage.ErrTypeNotAllowed, name)
			continue
		}
		require.NoError(t, err, name)
		require.Equal(t, c.want, saved.ContentType, name)
	}
}

func TestTypeMustAlsoBeConfigured(t *testing.T) {
	s := storage.Store{Root: t.TempDir(), Max: 1 << 20, Allowed: []string{"application/pdf"}}
	_, err := s.Save(context.Background(), uuid.New(), "iso50001", "a.png", bytes.NewReader(append([]byte("\x89PNG\r\n\x1a\n"), 0, 0)))
	require.True(t, errors.Is(err, storage.ErrTypeNotAllowed))
}

func TestCleanName(t *testing.T) {
	require.Equal(t, "Politika.pdf", storage.CleanName(`C:\Users\x\..\Politika.pdf`))
	require.Equal(t, "passwd", storage.CleanName("../../etc/passwd"))
	require.Equal(t, "ab.pdf", storage.CleanName("a\x00b\n.pdf"))
	require.Equal(t, "dosya", storage.CleanName("  "))
	require.Len(t, []rune(storage.CleanName(strings.Repeat("ğ", 300)+".pdf")), 200)
}
