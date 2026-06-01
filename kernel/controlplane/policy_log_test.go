// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestEdictLog_ListsAndFiltersDenied — `agt edict log` lists policy.decision
// events newest-first, and `--denied` keeps only denials (M63).
func TestEdictLog_ListsAndFiltersDenied(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	dec := func(tool, capability string, allow, hard bool) {
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "policy", Kind: event.KindPolicyDecision, Actor: "agent-x",
			CorrelationID: "run-1",
			Payload: map[string]any{
				"tool": tool, "capability": capability, "allow": allow,
				"hard_denied": hard, "reason": "test",
			},
		})
	}
	dec("shell", "shell", true, false)
	dec("http", "net", false, false)
	dec("file", "fs", false, true)

	// All decisions.
	res, err := c.Call(context.Background(), controlplane.CmdEdictLog, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	all, _ := res["decisions"].([]any)
	if len(all) != 3 {
		t.Fatalf("decisions = %d want 3", len(all))
	}

	// Denied only.
	dres, err := c.Call(context.Background(), controlplane.CmdEdictLog,
		map[string]any{"denied": true})
	if err != nil {
		t.Fatal(err)
	}
	denied, _ := dres["decisions"].([]any)
	if len(denied) != 2 {
		t.Fatalf("denied decisions = %d want 2", len(denied))
	}
	for _, raw := range denied {
		m, _ := raw.(map[string]any)
		if allow, _ := m["allow"].(bool); allow {
			t.Errorf("--denied returned an allowed decision: %v", m)
		}
	}
}

// TestEdictStats_Aggregates — `agt edict stats` counts allow/deny/hard-denied,
// computes the denial rate, and breaks denials down by capability (M64).
func TestEdictStats_Aggregates(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	dec := func(capability string, allow, hard bool) {
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "policy", Kind: event.KindPolicyDecision, Actor: "agent-x",
			CorrelationID: "run-1",
			Payload: map[string]any{
				"tool": "t", "capability": capability, "allow": allow, "hard_denied": hard,
			},
		})
	}
	dec("shell", true, false)
	dec("net", false, false)
	dec("net", false, false)
	dec("fs", false, true)

	res, err := c.Call(context.Background(), controlplane.CmdEdictStats, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if got := intOf(res["total"]); got != 4 {
		t.Errorf("total = %d want 4", got)
	}
	if got := intOf(res["allowed"]); got != 1 {
		t.Errorf("allowed = %d want 1", got)
	}
	if got := intOf(res["denied"]); got != 3 {
		t.Errorf("denied = %d want 3", got)
	}
	if got := intOf(res["hard_denied"]); got != 1 {
		t.Errorf("hard_denied = %d want 1", got)
	}
	if rate, _ := res["denial_rate"].(float64); rate < 0.74 || rate > 0.76 {
		t.Errorf("denial_rate = %v want 0.75", rate)
	}
	byCap, _ := res["denied_by_capability"].(map[string]any)
	if got := intOf(byCap["net"]); got != 2 {
		t.Errorf("denied_by_capability[net] = %d want 2", got)
	}
	if got := intOf(byCap["fs"]); got != 1 {
		t.Errorf("denied_by_capability[fs] = %d want 1", got)
	}
}

// TestEdictLog_SinceWindow — args.since_ms restricts the log to decisions within
// the window (M65): a huge window includes a just-published decision; a tiny
// window after a brief sleep excludes it.
func TestEdictLog_SinceWindow(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	_, _ = k.Bus().Publish(event.Spec{
		Subject: "policy", Kind: event.KindPolicyDecision, Actor: "agent-x",
		CorrelationID: "run-1",
		Payload:       map[string]any{"tool": "t", "capability": "c", "allow": true},
	})

	res, err := c.Call(context.Background(), controlplane.CmdEdictLog,
		map[string]any{"since_ms": int64(3_600_000)}) // 1h window includes it
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := res["decisions"].([]any); len(got) != 1 {
		t.Errorf("1h window decisions = %d want 1", len(got))
	}

	time.Sleep(5 * time.Millisecond)
	res, err = c.Call(context.Background(), controlplane.CmdEdictLog,
		map[string]any{"since_ms": int64(1)}) // 1ms window excludes it
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := res["decisions"].([]any); len(got) != 0 {
		t.Errorf("1ms window decisions = %d want 0", len(got))
	}
}

// TestEdictLog_ToolAndCapabilityFilters — `agt edict log --tool` / `--capability`
// scope the decision log, the drill-down from edict stats' breakdown (M74).
func TestEdictLog_ToolAndCapabilityFilters(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	dec := func(tool, capability string, allow bool) {
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "policy", Kind: event.KindPolicyDecision, Actor: "agent-x",
			CorrelationID: "run-1",
			Payload: map[string]any{
				"tool": tool, "capability": capability, "allow": allow, "reason": "t",
			},
		})
	}
	dec("shell", "shell", true)
	dec("http", "net", false)
	dec("curl", "net", false)

	// --tool shell → 1.
	tres, err := c.Call(context.Background(), controlplane.CmdEdictLog,
		map[string]any{"tool": "shell"})
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := tres["decisions"].([]any); len(got) != 1 {
		t.Errorf("--tool shell = %d want 1", len(got))
	}

	// --capability net → 2 (http + curl).
	cres, err := c.Call(context.Background(), controlplane.CmdEdictLog,
		map[string]any{"capability": "net"})
	if err != nil {
		t.Fatal(err)
	}
	netd, _ := cres["decisions"].([]any)
	if len(netd) != 2 {
		t.Fatalf("--capability net = %d want 2", len(netd))
	}
	for _, raw := range netd {
		m, _ := raw.(map[string]any)
		if m["capability"] != "net" {
			t.Errorf("--capability net returned %v", m["capability"])
		}
	}

	// Combined with --denied: net + denied → still 2 (both net are denials).
	dres, err := c.Call(context.Background(), controlplane.CmdEdictLog,
		map[string]any{"capability": "net", "denied": true})
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := dres["decisions"].([]any); len(got) != 2 {
		t.Errorf("--capability net --denied = %d want 2", len(got))
	}
}

// TestEdictStats_ToolScope — `agt edict stats --tool` scopes the aggregate to one
// tool (M76), symmetric with edict log's M74 filters.
func TestEdictStats_ToolScope(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	dec := func(tool, capability string, allow bool) {
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "policy", Kind: event.KindPolicyDecision, Actor: "agent-x",
			CorrelationID: "run-1",
			Payload:       map[string]any{"tool": tool, "capability": capability, "allow": allow},
		})
	}
	dec("shell", "shell", true)
	dec("http", "net", false)
	dec("http", "net", false)

	// Unscoped → 3 total.
	all, err := c.Call(context.Background(), controlplane.CmdEdictStats, nil)
	if err != nil {
		t.Fatal(err)
	}
	if tot, _ := all["total"].(float64); tot != 3 {
		t.Errorf("unscoped total = %v want 3", all["total"])
	}

	// --tool http → 2 total, both denied (rate 1.0).
	res, err := c.Call(context.Background(), controlplane.CmdEdictStats,
		map[string]any{"tool": "http"})
	if err != nil {
		t.Fatal(err)
	}
	if tot, _ := res["total"].(float64); tot != 2 {
		t.Errorf("--tool http total = %v want 2", res["total"])
	}
	if dn, _ := res["denied"].(float64); dn != 2 {
		t.Errorf("--tool http denied = %v want 2", res["denied"])
	}
	if rate, _ := res["denial_rate"].(float64); rate != 1.0 {
		t.Errorf("--tool http denial_rate = %v want 1.0", rate)
	}
}
