// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestWardenLog_FoldsExecAndIssues — `agt warden log` folds warden.executed +
// downgrade + limit newest-first, and --issues drops plain execs (M96).
func TestWardenLog_FoldsExecAndIssues(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	k.Bus().Publish(event.Spec{
		Subject: "warden.exec", Kind: event.KindWardenExecuted, Actor: "tool",
		Payload: map[string]any{"profile_effective": "namespace", "argv0": "ls", "exit_code": 0, "duration_ms": 12},
	})
	k.Bus().Publish(event.Spec{
		Subject: "warden.profile", Kind: event.KindWardenProfileDowngraded, Actor: "tool",
		Payload: map[string]any{"requested": "namespace", "effective": "basic", "reason": "no userns on host"},
	})
	k.Bus().Publish(event.Spec{
		Subject: "warden.limit", Kind: event.KindWardenLimitExceeded, Actor: "tool",
		Payload: map[string]any{"limit": "stdout_bytes", "argv0": "cat"},
	})

	res, err := c.Call(context.Background(), controlplane.CmdWardenLog, nil)
	if err != nil {
		t.Fatal(err)
	}
	all, _ := res["executions"].([]any)
	if len(all) != 3 {
		t.Fatalf("executions = %d want 3", len(all))
	}

	// --issues drops the plain exec → downgrade + limit only.
	ires, err := c.Call(context.Background(), controlplane.CmdWardenLog, map[string]any{"issues": true})
	if err != nil {
		t.Fatal(err)
	}
	issues, _ := ires["executions"].([]any)
	if len(issues) != 2 {
		t.Fatalf("--issues = %d want 2", len(issues))
	}
	for _, raw := range issues {
		m, _ := raw.(map[string]any)
		if m["kind"] == "exec" {
			t.Errorf("--issues returned a plain exec: %v", m)
		}
	}
}

// TestWardenStats_Aggregates — `agt warden stats` counts execs, downgrades (+
// rate), limit breaches, and breaks down by effective profile (M97).
func TestWardenStats_Aggregates(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	exec := func(profile string, downgraded bool) {
		k.Bus().Publish(event.Spec{
			Subject: "warden.exec", Kind: event.KindWardenExecuted, Actor: "tool",
			Payload: map[string]any{"profile_effective": profile, "argv0": "ls", "exit_code": 0, "downgraded": downgraded},
		})
	}
	exec("namespace", false)
	exec("none", true)
	exec("none", true)
	k.Bus().Publish(event.Spec{
		Subject: "warden.limit", Kind: event.KindWardenLimitExceeded, Actor: "tool",
		Payload: map[string]any{"limit": "stdout_bytes", "argv0": "cat"},
	})

	res, err := c.Call(context.Background(), controlplane.CmdWardenStats, nil)
	if err != nil {
		t.Fatal(err)
	}
	if e, _ := res["executions"].(float64); e != 3 {
		t.Errorf("executions = %v want 3", res["executions"])
	}
	if d, _ := res["downgraded"].(float64); d != 2 {
		t.Errorf("downgraded = %v want 2", res["downgraded"])
	}
	if rate, _ := res["downgrade_rate"].(float64); rate < 0.66 || rate > 0.67 {
		t.Errorf("downgrade_rate = %v want ~0.667", rate)
	}
	if lb, _ := res["limit_breaches"].(float64); lb != 1 {
		t.Errorf("limit_breaches = %v want 1", res["limit_breaches"])
	}
	byP, _ := res["by_profile"].(map[string]any)
	if n, _ := byP["none"].(float64); n != 2 {
		t.Errorf("by_profile[none] = %v want 2", byP["none"])
	}
}
