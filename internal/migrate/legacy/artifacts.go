package legacy

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/google/uuid"

	"github.com/MErenTalan/ekokod-rewrite/internal/storage"
)

// ArtifactsReport is artifacts_report.json (R428): both orphan directions.
type ArtifactsReport struct {
	Referenced   int            `json:"referenced"`
	Copied       int            `json:"copied"`
	AlreadyThere int            `json:"already_there"`
	TotalBytes   int64          `json:"total_bytes"`
	ByOwnerType  map[string]int `json:"by_owner_type"`
	Missing      []string       `json:"referenced_missing"`
	Escaped      []string       `json:"path_escape"`
	Unreferenced []string       `json:"unreferenced"`
}

type artifactRef struct {
	Ref          string  `json:"ref"`
	CompanyID    string  `json:"company_id"`
	OwnerType    string  `json:"owner_type"`
	OwnerID      string  `json:"owner_id"`
	ClauseID     *string `json:"clause_id"`
	SourcePath   string  `json:"source_path"`
	OriginalName string  `json:"original_name"`
}

// Artifacts copies every file the transform referenced into the storage
// layout (<dest>/<owner_type>/<company>/<id>) and writes stored_files.ndjson
// into dir for load. Re-running copies nothing that is already there.
func Artifacts(dir string, sources []string, dest string) (ArtifactsReport, error) {
	rep := ArtifactsReport{ByOwnerType: map[string]int{}, Missing: []string{}, Escaped: []string{}, Unreferenced: []string{}}
	roots := make([]string, 0, len(sources))
	for _, s := range sources {
		r, err := filepath.EvalSymlinks(filepath.Clean(s))
		if err != nil {
			return rep, fmt.Errorf("legacy: artifact source %s: %w", s, err)
		}
		roots = append(roots, r)
	}
	refs, err := readRefs(filepath.Join(dir, "artifact_refs.ndjson"))
	if err != nil {
		return rep, err
	}
	out, err := os.Create(filepath.Join(dir, "stored_files.ndjson")) //nolint:gosec // staging directory
	if err != nil {
		return rep, err
	}
	w := bufio.NewWriter(out)
	used := map[string]bool{}
	for _, r := range refs {
		rep.Referenced++
		src, escaped := resolveArtifact(roots, r.SourcePath)
		switch {
		case escaped:
			rep.Escaped = append(rep.Escaped, r.SourcePath)
			continue
		case src == "":
			rep.Missing = append(rep.Missing, r.SourcePath)
			continue
		}
		used[src] = true
		row, copied, err := copyArtifact(r, src, dest)
		if err != nil {
			return rep, err
		}
		if copied {
			rep.Copied++
		} else {
			rep.AlreadyThere++
		}
		rep.TotalBytes += row["size_bytes"].(int64)
		rep.ByOwnerType[r.OwnerType]++
		b, err := json.Marshal(row)
		if err != nil {
			return rep, err
		}
		if _, err := w.Write(append(b, '\n')); err != nil {
			return rep, err
		}
	}
	if err := errors.Join(w.Flush(), out.Close()); err != nil {
		return rep, err
	}
	for _, root := range roots {
		if err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.Type().IsRegular() && !used[p] {
				rep.Unreferenced = append(rep.Unreferenced, p)
			}
			return nil
		}); err != nil {
			return rep, err
		}
	}
	sort.Strings(rep.Missing)
	sort.Strings(rep.Escaped)
	sort.Strings(rep.Unreferenced)
	b, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return rep, err
	}
	return rep, os.WriteFile(filepath.Join(dir, "artifacts_report.json"), append(b, '\n'), 0o600)
}

func readRefs(path string) ([]artifactRef, error) {
	f, err := os.Open(path) //nolint:gosec // staging directory
	if err != nil {
		return nil, fmt.Errorf("legacy: %w (run transform first)", err)
	}
	defer func() { _ = f.Close() }()
	var refs []artifactRef
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		var r artifactRef
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			return nil, err
		}
		refs = append(refs, r)
	}
	return refs, sc.Err()
}

// resolveArtifact finds a legacy path under one of the roots: as an absolute
// path inside a root, joined onto a root, or joined without its first segment
// (legacy wrote "/uploads/…" relative to the app directory). A candidate that
// resolves outside every root is an escape and is never copied.
func resolveArtifact(roots []string, legacyPath string) (string, bool) {
	p := filepath.Clean(filepath.FromSlash(legacyPath))
	rel := strings.TrimPrefix(p, string(filepath.Separator))
	var cands []string
	if filepath.IsAbs(p) {
		cands = append(cands, p)
	}
	for _, r := range roots {
		cands = append(cands, filepath.Join(r, rel))
		if _, rest, ok := strings.Cut(rel, string(filepath.Separator)); ok {
			cands = append(cands, filepath.Join(r, rest))
		}
	}
	escaped := false
	for _, c := range cands {
		real, err := filepath.EvalSymlinks(c)
		if err != nil {
			continue
		}
		inside := false
		for _, r := range roots {
			if strings.HasPrefix(real, r+string(filepath.Separator)) {
				inside = true
			}
		}
		if !inside {
			if !filepath.IsAbs(p) || c != p {
				escaped = true
			}
			continue
		}
		if st, err := os.Stat(real); err == nil && st.Mode().IsRegular() {
			return real, false
		}
	}
	return "", escaped
}

func copyArtifact(r artifactRef, src, dest string) (map[string]any, bool, error) {
	id := ID("stored_files", r.Ref)
	company, err := uuid.Parse(r.CompanyID)
	if err != nil {
		return nil, false, err
	}
	f, err := os.Open(src) //nolint:gosec // resolved inside a source root
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = f.Close() }()
	head := make([]byte, 512)
	n, _ := io.ReadFull(f, head)
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, false, err
	}
	sum, size, err := hashFile(f)
	if err != nil {
		return nil, false, err
	}
	target := filepath.Join(dest, r.OwnerType, company.String(), id.String())
	copied := true
	if st, err := os.Stat(target); err == nil && st.Size() == size {
		if tf, err := os.Open(target); err == nil { //nolint:gosec // our own storage tree
			existing, _, herr := hashFile(tf)
			_ = tf.Close()
			copied = herr != nil || existing != sum
		}
	}
	if copied {
		if err := writeCopy(src, target); err != nil {
			return nil, false, err
		}
	}
	name := storage.CleanName(r.OriginalName)
	ct := storage.Sniff(head[:n], name)
	if ct == "" {
		ct = "application/octet-stream"
	}
	return map[string]any{"id": id.String(), "company_id": r.CompanyID, "owner_type": r.OwnerType, "owner_id": r.OwnerID,
		"clause_id": r.ClauseID, "original_name": name, "stored_path": id.String(), "content_type": ct, "size_bytes": size, "checksum": sum}, copied, nil
}

func hashFile(f io.Reader) (string, int64, error) {
	h := sha256.New()
	n, err := io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil)), n, err
}

func writeCopy(src, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	in, err := os.Open(src) //nolint:gosec // resolved inside a source root
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	tmp, err := os.CreateTemp(filepath.Dir(target), ".legacy-*")
	if err != nil {
		return err
	}
	if _, err := io.Copy(tmp, in); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := errors.Join(tmp.Sync(), tmp.Close()); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), target)
}
