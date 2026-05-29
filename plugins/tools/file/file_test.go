// SPDX-License-Identifier: MIT

package file

import (
	"context"
	"encoding/json"
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

