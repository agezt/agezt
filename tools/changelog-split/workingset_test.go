// SPDX-License-Identifier: MIT

package main

// Working-set loss gate. `--force` reclaims ownership of hand-held tree files,
// and adopting them used to mean silently replacing the canonical
// `unreleased/current.md` — 86,688 bytes in this repository — with the ~288
// byte stub a pointer-only root generates. Force means "adopt", not
// "discard the only copy of the working set", so discarding now needs its own
// opt-in plus a timestamped backup.
//
// The gate counts LINES, not bytes: a release slice legitimately shrinks
// current.md by moving `**M###**` chunks into milestone buckets, and that must
// keep working (TestLegitimateReleaseSliceShrinkIsAllowed).

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// bigWorkingSet returns a working set of at least min bytes made of DISTINCT
// lines. Distinctness matters: the gate accounts content line-wise, so filler
// that repeats one line would be "preserved" by a single copy in the generated
// output and the test would prove nothing.
func bigWorkingSet(t *testing.T, min int) string {
	t.Helper()
	var b strings.Builder
	b.WriteString("# Changelog — current\n\n" +
		"This file holds the active `[Unreleased]` working set.\n\n")
	for i := 1; b.Len() < min; i++ {
		fmt.Fprintf(&b, "- **M9%03d** working set entry %05d carries unique prose that no generated output reproduces\n", i%1000, i)
	}
	return b.String()
}

// stubRootCurrent is what an emit driven by a pointer-only root renders for
// current.md — the truncation target.
func stubRootCurrent(t *testing.T) string {
	t.Helper()
	res, err := buildSplit(pointerSample)
	if err != nil {
		t.Fatalf("buildSplit: %v", err)
	}
	return res.Current
}

func plantedCurrent(t *testing.T, out, content string) {
	t.Helper()
	path := filepath.Join(out, "unreleased", "current.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("plant current.md: %v", err)
	}
}

func backupsOf(t *testing.T, out string) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(out, "unreleased", "current.md.bak-*"))
	if err != nil {
		t.Fatalf("glob backups: %v", err)
	}
	return matches
}

// TestForceCannotTruncateWorkingSet is the headline regression: `--force` must
// refuse to turn an 86 KB working set into a few-hundred-byte stub.
func TestForceCannotTruncateWorkingSet(t *testing.T) {
	const target = 86_688 // the real CHANGELOG/unreleased/current.md size
	stub := stubRootCurrent(t)
	res, err := buildSplit(pointerSample)
	if err != nil {
		t.Fatalf("buildSplit: %v", err)
	}

	dir := t.TempDir()
	out := filepath.Join(dir, "CHANGELOG")
	working := bigWorkingSet(t, target)

	// Assert the premises of the bug before testing the fix, so a change to the
	// generator that quietly removes the size gap fails here loudly.
	if len(working) < target {
		t.Fatalf("working set is %d bytes, want >= %d", len(working), target)
	}
	if len(stub) >= target/10 {
		t.Fatalf("stub is %d bytes — the truncation scenario no longer holds", len(stub))
	}

	plantedCurrent(t, out, working)
	// Hand-held README lets us prove the refusal happens BEFORE any write.
	readme := filepath.Join(out, "README.md")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(readme, []byte("# Changelog\n\nhand-held, must not be touched\n"), 0o644); err != nil {
		t.Fatalf("plant README: %v", err)
	}

	err = writeResultForce(filepath.Join(dir, "CHANGELOG.md"), out, res, true, false)
	if err == nil {
		t.Fatal("--emit --force truncated the working set; expected a refusal")
	}
	if !strings.Contains(err.Error(), "refusing to shrink") {
		t.Fatalf("expected a working-set refusal, got: %v", err)
	}
	if got := mustRead(t, filepath.Join(out, "unreleased", "current.md")); got != working {
		t.Errorf("current.md changed despite the refusal: %d bytes -> %d bytes", len(working), len(got))
	}
	if got := mustRead(t, readme); !strings.Contains(got, "must not be touched") {
		t.Error("the refused emit still wrote other files; the gate must abort before any write")
	}
	if n := len(backupsOf(t, out)); n != 0 {
		t.Errorf("a refusal wrote %d backup(s); nothing should be created when the emit aborts", n)
	}
}

