// SPDX-License-Identifier: MIT

package file

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileCoverageNewAndWithinRoot(t *testing.T) {
	// New with empty root: error.
	if _, err := New(""); err == nil || !strings.Contains(err.Error(), "root required") {
		t.Fatalf("empty root = %v", err)
	}
	// New with a non-existent path on Windows: may fail; on POSIX we expect mkdir.
	if _, err := New("/this/should/not/exist/anywhere/agezt"); err != nil && !strings.Contains(err.Error(), "root") {
		t.Fatalf("nonexistent root = %v", err)
	}
	// New with a non-directory.
	dir := t.TempDir()
	file := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, err := New(file); err == nil {
		t.Fatalf("non-directory root should error")
	}
	// New with a real directory.
	real := t.TempDir()
	tool, err := New(real)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if tool.Root() == "" {
		t.Fatal("Root() should be non-empty")
	}
	if !filepath.IsAbs(tool.Root()) {
		t.Fatalf("Root() should be absolute: %q", tool.Root())
	}

	// withinRoot: dot/descendant inside, parent or sibling outside.
	cases := map[string]bool{
		"/abs":                 true, // equal to root
		"/abs/sub":             true,
		"/abs/sub/inner/x.txt": true,
		"/ab/sub":              false,
		"/other":               false,
		"/absuffix":            false, // lexicographic prefix, not directory
	}
	for p, want := range cases {
		if got := withinRoot("/abs", p); got != want {
			t.Errorf("withinRoot(/abs, %q) = %v, want %v", p, got, want)
		}
	}
}

func TestFileCoverageEntryEscapesRoot(t *testing.T) {
	dir := t.TempDir()
	tool, _ := New(dir)
	// Regular file: not a symlink, so does not escape.
	if tool.entryEscapesRoot(filepath.Join(dir, "a.txt"), fakeDirEntry{typ: 0}) {
		t.Fatal("regular file should not be flagged as escape")
	}
	// Symlink to outside: flagged as escape (EvalSymlinks resolves to a path
	// outside root).
	outside := t.TempDir()
	link := filepath.Join(dir, "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}
	if !tool.entryEscapesRoot(link, fakeDirEntry{typ: fs.ModeSymlink}) {
		t.Fatal("symlink to outside should be flagged as escape")
	}
	// Symlink to inside: not flagged.
	inside := filepath.Join(dir, "inside.txt")
	if err := os.WriteFile(inside, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.Symlink(inside, filepath.Join(dir, "link2")); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}
	if tool.entryEscapesRoot(filepath.Join(dir, "link2"), fakeDirEntry{typ: fs.ModeSymlink}) {
		t.Fatal("symlink to inside should not be flagged")
	}
}

type fakeDirEntry struct{ typ fs.FileMode }

func (f fakeDirEntry) Name() string               { return "" }
func (f fakeDirEntry) IsDir() bool                { return f.typ&fs.ModeDir != 0 }
func (f fakeDirEntry) Type() fs.FileMode          { return f.typ }
func (f fakeDirEntry) Info() (fs.FileInfo, error) { return nil, nil }

func TestFileCoverageInvokeHappyPaths(t *testing.T) {
	dir := t.TempDir()
	tool, _ := New(dir)

	// write + read.
	res, err := tool.Invoke(context.Background(), json.RawMessage(`{"op":"write","path":"a.txt","content":"hello"}`))
	if err != nil {
		t.Fatalf("Invoke write: %v", err)
	}
	if res.IsError {
		t.Fatalf("write error = %+v", res)
	}
	res, _ = tool.Invoke(context.Background(), json.RawMessage(`{"op":"read","path":"a.txt"}`))
	if res.IsError || !strings.Contains(res.Output, "hello") {
		t.Fatalf("read output = %s", res.Output)
	}

	// append.
	tool.Invoke(context.Background(), json.RawMessage(`{"op":"append","path":"a.txt","content":" world"}`))
	res, _ = tool.Invoke(context.Background(), json.RawMessage(`{"op":"read","path":"a.txt"}`))
	if !strings.Contains(res.Output, "hello world") {
		t.Fatalf("append output = %s", res.Output)
	}

	// list.
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("y"), 0o644); err != nil {
		t.Fatalf("write b: %v", err)
	}
	res, _ = tool.Invoke(context.Background(), json.RawMessage(`{"op":"list","path":""}`))
	if res.IsError || !strings.Contains(res.Output, "a.txt") || !strings.Contains(res.Output, "b.txt") {
		t.Fatalf("list = %s", res.Output)
	}

	// stat.
	res, _ = tool.Invoke(context.Background(), json.RawMessage(`{"op":"stat","path":"a.txt"}`))
	if res.IsError || !strings.Contains(res.Output, `"size"`) {
		t.Fatalf("stat = %s", res.Output)
	}

	// search.
	res, _ = tool.Invoke(context.Background(), json.RawMessage(`{"op":"search","path":".","pattern":"hello"}`))
	if res.IsError || !strings.Contains(res.Output, "hello world") {
		t.Fatalf("search = %s", res.Output)
	}

	// search with regex.
	res, _ = tool.Invoke(context.Background(), json.RawMessage(`{"op":"search","path":".","pattern":"h(ello)","regex":true}`))
	if res.IsError || !strings.Contains(res.Output, "h(ello)") {
		t.Fatalf("regex search = %s", res.Output)
	}

	// glob.
	res, _ = tool.Invoke(context.Background(), json.RawMessage(`{"op":"glob","pattern":"*.txt"}`))
	if res.IsError || !strings.Contains(res.Output, "a.txt") {
		t.Fatalf("glob = %s", res.Output)
	}

	// replace (single match, unique).
	tool.Invoke(context.Background(), json.RawMessage(`{"op":"write","path":"r.txt","content":"foo bar foo"}`))
	res, _ = tool.Invoke(context.Background(), json.RawMessage(`{"op":"replace","path":"r.txt","find":"bar","replacement":"baz"}`))
	if res.IsError {
		t.Fatalf("replace = %+v", res)
	}
	res, _ = tool.Invoke(context.Background(), json.RawMessage(`{"op":"read","path":"r.txt"}`))
	if !strings.Contains(res.Output, "foo baz foo") {
		t.Fatalf("replace result = %s", res.Output)
	}

	// delete.
	res, _ = tool.Invoke(context.Background(), json.RawMessage(`{"op":"delete","path":"a.txt"}`))
	if res.IsError {
		t.Fatalf("delete = %+v", res)
	}
	if _, err := os.Stat(filepath.Join(dir, "a.txt")); !os.IsNotExist(err) {
		t.Fatalf("file should be gone, got err = %v", err)
	}
}

