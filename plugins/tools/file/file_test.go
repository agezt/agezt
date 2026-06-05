// SPDX-License-Identifier: MIT

package file

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTool(t *testing.T) *Tool {
	t.Helper()
	dir := t.TempDir()
	tool, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return tool
}

func invoke(t *testing.T, tool *Tool, in fileInput) string {
	t.Helper()
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	r, err := tool.Invoke(context.Background(), raw)
	if err != nil {
		t.Fatalf("Invoke(%+v): %v", in, err)
	}
	return r.Output
}

func invokeExpectErr(t *testing.T, tool *Tool, in fileInput, wantSubstr string) {
	t.Helper()
	raw, _ := json.Marshal(in)
	r, err := tool.Invoke(context.Background(), raw)
	if err != nil {
		t.Fatalf("Invoke unexpectedly errored: %v", err)
	}
	if !r.IsError {
		t.Errorf("expected IsError; got output %q", r.Output)
	}
	if wantSubstr != "" && !strings.Contains(r.Output, wantSubstr) {
		t.Errorf("output %q missing %q", r.Output, wantSubstr)
	}
}

func TestWriteReadRoundtrip(t *testing.T) {
	tool := newTool(t)
	invoke(t, tool, fileInput{Op: "write", Path: "hello.txt", Content: "hi there"})
	got := invoke(t, tool, fileInput{Op: "read", Path: "hello.txt"})
	if got != "hi there" {
		t.Errorf("read=%q want %q", got, "hi there")
	}
}

func TestAppend(t *testing.T) {
	tool := newTool(t)
	invoke(t, tool, fileInput{Op: "write", Path: "a", Content: "one\n"})
	invoke(t, tool, fileInput{Op: "append", Path: "a", Content: "two\n"})
	got := invoke(t, tool, fileInput{Op: "read", Path: "a"})
	if got != "one\ntwo\n" {
		t.Errorf("got %q", got)
	}
}

func TestList(t *testing.T) {
	tool := newTool(t)
	invoke(t, tool, fileInput{Op: "write", Path: "a.txt", Content: "x"})
	invoke(t, tool, fileInput{Op: "write", Path: "sub/b.txt", Content: "y"})
	out := invoke(t, tool, fileInput{Op: "list", Path: "."})
	if !strings.Contains(out, `"a.txt"`) || !strings.Contains(out, `"sub"`) {
		t.Errorf("list output missing entries: %s", out)
	}
}

func TestSearch(t *testing.T) {
	tool := newTool(t)
	invoke(t, tool, fileInput{Op: "write", Path: "doc.txt", Content: "line one\nline two needle\nthree\n"})
	invoke(t, tool, fileInput{Op: "write", Path: "sub/other.txt", Content: "no match here\nneedle in the haystack"})
	out := invoke(t, tool, fileInput{Op: "search", Pattern: "needle"})
	if !strings.Contains(out, "doc.txt") || !strings.Contains(out, "other.txt") {
		t.Errorf("search missed expected hits: %s", out)
	}
	if !strings.Contains(out, `"line": 2`) {
		t.Errorf("line number wrong in: %s", out)
	}
}

func TestStat(t *testing.T) {
	tool := newTool(t)
	invoke(t, tool, fileInput{Op: "write", Path: "x", Content: "abcde"})
	out := invoke(t, tool, fileInput{Op: "stat", Path: "x"})
	if !strings.Contains(out, `"size": 5`) {
		t.Errorf("stat missing size: %s", out)
	}
}

func TestDelete(t *testing.T) {
	tool := newTool(t)
	invoke(t, tool, fileInput{Op: "write", Path: "z", Content: "bye"})
	invoke(t, tool, fileInput{Op: "delete", Path: "z"})
	invokeExpectErr(t, tool, fileInput{Op: "read", Path: "z"}, "stat")
}

func TestDelete_RejectsRoot(t *testing.T) {
	tool := newTool(t)
	invokeExpectErr(t, tool, fileInput{Op: "delete", Path: "."}, "workspace root")
}

func TestDelete_RefusesDir(t *testing.T) {
	tool := newTool(t)
	invoke(t, tool, fileInput{Op: "write", Path: "sub/inner", Content: "x"})
	invokeExpectErr(t, tool, fileInput{Op: "delete", Path: "sub"}, "recursive")
}

