// SPDX-License-Identifier: MIT

package main

// Coverage tests for changelog-lint. main_test.go covers the happy path and two
// failure cases; this file drives every remaining error branch of
// checkMainChangelog, checkSplitTree, checkHeading, and checkChangelogLikeFile,
// plus main()'s in-process success path.
//
// main() exits via os.Exit(1) only when lint() errors; the -coverprofile writer
// is bypassed by os.Exit, so that one branch stays uncovered. Its success path
// (the final Println) is exercised here.

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tree builds a minimal valid split-changelog layout under a fresh temp dir and
// returns (mainPath, rootDir). Individual tests then corrupt one part to drive a
// specific error branch. Every file is intentionally well-formed by default.
func tree(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	root := filepath.Join(dir, "CHANGELOG")
	write(t, filepath.Join(dir, "CHANGELOG.md"),
		"# Changelog\n\n## [Unreleased]\n\nSee `CHANGELOG/unreleased/current.md`.\n\n## [1.0.0]\n\n### Added\n- x\n")
	write(t, filepath.Join(root, "README.md"), "# Changelog\n\nreadme\n")
	write(t, filepath.Join(root, "REORG-LOG.md"), "# Changelog Reorg Log\n\nlog\n")
	write(t, filepath.Join(root, "unreleased", "current.md"), "# Changelog — current\n\n### Added\n- work\n")
	write(t, filepath.Join(root, "v1.0.0.md"), "# Changelog\n\n### Added\n- release\n")
	return filepath.Join(dir, "CHANGELOG.md"), root
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// wantErr asserts lint returns an error containing substr.
func wantErr(t *testing.T, main, root, substr string) {
	t.Helper()
	err := lint(main, root)
	if err == nil {
		t.Fatalf("expected error containing %q, got nil", substr)
	}
	if substr != "" && !strings.Contains(err.Error(), substr) {
		t.Fatalf("error %q does not contain %q", err.Error(), substr)
	}
}

// TestCheckMainChangelogReadError covers checkMainChangelog's os.ReadFile error
// branch (main file does not exist).
func TestCheckMainChangelogReadError(t *testing.T) {
	_, root := tree(t)
	wantErr(t, filepath.Join(t.TempDir(), "nope.md"), root, "read")
}

// TestCheckMainChangelogMissingUnreleased covers the missing-[Unreleased] branch.
func TestCheckMainChangelogMissingUnreleased(t *testing.T) {
	main, root := tree(t)
	write(t, main, "# Changelog\n\nno unreleased header\n")
	wantErr(t, main, root, "[Unreleased]")
}

// TestCheckMainChangelogMissingPointer covers the missing-pointer branch (the
// [Unreleased] header exists but does not reference the current.md path).
func TestCheckMainChangelogMissingPointer(t *testing.T) {
	main, root := tree(t)
	write(t, main, "# Changelog\n\n## [Unreleased]\n\nno pointer here\n")
	wantErr(t, main, root, "current.md")
}

// TestCheckSplitTreeMissingRequired covers checkSplitTree's missing-required-file
// branch by removing README.md from an otherwise valid tree.
func TestCheckSplitTreeMissingRequired(t *testing.T) {
	main, root := tree(t)
	if err := os.Remove(filepath.Join(root, "README.md")); err != nil {
		t.Fatalf("remove README: %v", err)
	}
	wantErr(t, main, root, "missing required split file")
}

// TestCheckSplitTreeReadmeHeading covers the checkHeading failure for README.md.
func TestCheckSplitTreeReadmeHeading(t *testing.T) {
	main, root := tree(t)
	write(t, filepath.Join(root, "README.md"), "wrong heading\n")
	wantErr(t, main, root, "README.md")
}

// TestCheckSplitTreeReorgHeading covers the checkHeading failure for REORG-LOG.md.
func TestCheckSplitTreeReorgHeading(t *testing.T) {
	main, root := tree(t)
	write(t, filepath.Join(root, "REORG-LOG.md"), "# Changelog\n") // valid README-style, wrong for reorg
	wantErr(t, main, root, "REORG-LOG.md")
}

// TestCheckSplitTreeCurrentBadFile covers the checkChangelogLikeFile failure for
// unreleased/current.md (missing changelog heading).
func TestCheckSplitTreeCurrentBadFile(t *testing.T) {
	main, root := tree(t)
	write(t, filepath.Join(root, "unreleased", "current.md"), "not a changelog\n\n### Added\n- x\n")
	wantErr(t, main, root, "current.md")
}

// TestCheckSplitTreeReleaseFileBad covers the release-file checkChangelogLikeFile
// error branch (a vX.Y.Z.md that lacks a changelog heading).
func TestCheckSplitTreeReleaseFileBad(t *testing.T) {
	main, root := tree(t)
	write(t, filepath.Join(root, "v1.0.0.md"), "bad release file\n\n### Added\n- x\n")
	wantErr(t, main, root, "v1.0.0.md")
}

// TestCheckSplitTreeNoReleases covers the releaseCount == 0 branch by removing
// the only release file. A non-release, non-required file is added so the loop
// still iterates and skips it.
func TestCheckSplitTreeNoReleases(t *testing.T) {
	main, root := tree(t)
	if err := os.Remove(filepath.Join(root, "v1.0.0.md")); err != nil {
		t.Fatalf("remove release: %v", err)
	}
	// A stray non-release file exercises the loop body without incrementing the
	// release count (it is neither README/REORG nor a release pattern).
	write(t, filepath.Join(root, "notes.txt"), "ignored")
	wantErr(t, main, root, "no released version files")
}

// TestCheckSplitTreeUnexpectedBucket covers the unreleased-bucket-name rejection
// branch (a file in unreleased/ that is neither current.md nor a bucket pattern).
func TestCheckSplitTreeUnexpectedBucket(t *testing.T) {
	main, root := tree(t)
	write(t, filepath.Join(root, "unreleased", "weird.md"), "# Changelog\n\n### Added\n- x\n")
	wantErr(t, main, root, "unexpected file in split unreleased dir")
}

// TestCheckSplitTreeBucketBadFile covers the bucket-file checkChangelogLikeFile
// error branch: a validly-named bucket file whose content is not changelog-like.
func TestCheckSplitTreeBucketBadFile(t *testing.T) {
	main, root := tree(t)
	write(t, filepath.Join(root, "unreleased", "m100-m199.md"), "not a changelog\n\n### Added\n- x\n")
	wantErr(t, main, root, "m100-m199.md")
}

// TestCheckSplitTreeIgnoresSubdirs covers the e.IsDir() skip branches in both
// the root loop and the unreleased loop by planting directories that must be
// ignored, leaving an otherwise-valid tree that lints clean.
func TestCheckSplitTreeIgnoresSubdirs(t *testing.T) {
	main, root := tree(t)
	// A subdirectory in the split root and one in unreleased/ — both skipped.
	if err := os.MkdirAll(filepath.Join(root, "archive"), 0o755); err != nil {
		t.Fatalf("mkdir archive: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "unreleased", "nested"), 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	if err := lint(main, root); err != nil {
		t.Fatalf("expected clean lint with ignored subdirs, got %v", err)
	}
}

// TestCheckHeadingReadError covers checkHeading's os.ReadFile error branch by
// pointing the required README.md path at a directory (unreadable as a file).
func TestCheckHeadingReadError(t *testing.T) {
	main, root := tree(t)
	// Replace README.md with a directory so os.Stat passes (it exists) but
	// os.ReadFile inside checkHeading fails.
	readme := filepath.Join(root, "README.md")
	if err := os.Remove(readme); err != nil {
		t.Fatalf("remove readme: %v", err)
	}
	if err := os.Mkdir(readme, 0o755); err != nil {
		t.Fatalf("mkdir readme-as-dir: %v", err)
	}
	wantErr(t, main, root, "read")
}

// TestCheckChangelogLikeFileReadError covers checkChangelogLikeFile's read error
// branch (current.md replaced by a directory).
func TestCheckChangelogLikeFileReadError(t *testing.T) {
	main, root := tree(t)
	current := filepath.Join(root, "unreleased", "current.md")
	if err := os.Remove(current); err != nil {
		t.Fatalf("remove current: %v", err)
	}
	if err := os.Mkdir(current, 0o755); err != nil {
		t.Fatalf("mkdir current-as-dir: %v", err)
	}
	wantErr(t, main, root, "read")
}

// TestCheckChangelogLikeFileNoSection covers the requireSection branch: a file
// with a valid changelog heading but no `### ` subsection.
func TestCheckChangelogLikeFileNoSection(t *testing.T) {
	main, root := tree(t)
	write(t, filepath.Join(root, "unreleased", "current.md"), "# Changelog — current\n\njust prose, no subsection\n")
	wantErr(t, main, root, "subsection")
}

// TestCheckHeadingExactMatch covers checkHeading's "text == prefix" acceptance
// branch (a file whose entire content is exactly the heading, no trailing text).
func TestCheckHeadingExactMatch(t *testing.T) {
	main, root := tree(t)
	// README.md content is exactly the prefix with no trailing newline.
	write(t, filepath.Join(root, "README.md"), "# Changelog")
	if err := lint(main, root); err != nil {
		t.Fatalf("exact-heading README should lint clean, got %v", err)
	}
}

// TestMainSuccess drives main() in-process against a valid tree, covering flag
// parsing, the lint success path, and the final Println. Only the os.Exit error
// branch stays uncovered.
func TestMainSuccess(t *testing.T) {
	mainPath, root := tree(t)

	origArgs := os.Args
	origFlags := flag.CommandLine
	defer func() {
		os.Args = origArgs
		flag.CommandLine = origFlags
	}()
	flag.CommandLine = flag.NewFlagSet("changelog-lint", flag.ExitOnError)
	os.Args = []string{"changelog-lint", "-main", mainPath, "-dir", root}

	// A valid tree makes lint() return nil, so main() prints OK and returns.
	main()
}
