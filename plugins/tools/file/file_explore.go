// SPDX-License-Identifier: MIT

package file

// File-tool exploration + deletion: doList + doSearch + doGlob + doStat +
// doDelete + path resolution helpers (resolve, resolveNewWithinRoot,
// entryEscapesRoot, withinRoot) + ErrEscape + errResult + fileObservation.
// Carved out of file.go during the Day 159 god-file split so the main file
// can focus on lifecycle + read + write + file I/O helpers.
// Public API unchanged.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
)
func (t *Tool) doList(in fileInput) (agent.Result, error) {
	target := in.Path
	if target == "" {
		target = "."
	}
	p, err := t.resolve(target)
	if err != nil {
		return errResult(err.Error()), nil
	}
	entries, err := os.ReadDir(p)
	if err != nil {
		return errResult("readdir: " + err.Error()), nil
	}
	cap := in.MaxResults
	if cap <= 0 || cap > MaxListEntries {
		cap = MaxListEntries
	}

	type entry struct {
		Name  string `json:"name"`
		IsDir bool   `json:"is_dir"`
		Size  int64  `json:"size,omitempty"`
	}
	out := make([]entry, 0, len(entries))
	for _, e := range entries {
		if len(out) >= cap {
			break
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, entry{
			Name:  e.Name(),
			IsDir: e.IsDir(),
			Size:  info.Size(),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	body, err := json.MarshalIndent(map[string]any{
		"path":    target,
		"entries": out,
		"count":   len(out),
		"capped":  len(out) == cap && len(entries) > cap,
	}, "", "  ")
	if err != nil {
		return errResult("marshal: " + err.Error()), nil
	}
	return fileObservation(target, string(body)), nil
}

func (t *Tool) doSearch(in fileInput) (agent.Result, error) {
	if in.Pattern == "" {
		return errResult("search requires a pattern"), nil
	}
	target := in.Path
	if target == "" {
		target = "."
	}
	root, err := t.resolve(target)
	if err != nil {
		return errResult(err.Error()), nil
	}
	cap := in.MaxResults
	if cap <= 0 || cap > MaxSearchHits {
		cap = MaxSearchHits
	}

	type hit struct {
		Path string `json:"path"`
		Line int    `json:"line"`
		Text string `json:"text"`
	}
	var hits []hit
	pattern := in.Pattern

	// matcher is a literal substring test by default, or an RE2 regexp when
	// regex=true (M115) — letting an agent grep for code patterns, not just
	// fixed strings. A bad regex errors loudly instead of matching nothing.
	matcher := func(line string) bool { return strings.Contains(line, pattern) }
	if in.Regex {
		re, rerr := regexp.Compile(pattern)
		if rerr != nil {
			return errResult("search: bad regex: " + rerr.Error()), nil
		}
		matcher = re.MatchString
	}

	walkErr := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable
		}
		if d.IsDir() {
			return nil
		}
		if t.entryEscapesRoot(p, d) {
			return nil // skip a symlink whose target leaves the workspace
		}
		if len(hits) >= cap {
			return filepath.SkipAll
		}
		if info, ierr := d.Info(); ierr == nil && info.Size() > MaxScanBytes {
			return nil // skip a file too large to scan safely
		}
		data, err := func() ([]byte, error) {
			f, oerr := openFileNoFollow(p, os.O_RDONLY, 0, t.root)
			if oerr != nil {
				return nil, oerr
			}
			defer f.Close()
			return io.ReadAll(f)
		}()
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(t.root, p)
		rel = filepath.ToSlash(rel)
		for i, line := range strings.Split(string(data), "\n") {
			if len(hits) >= cap {
				break
			}
			if matcher(line) {
				hits = append(hits, hit{Path: rel, Line: i + 1, Text: line})
			}
		}
		return nil
	})
	if walkErr != nil && !errors.Is(walkErr, filepath.SkipAll) {
		return errResult("walk: " + walkErr.Error()), nil
	}

	body, err := json.MarshalIndent(map[string]any{
		"pattern": pattern,
		"hits":    hits,
		"count":   len(hits),
		"capped":  len(hits) == cap,
	}, "", "  ")
	if err != nil {
		return errResult("marshal: " + err.Error()), nil
	}
	return fileObservation(target, string(body)), nil
}