func TestContainment_RejectsDotDot(t *testing.T) {
	tool := newTool(t)
	invokeExpectErr(t, tool, fileInput{Op: "read", Path: "../secret"}, "escapes")
}

func TestContainment_RejectsAbsoluteOutsideRoot(t *testing.T) {
	tool := newTool(t)
	outside := filepath.Join(os.TempDir(), "definitely-not-in-root.txt")
	invokeExpectErr(t, tool, fileInput{Op: "read", Path: outside}, "escapes")
}

func TestContainment_AllowsAbsoluteInsideRoot(t *testing.T) {
	tool := newTool(t)
	invoke(t, tool, fileInput{Op: "write", Path: "x", Content: "ok"})
	abs := filepath.Join(tool.Root(), "x")
	got := invoke(t, tool, fileInput{Op: "read", Path: abs})
	if got != "ok" {
		t.Errorf("absolute-within-root read got %q", got)
	}
}

// A symlink inside root that points outside root must be refused whether it is
// reached by its relative path or its absolute path. The absolute branch used
// to skip the symlink check, letting the agent read the target (M252).
func TestContainment_SymlinkEscapeBlockedBothPaths(t *testing.T) {
	tool := newTool(t)

	// A secret file OUTSIDE the workspace root.
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secret, []byte("top secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	// A symlink INSIDE root pointing at the outside secret.
	link := filepath.Join(tool.Root(), "link")
	if err := os.Symlink(secret, link); err != nil {
		t.Skipf("symlinks unavailable on this platform: %v", err)
	}

	// Relative path to the symlink — blocked before and after.
	invokeExpectErr(t, tool, fileInput{Op: "read", Path: "link"}, "escapes")
	// Absolute path to the SAME symlink — must be blocked too (the bug).
	invokeExpectErr(t, tool, fileInput{Op: "read", Path: link}, "escapes")
}

// TestContainment_SearchGlobDoNotFollowSymlinkOutsideRoot: search/glob walk the
// tree and must NOT read or enumerate an in-root symlink whose target is outside the
// workspace — otherwise they are an arbitrary-file-read primitive that bypasses the
// per-op resolve() containment (M427).
func TestContainment_SearchGlobDoNotFollowSymlinkOutsideRoot(t *testing.T) {
	tool := newTool(t)

	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secret, []byte("TOPSECRET-NEEDLE"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(tool.Root(), "link.txt")
	if err := os.Symlink(secret, link); err != nil {
		t.Skipf("symlinks unavailable on this platform: %v", err)
	}
	// A legitimate in-root file so the walk has real content to scan.
	if err := os.WriteFile(filepath.Join(tool.Root(), "real.txt"), []byte("hello world"), 0o600); err != nil {
		t.Fatal(err)
	}

	// A leak shows up as a hit (the result JSON gains a "text" field carrying the
	// matched line); the no-match result is count 0 / hits null. The "pattern" field
	// always echoes the needle, so assert on the hit marker, not the raw needle.
	if out := invoke(t, tool, fileInput{Op: "search", Pattern: "TOPSECRET-NEEDLE"}); strings.Contains(out, `"text"`) {
		t.Errorf("search leaked out-of-root file content via an in-root symlink: %s", out)
	}
	if g := invoke(t, tool, fileInput{Op: "glob", Pattern: "*.txt"}); strings.Contains(g, "link.txt") {
		t.Errorf("glob enumerated an out-of-root symlink entry: %s", g)
	}
}

// Writing a NEW file through a symlinked parent directory must be refused — the
// file would land outside root even though the target itself didn't exist yet
// (M253).
func TestContainment_NewFileUnderSymlinkedParentBlocked(t *testing.T) {
	tool := newTool(t)
	outside := t.TempDir()
	link := filepath.Join(tool.Root(), "linkdir")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable on this platform: %v", err)
	}

	invokeExpectErr(t, tool, fileInput{Op: "write", Path: "linkdir/escapee.txt", Content: "x"}, "escapes")
	if _, err := os.Stat(filepath.Join(outside, "escapee.txt")); err == nil {
		t.Error("file escaped the workspace via a symlinked parent directory")
	}
}

// A genuinely new nested path inside root still succeeds (the M253 hardening
// must not break legitimate writes that create parent directories).
func TestContainment_NewNestedFileAllowed(t *testing.T) {
	tool := newTool(t)
	invoke(t, tool, fileInput{Op: "write", Path: "a/b/c.txt", Content: "ok"})
	if got := invoke(t, tool, fileInput{Op: "read", Path: "a/b/c.txt"}); got != "ok" {
		t.Errorf("nested new file read got %q", got)
	}
}

func TestUnknownOp(t *testing.T) {
	tool := newTool(t)
	invokeExpectErr(t, tool, fileInput{Op: "fry"}, "unknown op")
}

func TestEmptyOp(t *testing.T) {
	tool := newTool(t)
	invokeExpectErr(t, tool, fileInput{Op: ""}, "op is required")
}

func TestReadTruncatesHugeFile(t *testing.T) {
	tool := newTool(t)
	huge := strings.Repeat("a", MaxReadBytes+1000)
	invoke(t, tool, fileInput{Op: "write", Path: "big", Content: huge})
	out := invoke(t, tool, fileInput{Op: "read", Path: "big"})
	if !strings.HasPrefix(out, "[file truncated") {
		t.Errorf("expected truncation notice; got prefix %q", out[:min(40, len(out))])
	}
}

func TestNew_CreatesMissingRoot(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "newly-made")
	tool, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := os.Stat(tool.Root()); err != nil {
		t.Errorf("root not created: %v", err)
	}
}

