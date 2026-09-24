package legacy

import (
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ManifestEntry is one extracted collection.
type ManifestEntry struct {
	Collection string `json:"collection"`
	Count      int    `json:"count"`
	SHA256     string `json:"sha256"`
}

// Manifest describes an extract directory (08 §4).
type Manifest struct {
	Collections []ManifestEntry `json:"collections"`
}

const manifestFile = "manifest.json"

// Extract is R405: one gzipped canonical-extended-JSON file per collection, in
// _id order, with a manifest of counts and SHA-256 sums. Unchanged input gives
// byte-identical output; nothing is transformed.
func Extract(ctx context.Context, src Source, dir string) (Manifest, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return Manifest{}, err
	}
	names, err := src.Collections(ctx)
	if err != nil {
		return Manifest{}, err
	}
	var m Manifest
	for _, name := range names {
		entry, err := extractOne(ctx, src, dir, name)
		if err != nil {
			return Manifest{}, fmt.Errorf("extract %s: %w", name, err)
		}
		m.Collections = append(m.Collections, entry)
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return Manifest{}, err
	}
	return m, os.WriteFile(filepath.Join(dir, manifestFile), append(data, '\n'), 0o600)
}

func extractOne(ctx context.Context, src Source, dir, name string) (ManifestEntry, error) {
	path := filepath.Join(dir, name+".ndjson.gz")
	f, err := os.Create(path) //nolint:gosec // dir is the operator's staging directory
	if err != nil {
		return ManifestEntry{}, err
	}
	defer func() { _ = f.Close() }()
	// A zero header (no name, no mtime) keeps the bytes reproducible.
	gz, err := gzip.NewWriterLevel(f, gzip.BestCompression)
	if err != nil {
		return ManifestEntry{}, err
	}
	w := bufio.NewWriter(gz)
	count := 0
	err = src.Iterate(ctx, name, func(doc bson.Raw) error {
		line, err := bson.MarshalExtJSON(doc, true, false)
		if err != nil {
			return err
		}
		count++
		if _, err := w.Write(line); err != nil {
			return err
		}
		return w.WriteByte('\n')
	})
	if err != nil {
		return ManifestEntry{}, err
	}
	if err := w.Flush(); err != nil {
		return ManifestEntry{}, err
	}
	if err := gz.Close(); err != nil {
		return ManifestEntry{}, err
	}
	if err := f.Close(); err != nil {
		return ManifestEntry{}, err
	}
	sum, err := fileSHA(path)
	return ManifestEntry{Collection: name, Count: count, SHA256: sum}, err
}

func fileSHA(path string) (string, error) {
	f, err := os.Open(path) //nolint:gosec // staging directory
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ReadManifest loads an extract's manifest.
func ReadManifest(dir string) (Manifest, error) {
	var m Manifest
	data, err := os.ReadFile(filepath.Join(dir, manifestFile)) //nolint:gosec // staging directory
	if err != nil {
		return m, err
	}
	return m, json.Unmarshal(data, &m)
}

// VerifyManifest re-checks every file's SHA-256 before a transform trusts it.
func VerifyManifest(dir string) error {
	m, err := ReadManifest(dir)
	if err != nil {
		return err
	}
	for _, e := range m.Collections {
		sum, err := fileSHA(filepath.Join(dir, e.Collection+".ndjson.gz"))
		if err != nil {
			return err
		}
		if sum != e.SHA256 {
			return fmt.Errorf("extract %s: checksum mismatch (manifest %s, file %s)", e.Collection, e.SHA256, sum)
		}
	}
	return nil
}

// ReadExtract streams one extracted collection back as documents.
func ReadExtract(dir, collection string, fn func(bson.M) error) error {
	f, err := os.Open(filepath.Join(dir, collection+".ndjson.gz")) //nolint:gosec // staging directory
	if os.IsNotExist(err) {
		return nil // a collection the legacy database never had
	}
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	sc := bufio.NewScanner(gz)
	sc.Buffer(make([]byte, 1<<20), 1<<30) // analyzer documents embed years of readings
	for sc.Scan() {
		var doc bson.M
		if err := bson.UnmarshalExtJSON(sc.Bytes(), true, &doc); err != nil {
			return fmt.Errorf("%s: %w", collection, err)
		}
		if err := fn(doc); err != nil {
			return err
		}
	}
	return sc.Err()
}
