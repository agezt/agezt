// SPDX-License-Identifier: MIT

// Command changelog-lint validates the split changelog layout produced by
// tools/changelog-split.
//
// It checks three things:
//  1. the root CHANGELOG.md still contains an `[Unreleased]` section that points
//     at `CHANGELOG/unreleased/current.md`;
//  2. the split tree exists (`CHANGELOG/README.md`, `CHANGELOG/REORG-LOG.md`,
//     `CHANGELOG/unreleased/current.md`, released version files, and optional
//     historical unreleased bucket files);
//  3. the split markdown files have a minimal valid shape (`# Changelog...`
//     heading and at least one `###` subsection where appropriate).
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var releaseFileRe = regexp.MustCompile(`^v\d+\.\d+\.\d+\.md$`)
var unreleasedBucketRe = regexp.MustCompile(`^m\d+-m\d+\.md$|^m\d+\+\.md$`)

// splitBackupRe matches the timestamped backups that tools/changelog-split writes
// BESIDE a file it is about to discard (--discard-working-set), e.g.
// "current.md.bak-20260101T000000Z". The suffix is exact — Go's reference layout
// "20060102T150405Z" — so a look-alike with a malformed stamp is still rejected.
// Those files are preserved history, not layout artifacts: the split tree is
// canonical, and the backup is what lets an operator recover a discarded working
// set. Failing the gate on them would punish taking the safety copy the sibling
// tool promises, and would turn a green tree red the moment it is adopted.
var splitBackupRe = regexp.MustCompile(`\.md\.bak-\d{8}T\d{6}Z$`)

func main() {
	mainPath := flag.String("main", "CHANGELOG.md", "root changelog file")
	rootDir := flag.String("dir", "CHANGELOG", "split changelog directory")
	flag.Parse()

	if err := lint(*mainPath, *rootDir); err != nil {
		fmt.Fprintf(os.Stderr, "changelog-lint: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("OK: changelog split layout looks valid.")
}

func lint(mainPath, rootDir string) error {
	if err := checkMainChangelog(mainPath); err != nil {
		return err
	}
	if err := checkSplitTree(rootDir); err != nil {
		return err
	}
	return nil
}

func checkMainChangelog(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	text := strings.ReplaceAll(string(b), "\r\n", "\n")
	if !strings.Contains(text, "## [Unreleased]") {
		return errors.New("root CHANGELOG.md is missing `## [Unreleased]`")
	}
	if !strings.Contains(text, "CHANGELOG/unreleased/current.md") {
		return errors.New("root CHANGELOG.md does not point at `CHANGELOG/unreleased/current.md`")
	}
	return nil
}

func checkSplitTree(root string) error {
	required := []string{
		filepath.Join(root, "README.md"),
		filepath.Join(root, "REORG-LOG.md"),
		filepath.Join(root, "unreleased", "current.md"),
	}
	for _, p := range required {
		if _, err := os.Stat(p); err != nil {
			return fmt.Errorf("missing required split file %s", filepath.ToSlash(p))
		}
	}
	if err := checkHeading(filepath.Join(root, "README.md"), "# Changelog"); err != nil {
		return err
	}
	if err := checkHeading(filepath.Join(root, "REORG-LOG.md"), "# Changelog Reorg Log"); err != nil {
		return err
	}
	if err := checkChangelogLikeFile(filepath.Join(root, "unreleased", "current.md"), true); err != nil {
		return err
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return fmt.Errorf("read split root %s: %w", root, err)
	}
	var releaseCount int
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if name == "README.md" || name == "REORG-LOG.md" {
			continue
		}
		if releaseFileRe.MatchString(name) {
			releaseCount++
			if err := checkChangelogLikeFile(filepath.Join(root, name), true); err != nil {
				return err
			}
		}
	}
	if releaseCount == 0 {
		return errors.New("split tree has no released version files (expected at least one vX.Y.Z.md)")
	}

	unreleasedDir := filepath.Join(root, "unreleased")
	unEntries, err := os.ReadDir(unreleasedDir)
	if err != nil {
		return fmt.Errorf("read split unreleased dir %s: %w", unreleasedDir, err)
	}
	for _, e := range unEntries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if name == "current.md" {
			continue
		}
		// A timestamped backup written beside a file that --discard-working-set
		// is about to replace is preserved history, not a layout artifact, so it
		// is excused from the allow-list below. It is not shape-checked either:
		// its whole purpose is to hold the superseded bytes verbatim.
		if splitBackupRe.MatchString(name) {
			continue
		}
		if !unreleasedBucketRe.MatchString(name) {
			return fmt.Errorf("unexpected file in split unreleased dir: %s", filepath.ToSlash(filepath.Join(unreleasedDir, name)))
		}
		if err := checkChangelogLikeFile(filepath.Join(unreleasedDir, name), true); err != nil {
			return err
		}
	}
	return nil
}

func checkHeading(path, prefix string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	text := strings.ReplaceAll(string(b), "\r\n", "\n")
	if !strings.HasPrefix(text, prefix+"\n") && text != prefix {
		return fmt.Errorf("%s does not start with %q", filepath.ToSlash(path), prefix)
	}
	return nil
}

func checkChangelogLikeFile(path string, requireSection bool) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	text := strings.ReplaceAll(string(b), "\r\n", "\n")
	first := strings.SplitN(text, "\n", 2)[0]
	if !strings.HasPrefix(first, "# Changelog") {
		return fmt.Errorf("%s does not start with a changelog heading", filepath.ToSlash(path))
	}
	if requireSection && !strings.Contains(text, "### ") {
		return fmt.Errorf("%s has no `###` subsection", filepath.ToSlash(path))
	}
	return nil
}