func TestDefinitionMentionsWorkspaceContainment(t *testing.T) {
	tool := newTool(t)
	def := tool.Definition()
	if def.Name != "file" {
		t.Errorf("name=%q want file", def.Name)
	}
	if !strings.Contains(def.Description, "workspace") {
		t.Errorf("description missing workspace containment hint: %q", def.Description)
	}
	var schema map[string]any
	if err := json.Unmarshal(def.InputSchema, &schema); err != nil {
		t.Fatalf("schema parse: %v", err)
	}
	if schema["type"] != "object" {
		t.Errorf("schema.type=%v want object", schema["type"])
	}
}

func TestReplace_UniqueMatch(t *testing.T) {
	tool := newTool(t)
	invoke(t, tool, fileInput{Op: "write", Path: "code.txt", Content: "alpha\nBETA\ngamma\n"})

	out := invoke(t, tool, fileInput{Op: "replace", Path: "code.txt", Find: "BETA", Replacement: "beta"})
	if !strings.Contains(out, "replaced 1") {
		t.Errorf("replace output = %q", out)
	}
	if got := invoke(t, tool, fileInput{Op: "read", Path: "code.txt"}); !strings.Contains(got, "beta") || strings.Contains(got, "BETA") {
		t.Errorf("file after replace = %q", got)
	}
}

func TestReplace_AmbiguousRequiresAll(t *testing.T) {
	tool := newTool(t)
	invoke(t, tool, fileInput{Op: "write", Path: "x.txt", Content: "x x x"})

	// Without all=true, multiple matches must error (and not change the file).
	invokeExpectErr(t, tool, fileInput{Op: "replace", Path: "x.txt", Find: "x", Replacement: "y"}, "matches 3 times")
	if got := invoke(t, tool, fileInput{Op: "read", Path: "x.txt"}); got != "x x x" {
		t.Errorf("file changed despite ambiguous replace: %q", got)
	}

	// With all=true, replace every occurrence.
	out := invoke(t, tool, fileInput{Op: "replace", Path: "x.txt", Find: "x", Replacement: "y", All: true})
	if !strings.Contains(out, "replaced 3") {
		t.Errorf("replace all output = %q", out)
	}
	if got := invoke(t, tool, fileInput{Op: "read", Path: "x.txt"}); got != "y y y" {
		t.Errorf("file after replace-all = %q", got)
	}
}

func TestReplace_NotFound(t *testing.T) {
	tool := newTool(t)
	invoke(t, tool, fileInput{Op: "write", Path: "f.txt", Content: "hello"})
	invokeExpectErr(t, tool, fileInput{Op: "replace", Path: "f.txt", Find: "absent", Replacement: "x"}, "not found")
}

