// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"testing"

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
