// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestCmdRunsShow_HelpExitsCleanly — pure flag parsing, no
// daemon connection required.
func TestCmdRunsShow_HelpExitsCleanly(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cmdRunsShow([]string{"--help"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit=%d want 0; stderr=%q", code, errOut.String())
	}
	for _, want := range []string{"correlation", "task arc", "--json"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("--help missing %q; got %q", want, out.String())
		}
	}
}

// TestCmdRunsShow_RejectsMissingCorrelation — guard against
// accidental `agt runs show` with no args.
func TestCmdRunsShow_RejectsMissingCorrelation(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cmdRunsShow(nil, &out, &errOut)
	if code != 2 {
		t.Errorf("exit=%d want 2", code)
	}
	if !strings.Contains(errOut.String(), "correlation id required") {
		t.Errorf("stderr should explain requirement; got %q", errOut.String())
	}
}

// TestCmdRunsShow_RejectsExtraPositional — second positional
// would be a silent drop in the old shape; must fail loudly.
func TestCmdRunsShow_RejectsExtraPositional(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cmdRunsShow([]string{"corr-1", "extra"}, &out, &errOut)
	if code != 2 {
		t.Errorf("exit=%d want 2", code)
	}
	if !strings.Contains(errOut.String(), "unexpected arg") {
		t.Errorf("stderr should reject extra positional; got %q", errOut.String())
	}
}

// TestRenderTaskArc_CompletedRunHasHeader — exercises the
// renderer directly with synthetic events so we don't need a
// daemon. Asserts the operator-facing fields (correlation,
// intent, status, duration) are present.
func TestRenderTaskArc_CompletedRunHasHeader(t *testing.T) {
	summary := map[string]any{
		"intent":      "do thing",
		"status":      "completed",
		"iters":       float64(3),
		"duration_ms": float64(1234),
	}
	events := []map[string]any{
		{"kind": "task.received", "seq": float64(1)},
		{"kind": "llm.request", "seq": float64(2)},
		{"kind": "llm.response", "seq": float64(3), "payload": map[string]any{
			"usage": map[string]any{"input_tokens": float64(100), "output_tokens": float64(50)},
		}},
		{"kind": "task.completed", "seq": float64(4)},
	}
	var buf bytes.Buffer
	renderTaskArc(&buf, "run-AAA", summary, events)
	s := buf.String()
	for _, want := range []string{
		"correlation: run-AAA",
		"intent     : do thing",
		"status     : completed (3 iters, 1.2s)",
		"round 1",
		"llm.request",
		"llm.response",
		"input=100, output=50 tokens",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("output missing %q; got:\n%s", want, s)
		}
	}
}

// TestRenderTaskArc_RunningShowsAbandonedHint — task.received
// without a corresponding task.completed is the "operator
// killed daemon mid-run" case; status line must surface the
// hint so it's obvious why no answer is present.
func TestRenderTaskArc_RunningShowsAbandonedHint(t *testing.T) {
	summary := map[string]any{
		"intent": "stranded",
		"status": "running",
	}
	events := []map[string]any{
		{"kind": "task.received", "seq": float64(1)},
	}
	var buf bytes.Buffer
	renderTaskArc(&buf, "stranded", summary, events)
	if !strings.Contains(buf.String(), "abandoned") {
		t.Errorf("running run should hint abandoned; got %q", buf.String())
	}
}

// TestRenderTaskArc_ToolEventsAreIndented — tool events
// rendered inside a round must indent further than the round
// header so the visual nesting matches the actual structure.
func TestRenderTaskArc_ToolEventsAreIndented(t *testing.T) {
	summary := map[string]any{"intent": "x", "status": "completed"}
	events := []map[string]any{
		{"kind": "llm.request", "seq": float64(1)},
		{"kind": "tool.invoked", "seq": float64(2), "payload": map[string]any{"tool": "shell"}},
		{"kind": "tool.result", "seq": float64(3), "payload": map[string]any{}},
		{"kind": "llm.response", "seq": float64(4)},
	}
	var buf bytes.Buffer
	renderTaskArc(&buf, "x", summary, events)
	s := buf.String()
	// 4-space indent for in-round tool events; 2 for outside.
	if !strings.Contains(s, "    tool.invoked: shell") {
		t.Errorf("tool.invoked not indented as in-round; got:\n%s", s)
	}
	if !strings.Contains(s, "    tool.result : ok") {
		t.Errorf("tool.result not indented as in-round; got:\n%s", s)
	}
}

// TestRenderTaskArc_FinalAnswerSurfacesFromLlmResponse —
// the assistant's last content is the final answer; the
// renderer must pull it out of payload.message.content and
// print it under "final answer:".
func TestRenderTaskArc_FinalAnswerSurfacesFromLlmResponse(t *testing.T) {
	summary := map[string]any{"intent": "x", "status": "completed"}
	events := []map[string]any{
		{"kind": "llm.request", "seq": float64(1)},
		{"kind": "llm.response", "seq": float64(2), "payload": map[string]any{
			"message": map[string]any{"content": "all done"},
		}},
		{"kind": "task.completed", "seq": float64(3)},
	}
	var buf bytes.Buffer
	renderTaskArc(&buf, "x", summary, events)
	if !strings.Contains(buf.String(), "final answer:") {
		t.Errorf("missing final-answer header; got:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), "all done") {
		t.Errorf("final answer body missing; got:\n%s", buf.String())
	}
}
