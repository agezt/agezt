// SPDX-License-Identifier: MIT

package forgetool

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/toolforge"
)

// fakeKernel drives the tool against a real toolforge.Store with a canned
// sandbox verdict — the kernel's journaling is covered by runtime tests.
type fakeKernel struct {
	store   *toolforge.Store
	testOut string
	testOK  bool
}

func (f *fakeKernel) DraftScriptTool(_ string, st toolforge.ScriptTool) (toolforge.ScriptTool, error) {
	return f.store.Add(st)
}

func (f *fakeKernel) UpdateScriptTool(_, ref string, mutate func(*toolforge.ScriptTool)) (toolforge.ScriptTool, bool, error) {
	st, err := f.store.Update(ref, mutate)
	if errors.Is(err, toolforge.ErrNotFound) {
		return toolforge.ScriptTool{}, false, nil
	}
	if err != nil {
		return toolforge.ScriptTool{}, false, err
	}
	return st, true, nil
}

func (f *fakeKernel) TestScriptTool(_ context.Context, _, ref, _ string) (toolforge.ScriptTool, string, error) {
	st, err := f.store.RecordTest(ref, f.testOK)
	return st, f.testOut, err
}

func (f *fakeKernel) ToolForge() *toolforge.Store { return f.store }

func newBound(t *testing.T) (*Tool, *fakeKernel) {
	t.Helper()
	store, err := toolforge.Open(t.TempDir())
	if err != nil {
		t.Fatalf("toolforge.Open: %v", err)
	}
	fk := &fakeKernel{store: store, testOut: "ok", testOK: true}
	tool := New()
	tool.Bind(fk)
	return tool, fk
}

func invoke(t *testing.T, tool *Tool, in map[string]any) (string, bool) {
	t.Helper()
	raw, _ := json.Marshal(in)
	res, err := tool.Invoke(context.Background(), raw)
	if err != nil {
		t.Fatalf("Invoke(%v): %v", in, err)
	}
	return res.Output, res.IsError
}

// TestAuthoringLoop walks the agent's whole surface: draft → test (pass) →
// list/show → update code (demote warning) → and confirms promotion is NOT a
// tool op (the operator owns it).
func TestAuthoringLoop(t *testing.T) {
	tool, fk := newBound(t)

	out, isErr := invoke(t, tool, map[string]any{
		"op": "draft", "name": "echo", "description": "echoes input",
		"language": "python", "code": "print(open('stdin.txt').read())",
	})
	if isErr || !strings.Contains(out, "drafted") {
		t.Fatalf("draft: err=%v out=%s", isErr, out)
	}

	out, isErr = invoke(t, tool, map[string]any{"op": "test", "ref": "echo", "input": `{"x":1}`})
	if isErr || !strings.Contains(out, "PASSED") {
		t.Fatalf("test: err=%v out=%s", isErr, out)
	}

	out, _ = invoke(t, tool, map[string]any{"op": "list"})
	if !strings.Contains(out, `"echo"`) || !strings.Contains(out, `"draft"`) {
		t.Fatalf("list: %s", out)
	}
	out, _ = invoke(t, tool, map[string]any{"op": "show", "ref": "echo"})
	if !strings.Contains(out, "stdin.txt") {
		t.Fatalf("show missing code: %s", out)
	}

	out, isErr = invoke(t, tool, map[string]any{"op": "update", "ref": "echo", "code": "print('v2')"})
	if isErr || !strings.Contains(out, "re-test") {
		t.Fatalf("update should warn about the demotion: err=%v out=%s", isErr, out)
	}
	if st, _ := fk.store.Get("echo"); st.TestedOK || st.Status != toolforge.StatusDraft {
		t.Fatalf("code update did not demote: %+v", st)
	}

	// Promotion is the operator's move, not an agent op.
	out, isErr = invoke(t, tool, map[string]any{"op": "promote", "ref": "echo"})
	if !isErr || !strings.Contains(out, "unknown op") {
		t.Fatalf("promote must not be a tool op: err=%v out=%s", isErr, out)
	}
}

// TestFailingTestReportsFAILED: the verdict and the sandbox output both reach
// the agent so it can iterate.
func TestFailingTestReportsFAILED(t *testing.T) {
	tool, fk := newBound(t)
	invoke(t, tool, map[string]any{
		"op": "draft", "name": "broken", "description": "d", "language": "python", "code": "boom(",
	})
	fk.testOK, fk.testOut = false, "SyntaxError: unexpected EOF"
	out, isErr := invoke(t, tool, map[string]any{"op": "test", "ref": "broken"})
	if !isErr || !strings.Contains(out, "FAILED") || !strings.Contains(out, "SyntaxError") {
		t.Fatalf("failing test misreported: err=%v out=%s", isErr, out)
	}
}

// TestUnbound: before Bind, every op reports the forge unavailable.
func TestUnbound(t *testing.T) {
	out, isErr := invoke(t, New(), map[string]any{"op": "list"})
	if !isErr || !strings.Contains(out, "not available") {
		t.Fatalf("unbound: err=%v out=%s", isErr, out)
	}
}