func TestFileCoverageInvokeValidation(t *testing.T) {
	dir := t.TempDir()
	tool, _ := New(dir)

	// Parse error: hard.
	_, err := tool.Invoke(context.Background(), json.RawMessage(`{`))
	if err == nil || !strings.Contains(err.Error(), "parse input") {
		t.Fatalf("parse error = %v", err)
	}

	// Unknown op.
	res, _ := tool.Invoke(context.Background(), json.RawMessage(`{"op":"wat"}`))
	if !res.IsError {
		t.Fatalf("unknown op should be error: %+v", res)
	}

	// Path traversal: rejected.
	res, _ = tool.Invoke(context.Background(), json.RawMessage(`{"op":"read","path":"../escape.txt"}`))
	if !res.IsError {
		t.Fatalf("path traversal should be rejected: %s", res.Output)
	}

	// Absolute path outside root: rejected.
	res, _ = tool.Invoke(context.Background(), json.RawMessage(`{"op":"read","path":"/etc/hosts"}`))
	if !res.IsError {
		t.Fatalf("absolute outside should be rejected: %s", res.Output)
	}
}

func TestFileCoverageReadHelper(t *testing.T) {
	// readUpTo: short input returns the whole content; EOF is normalized to nil.
	short := strings.NewReader("hello")
	got, err := readUpTo(short, 100)
	if err != nil || string(got) != "hello" {
		t.Fatalf("readUpTo short = %q err %v", got, err)
	}
	// Long input > max: returns max bytes (truncation is implicit; EOF is swallowed).
	long := strings.NewReader(strings.Repeat("x", 200))
	got, err = readUpTo(long, 100)
	if len(got) != 100 {
		t.Fatalf("readUpTo long = %d bytes, want 100", len(got))
	}
	if err != nil {
		t.Fatalf("readUpTo long err = %v, want nil (EOF swallowed)", err)
	}
}
