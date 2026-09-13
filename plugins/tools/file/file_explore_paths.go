// SPDX-License-Identifier: MIT

package file

// File-tool path-resolution + result helpers: resolve +
// resolveNewWithinRoot + entryEscapesRoot + withinRoot + errResult +
// fileObservation. Carved out of file_explore.go during the Day 189
// god-file split so the explore file can stay focused on the four
// READ methods and the delete file can stay focused on doDelete.
// Public API unchanged.

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
)

func (t *Tool) resolve(rel string) (string, error) {
	if rel == "" {
		return "", errors.New("path required")
	}
	var clean string
	if filepath.IsAbs(rel) {
		c, err := filepath.Abs(rel)
		if err != nil {
			return "", fmt.Errorf("resolve abs: %w", err)
		}
		clean = c
	} else {
		c, err := filepath.Abs(filepath.Join(t.root, rel))
		if err != nil {
			return "", fmt.Errorf("resolve: %w", err)
		}
		clean = c
	}
	// Resolve symlinks ONLY if the target exists; we need to allow writing
	// to new files inside root without failing the symlink check. This applies
	// to BOTH relative and absolute in-root paths (M252): the absolute branch
	// previously skipped symlink resolution, so a symlink inside root pointing
	// outside root was blocked when reached by its relative path but slipped
	// through when reached by its absolute path — a containment bypass.
	if _, err := os.Lstat(clean); err == nil {
		resolved, err := filepath.EvalSymlinks(clean)
		if err != nil {
			return "", fmt.Errorf("resolve symlink: %w", err)
		}
		if !withinRoot(t.root, resolved) {
			return "", fmt.Errorf("%w: %s (resolved to %s)", ErrEscape, rel, resolved)
		}
		return resolved, nil
	}
	// New file/dir: the target doesn't exist yet (EvalSymlinks would fail), so
	// resolve the deepest ANCESTOR that does exist and verify its real location
	// is inside root. A lexical-only check here would miss a symlinked parent
	// directory — e.g. writing "link/new.txt" where "<root>/link" -> /etc would
	// place the new file outside root (M253).
	return t.resolveNewWithinRoot(clean, rel)
}

// resolveNewWithinRoot canonicalizes a not-yet-existing path by symlink-
// resolving its deepest existing ancestor and confirming the result stays in
// root, then re-appending the non-existent suffix (which has no symlinks).
func (t *Tool) resolveNewWithinRoot(clean, rel string) (string, error) {
	dir := filepath.Dir(clean)
	suffix := filepath.Base(clean)
	for {
		if _, err := os.Lstat(dir); err == nil {
			resolvedDir, err := filepath.EvalSymlinks(dir)
			if err != nil {
				return "", fmt.Errorf("resolve symlink: %w", err)
			}
			if !withinRoot(t.root, resolvedDir) {
				return "", fmt.Errorf("%w: %s (parent resolves to %s)", ErrEscape, rel, resolvedDir)
			}
			final := filepath.Join(resolvedDir, suffix)
			if !withinRoot(t.root, final) {
				return "", fmt.Errorf("%w: %s", ErrEscape, rel)
			}
			return final, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break // reached the filesystem root without an existing ancestor
		}
		suffix = filepath.Join(filepath.Base(dir), suffix)
		dir = parent
	}
	// No existing ancestor (root always exists, so this is a safety net).
	if !withinRoot(t.root, clean) {
		return "", fmt.Errorf("%w: %s", ErrEscape, rel)
	}
	return clean, nil
}

// withinRoot reports whether child is the root or a descendant of it. Both
// arguments must be already-canonicalized absolute paths.
// entryEscapesRoot reports whether a walked entry is a symlink whose real target
// resolves outside the tool's root. WalkDir lstat-types entries (it never follows a
// link), so a symlink-to-file passes the d.IsDir() check and a later os.ReadFile would
// FOLLOW it out of the workspace — the per-op resolve() containment never runs during
// a walk. search/glob must re-check each link or they become an arbitrary-file-read
// primitive via an in-root symlink (M427). An unresolvable link is also skipped.
func (t *Tool) entryEscapesRoot(p string, d fs.DirEntry) bool {
	if d.Type()&fs.ModeSymlink == 0 {
		return false
	}
	resolved, err := filepath.EvalSymlinks(p)
	if err != nil {
		return true
	}
	return !withinRoot(t.root, resolved)
}

func withinRoot(root, child string) bool {
	rel, err := filepath.Rel(root, child)
	if err != nil {
		return false
	}
	// On Windows, filepath.Rel may produce paths like "..\\foo" — any
	// path starting with ".." escapes.
	if rel == "." {
		return true
	}
	return !strings.HasPrefix(rel, "..") && rel != ".."
}

func errResult(msg string) agent.Result {
	return agent.Result{Output: msg, IsError: true}
}

func fileObservation(path, output string) agent.Result {
	path = strings.TrimSpace(path)
	if path == "" {
		path = "."
	}
	return agent.Result{
		Output:            output,
		ObservationTrust:  agent.ObservationUntrusted,
		ObservationSource: "workspace:" + filepath.ToSlash(path),
	}
}

