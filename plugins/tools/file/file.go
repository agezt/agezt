// SPDX-License-Identifier: MIT

// Package file is the in-process file tool. It reads, writes, lists, and
// searches files inside a configured workspace root and refuses to operate
// outside it (no `..` escape, no absolute paths outside root, no symlink
// escape).
//
// Ops: read, write, append, list, search, stat, delete, replace, glob.
// `replace` does a surgical find/replace edit so the model need not rewrite a
// whole file (M114). A unified-diff `patch` op is still deferred — `replace`
// covers small edits. The advertised op enum and the dispatch switch are kept in
// lockstep by TestFile_EveryAdvertisedOpIsDispatched.
//
// Containment policy: the root directory is resolved with filepath.Abs +
// EvalSymlinks at New(); every requested path is resolved the same way
// and rejected if its absolute, symlink-resolved form does not have the
// root as a prefix. This is the M1 minimum; Warden namespace isolation
// (TASKS P1-WARD-01) provides deeper containment when it lands.
package file

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/edict"
)

// MaxReadBytes caps how much of a single file the tool will return so a
// huge file can't blow the model's context.
const MaxReadBytes = 256 * 1024

// MaxListEntries caps a directory listing.
const MaxListEntries = 1000

// MaxSearchHits caps grep-style search results.
const MaxSearchHits = 200

// MaxScanBytes caps the size of a single file that search/replace will read whole
// into memory, so an agent that grows a workspace file to gigabytes can't OOM the
// daemon by grepping or replacing in it (M427). Generous for real source files.
const MaxScanBytes = 8 * 1024 * 1024

