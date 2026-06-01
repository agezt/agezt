// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// toolInvoked + toolResult publish the journal pair the agent loop writes per
// tool call, so the tests exercise the same fold handleToolLog walks.
func toolInvoked(k *runtime.Kernel, callID, tool, input string) {
	_, _ = k.Bus().Publish(event.Spec{
		Subject: "tool", Kind: event.KindToolInvoked, Actor: "tool",
		CorrelationID: "run-1",
		Payload:       map[string]any{"tool": tool, "call_id": callID, "input": input},
	})
}

func toolResult(k *runtime.Kernel, callID, tool, output string, isErr bool) {
	_, _ = k.Bus().Publish(event.Spec{
		Subject: "tool", Kind: event.KindToolResult, Actor: "tool",
		CorrelationID: "run-1",
		Payload:       map[string]any{"tool": tool, "call_id": callID, "output": output, "error": isErr},
	})
}

// TestToolLog_ListsAndJoinsInput — `agt tool log` lists tool.result events
// newest-first, joining each with its tool.invoked input by call_id (M66).
func TestToolLog_ListsAndJoinsInput(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	toolInvoked(k, "call-1", "shell", "ls -la")
	toolResult(k, "call-1", "shell", "total 8\n...", false)
	toolInvoked(k, "call-2", "http", "GET /x")
	toolResult(k, "call-2", "http", "boom", true)

	res, err := c.Call(context.Background(), controlplane.CmdToolLog, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	invs, _ := res["invocations"].([]any)
	if len(invs) != 2 {
		t.Fatalf("invocations = %d want 2", len(invs))
	}
	// Newest first: call-2 (http) precedes call-1 (shell).
	first, _ := invs[0].(map[string]any)
	if first["tool"] != "http" {
		t.Errorf("newest tool = %v want http", first["tool"])
	}
	// Input is journaled as raw JSON (tc.Input is json.RawMessage), so a string
	// value round-trips with its quotes — the preview faithfully shows the raw form.
	if input, _ := first["input"].(string); input != `"GET /x"` {
		t.Errorf("joined input = %q want %q", input, `"GET /x"`)
	}
	if isErr, _ := first["error"].(bool); !isErr {
		t.Errorf("http call error = false want true")
	}
}

// TestToolLog_FiltersErrorsAndTool — `--errors` keeps only failed calls and a
// tool filter scopes to one tool (M66).
func TestToolLog_FiltersErrorsAndTool(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	toolResult(k, "a", "shell", "ok", false)
	toolResult(k, "b", "http", "fail", true)
	toolResult(k, "c", "shell", "boom", true)

	// Errors only → b + c.
	eres, err := c.Call(context.Background(), controlplane.CmdToolLog,
		map[string]any{"errors": true})
	if err != nil {
		t.Fatal(err)
	}
	errs, _ := eres["invocations"].([]any)
	if len(errs) != 2 {
		t.Fatalf("error invocations = %d want 2", len(errs))
	}
	for _, raw := range errs {
		m, _ := raw.(map[string]any)
		if isErr, _ := m["error"].(bool); !isErr {
			t.Errorf("--errors returned a successful call: %v", m)
		}
	}

	// Tool filter → only shell calls (a + c).
	tres, err := c.Call(context.Background(), controlplane.CmdToolLog,
		map[string]any{"tool": "shell"})
	if err != nil {
		t.Fatal(err)
	}
	shells, _ := tres["invocations"].([]any)
	if len(shells) != 2 {
		t.Fatalf("shell invocations = %d want 2", len(shells))
	}
	for _, raw := range shells {
		m, _ := raw.(map[string]any)
		if m["tool"] != "shell" {
			t.Errorf("--tool shell returned %v", m["tool"])
		}
	}
}

// TestToolLog_SinceWindow — args.since_ms restricts the log to calls within the
// window (M66, via the shared sinceCutoff helper): a 1h window includes a
// just-published result; a 1ms window after a brief sleep excludes it.
func TestToolLog_SinceWindow(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	toolResult(k, "x", "shell", "ok", false)

	res, err := c.Call(context.Background(), controlplane.CmdToolLog,
		map[string]any{"since_ms": int64(3_600_000)}) // 1h includes it
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := res["invocations"].([]any); len(got) != 1 {
		t.Errorf("1h window invocations = %d want 1", len(got))
	}

	time.Sleep(5 * time.Millisecond)
	res, err = c.Call(context.Background(), controlplane.CmdToolLog,
		map[string]any{"since_ms": int64(1)}) // 1ms excludes it
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := res["invocations"].([]any); len(got) != 0 {
		t.Errorf("1ms window invocations = %d want 0", len(got))
	}
}
