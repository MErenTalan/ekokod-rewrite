package legacy_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MErenTalan/ekokod-rewrite/internal/migrate/legacy"
	"github.com/MErenTalan/ekokod-rewrite/internal/storage"
)

// legacyTree mirrors the legacy host: /opt/… and the app's /uploads/….
func legacyTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for p, body := range map[string]string{
		"opt/bills/b-2026-08.pdf":                             "%PDF-1.4 bill",
		"opt/bills/orphan.pdf":                                "%PDF-1.4 orphan",
		"opt/reports/r.pdf":                                   "%PDF-1.4 report",
		"uploads/64f000000000000000000d01/5.1/1-politika.pdf": "%PDF-1.4 policy",
	} {
		full := filepath.Join(root, filepath.FromSlash(p))
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o750))
		require.NoError(t, os.WriteFile(full, []byte(body), 0o600))
	}
	return root
}

func TestArtifactsAreCopiedRegisteredAndOrphansReportedBothWays(t *testing.T) {
	out, _, _ := transformWith(t, carbonISOSource(t), nil)
	root, dest := legacyTree(t), t.TempDir()

	rep, err := legacy.Artifacts(out, []string{root}, dest)
	require.NoError(t, err)
	require.Equal(t, 5, rep.Referenced, "bill pdf, report pdf + xlsx, carbon pdf, ISO file")
	require.Equal(t, 3, rep.Copied)
	require.Equal(t, []string{"/opt/documents/ghg.pdf", "/opt/reports/r.xlsx"}, rep.Missing, "referenced but absent")
	require.Equal(t, []string{filepath.Join(root, "opt", "bills", "orphan.pdf")}, rep.Unreferenced, "on disk, referenced by nothing")
	require.Equal(t, map[string]int{"legacy_bill": 1, "legacy_report": 1, "iso50001": 1}, rep.ByOwnerType)
	require.Equal(t, int64(len("%PDF-1.4 bill")+len("%PDF-1.4 report")+len("%PDF-1.4 policy")), rep.TotalBytes)

	files := rows(t, out, "stored_files")
	require.Len(t, files, 3)
	var iso map[string]any
	for _, f := range files {
		if f["owner_type"] == "iso50001" {
			iso = f
		}
	}
	require.Equal(t, []any{"Politika.pdf", "application/pdf", "5.1"}, []any{iso["original_name"], iso["content_type"], iso["clause_id"]})
	opened, err := storage.Store{Root: dest}.Open(uuid.MustParse(iso["company_id"].(string)), "iso50001", iso["stored_path"].(string))
	require.NoError(t, err, "the existing download path opens a migrated file")
	_ = opened.Close()

	first, err := os.ReadFile(filepath.Join(out, "stored_files.ndjson"))
	require.NoError(t, err)
	again, err := legacy.Artifacts(out, []string{root}, dest)
	require.NoError(t, err)
	require.Zero(t, again.Copied, "idempotent: nothing is copied twice")
	require.Equal(t, 3, again.AlreadyThere)
	second, err := os.ReadFile(filepath.Join(out, "stored_files.ndjson"))
	require.NoError(t, err)
	require.Equal(t, string(first), string(second))
}

func TestAnArtifactPathCannotLeaveItsSourceRoot(t *testing.T) {
	root, outside, out := t.TempDir(), t.TempDir(), t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret.pdf"), []byte("%PDF-1.4 secret"), 0o600))
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "link")))
	ref := `{"ref":"x","company_id":"` + uuid.NewString() + `","owner_type":"legacy_bill","owner_id":"` + uuid.NewString() + `","source_path":"/link/secret.pdf","original_name":"s.pdf"}` + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(out, "artifact_refs.ndjson"), []byte(ref), 0o600))

	rep, err := legacy.Artifacts(out, []string{root}, t.TempDir())
	require.NoError(t, err)
	require.Equal(t, []string{"/link/secret.pdf"}, rep.Escaped)
	require.Zero(t, rep.Copied)
}
