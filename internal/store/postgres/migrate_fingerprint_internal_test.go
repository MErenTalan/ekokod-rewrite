package postgres

import (
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
)

// TestFingerprintFSChangesWithContent is F4 Task 0's requirement 6c
// (pure-function level): the shared isolated-database template's name
// folds in MigrationsFingerprint specifically so that a CHANGED migration
// set never reuses a stale template — this proves the fingerprint itself
// actually changes when a migration file's content changes, using an
// in-memory fstest.MapFS rather than editing the real embedded migrations.
func TestFingerprintFSChangesWithContent(t *testing.T) {
	fsA := fstest.MapFS{
		"migrations/00001_a.sql": {Data: []byte("create table a (id int);")},
	}
	fsB := fstest.MapFS{
		"migrations/00001_a.sql": {Data: []byte("create table a (id int, extra text);")},
	}

	fpA, err := fingerprintFS(fsA, "migrations")
	require.NoError(t, err)
	fpB, err := fingerprintFS(fsB, "migrations")
	require.NoError(t, err)

	require.NotEqual(t, fpA, fpB, "changing a migration file's content must change its fingerprint")
}

// TestFingerprintFSIsDeterministic pins the other half of the safety
// property: the SAME migration set must always fingerprint identically,
// otherwise every process sharing a server would build its own template
// under its own name and no sharing would happen at all.
func TestFingerprintFSIsDeterministic(t *testing.T) {
	fsys := fstest.MapFS{
		"migrations/00001_a.sql": {Data: []byte("create table a (id int);")},
		"migrations/00002_b.sql": {Data: []byte("create table b (id int);")},
	}

	first, err := fingerprintFS(fsys, "migrations")
	require.NoError(t, err)
	second, err := fingerprintFS(fsys, "migrations")
	require.NoError(t, err)

	require.Equal(t, first, second, "fingerprinting the same migration set twice must give the same result")
}

// TestFingerprintFSDiffersOnFilenameAlone pins that a rename (same bytes,
// different filename — e.g. renumbering a migration) also changes the
// fingerprint: the NUL-separated name+content hashing in fingerprintFS
// exists precisely so a filename change is not silently absorbed.
func TestFingerprintFSDiffersOnFilenameAlone(t *testing.T) {
	fsA := fstest.MapFS{
		"migrations/00001_a.sql": {Data: []byte("create table a (id int);")},
	}
	fsB := fstest.MapFS{
		"migrations/00002_a.sql": {Data: []byte("create table a (id int);")},
	}

	fpA, err := fingerprintFS(fsA, "migrations")
	require.NoError(t, err)
	fpB, err := fingerprintFS(fsB, "migrations")
	require.NoError(t, err)

	require.NotEqual(t, fpA, fpB, "renaming a migration file must change its fingerprint even with identical content")
}