// TestDiscardWorkingSetBacksUpBeforeOverwrite covers the escape hatch: the
// separate opt-in proceeds, but only after preserving the discarded content.
func TestDiscardWorkingSetBacksUpBeforeOverwrite(t *testing.T) {
	res, err := buildSplit(pointerSample)
	if err != nil {
		t.Fatalf("buildSplit: %v", err)
	}
	dir := t.TempDir()
	out := filepath.Join(dir, "CHANGELOG")
	working := bigWorkingSet(t, 86_688)
	plantedCurrent(t, out, working)

	if err := writeResultForce(filepath.Join(dir, "CHANGELOG.md"), out, res, true, true); err != nil {
		t.Fatalf("--force --discard-working-set should proceed: %v", err)
	}
	if got := mustRead(t, filepath.Join(out, "unreleased", "current.md")); got != res.Current {
		t.Error("current.md should have been replaced once discard was explicitly opted into")
	}
	bak := backupsOf(t, out)
	if len(bak) != 1 {
		t.Fatalf("expected exactly 1 timestamped backup, got %d", len(bak))
	}
	if got := mustRead(t, bak[0]); got != working {
		t.Errorf("backup lost content: %d bytes want %d", len(got), len(working))
	}
}

// TestLegitimateReleaseSliceShrinkIsAllowed is the non-regression case that
// keeps this a guard rather than a veto: a release slice shrinks current.md by
// moving `**M###**` chunks into bucket files. That content IS in the write set,
// so it must be allowed without --discard-working-set.
func TestLegitimateReleaseSliceShrinkIsAllowed(t *testing.T) {
	res, err := buildSplit(sample)
	if err != nil {
		t.Fatalf("buildSplit: %v", err)
	}
	keys := make([]string, 0, len(res.Buckets))
	for k := range res.Buckets {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		t.Fatal("sample root no longer produces buckets; this test needs a real release slice")
	}

	dir := t.TempDir()
	out := filepath.Join(dir, "CHANGELOG")
	// current.md whose entire content is a generated bucket doc: it carries the
	// sentinel (so the ownership gate does not skip it) and every one of its
	// lines reappears in the write set (so the loss gate must stay silent).
	plantedCurrent(t, out, res.Buckets[keys[0]])

	if err := writeResultForce(filepath.Join(dir, "CHANGELOG.md"), out, res, false, false); err != nil {
		t.Fatalf("a legitimate release slice was refused: %v", err)
	}
	if got := mustRead(t, filepath.Join(out, "unreleased", "current.md")); got != res.Current {
		t.Error("current.md was not updated; a slice must be able to move content out of it")
	}
	if n := len(backupsOf(t, out)); n != 0 {
		t.Errorf("a slice that loses nothing wrote %d backup(s)", n)
	}
}

// TestWorkingSetLossAccountsLineWise pins the mechanism directly: content that
// survives in the generated output is not "lost", and content that does not is.
func TestWorkingSetLossAccountsLineWise(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "CHANGELOG")
	res, err := buildSplit(sample)
	if err != nil {
		t.Fatalf("buildSplit: %v", err)
	}
	keys := make([]string, 0, len(res.Buckets))
	for k := range res.Buckets {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	plantedCurrent(t, out, res.Current+"\n"+"- **M999** an entry that exists nowhere else\n")
	lost, err := workingSetLoss(out, res)
	if err != nil {
		t.Fatalf("workingSetLoss: %v", err)
	}
	if len(lost) != 1 || !strings.Contains(lost[0], "M999") {
		t.Fatalf("want exactly the one orphaned M999 line, got %#v", lost)
	}
}