func TestReplace_GuardsEmptyAndIdentical(t *testing.T) {
	tool := newTool(t)
	invoke(t, tool, fileInput{Op: "write", Path: "f.txt", Content: "hello"})
	invokeExpectErr(t, tool, fileInput{Op: "replace", Path: "f.txt", Find: "", Replacement: "x"}, "non-empty")
	invokeExpectErr(t, tool, fileInput{Op: "replace", Path: "f.txt", Find: "hello", Replacement: "hello"}, "identical")
}

func TestReplace_RejectsEscape(t *testing.T) {
	tool := newTool(t)
	invokeExpectErr(t, tool, fileInput{Op: "replace", Path: "../outside.txt", Find: "a", Replacement: "b"}, "")
}

func TestSearch_RegexMode(t *testing.T) {
	tool := newTool(t)
	invoke(t, tool, fileInput{Op: "write", Path: "a.go", Content: "func Foo() {}\nfunc Bar() {}\nvar x = 1\n"})

	// Literal search for "func(" finds nothing; regex finds both func defs.
	lit := invoke(t, tool, fileInput{Op: "search", Pattern: `func \w+\(`})
	if strings.Contains(lit, `"count": 2`) {
		t.Errorf("literal search should not regex-match: %s", lit)
	}
	rx := invoke(t, tool, fileInput{Op: "search", Pattern: `func \w+\(`, Regex: true})
	if !strings.Contains(rx, `"count": 2`) {
		t.Errorf("regex search should find 2 func defs: %s", rx)
	}
	if !strings.Contains(rx, "Foo") || !strings.Contains(rx, "Bar") {
		t.Errorf("regex hits missing Foo/Bar: %s", rx)
	}
}

func TestSearch_BadRegexErrors(t *testing.T) {
	tool := newTool(t)
	invoke(t, tool, fileInput{Op: "write", Path: "a.txt", Content: "hello"})
	invokeExpectErr(t, tool, fileInput{Op: "search", Pattern: "func(", Regex: true}, "bad regex")
}

func TestRead_LineRange(t *testing.T) {
	tool := newTool(t)
	invoke(t, tool, fileInput{Op: "write", Path: "n.txt", Content: "L1\nL2\nL3\nL4\nL5\n"})

	// Explicit range [2,4].
	out := invoke(t, tool, fileInput{Op: "read", Path: "n.txt", StartLine: 2, EndLine: 4})
	if !strings.Contains(out, "[lines 2-4]") {
		t.Errorf("missing range header: %q", out)
	}
	if !strings.Contains(out, "L2") || !strings.Contains(out, "L4") {
		t.Errorf("range should include L2..L4: %q", out)
	}
	if strings.Contains(out, "L1") || strings.Contains(out, "L5") {
		t.Errorf("range leaked out-of-range lines: %q", out)
	}

	// start_line only → window from there to EOF (within default window).
	out = invoke(t, tool, fileInput{Op: "read", Path: "n.txt", StartLine: 4})
	if !strings.Contains(out, "L4") || !strings.Contains(out, "L5") || strings.Contains(out, "L3") {
		t.Errorf("start-only window wrong: %q", out)
	}
}

func TestRead_LineRange_Errors(t *testing.T) {
	tool := newTool(t)
	invoke(t, tool, fileInput{Op: "write", Path: "n.txt", Content: "a\nb\n"})
	// end before start.
	invokeExpectErr(t, tool, fileInput{Op: "read", Path: "n.txt", StartLine: 5, EndLine: 2}, "before")
	// range past EOF → no lines.
	invokeExpectErr(t, tool, fileInput{Op: "read", Path: "n.txt", StartLine: 100, EndLine: 200}, "no lines")
}

