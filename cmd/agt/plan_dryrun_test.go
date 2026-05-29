// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestRunDryRunPreview_RejectsInvalidPlan — preview must fail
// loudly when the daemon returns a malformed plan; rendering
// a non-executable plan would mislead the reviewer.
func TestRunDryRunPreview_RejectsInvalidPlan(t *testing.T) {
	var out, errOut bytes.Buffer
	// Plan with a cycle (a→b, b→a). validateJSON catches it.
	bad := `{"nodes":[
		{"id":"a","kind":"loop","intent":"x","deps":["b"]},
		{"id":"b","kind":"loop","intent":"y","deps":["a"]}
	]}`
	code := runDryRunPreview(bad, 2, "", &out, &errOut)
	if code != 1 {
		t.Errorf("exit=%d want 1", code)
	}
	if !strings.Contains(errOut.String(), "cycle") {
		t.Errorf("stderr should mention cycle; got %q", errOut.String())
	}
}

// TestRunDryRunPreview_RendersFullPipelineWithoutModel covers
// the no-model path: preview prints validation + Mermaid + a
// hint telling the operator how to add cost.
func TestRunDryRunPreview_RendersFullPipelineWithoutModel(t *testing.T) {
	var out, errOut bytes.Buffer
	plan := `{"name":"smoke","nodes":[
		{"id":"a","kind":"loop","intent":"do thing"}
	]}`
	code := runDryRunPreview(plan, 1, "", &out, &errOut)
	if code != 0 {
		t.Fatalf("exit=%d want 0; stderr=%q", code, errOut.String())
	}
	for _, want := range []string{
		"dry run (no execution)",
		"plan JSON:",
		"```mermaid",
		"graph TD",
		`a["loop:`,
		"pass --model",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q; got %q", want, out.String())
		}
	}
}

// TestRunDryRunPreview_WithModelRendersCostBlock — when
// --model is supplied, the preview prints a per-node cost
// breakdown plus a total. Unknown models pass through (they
// price as $0.0000 — governor.CostMicrocents doesn't error
// on missing pricing, since static fallbacks already cover
// the common case).
func TestRunDryRunPreview_WithModelRendersCostBlock(t *testing.T) {
	var out, errOut bytes.Buffer
	plan := `{"nodes":[{"id":"a","kind":"loop","intent":"x"}]}`
	code := runDryRunPreview(plan, 1, "claude-sonnet-4-6", &out, &errOut)
	if code != 0 {
		t.Fatalf("exit=%d want 0; stderr=%q", code, errOut.String())
	}
	for _, want := range []string{
		"cost estimate (model=claude-sonnet-4-6)",
		"total:",
		"assumed", // assumed N input + N output tokens
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q; got %q", want, out.String())
		}
	}
}

// TestJsonPretty_HandlesGarbageInput — invalid JSON returns
// the input bytes verbatim + error; callers fall back to
// rendering the raw response so operators see *something*.
func TestJsonPretty_HandlesGarbageInput(t *testing.T) {
	in := []byte("not json {")
	out, err := jsonPretty(in)
	if err == nil {
		t.Error("expected error on invalid JSON")
	}
	if !bytes.Equal(in, out) {
		t.Errorf("expected fallback to original bytes; got %q", out)
	}
}
