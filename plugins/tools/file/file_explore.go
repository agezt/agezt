// SPDX-License-Identifier: MIT

package file

import (
	"encoding/json"
	"errors"
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

