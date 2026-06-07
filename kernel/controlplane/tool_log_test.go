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

// TestToolStats_Aggregates — `agt tool stats` counts total/errored, computes the
// error rate, and breaks calls + errors down by tool (M67).
func TestToolStats_Aggregates(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	toolResult(k, "a", "shell", "ok", false)
	toolResult(k, "b", "shell", "boom", true)
	toolResult(k, "c", "http", "ok", false)
	toolResult(k, "d", "http", "fail", true)

	res, err := c.Call(context.Background(), controlplane.CmdToolStats, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if total, _ := res["total"].(float64); total != 4 {
		t.Errorf("total = %v want 4", res["total"])
	}
	if errored, _ := res["errored"].(float64); errored != 2 {
		t.Errorf("errored = %v want 2", res["errored"])
	}
	if rate, _ := res["error_rate"].(float64); rate != 0.5 {
		t.Errorf("error_rate = %v want 0.5", rate)
	}
	byTool, _ := res["by_tool"].(map[string]any)
	shell, _ := byTool["shell"].(map[string]any)
	if calls, _ := shell["calls"].(float64); calls != 2 {
		t.Errorf("shell calls = %v want 2", shell["calls"])
	}
	if errs, _ := shell["errors"].(float64); errs != 1 {
		t.Errorf("shell errors = %v want 1", shell["errors"])
	}

	// Tool filter scopes the aggregate to one tool.
	fres, err := c.Call(context.Background(), controlplane.CmdToolStats,
		map[string]any{"tool": "http"})
	if err != nil {
		t.Fatal(err)
	}
	if total, _ := fres["total"].(float64); total != 2 {
		t.Errorf("filtered total = %v want 2", fres["total"])
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

// TestToolLog_ReportsLatency — each invocation row carries a duration_ms field
// (M71), the invoked→result span joined by call_id; back-to-back publish yields
// a small non-negative latency.
func TestToolLog_ReportsLatency(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	toolInvoked(k, "call-1", "shell", "ls")
	time.Sleep(3 * time.Millisecond)
	toolResult(k, "call-1", "shell", "done", false)

	res, err := c.Call(context.Background(), controlplane.CmdToolLog, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	invs, _ := res["invocations"].([]any)
	if len(invs) != 1 {
		t.Fatalf("invocations = %d want 1", len(invs))
	}
	m, _ := invs[0].(map[string]any)
	d, ok := m["duration_ms"].(float64)
	if !ok {
		t.Fatalf("duration_ms missing or not numeric: %v", m["duration_ms"])
	}
	if d < 0 {
		t.Errorf("duration_ms = %v want >= 0", d)
	}

	// stats carries a latency distribution block (M71).
	sres, err := c.Call(context.Background(), controlplane.CmdToolStats, nil)
	if err != nil {
		t.Fatal(err)
	}
	dur, _ := sres["duration_ms"].(map[string]any)
	if dur == nil {
		t.Fatalf("tool stats missing duration_ms block")
	}
	if cnt, _ := dur["count"].(float64); cnt != 1 {
		t.Errorf("latency count = %v want 1", dur["count"])
	}
}

// TestToolLog_SlowFilter — --slow keeps only calls at/above the latency floor
// (M73). One fast call (invoked+result back-to-back) and one slow call (a gap
// between them); the floor is then chosen RELATIVE to the two calls' actually-
// measured spans, not an absolute magic number.
//
// History: earlier cuts used a fixed floor (10ms, then 50ms) and assumed the
// back-to-back "fast" call's span stayed below it. Both flaked on Windows CI —
// a stalled runner can space two consecutive Publish calls ≥50ms apart, so the
// fast call measured "slow" and was wrongly counted. There is no timestamp
// injection seam (Bus.Publish / Journal.Append stamp time.Now themselves), so
// instead of fighting absolute jitter we read the real measured spans and put
// the floor strictly between them. That keeps exactly the slow call regardless
// of absolute timer granularity — the only way it fails is if the fast call's
// span exceeds the slow call's (a >120ms stall landing precisely between the
// fast pair), which we detect and skip rather than report as a false failure.
func TestToolLog_SlowFilter(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	// Fast call: invoked+result back-to-back (~0ms, modulo scheduler jitter).
	toolInvoked(k, "fast", "shell", "a")
	toolResult(k, "fast", "shell", "ok", false)
	// Slow call: a clearly-slow gap, wide enough to dominate the fast call's jitter.
	toolInvoked(k, "slow", "http", "b")
	time.Sleep(120 * time.Millisecond)
	toolResult(k, "slow", "http", "ok", false)

	// No floor → both, and learn each call's ACTUAL measured span.
	all, err := c.Call(context.Background(), controlplane.CmdToolLog, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := all["invocations"].([]any)
	if len(got) != 2 {
		t.Fatalf("unfiltered = %d want 2", len(got))
	}
	durByTool := map[string]int64{}
	for _, iv := range got {
		m, _ := iv.(map[string]any)
		tool, _ := m["tool"].(string)
		d, _ := m["duration_ms"].(float64)
		durByTool[tool] = int64(d)
	}
	dFast, dSlow := durByTool["shell"], durByTool["http"]
	if dSlow <= dFast {
		t.Skipf("environment too noisy to separate spans (fast=%dms slow=%dms)", dFast, dSlow)
	}

	// Floor strictly between the two observed spans → keeps only the slow call,
	// deterministically, independent of absolute timer jitter.
	floor := dFast + (dSlow-dFast)/2
	if floor <= dFast {
		floor = dFast + 1
	}
	res, err := c.Call(context.Background(), controlplane.CmdToolLog,
		map[string]any{"slow_ms": floor})
	if err != nil {
		t.Fatal(err)
	}
	slow, _ := res["invocations"].([]any)
	if len(slow) != 1 {
		t.Fatalf("slow-filtered = %d want 1 (floor=%dms, fast=%dms, slow=%dms)",
			len(slow), floor, dFast, dSlow)
	}
	m, _ := slow[0].(map[string]any)
	if m["tool"] != "http" {
		t.Errorf("slow call tool = %v want http", m["tool"])
	}
}

// TestToolStats_PerToolLatency — the by_tool breakdown carries a per-tool avg_ms
// for tools with a joinable invoked→result span (M75).
func TestToolStats_PerToolLatency(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	toolInvoked(k, "s1", "shell", "a")
	time.Sleep(15 * time.Millisecond)
	toolResult(k, "s1", "shell", "ok", false)

	res, err := c.Call(context.Background(), controlplane.CmdToolStats, nil)
	if err != nil {
		t.Fatal(err)
	}
	byTool, _ := res["by_tool"].(map[string]any)
	shell, _ := byTool["shell"].(map[string]any)
	avg, ok := shell["avg_ms"].(float64)
	if !ok {
		t.Fatalf("shell entry missing avg_ms: %v", shell)
	}
	if avg < 0 {
		t.Errorf("shell avg_ms = %v want >= 0", avg)
	}
}

// TestToolStats_ErrorsByMessage — the aggregate buckets failed calls by their
// error message, most-frequent surfaced via the map (M79).
func TestToolStats_ErrorsByMessage(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	toolResult(k, "a", "shell", "boom", true)
	toolResult(k, "b", "http", "boom", true)
	toolResult(k, "c", "http", "denied by policy", true)
	toolResult(k, "d", "shell", "ok", false) // success: not bucketed

	res, err := c.Call(context.Background(), controlplane.CmdToolStats, nil)
	if err != nil {
		t.Fatal(err)
	}
	byErr, _ := res["errors_by_message"].(map[string]any)
	if byErr == nil {
		t.Fatalf("missing errors_by_message")
	}
	if got, _ := byErr["boom"].(float64); got != 2 {
		t.Errorf("errors_by_message[boom] = %v want 2", byErr["boom"])
	}
	if got, _ := byErr["denied by policy"].(float64); got != 1 {
		t.Errorf("errors_by_message[denied by policy] = %v want 1", byErr["denied by policy"])
	}
	if _, ok := byErr["ok"]; ok {
		t.Errorf("successful call was bucketed as an error")
	}
}
