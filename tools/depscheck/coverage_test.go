// SPDX-License-Identifier: MIT

package main

// Direct unit tests for the depscheck helper functions. The pre-existing
// main_test.go exercises the tool end-to-end via `go run`, which runs in a
// separate process and therefore records zero statement coverage for this
// package. These tests call readAllowlist, listModules, and stderr in-process
// so every reachable branch is measured.
//
// main() and fail() both terminate with os.Exit(1); Go's -coverprofile writer
// flushes via an atexit hook that os.Exit bypasses, so their bodies cannot be
// covered through a `go test -coverprofile` run without a source refactor
// (extracting the exit call behind an injectable func). They are the only
// remaining uncovered lines and are documented here as such.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestReadAllowlistParsesEntries covers the happy path of readAllowlist:
// non-empty lines become map keys, while blank lines and '#' comments are
// skipped. Writing a temp file lets us assert on exact, controlled content.
func TestReadAllowlistParsesEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "allowlist.txt")
	content := "" +
		"# a comment line\n" +
		"\n" +
		"   \n" +
		"github.com/example/one\n" +
		"  github.com/example/two  \n" + // surrounding whitespace is trimmed
		"# another comment\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp allowlist: %v", err)
	}

	got, err := readAllowlist(path)
	if err != nil {
		t.Fatalf("readAllowlist returned error: %v", err)
	}

	want := map[string]bool{
		"github.com/example/one": true,
		"github.com/example/two": true,
	}
	if len(got) != len(want) {
		t.Fatalf("got %d entries, want %d: %v", len(got), len(want), got)
	}
	for k := range want {
		if !got[k] {
			t.Errorf("expected allowlist to contain %q", k)
		}
	}
	// Comments and blank lines must never leak into the map.
	if got["# a comment line"] || got["# another comment"] || got[""] {
		t.Errorf("comment or blank line leaked into allowlist: %v", got)
	}
}

// TestReadAllowlistMissingFile covers the error branch: os.Open fails for a
// path that does not exist, and readAllowlist propagates that error with a nil
// map.
func TestReadAllowlistMissingFile(t *testing.T) {
	got, err := readAllowlist(filepath.Join(t.TempDir(), "does-not-exist.txt"))
	if err == nil {
		t.Fatal("expected error for missing allowlist file, got nil")
	}
	if got != nil {
		t.Errorf("expected nil map on error, got %v", got)
	}
}

// TestListModulesSuccess covers listModules' happy path. It shells out to the
// real `go list -m` in the module root, so the result should contain the
// module's own dependencies (at minimum, a non-empty slice with no blank
// entries — blanks are filtered by the trim/skip logic).
func TestListModulesSuccess(t *testing.T) {
	root := repoRoot(t)
	// listModules runs `go` from the current working directory, so switch to
	// the module root for the duration of the call.
	restore := chdir(t, root)
	defer restore()

	mods, err := listModules()
	if err != nil {
		t.Fatalf("listModules returned error: %v", err)
	}
	if len(mods) == 0 {
		t.Fatal("expected at least one module in the build list")
	}
	for _, m := range mods {
		if strings.TrimSpace(m) == "" {
			t.Errorf("listModules returned a blank module entry")
		}
	}
}

// TestListModulesError covers the error branch of listModules. Running the
// `go list` command from a directory that is not part of any module makes the
// go tool exit non-zero, which listModules wraps (exercising the stderr
// helper's ExitError path too).
func TestListModulesError(t *testing.T) {
	// A bare temp dir has no go.mod, so `go list -m all` fails.
	restore := chdir(t, t.TempDir())
	defer restore()

	// Guard against the ambient GOFLAGS/GO111MODULE making this succeed; force
	// module mode so `go list -m` genuinely needs a module and errors out.
	t.Setenv("GO111MODULE", "on")

	mods, err := listModules()
	if err == nil {
		t.Fatalf("expected error running listModules outside a module, got mods=%v", mods)
	}
	if mods != nil {
		t.Errorf("expected nil slice on error, got %v", mods)
	}
}

// TestStderrExitError covers stderr's ExitError branch: a command that exits
// non-zero and writes to stderr yields a *exec.ExitError whose Stderr field is
// returned verbatim.
func TestStderrExitError(t *testing.T) {
	// `go` with a bogus subcommand exits non-zero and prints to stderr.
	cmd := exec.Command(filepath.Join(goBin(t), "go"), "this-is-not-a-command")
	_, err := cmd.Output()
	if err == nil {
		t.Fatal("expected the bogus go command to fail")
	}
	got := stderr(err)
	if got == "" {
		t.Error("expected stderr() to return the captured stderr for an ExitError")
	}
}

// TestStderrNonExitError covers stderr's fallback branch: for any error that
// is not a *exec.ExitError, the function returns the empty string.
func TestStderrNonExitError(t *testing.T) {
	if got := stderr(os.ErrNotExist); got != "" {
		t.Errorf("expected empty string for non-ExitError, got %q", got)
	}
}

// TestMainHappyPath calls main() in-process from the repository root, where the
// allowlist is complete and every module is justified, so main runs to its
// final "OK:" print and returns normally (no os.Exit). This covers the entire
// success path of main().
//
// The two error branches in main (fail on allowlist/list errors) and the
// len(unjustified) > 0 block all end in os.Exit(1); Go's -coverprofile atexit
// flush is skipped by os.Exit, so those statements — and fail() itself — are
// the only lines that remain uncovered and cannot be measured without a source
// refactor that injects the exit behavior. main's happy path below is the
// maximal in-process coverage achievable.
func TestMainHappyPath(t *testing.T) {
	restore := chdir(t, repoRoot(t))
	defer restore()

	// main() writes "OK: ..." to os.Stdout and returns. If the repository's
	// allowlist were incomplete it would call os.Exit(1) and abort the test
	// process, but that is exactly the invariant the CI keeps green, so a clean
	// checkout runs main to completion here.
	main()
}

// chdir switches the process working directory to dir and returns a function
// that restores the original. Helper kept local so the coverage tests are
// self-contained.
func chdir(t *testing.T, dir string) func() {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
	return func() {
		if err := os.Chdir(orig); err != nil {
			t.Fatalf("restore chdir %s: %v", orig, err)
		}
	}
}
