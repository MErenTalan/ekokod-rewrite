// Package storage keeps uploaded files outside any web root (09 §F11, R336,
// R337). Paths are always server-generated; a name only ever contributes its
// extension, and Open refuses anything outside the company's directory.
package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
)

var (
	// ErrTooLarge is a file over Store.Max.
	ErrTooLarge = errors.New("storage: file too large")
	// ErrTypeNotAllowed is a file whose bytes are not an allowed type, or
	// whose bytes and extension disagree.
	ErrTypeNotAllowed = errors.New("storage: file type not allowed")
	// ErrOutsideRoot is a path that does not resolve inside the company directory.
	ErrOutsideRoot = errors.New("storage: path outside the storage root")
)

// Store is a directory tree <Root>/<area>/<company>/<uuid>.
type Store struct {
	Root    string
	Max     int64
	Allowed []string
}

// Saved is what the metadata row records.
type Saved struct {
	Path        string // relative to the company directory
	ContentType string
	Size        int64
	Checksum    string // sha256, hex
}

const sniffLen = 512

func (s Store) dir(company uuid.UUID, area string) string {
	return filepath.Join(s.Root, area, company.String())
}

// Save streams r to disk: sniffed type first, then a capped copy with its
// checksum, then an atomic rename. A refusal leaves nothing behind.
func (s Store) Save(ctx context.Context, company uuid.UUID, area, name string, r io.Reader) (Saved, error) {
	dir := s.dir(company, area)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Saved{}, fmt.Errorf("storage: %w", err)
	}
	head := make([]byte, sniffLen)
	n, err := io.ReadFull(r, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return Saved{}, fmt.Errorf("storage: read: %w", err)
	}
	head = head[:n]
	contentType := Sniff(head, name)
	if contentType == "" || !s.allowed(contentType) {
		return Saved{}, ErrTypeNotAllowed
	}
	tmp, err := os.CreateTemp(dir, ".upload-*")
	if err != nil {
		return Saved{}, fmt.Errorf("storage: %w", err)
	}
	keep := false
	defer func() {
		if !keep {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
		}
	}()
	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(tmp, hash), io.LimitReader(io.MultiReader(bytes.NewReader(head), r), s.Max+1))
	if err != nil {
		return Saved{}, fmt.Errorf("storage: write: %w", err)
	}
	if written > s.Max {
		return Saved{}, ErrTooLarge
	}
	if err := ctx.Err(); err != nil {
		return Saved{}, err
	}
	if err := tmp.Sync(); err != nil {
		return Saved{}, fmt.Errorf("storage: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return Saved{}, fmt.Errorf("storage: %w", err)
	}
	rel := uuid.NewString()
	if err := os.Rename(tmp.Name(), filepath.Join(dir, rel)); err != nil {
		return Saved{}, fmt.Errorf("storage: %w", err)
	}
	keep = true
	return Saved{Path: rel, ContentType: contentType, Size: written, Checksum: hex.EncodeToString(hash.Sum(nil))}, nil
}

func (s Store) allowed(contentType string) bool {
	for _, a := range s.Allowed {
		if a == contentType {
			return true
		}
	}
	return false
}

// Open opens a stored file after proving it resolves inside the company's
// directory, following symlinks (R337).
func (s Store) Open(company uuid.UUID, area, path string) (*os.File, error) {
	if path == "" || filepath.IsAbs(path) || !filepath.IsLocal(path) {
		return nil, ErrOutsideRoot
	}
	dir, err := filepath.EvalSymlinks(s.dir(company, area))
	if err != nil {
		return nil, ErrOutsideRoot
	}
	full, err := filepath.EvalSymlinks(filepath.Join(dir, path))
	if err != nil || !strings.HasPrefix(full, dir+string(filepath.Separator)) {
		return nil, ErrOutsideRoot
	}
	return os.Open(full) //nolint:gosec // resolved and prefix-checked above
}

// Sniff is R336: the bytes decide the type, and the extension must agree.
func Sniff(head []byte, name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	switch {
	case bytes.HasPrefix(head, []byte("%PDF-")):
		return pick(ext == ".pdf", "application/pdf")
	case bytes.HasPrefix(head, []byte("PK\x03\x04")):
		switch ext {
		case ".xlsx":
			return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
		case ".docx":
			return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
		}
		return ""
	case bytes.HasPrefix(head, []byte{0xD0, 0xCF, 0x11, 0xE0}):
		return pick(ext == ".xls", "application/vnd.ms-excel")
	case bytes.HasPrefix(head, []byte("\x89PNG\r\n\x1a\n")):
		return pick(ext == ".png", "image/png")
	case bytes.HasPrefix(head, []byte{0xFF, 0xD8, 0xFF}):
		return pick(ext == ".jpg" || ext == ".jpeg", "image/jpeg")
	case ext == ".csv" && isText(head):
		return "text/csv"
	}
	return ""
}

func pick(ok bool, contentType string) string {
	if ok {
		return contentType
	}
	return ""
}

// isText accepts UTF-8 without NULs; a rune cut at the sniff boundary is fine.
func isText(head []byte) bool {
	if bytes.IndexByte(head, 0) >= 0 {
		return false
	}
	for trim := 0; trim <= 3 && trim <= len(head); trim++ {
		if utf8.Valid(head[:len(head)-trim]) {
			return true
		}
	}
	return false
}

const maxName = 200

// CleanName is the display name: base name only, no control characters, at
// most 200 characters. It is never part of a path.
func CleanName(name string) string {
	name = strings.ReplaceAll(name, `\`, "/")
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." {
		return "dosya"
	}
	if r := []rune(name); len(r) > maxName {
		ext := []rune(filepath.Ext(name))
		name = string(r[:maxName-len(ext)]) + string(ext)
	}
	return name
}