// Tool is the file tool implementation of agent.Tool.
type Tool struct {
	root         string // absolute, symlink-resolved
	rollbackBase string // AGEZT home for checkpoint catalog; empty disables checkpoints
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

// NewWithCheckpoint returns a file tool that writes pre-mutation rollback
// snapshots under baseDir. Tests and embedded callers that need no checkpointing
// can keep using New.
func NewWithCheckpoint(root, baseDir string) (*Tool, error) {
	t, err := New(root)
	if err != nil {
		return nil, err
	}
	t.rollbackBase = baseDir
	return t, nil
}

// Root returns the canonicalized root path.
func (t *Tool) Root() string { return t.root }

// Definition implements agent.Tool.
func (t *Tool) Definition() agent.ToolDef {
	return agent.ToolDef{
		Name: "file",
		Capability: agent.ToolCapability{
			// No fallback axis on purpose: an op outside this map is also outside the
			// schema enum above, so it cannot reach a handler. Deferring leaves it on
			// the policy engine's unknown-capability path, which denies — the right
			// answer for a call nobody can service.
			Field: "op",
			ByValue: map[string]string{
				"read":    string(edict.CapFileRead),
				"stat":    string(edict.CapFileRead),
				"search":  string(edict.CapFileRead),
				"list":    string(edict.CapFileList),
				"glob":    string(edict.CapFileList),
				"write":   string(edict.CapFileWrite),
				"append":  string(edict.CapFileWrite),
				"replace": string(edict.CapFileWrite),
				"delete":  string(edict.CapFileDelete),
			},
		},
		Description: "Read, write, list, search, and edit files in the workspace. " +
			"All paths are relative to the workspace root; absolute paths or `..` " +
			"escape are rejected. Prefer `replace` for small edits over rewriting a whole file.",
		Effect: agent.ToolEffect{
			Class: agent.EffectReversible,
			PredictedEffects: []string{
				"read workspace files for read/list/search/stat/glob operations",
				"mutate workspace files for write/append/delete/replace operations",
			},
			AffectedResources: []string{"workspace files under " + t.root},
			RollbackNotes:     "Read-only file operations need no rollback. Daemon-registered mutating operations write pre-mutation file.snapshot checkpoints that `agt rollback apply` can restore; otherwise use version control or backups.",
			Confidence:        0.85,
		},
		InputSchema: json.RawMessage(`{
  "type": "object",
  "required": ["op"],
  "properties": {
    "op":   {"type": "string", "enum": ["read","write","append","list","search","stat","delete","replace","glob"]},
    "path": {"type": "string", "description": "Path relative to the workspace root."},
    "content": {"type": "string", "description": "For write/append: the bytes to write."},
    "pattern": {"type": "string", "description": "For search: a substring (or RE2 regex when regex=true) to grep for. For glob: a filename pattern (*, ?, [..]) matched against each file's name across the tree."},
    "regex": {"type": "boolean", "description": "For search: treat 'pattern' as an RE2 regular expression instead of a literal substring."},
    "start_line": {"type": "integer", "description": "For read: 1-based first line to return (pages a large file; pair with search)."},
    "end_line": {"type": "integer", "description": "For read: 1-based last line to return (default: start_line + 200)."},
    "find": {"type": "string", "description": "For replace: the exact substring to find. Must be unique unless all=true."},
    "replacement": {"type": "string", "description": "For replace: the text to substitute for 'find'."},
    "all": {"type": "boolean", "description": "For replace: replace every occurrence instead of requiring a single unique match."},
    "max_results": {"type": "integer", "description": "For list/search: cap the entries returned."}
  }
}`),
	}
}

type fileInput struct {
	Op          string `json:"op"`
	Path        string `json:"path,omitempty"`
	Content     string `json:"content,omitempty"`
	Pattern     string `json:"pattern,omitempty"`
	Find        string `json:"find,omitempty"`
	Replacement string `json:"replacement,omitempty"`
	All         bool   `json:"all,omitempty"`
	Regex       bool   `json:"regex,omitempty"`
	StartLine   int    `json:"start_line,omitempty"`
	EndLine     int    `json:"end_line,omitempty"`
	MaxResults  int    `json:"max_results,omitempty"`
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

	// Per-agent workdir (M792): a run executing AS a named agent whose profile
	// names a workspace subdirectory operates THERE — relative paths rebase
	// under it (and an empty list/glob path means "my directory"). Absolute
	// paths are untouched; full root containment below still applies, and the
	// workdir itself is escape-proofed twice (profile validation + ctx setter).
	if wd := agent.WorkdirFromContext(ctx); wd != "" {
		if in.Path == "" {
			in.Path = wd
		} else if !filepath.IsAbs(in.Path) {
			in.Path = filepath.Join(wd, in.Path)
		}
	}

	switch in.Op {
	case "read":
		return t.doRead(in)
	case "write":
		return t.doWrite(ctx, in, false)
	case "append":
		return t.doWrite(ctx, in, true)
	case "list":
		return t.doList(in)
	case "search":
		return t.doSearch(in)
	case "stat":
		return t.doStat(in)
	case "delete":
		return t.doDelete(ctx, in)
	case "replace":
		return t.doReplace(ctx, in)
	case "glob":
		return t.doGlob(in)
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
	// Line-range read (M117): page a region of a file rather than the whole
	// thing — essential for large files (where the default read truncates to the
	// first MaxReadBytes) and for reading around a `search` hit.
	if in.StartLine > 0 || in.EndLine > 0 {
		return t.doReadRange(in, p)
	}
	if info.Size() > MaxReadBytes {
		// Partial read with a notice.
		f, err := openFileNoFollow(p, os.O_RDONLY, 0, t.root)
		if err != nil {
			return errResult("read: " + err.Error()), nil
		}
		defer f.Close()
		buf, rerr := readUpTo(f, MaxReadBytes)
		if rerr != nil {
			return errResult("read: " + rerr.Error()), nil
		}
		out := fmt.Sprintf("[file truncated: showing first %d of %d bytes]\n%s",
			len(buf), info.Size(), string(buf))
		return fileObservation(in.Path, out), nil
	}
	f, err := openFileNoFollow(p, os.O_RDONLY, 0, t.root)
	if err != nil {
		return errResult("read: " + err.Error()), nil
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return errResult("read: " + err.Error()), nil
	}
	return fileObservation(in.Path, string(data)), nil
}

// defaultReadRangeLines is the window size when only start_line is given.
const defaultReadRangeLines = 200

// maxReadRangeLines caps a single line-range read.
const maxReadRangeLines = 5000

// doReadRange returns lines [start_line, end_line] of a file (M117), bounded by
// maxReadRangeLines and MaxReadBytes. The output is the raw line content (usable
// directly for a follow-up `replace`) under a "[lines X-Y]" header.
func (t *Tool) doReadRange(in fileInput, p string) (agent.Result, error) {
	start := in.StartLine
	if start < 1 {
		start = 1
	}
	end := in.EndLine
	if end <= 0 {
		end = start + defaultReadRangeLines - 1
	}
	if end < start {
		return errResult("read: end_line is before start_line"), nil
	}
	if end-start+1 > maxReadRangeLines {
		end = start + maxReadRangeLines - 1
	}

	f, err := openFileNoFollow(p, os.O_RDONLY, 0, t.root)
	if err != nil {
		return errResult("read: " + err.Error()), nil
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024) // tolerate long lines

	var b strings.Builder
	line, written, emitted := 0, 0, 0
	truncated := false
	for sc.Scan() {
		line++
		if line < start {
			continue
		}
		if line > end {
			break
		}
		text := sc.Text()
		if written+len(text)+1 > MaxReadBytes {
			truncated = true
			break
		}
		b.WriteString(text)
		b.WriteByte('\n')
		written += len(text) + 1
		emitted++
	}
	if err := sc.Err(); err != nil {
		return errResult("scan: " + err.Error()), nil
	}
	if emitted == 0 {
		return errResult(fmt.Sprintf("read: no lines in range [%d,%d]; file has %d line(s)", start, end, line)), nil
	}
	header := fmt.Sprintf("[lines %d-%d]", start, start+emitted-1)
	if truncated {
		header += " [truncated at byte cap]"
	}
	return fileObservation(in.Path, header+"\n"+b.String()), nil
}

