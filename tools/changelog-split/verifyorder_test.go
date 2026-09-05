// SPDX-License-Identifier: MIT

package main

// `--verify` reported drift/missing entries in Go's randomised map order, so the
// same broken tree produced a differently-ordered message on every run — noise
// in CI logs and un-diffable gate output. The report must be ordered by path.
//
// The kinds are deliberately INTERLEAVED (drift, missing, drift, drift): sorting
// by message text would group every "drift" before every "missing", which is a
// different contract than "ordered by path".

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyResultReportsPathsInSortedOrder(t *testing.T) {
	res := buildSample(t)
	dir := t.TempDir()
	out := filepath.Join(dir, "CHANGELOG")
	if err := writeResult(filepath.Join(dir, "CHANGELOG.md"), out, res); err != nil {
		t.Fatalf("writeResult: %v", err)
	}

	readme := filepath.Join(out, "README.md")
	reorg := filepath.Join(out, "REORG-LOG.md")
	current := filepath.Join(out, "unreleased", "current.md")
	rel := filepath.Join(out, "v0.1.0.md")

	tamper := func(path string) {
		t.Helper()
		if err := os.WriteFile(path, []byte("tampered\n"), 0o644); err != nil {
			t.Fatalf("tamper %s: %v", path, err)
		}
	}
	tamper(readme)
	if err := os.Remove(reorg); err != nil { // removed → reported as "missing"
		t.Fatalf("remove %s: %v", reorg, err)
	}
	tamper(current)
	tamper(rel)

	// Slash form and ASCII order of the differing suffixes:
	// README.md < REORG-LOG.md < unreleased/current.md < v0.1.0.md
	want := strings.Join([]string{
		"drift " + filepath.ToSlash(readme),
		"missing " + filepath.ToSlash(reorg),
		"drift " + filepath.ToSlash(current),
		"drift " + filepath.ToSlash(rel),
	}, "; ")

	// One pass proves nothing: an unsorted implementation has a 1/4! chance of
	// landing in sorted order per run, so repeat until a lucky ordering is not
	// credible (~10^-42 for 32 passes).
	for i := 0; i < 32; i++ {
		err := verifyResult(out, res)
		if err == nil {
			t.Fatal("expected verify to report the tampered tree")
		}
		if got := err.Error(); got != want {
			t.Fatalf("run %d: verify message not path-ordered\n got: %s\nwant: %s", i, got, want)
		}
	}
}

// TestVerifyResultSortsAcrossPlatformSeparators pins that the printed path uses
// forward slashes, so the message text is identical on Windows and POSIX rather
// than merely self-consistent per platform.
func TestVerifyResultSortsAcrossPlatformSeparators(t *testing.T) {
	res := buildSample(t)
	dir := t.TempDir()
	out := filepath.Join(dir, "CHANGELOG")
	if err := writeResult(filepath.Join(dir, "CHANGELOG.md"), out, res); err != nil {
		t.Fatalf("writeResult: %v", err)
	}
	p := filepath.Join(out, "unreleased", "current.md")
	if err := os.WriteFile(p, []byte("tampered\n"), 0o644); err != nil {
		t.Fatalf("tamper: %v", err)
	}
	err := verifyResult(out, res)
	if err == nil {
		t.Fatal("expected a drift report")
	}
	if strings.Contains(err.Error(), "\\") {
		t.Errorf("verify message leaked an OS separator: %v", err)
	}
	if want := "drift " + filepath.ToSlash(p); !strings.Contains(err.Error(), want) {
		t.Errorf("verify message = %v, want it to contain %q", err, want)
	}
}
