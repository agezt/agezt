// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"path/filepath"
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
