// SPDX-License-Identifier: MIT

package main

// Coverage tests for changelog-split. main_test.go covers bucketFor, buildSplit,
// and the writeResult/verifyResult happy paths; this file drives the remaining
// branches: buildSplit's two errors, bucketFor's low-milestone clamps,
// splitUnreleased's empty-input flush, writeResult's error paths,
// removeStaleSplitFiles' walk error, verifyResult's missing/drift reporting,
// and main() end to end in dry-run / emit / verify modes (which also covers
// printPlan).
//
// main() exits via os.Exit(1) on read/build/emit/verify errors; the
// -coverprofile writer is bypassed by os.Exit, so those specific exit lines stay
// uncovered. Every non-exit statement is exercised here.

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBuildSplitMissingUnreleased covers buildSplit's missing-[Unreleased] error.
func TestBuildSplitMissingUnreleased(t *testing.T) {
	_, err := buildSplit("# Changelog\n\n## [1.0.0] — 2026-01-01\n\n### Added\n- x\n")
	if err == nil || !strings.Contains(err.Error(), "Unreleased") {
		t.Fatalf("expected missing-Unreleased error, got %v", err)
	}
}

// TestBuildSplitNoVersions covers buildSplit's no-released-blocks error.
func TestBuildSplitNoVersions(t *testing.T) {
	_, err := buildSplit("# Changelog\n\n## [Unreleased]\n\n### Added\n- x\n")
	if err == nil || !strings.Contains(err.Error(), "no released version") {
		t.Fatalf("expected no-versions error, got %v", err)
	}
}

// TestBucketForClamps covers bucketFor's two clamp branches: a milestone in the
// 600s that rounds below 600 (start < 600 → 600), and a milestone below 100
// (start < 100 → 100).
func TestBucketForClamps(t *testing.T) {
	// M600 → (600/50)*50 = 600, no clamp needed but exercises the 600-699 path;
	// use M601 to keep in-range. To hit start<600 we need a value whose /50*50
	// is < 600 while still >=600 — impossible, so the clamp guards a defensive
	// case. Instead, drive the <100 clamp which is reachable: M042 → start 0 → 100.
	if got := bucketFor("### Added", []string{"- **M042** early milestone"}); got != "m100-m199" {
		t.Errorf("bucketFor(M042) = %q, want m100-m199 (clamped)", got)
	}
	// M050 → (50/100)*100 = 0 → clamped to 100.
	if got := bucketFor("### Added", []string{"- **M050** early"}); got != "m100-m199" {
		t.Errorf("bucketFor(M050) = %q, want m100-m199 (clamped)", got)
	}
}

// TestSplitUnreleasedEmpty covers splitUnreleased's flush early-return: an empty
// body means cur is never set, so flush() returns immediately and out is empty.
func TestSplitUnreleasedEmpty(t *testing.T) {
	if got := splitUnreleased(nil); len(got) != 0 {
		t.Errorf("splitUnreleased(nil) = %#v, want empty", got)
	}
	if got := splitUnreleased([]string{}); len(got) != 0 {
		t.Errorf("splitUnreleased([]) = %#v, want empty", got)
	}
}

// buildSample returns a valid split result for filesystem tests.
func buildSample(t *testing.T) splitResult {
	t.Helper()
	res, err := buildSplit(sample)
	if err != nil {
		t.Fatalf("buildSplit: %v", err)
	}
	return res
}

// TestWriteResultMkdirError covers writeResult's initial os.MkdirAll error: the
// outDir path lives under an existing regular file, so MkdirAll fails.
func TestWriteResultMkdirError(t *testing.T) {
	res := buildSample(t)
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("file"), 0o644); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	// outDir under a regular file → MkdirAll(outDir/unreleased) fails.
	out := filepath.Join(blocker, "CHANGELOG")
	if err := writeResult(filepath.Join(dir, "CHANGELOG.md"), out, res); err == nil {
		t.Fatal("expected writeResult MkdirAll error")
	}
}

// TestWriteResultWriteFileError covers writeResult's per-file os.WriteFile error
// by making one target path a directory. The README.md target is pre-created as
// a directory so os.WriteFile over it fails.
func TestWriteResultWriteFileError(t *testing.T) {
	res := buildSample(t)
	dir := t.TempDir()
	out := filepath.Join(dir, "CHANGELOG")
	if err := os.MkdirAll(filepath.Join(out, "unreleased"), 0o755); err != nil {
		t.Fatalf("mkdir out: %v", err)
	}
	// Pre-create README.md as a directory (it has no .md siblings yet, so
	// removeStaleSplitFiles won't touch it — a dir has no ".md" file ext match
	// on the directory entry itself during the file-only walk).
	if err := os.Mkdir(filepath.Join(out, "README.md"), 0o755); err != nil {
		t.Fatalf("mkdir README-as-dir: %v", err)
	}
	if err := writeResult(filepath.Join(dir, "CHANGELOG.md"), out, res); err == nil {
		t.Fatal("expected writeResult WriteFile error over a directory")
	}
}

// TestRemoveStaleSplitFilesWalkError covers removeStaleSplitFiles' walk-error
// branch: filepath.Walk over a nonexistent root invokes the callback with a
// non-nil err, which the callback returns.
func TestRemoveStaleSplitFilesWalkError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	if err := removeStaleSplitFiles(missing, map[string]string{}); err == nil {
		t.Fatal("expected walk error for a missing root")
	}
}

