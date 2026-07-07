// SPDX-License-Identifier: MIT

package acpagent

import (
	"context"
	"encoding/json"
	"errors"
	"runtime"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
)

func TestACPAgentCoverageDefinitionAndHelpers(t *testing.T) {
	tool := New("claude-code-acp", "/workspace")
	def := tool.Definition()
	if def.Name != "acp_agent" {
		t.Fatalf("Name = %q", def.Name)
	}
	if len(def.InputSchema) == 0 {
		t.Fatal("InputSchema should not be empty")
	}
	if def.Effect.Class != agent.EffectCompensable {
		t.Fatalf("Effect.Class = %v, want %v", def.Effect.Class, agent.EffectCompensable)
	}

	shell, arg := platformShell()
	if shell == "" || arg == "" {
		t.Fatalf("platformShell returned empty: %q/%q", shell, arg)
	}
	if runtime.GOOS == "windows" {
		if shell != "cmd" || arg != "/C" {
			t.Fatalf("windows shell = %q/%q, want cmd/C", shell, arg)
		}
	} else {
		if shell != "sh" || arg != "-c" {
			t.Fatalf("unix shell = %q/%q, want sh/-c", shell, arg)
		}
	}

	if got := AbsCwd("relative/path"); !strings.HasSuffix(got, "relative/path") && !strings.HasSuffix(got, "relative\\path") {
		t.Fatalf("AbsCwd should preserve the relative tail, got %q", got)
	}

	if truncate("short", 100) != "short" {
		t.Fatalf("truncate should not truncate short input")
	}
	// Truncation only kicks in for inputs longer than MaxOutputBytes (60 KiB),
	// so exercise that branch with a long string.
	long := strings.Repeat("x", MaxOutputBytes+10)
	out := truncate(long, MaxOutputBytes)
	if !strings.Contains(out, "… [truncated") {
		t.Fatalf("truncate long should include truncation marker, got %q", out)
	}
}

func TestACPAgentCoverageRender(t *testing.T) {
	// Empty answer.
	r := render("", "end_turn")
	if !strings.Contains(r, "no message") {
		t.Fatalf("empty answer render = %q", r)
	}
	// Non-empty answer with stop reason end_turn — no footer.
	r = render("hello", "end_turn")
	if r != "hello" {
		t.Fatalf("end_turn render = %q", r)
	}
	// Non-empty with non-default stop reason.
	r = render("answer", "max_tokens")
	if !strings.Contains(r, "answer") || !strings.Contains(r, "[stopReason: max_tokens]") {
		t.Fatalf("max_tokens render = %q", r)
	}
	// Truncation path: render truncates at MaxOutputBytes.
	r = render(strings.Repeat("x", MaxOutputBytes+10), "end_turn")
	if !strings.Contains(r, "… [truncated") {
		t.Fatalf("long answer render should truncate, got %q", r)
	}
}

func TestACPAgentCoverageInvokeValidation(t *testing.T) {
	tool := &Tool{Cmd: "x", Cwd: "/w", dial: func(_ context.Context, _, _ string) (*transport, error) {
		return nil, errors.New("dial failed")
	}}
	// Malformed JSON.
	res, err := tool.Invoke(context.Background(), json.RawMessage(`{`))
	if err != nil {
		t.Fatalf("Invoke malformed: %v", err)
	}
	if !res.IsError || !strings.Contains(res.Output, "invalid input") {
		t.Fatalf("malformed input = %+v", res)
	}
	// Empty task.
	res, err = tool.Invoke(context.Background(), json.RawMessage(`{"task":""}`))
	if err != nil || !res.IsError || !strings.Contains(res.Output, "task is required") {
		t.Fatalf("empty task = %+v err %v", res, err)
	}
	// Dial failure.
	res, err = tool.Invoke(context.Background(), json.RawMessage(`{"task":"do it"}`))
	if err != nil || !res.IsError || !strings.Contains(res.Output, "spawn ACP agent failed") {
		t.Fatalf("dial failure = %+v err %v", res, err)
	}
}
