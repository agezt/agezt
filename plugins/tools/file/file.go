// SPDX-License-Identifier: MIT

// Package file is the in-process file tool. It reads, writes, lists, and
// searches files inside a configured workspace root and refuses to operate
// outside it (no `..` escape, no absolute paths outside root, no symlink
// escape).
//
// Scope (M1.a): read, write, list, search. A future `patch` op (unified
// diff) is deferred; for M1 the model can use write to rewrite a whole
// file or append.
//
// Containment policy: the root directory is resolved with filepath.Abs +
// EvalSymlinks at New(); every requested path is resolved the same way
// and rejected if its absolute, symlink-resolved form does not have the
// root as a prefix. This is the M1 minimum; Warden namespace isolation
// (TASKS P1-WARD-01) provides deeper containment when it lands.
package file

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
)

// MaxReadBytes caps how much of a single file the tool will return so a
// huge file can't blow the model's context.
const MaxReadBytes = 256 * 1024

// MaxListEntries caps a directory listing.
const MaxListEntries = 1000

// MaxSearchHits caps grep-style search results.
const MaxSearchHits = 200

// Tool is the file tool implementation of agent.Tool.
type Tool struct {
	root string // absolute, symlink-resolved
}

// New returns a Tool scoped to root. The directory must exist and be
// readable. root is canonicalized so the containment check is robust
// against `..` and symlinks.
func New(root string) (*Tool, error) {
	if root == "" {
		return nil, errors.New("file: root required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("file: abs root: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		// If the root doesn't exist yet, allow it to be created on first
		// write but use the requested abs path as the containment root.
		if errors.Is(err, fs.ErrNotExist) {
			if mkErr := os.MkdirAll(abs, 0o755); mkErr != nil {
				return nil, fmt.Errorf("file: mkdir root %s: %w", abs, mkErr)
			}
			resolved, err = filepath.EvalSymlinks(abs)
			if err != nil {
				return nil, fmt.Errorf("file: resolve root after mkdir: %w", err)
			}
		} else {
			return nil, fmt.Errorf("file: resolve root %s: %w", abs, err)
		}
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return nil, fmt.Errorf("file: stat root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("file: root %s is not a directory", resolved)
	}
	return &Tool{root: resolved}, nil
}

// Root returns the canonicalized root path.
func (t *Tool) Root() string { return t.root }

// Definition implements agent.Tool.
func (t *Tool) Definition() agent.ToolDef {
	return agent.ToolDef{
		Name: "file",
		Description: "Read, write, list, and search files in the workspace. " +
			"All paths are relative to the workspace root; absolute paths or `..` " +
			"escape are rejected.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "required": ["op"],
  "properties": {
    "op":   {"type": "string", "enum": ["read","write","append","list","search","stat","delete"]},
    "path": {"type": "string", "description": "Path relative to the workspace root."},
    "content": {"type": "string", "description": "For write/append: the bytes to write."},
    "pattern": {"type": "string", "description": "For search: a literal substring (not regex) to grep for."},
    "max_results": {"type": "integer", "description": "For list/search: cap the entries returned."}
  }
}`),
	}
}

type fileInput struct {
	Op         string `json:"op"`
	Path       string `json:"path,omitempty"`
	Content    string `json:"content,omitempty"`
	Pattern    string `json:"pattern,omitempty"`
	MaxResults int    `json:"max_results,omitempty"`
}

// Invoke implements agent.Tool.
func (t *Tool) Invoke(ctx context.Context, raw json.RawMessage) (agent.Result, error) {
	var in fileInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return agent.Result{}, fmt.Errorf("file: parse input: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return agent.Result{}, err
	}

	switch in.Op {
	case "read":
		return t.doRead(in)
	case "write":
		return t.doWrite(in, false)
	case "append":
		return t.doWrite(in, true)
	case "list":
		return t.doList(in)
	case "search":
		return t.doSearch(in)
	case "stat":
		return t.doStat(in)
	case "delete":
		return t.doDelete(in)
	case "":
		return errResult("op is required"), nil
	default:
		return errResult(fmt.Sprintf("unknown op %q", in.Op)), nil
	}
}

// ----- ops -----

func (t *Tool) doRead(in fileInput) (agent.Result, error) {
	p, err := t.resolve(in.Path)
	if err != nil {
		return errResult(err.Error()), nil
	}
	info, err := os.Stat(p)
	if err != nil {
		return errResult("stat: " + err.Error()), nil
	}
	if info.IsDir() {
		return errResult(in.Path + " is a directory; use op=list"), nil
	}
	if info.Size() > MaxReadBytes {
		// Partial read with a notice.
		f, err := os.Open(p)
		if err != nil {
			return errResult("open: " + err.Error()), nil
		}
		defer f.Close()
		buf := make([]byte, MaxReadBytes)
		n, _ := f.Read(buf)
		out := fmt.Sprintf("[file truncated: showing first %d of %d bytes]\n%s",
			n, info.Size(), string(buf[:n]))
		return agent.Result{Output: out}, nil
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return errResult("read: " + err.Error()), nil
	}
	return agent.Result{Output: string(data)}, nil
}

func (t *Tool) doWrite(in fileInput, appendMode bool) (agent.Result, error) {
	p, err := t.resolve(in.Path)
	if err != nil {
		return errResult(err.Error()), nil
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return errResult("mkdir parent: " + err.Error()), nil
	}
	flag := os.O_WRONLY | os.O_CREATE
	if appendMode {
		flag |= os.O_APPEND
	} else {
		flag |= os.O_TRUNC
	}
	f, err := os.OpenFile(p, flag, 0o644)
	if err != nil {
		return errResult("open: " + err.Error()), nil
	}
	defer f.Close()
	if _, err := f.WriteString(in.Content); err != nil {
		return errResult("write: " + err.Error()), nil
	}
	if err := f.Sync(); err != nil {
		return errResult("fsync: " + err.Error()), nil
	}
	verb := "wrote"
	if appendMode {
		verb = "appended"
	}
	return agent.Result{
		Output: fmt.Sprintf("%s %d bytes to %s", verb, len(in.Content), in.Path),
	}, nil
}

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
	return agent.Result{Output: string(body)}, nil
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

	walkErr := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable
		}
		if d.IsDir() {
			return nil
		}
		if len(hits) >= cap {
			return filepath.SkipAll
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(t.root, p)
		rel = filepath.ToSlash(rel)
		for i, line := range strings.Split(string(data), "\n") {
			if len(hits) >= cap {
				break
			}
			if strings.Contains(line, pattern) {
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
	return agent.Result{Output: string(body)}, nil
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
	return agent.Result{Output: string(body)}, nil
}

func (t *Tool) doDelete(in fileInput) (agent.Result, error) {
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
	if filepath.IsAbs(rel) {
		// Allow only if the absolute path is inside root.
		clean, err := filepath.Abs(rel)
		if err != nil {
			return "", fmt.Errorf("resolve abs: %w", err)
		}
		if !withinRoot(t.root, clean) {
			return "", fmt.Errorf("%w: %s", ErrEscape, rel)
		}
		return clean, nil
	}

	joined := filepath.Join(t.root, rel)
	clean, err := filepath.Abs(joined)
	if err != nil {
		return "", fmt.Errorf("resolve: %w", err)
	}
	// Resolve symlinks ONLY if the target exists; we need to allow writing
	// to new files inside root without failing the symlink check.
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
	// New file: ensure its eventual location is in root.
	if !withinRoot(t.root, clean) {
		return "", fmt.Errorf("%w: %s", ErrEscape, rel)
	}
	return clean, nil
}

// withinRoot reports whether child is the root or a descendant of it. Both
// arguments must be already-canonicalized absolute paths.
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