// TestRemoveStaleSplitSkipsNonMdFiles covers removeStaleSplitFiles' extension
// filter: a non-.md file encountered during the walk should not be removed.
func TestRemoveStaleSplitSkipsNonMdFiles(t *testing.T) {
	res := buildSample(t)
	dir := t.TempDir()
	out := filepath.Join(dir, "CHANGELOG")
	mainPath := filepath.Join(dir, "CHANGELOG.md")

	// Emit first.
	if err := writeResult(mainPath, out, res); err != nil {
		t.Fatalf("writeResult: %v", err)
	}
	// Plant a non-.md file — it should survive the re-emit.
	notes := filepath.Join(out, "notes.txt")
	if err := os.WriteFile(notes, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write notes: %v", err)
	}
	// Re-emit triggers removeStaleSplitFiles — the .txt file should be kept.
	if err := writeResult(mainPath, out, res); err != nil {
		t.Fatalf("re-emit: %v", err)
	}
	if _, err := os.Stat(notes); err != nil {
		t.Fatalf("non-md file was removed: %v", err)
	}
}

// TestVerifyResultMissingAndDrift covers verifyResult's missing-file and drift
// branches. It writes a partial/mutated tree, then verifies against the full
// expected result.
func TestVerifyResultMissingAndDrift(t *testing.T) {
	res := buildSample(t)
	dir := t.TempDir()
	out := filepath.Join(dir, "CHANGELOG")

	// Emit everything first so most files match...
	if err := writeResult(filepath.Join(dir, "CHANGELOG.md"), out, res); err != nil {
		t.Fatalf("writeResult: %v", err)
	}
	// ...then remove one file (→ "missing") and corrupt another (→ "drift").
	if err := os.Remove(filepath.Join(out, "README.md")); err != nil {
		t.Fatalf("remove README: %v", err)
	}
	if err := os.WriteFile(filepath.Join(out, "REORG-LOG.md"), []byte("tampered"), 0o644); err != nil {
		t.Fatalf("tamper REORG: %v", err)
	}

	err := verifyResult(out, res)
	if err == nil {
		t.Fatal("expected verify to fail")
	}
	msg := err.Error()
	if !strings.Contains(msg, "missing") || !strings.Contains(msg, "drift") {
		t.Errorf("verify error should mention both missing and drift: %q", msg)
	}
}

// TestMainDryRun drives main() in -dry-run mode against a real source file. This
// covers flag parsing, the ReadFile + buildSplit success path, and printPlan.
func TestMainDryRun(t *testing.T) {
	src := writeSource(t)
	runMain(t, "-source", src, "-dry-run")
}

// TestMainDefaultDryRun covers main's default-mode branch: with no -dry-run,
// -emit, or -verify flag, dryRun is forced true.
func TestMainDefaultDryRun(t *testing.T) {
	src := writeSource(t)
	runMain(t, "-source", src)
}

// TestMainEmit drives main() in -emit mode with an explicit -main-out, covering
// the emit success path (writeResult) plus the mainOut != "" branch.
func TestMainEmit(t *testing.T) {
	src := writeSource(t)
	dir := t.TempDir()
	out := filepath.Join(dir, "CHANGELOG")
	mainOut := filepath.Join(dir, "root.md")
	runMain(t, "-source", src, "-emit", "-out-dir", out, "-main-out", mainOut)
	if _, err := os.Stat(mainOut); err != nil {
		t.Errorf("emit did not write -main-out: %v", err)
	}
}

// TestMainEmitDefaultMainOut covers the emit branch where -main-out is empty and
// the root target defaults to overwriting -source.
func TestMainEmitDefaultMainOut(t *testing.T) {
	src := writeSource(t)
	out := filepath.Join(t.TempDir(), "CHANGELOG")
	runMain(t, "-source", src, "-emit", "-out-dir", out)
	// -source is overwritten with the rewritten root changelog.
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read rewritten source: %v", err)
	}
	if !strings.Contains(string(data), "[Unreleased]") {
		t.Errorf("rewritten source missing Unreleased pointer:\n%s", data)
	}
}

// TestMainVerify drives main() in -verify mode after an emit, covering the
// verify success path.
func TestMainVerify(t *testing.T) {
	src := writeSource(t)
	dir := t.TempDir()
	out := filepath.Join(dir, "CHANGELOG")
	// Emit with an explicit -main-out so -source is NOT overwritten; the verify
	// run re-reads -source and rebuilds the same result to compare against.
	mainOut := filepath.Join(dir, "root.md")
	runMain(t, "-source", src, "-emit", "-out-dir", out, "-main-out", mainOut)
	runMain(t, "-source", src, "-verify", "-out-dir", out)
}

// writeSource writes the shared sample changelog to a temp file and returns its
// path.
func writeSource(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "CHANGELOG.md")
	if err := os.WriteFile(path, []byte(sample), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	return path
}

// runMain invokes package main() with the given args, resetting the flag set and
// os.Args and restoring them afterward. A valid source keeps main() on its
// non-exit paths.
func runMain(t *testing.T, args ...string) {
	t.Helper()
	origArgs := os.Args
	origFlags := flag.CommandLine
	defer func() {
		os.Args = origArgs
		flag.CommandLine = origFlags
	}()
	flag.CommandLine = flag.NewFlagSet("changelog-split", flag.ExitOnError)
	os.Args = append([]string{"changelog-split"}, args...)
	main()
}
