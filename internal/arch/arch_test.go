package arch_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/packages"
)

const modulePath = "github.com/MErenTalan/ekokod-rewrite"

// underPackage reports whether pkgPath is target itself or a true
// subpackage of it (target followed by "/"). A bare strings.HasPrefix
// match with no trailing-slash requirement would also match any sibling
// package whose path merely starts with the same characters — e.g.
// target "internal/api" would wrongly match a future "internal/apikeys"
// or "internal/apiclient" package, silently exempting it from whichever
// guard is doing the matching. That is a false negative in exactly the
// class of bug these architecture guards exist to catch, so every
// package-path comparison in this file must go through this helper
// rather than a raw HasPrefix call.
func underPackage(pkgPath, target string) bool {
	return pkgPath == target || strings.HasPrefix(pkgPath, target+"/")
}

// forbiddenInDomain are packages that would give the domain layer I/O.
var forbiddenInDomain = []string{
	"net/http", "net", "database/sql", "os", "os/exec",
	"log", "log/slog", "io/ioutil", "path/filepath",
}

func loadPackages(t *testing.T, pattern string) []*packages.Package {
	t.Helper()
	cfg := &packages.Config{Mode: packages.NeedName | packages.NeedImports, Dir: repoRoot(t)}
	pkgs, err := packages.Load(cfg, pattern)
	require.NoError(t, err)
	require.NotEmpty(t, pkgs)
	return pkgs
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)
	return filepath.Join(wd, "..", "..")
}

// TestDomainHasNoProjectImports is named in the F0 acceptance criteria.
func TestDomainHasNoProjectImports(t *testing.T) {
	for _, pkg := range loadPackages(t, "./internal/domain/...") {
		for imported := range pkg.Imports {
			if underPackage(imported, modulePath) &&
				!underPackage(imported, modulePath+"/internal/domain") {
				t.Errorf("%s imports %s: internal/domain must not import project packages", pkg.PkgPath, imported)
			}
		}
	}
}

func TestDomainHasNoIOImports(t *testing.T) {
	for _, pkg := range loadPackages(t, "./internal/domain/...") {
		for imported := range pkg.Imports {
			for _, forbidden := range forbiddenInDomain {
				if imported == forbidden {
					t.Errorf("%s imports %s: internal/domain must be free of I/O", pkg.PkgPath, imported)
				}
			}
		}
	}
}

func TestOnlyTheCLIImportsTheAPIPackage(t *testing.T) {
	allowed := map[string]bool{
		modulePath + "/internal/cli": true,
		modulePath + "/internal/api": true,
	}
	for _, pkg := range loadPackages(t, "./...") {
		if allowed[pkg.PkgPath] || underPackage(pkg.PkgPath, modulePath+"/internal/api") {
			continue
		}
		for imported := range pkg.Imports {
			if underPackage(imported, modulePath+"/internal/api") {
				t.Errorf("%s imports %s: only internal/cli may wire the HTTP layer", pkg.PkgPath, imported)
			}
		}
	}
}

func TestStoreAndIntegrationDoNotImportAPIOrService(t *testing.T) {
	for _, pkg := range loadPackages(t, "./internal/store/...") {
		for imported := range pkg.Imports {
			require.False(t, underPackage(imported, modulePath+"/internal/api"),
				"%s must not import the HTTP layer", pkg.PkgPath)
		}
	}
}

// TestNoTLSVerificationBypass guards removed-behaviour item 16.
//
// Deviation from the brief: the brief's sample walk has no exemption for
// its own source file. Since this test's own code necessarily contains the
// literal string "InsecureSkipVerify" (the substring it searches for), an
// unmodified copy of the brief's walk fails against itself the moment it
// runs — verified live: it does, with arch_test.go named as the offending
// file. The requirement ("InsecureSkipVerify does not appear" in real code)
// wins over the sample's walk, so this file is exempted by its own path,
// obtained via runtime.Caller rather than a hardcoded string, so the
// exemption cannot silently widen if the file is ever renamed or moved.
func TestNoTLSVerificationBypass(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller(0) must resolve this test's own file")
	root := filepath.Join(repoRoot(t), "internal")
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		if path == thisFile {
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if strings.Contains(string(content), "InsecureSkipVerify") {
			t.Errorf("%s contains InsecureSkipVerify: TLS verification is never disabled", path)
		}
		return nil
	})
	require.NoError(t, err)
}

// loggerHandlerConstructionAllowlist names the only non-test file permitted
// to construct a bare slog.NewJSONHandler/slog.NewTextHandler.
// internal/platform/logging/logging.go (internal/platform/logging/logging.go)
// is where that construction legitimately happens: New() wraps whichever
// bare handler it builds in redactHandler (internal/platform/logging/redact.go)
// before returning it, so that file is the one place a bare handler is
// supposed to exist. Every other call site must go through logging.New.
//
// This is deliberately a single exact file path, not a directory or package
// prefix, so a new file added anywhere else — including a new file inside
// internal/platform/logging itself — is not silently exempted.
//
// Why this matters: redactAttr (internal/platform/logging/redact.go:63-76)
// is key-based only. It calls isSecretKey(a.Key) and never inspects the
// attribute's value, so a slog.String("error", err.Error()) call is only
// safe because logging.New's handler wraps the underlying handler; a call
// site that builds its own slog.NewJSONHandler/slog.NewTextHandler bypasses
// that wrapping entirely and every attribute value it logs — including
// error strings that may contain secrets — reaches the log unredacted.
//
// The walk covers all of internal/, not just internal/cli, because the
// requirement ("every long-running command's logger must go through
// logging.New") is not scoped to the CLI package: any future package that
// wires up a real (non-test) logger is bound by the same rule.
var loggerHandlerConstructionAllowlist = map[string]bool{
	"internal/platform/logging/logging.go": true,
}

// TestOnlyLoggingPackageConstructsBareSlogHandlers is the carried-forward
// guard from task 9's review (Minor-4): before newCommandLogger existed,
// reverting a call site's logger construction to a bare
// slog.NewJSONHandler compiled and passed the entire test suite silently,
// because internal/cli's unit test for newCommandLogger only pinned the
// helper itself, not every caller. This test pins every caller, repo-wide.
func TestOnlyLoggingPackageConstructsBareSlogHandlers(t *testing.T) {
	root := filepath.Join(repoRoot(t), "internal")
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, relErr := filepath.Rel(repoRoot(t), path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		if loggerHandlerConstructionAllowlist[rel] {
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if strings.Contains(string(content), "slog.NewJSONHandler") || strings.Contains(string(content), "slog.NewTextHandler") {
			t.Errorf("%s constructs a bare slog handler: every long-running command's logger must go through logging.New (internal/platform/logging/logging.go), whose redacting handler is the only thing keeping secret-looking attribute values out of the logs", rel)
		}
		return nil
	})
	require.NoError(t, err)
}