// doGlob finds files whose NAME matches a shell pattern (*, ?, [..]) anywhere
// under the workspace (or under `path`) — the cross-tree file-finder the agent
// lacked (M119). `list` shows one directory and `search` greps content; glob
// answers "where are the *.go files?". Directories are skipped; results are
// workspace-relative, sorted, and capped.
func (t *Tool) doGlob(in fileInput) (agent.Result, error) {
	if in.Pattern == "" {
		return errResult("glob requires a pattern"), nil
	}
	if _, perr := filepath.Match(in.Pattern, "probe"); perr != nil {
		return errResult("glob: bad pattern: " + perr.Error()), nil
	}
	target := in.Path
	if target == "" {
		target = "."
	}
	root, err := t.resolve(target)
	if err != nil {
		return errResult(err.Error()), nil
	}
	limit := in.MaxResults
	if limit <= 0 || limit > MaxListEntries {
		limit = MaxListEntries
	}

	var matches []string
	capped := false
	walkErr := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable
		}
		if d.IsDir() {
			return nil
		}
		if t.entryEscapesRoot(p, d) {
			return nil // don't leak the existence of out-of-root symlink targets
		}
		if len(matches) >= limit {
			capped = true
			return filepath.SkipAll
		}
		if ok, _ := filepath.Match(in.Pattern, d.Name()); ok {
			rel, rerr := filepath.Rel(t.root, p)
			if rerr == nil {
				matches = append(matches, filepath.ToSlash(rel))
			}
		}
		return nil
	})
	if walkErr != nil && !errors.Is(walkErr, filepath.SkipAll) {
		return errResult("walk: " + walkErr.Error()), nil
	}
	sort.Strings(matches)

	body, err := json.MarshalIndent(map[string]any{
		"pattern": in.Pattern,
		"matches": matches,
		"count":   len(matches),
		"capped":  capped,
	}, "", "  ")
	if err != nil {
		return errResult("marshal: " + err.Error()), nil
	}
	return fileObservation(target, string(body)), nil
}

func (t *Tool) doStat(in fileInput) (agent.Result, error) {
	p, err := t.resolve(in.Path)
	if err != nil {
		return errResult(err.Error()), nil
	}
	info, err := os.Stat(p)
	if err != nil {
		return errResult("stat: " + err.Error()), nil
	}
	body, _ := json.MarshalIndent(map[string]any{
		"path":     in.Path,
		"size":     info.Size(),
		"is_dir":   info.IsDir(),
		"mode":     info.Mode().String(),
		"mod_time": info.ModTime().UTC().Format("2006-01-02T15:04:05Z"),
	}, "", "  ")
	return fileObservation(in.Path, string(body)), nil
}

func (t *Tool) doDelete(ctx context.Context, in fileInput) (agent.Result, error) {
	p, err := t.resolve(in.Path)
	if err != nil {
		return errResult(err.Error()), nil
	}
	if p == t.root {
		return errResult("refusing to delete workspace root"), nil
	}
	info, err := os.Stat(p)
	if err != nil {
		return errResult("stat: " + err.Error()), nil
	}
	if info.IsDir() {
		// For M1 we refuse recursive delete; the model can list and delete
		// individual files. Recursive ops are a future Edict-gated action.
		return errResult(in.Path + " is a directory; recursive delete is not allowed in M1"), nil
	}
	if err := t.checkpointFileSnapshot(ctx, "file.delete", in.Path, p); err != nil {
		return errResult("checkpoint: " + err.Error()), nil
	}
	if err := os.Remove(p); err != nil {
		return errResult("remove: " + err.Error()), nil
	}
	return agent.Result{Output: "deleted " + in.Path}, nil
}

// ----- containment -----

// ErrEscape is returned when a requested path resolves outside the root.
var ErrEscape = errors.New("file: path escapes workspace root")

// resolve canonicalizes the requested relative path and asserts it lives
// inside t.root.
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