func TestGlob_FindsAcrossTree(t *testing.T) {
	tool := newTool(t)
	invoke(t, tool, fileInput{Op: "write", Path: "main.go", Content: "x"})
	invoke(t, tool, fileInput{Op: "write", Path: "pkg/util.go", Content: "x"})
	invoke(t, tool, fileInput{Op: "write", Path: "pkg/sub/deep.go", Content: "x"})
	invoke(t, tool, fileInput{Op: "write", Path: "README.md", Content: "x"})

	out := invoke(t, tool, fileInput{Op: "glob", Pattern: "*.go"})
	for _, want := range []string{"main.go", "pkg/util.go", "pkg/sub/deep.go"} {
		if !strings.Contains(out, want) {
			t.Errorf("glob *.go missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "README.md") {
		t.Errorf("glob *.go should not match README.md:\n%s", out)
	}
	if !strings.Contains(out, `"count": 3`) {
		t.Errorf("expected count 3:\n%s", out)
	}
}

func TestGlob_ScopedAndErrors(t *testing.T) {
	tool := newTool(t)
	invoke(t, tool, fileInput{Op: "write", Path: "a/x.txt", Content: "x"})
	invoke(t, tool, fileInput{Op: "write", Path: "b/y.txt", Content: "x"})

	// Scoped to subtree a/.
	out := invoke(t, tool, fileInput{Op: "glob", Pattern: "*.txt", Path: "a"})
	if !strings.Contains(out, "a/x.txt") || strings.Contains(out, "b/y.txt") {
		t.Errorf("scoped glob wrong:\n%s", out)
	}
	// Empty pattern errors.
	invokeExpectErr(t, tool, fileInput{Op: "glob", Pattern: ""}, "requires a pattern")
	// Bad pattern errors.
	invokeExpectErr(t, tool, fileInput{Op: "glob", Pattern: "[bad"}, "bad pattern")
}

// TestFile_EveryAdvertisedOpIsDispatched keeps the schema's op enum and the
// Invoke switch in lockstep: every op the tool advertises must be handled (never
// fall through to "unknown op"). Catches an enum/dispatch drift — e.g. advertising
// an op with no case, or the stale package doc that listed only 4 of the 9 ops.
func TestFile_EveryAdvertisedOpIsDispatched(t *testing.T) {
	tool, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Extract the advertised op enum from the tool's own schema.
	var schema struct {
		Properties struct {
			Op struct {
				Enum []string `json:"enum"`
			} `json:"op"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(tool.Definition().InputSchema, &schema); err != nil {
		t.Fatalf("parse schema: %v", err)
	}
	ops := schema.Properties.Op.Enum
	if len(ops) < 9 {
		t.Fatalf("schema advertises only %d ops, expected the full surface (read/write/append/list/search/stat/delete/replace/glob)", len(ops))
	}
	for _, op := range ops {
		raw, _ := json.Marshal(map[string]any{"op": op, "path": "x.txt", "pattern": "*"})
		r, err := tool.Invoke(context.Background(), raw)
		if err != nil {
			t.Errorf("op %q: transport error %v", op, err)
			continue
		}
		// The op may legitimately error (missing file, etc.) — but it must be
		// DISPATCHED, never reported as an unknown op.
		if r.IsError && strings.Contains(r.Output, "unknown op") {
			t.Errorf("op %q is advertised in the schema but not dispatched: %q", op, r.Output)
		}
	}
}

// TestAtomicWriteFile_PreservesOriginalOnWriteFailure pins M467: a write that
// fails partway must NOT leave the original file truncated or destroyed. The
// `replace`/`write` ops route through atomicWriteFile (temp + rename), so the
// original survives until the complete new content is renamed into place.
func TestAtomicWriteFile_PreservesOriginalOnWriteFailure(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "data.txt")
	if err := os.WriteFile(p, []byte("ORIGINAL-CONTENT"), 0o644); err != nil {
		t.Fatal(err)
	}

	orig := writeAll
	writeAll = func(*os.File, []byte) (int, error) { return 0, errors.New("simulated ENOSPC") }
	defer func() { writeAll = orig }()

	if err := atomicWriteFile(p, []byte("NEW-CONTENT-THAT-FAILS"), 0o644); err == nil {
		t.Fatal("expected a write error")
	}

	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("original file unreadable after failed write: %v", err)
	}
	if string(got) != "ORIGINAL-CONTENT" {
		t.Errorf("original damaged by a failed write: got %q, want ORIGINAL-CONTENT (write was not atomic)", got)
	}

	// No temp litter left behind.
	ents, _ := os.ReadDir(dir)
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), ".agezt-write-") {
			t.Errorf("temp file leaked after failed write: %s", e.Name())
		}
	}
}
