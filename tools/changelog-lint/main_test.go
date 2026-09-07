// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLintHappyPath(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "CHANGELOG")
	mustWrite := func(path, content string) {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	mustWrite(filepath.Join(dir, "CHANGELOG.md"), "# Changelog\n\n## [Unreleased]\n\nSee `CHANGELOG/unreleased/current.md`.\n\n## [1.0.0] — 2026-06-03\n\n### Added\n- release\n")
	mustWrite(filepath.Join(root, "README.md"), "# Changelog\n\nreadme\n")
	mustWrite(filepath.Join(root, "REORG-LOG.md"), "# Changelog Reorg Log\n\nlog\n")
	mustWrite(filepath.Join(root, "unreleased", "current.md"), "# Changelog — current\n\n### Added\n- work\n")
	mustWrite(filepath.Join(root, "unreleased", "m100-m199.md"), "# Changelog — m100-m199\n\n### Added\n- old\n")
	mustWrite(filepath.Join(root, "v1.0.0.md"), "# Changelog\n\n## [1.0.0] — 2026-06-03\n\n### Added\n- release\n")
	if err := lint(filepath.Join(dir, "CHANGELOG.md"), root); err != nil {
		t.Fatalf("lint happy path: %v", err)
	}
}

func TestLintMissingMainPointerFails(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "CHANGELOG")
	_ = os.MkdirAll(filepath.Join(root, "unreleased"), 0o755)
	if err := os.WriteFile(filepath.Join(dir, "CHANGELOG.md"), []byte("# Changelog\n\n## [Unreleased]\n\nno pointer\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# Changelog\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "REORG-LOG.md"), []byte("# Changelog Reorg Log\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "unreleased", "current.md"), []byte("# Changelog\n\n### Added\n- x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := lint(filepath.Join(dir, "CHANGELOG.md"), root)
	if err == nil {
		t.Fatal("expected missing pointer error")
	}
}

func TestLintUnexpectedBucketFileFails(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "CHANGELOG")
	mustWrite := func(path, content string) {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	mustWrite(filepath.Join(dir, "CHANGELOG.md"), "# Changelog\n\n## [Unreleased]\n\nSee `CHANGELOG/unreleased/current.md`.\n\n## [1.0.0] — 2026-06-03\n\n### Added\n- release\n")
	mustWrite(filepath.Join(root, "README.md"), "# Changelog\n")
	mustWrite(filepath.Join(root, "REORG-LOG.md"), "# Changelog Reorg Log\n")
	mustWrite(filepath.Join(root, "unreleased", "current.md"), "# Changelog\n\n### Added\n- x\n")
	mustWrite(filepath.Join(root, "unreleased", "weird.md"), "# Changelog\n\n### Added\n- weird\n")
	mustWrite(filepath.Join(root, "v1.0.0.md"), "# Changelog\n\n## [1.0.0] — 2026-06-03\n\n### Added\n- release\n")
	err := lint(filepath.Join(dir, "CHANGELOG.md"), root)
	if err == nil {
		t.Fatal("expected unexpected bucket file error")
	}
}

// layoutTree writes a minimal VALID split tree and returns the lint arguments.
func layoutTree(t *testing.T) (dir, root string, mustWrite func(path, content string)) {
	t.Helper()
	dir = t.TempDir()
	root = filepath.Join(dir, "CHANGELOG")
	mustWrite = func(path, content string) {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	mustWrite(filepath.Join(dir, "CHANGELOG.md"), "# Changelog\n\n## [Unreleased]\n\nSee `CHANGELOG/unreleased/current.md`.\n\n## [1.0.0] — 2026-06-03\n\n### Added\n- release\n")
	mustWrite(filepath.Join(root, "README.md"), "# Changelog\n")
	mustWrite(filepath.Join(root, "REORG-LOG.md"), "# Changelog Reorg Log\n")
	mustWrite(filepath.Join(root, "unreleased", "current.md"), "# Changelog\n\n### Added\n- x\n")
	mustWrite(filepath.Join(root, "v1.0.0.md"), "# Changelog\n\n## [1.0.0] — 2026-06-03\n\n### Added\n- release\n")
	return dir, root, mustWrite
}

// TestLintAcceptsWorkingSetBackups is the regression guard for an interaction
// between the two changelog tools: changelog-split's backupFile writes
// "<name>.bak-<UTC timestamp>" BESIDE the file it protects, so a
// --discard-working-set run legitimately leaves e.g.
// unreleased/current.md.bak-20260101T000000Z on disk. The unreleased allow-list
// must accept those, or the changelog-lint gate turns red on a tree the sibling
// tool just made safe — punishing the operator for taking the backup at all.
func TestLintAcceptsWorkingSetBackups(t *testing.T) {
	dir, root, mustWrite := layoutTree(t)
	mustWrite(filepath.Join(root, "unreleased", "current.md.bak-20260101T000000Z"),
		"# Changelog — current\n\n### Added\n- prior working set, preserved by --discard-working-set\n")
	mustWrite(filepath.Join(root, "unreleased", "m100-m199.md.bak-20260101T000000Z"),
		"# Changelog — m100-m199\n\n### Added\n- prior bucket content\n")
	mustWrite(filepath.Join(root, "v1.0.0.md.bak-20260101T000000Z"),
		"# Changelog\n\n## [1.0.0] — 2026-06-03\n\n### Added\n- superseded\n")

	if err := lint(filepath.Join(dir, "CHANGELOG.md"), root); err != nil {
		t.Fatalf("a --discard-working-set backup must not fail the layout gate: %v", err)
	}
}

// TestLintStillRejectsMalformedBackups is the counter-direction guard: exempting
// backups must not widen the allow-list. Only the exact ".md.bak-<UTC timestamp>"
// suffix that backupFile writes is excused — a look-alike with a malformed
// or missing timestamp is still an unexpected file. (A truncated-looking bucket
// range such as m100-m19.md is deliberately NOT listed here: unreleasedBucketRe is
// \d+ on both sides, so any digit width is valid by design.)
func TestLintStillRejectsMalformedBackups(t *testing.T) {
	for _, name := range []string{
		"current.md.bak-notatimestamp",   // not the tool's timestamp format
		"current.md.bak-",                // empty timestamp
		"current.md.bak-20260101T000000", // missing trailing Z
		"current.md.bak-20260101T00000Z", // wrong digit count
		"weird.md",                       // the pre-existing rejection case
	} {
		dir, root, mustWrite := layoutTree(t)
		mustWrite(filepath.Join(root, "unreleased", name), "# Changelog\n\n### Added\n- x\n")
		if err := lint(filepath.Join(dir, "CHANGELOG.md"), root); err == nil {
			t.Errorf("unexpected file %q should still fail the gate, got nil", name)
		}
	}
}

// TestLintRejectsStrayMarkdownAtSplitRoot is the mirror image of the unreleased
// directory check. The root loop matched names against releaseFileRe and validated
// the ones that hit, but had NO else branch, so any unrecognized .md file was
// silently skipped and the gate stayed green. That is how "vv1.1.0.md" — the
// doubled-prefix filename changelog-split actually emitted before its vFilename
// fix — passed validation while being referenced by nothing: the validator could
// not see the very class of defect its sibling tool had just produced.
func TestLintRejectsStrayMarkdownAtSplitRoot(t *testing.T) {
	for _, name := range []string{
		"vv1.1.0.md",  // the real artifact: doubled prefix, matches nothing
		"notes.md",    // hand-written file left at the split root
		"v1.0.md",     // two-component version: NOT ^v\d+\.\d+\.\d+\.md$
		"v1.0.0.1.md", // four components
		"RELEASE.md",  // plausible but unmapped
	} {
		dir, root, mustWrite := layoutTree(t)
		mustWrite(filepath.Join(root, name), "# Changelog\n\n### Added\n- x\n")
		if err := lint(filepath.Join(dir, "CHANGELOG.md"), root); err == nil {
			t.Errorf("stray markdown %q at the split root should fail the gate, got nil", name)
		} else if !strings.Contains(err.Error(), "unexpected file in split changelog dir") {
			t.Errorf("stray markdown %q gave the wrong error: %v", name, err)
		}
	}
}

// TestLintAcceptsKnownRootFiles is the counter-direction guard: closing the root
// hole must not reject anything the layout legitimately contains. It covers the
// valid release file, a root-level discard backup, the required README/REORG-LOG
// pair, a subdirectory, and a non-markdown stray file (notes.txt is deliberately
// tolerated — coverage_test.go:132 depends on that, and the ask is scoped to .md).
func TestLintAcceptsKnownRootFiles(t *testing.T) {
	dir, root, mustWrite := layoutTree(t)
	// layoutTree already writes README.md, REORG-LOG.md and a valid v1.0.0.md.
	mustWrite(filepath.Join(root, "v0.9.0.md"), "# Changelog\n\n## [0.9.0] — 2026-01-02\n\n### Added\n- older release\n")
	mustWrite(filepath.Join(root, "v1.0.0.md.bak-20260101T000000Z"), "# superseded bytes, kept verbatim\n")
	mustWrite(filepath.Join(root, "v0.9.0.md.bak-20260101T000000Z"), "# superseded bytes, kept verbatim\n")
	mustWrite(filepath.Join(root, "notes.txt"), "not markdown, tolerated\n")
	if err := os.MkdirAll(filepath.Join(root, "archive"), 0o755); err != nil {
		t.Fatalf("mkdir archive: %v", err)
	}
	if err := lint(filepath.Join(dir, "CHANGELOG.md"), root); err != nil {
		t.Fatalf("a legitimate split root was rejected: %v", err)
	}
}
