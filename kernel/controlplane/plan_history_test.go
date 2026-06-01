// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestPlanHistory_ListsAndJoinsOutcome — `agt plan history` folds plan.started
// joined with plan.completed/failed, newest-first, with a status filter (M83).
func TestPlanHistory_ListsAndJoinsOutcome(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	started := func(corr, name string, nodes int) {
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "plan", Kind: event.KindPlanStarted, Actor: "scheduler",
			CorrelationID: corr,
			Payload:       map[string]any{"plan_name": name, "node_count": nodes},
		})
	}
	completed := func(corr, name string) {
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "plan", Kind: event.KindPlanCompleted, Actor: "scheduler",
			CorrelationID: corr, Payload: map[string]any{"plan_name": name},
		})
	}
	failed := func(corr, name string) {
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "plan", Kind: event.KindPlanFailed, Actor: "scheduler",
			CorrelationID: corr, Payload: map[string]any{"plan_name": name},
		})
	}
	started("plan-1", "deploy", 2)
	completed("plan-1", "deploy")
	started("plan-2", "backup", 1)
	failed("plan-2", "backup")
	started("plan-3", "running-one", 3) // no terminal → running

	// All three.
	res, err := c.Call(context.Background(), controlplane.CmdPlanHistory, nil)
	if err != nil {
		t.Fatal(err)
	}
	all, _ := res["plans"].([]any)
	if len(all) != 3 {
		t.Fatalf("plans = %d want 3", len(all))
	}
	// Newest first → plan-3.
	first, _ := all[0].(map[string]any)
	if first["correlation_id"] != "plan-3" {
		t.Errorf("newest = %v want plan-3", first["correlation_id"])
	}
	if first["status"] != "running" {
		t.Errorf("plan-3 status = %v want running", first["status"])
	}

	// --status failed → just plan-2.
	fres, err := c.Call(context.Background(), controlplane.CmdPlanHistory,
		map[string]any{"status": "failed"})
	if err != nil {
		t.Fatal(err)
	}
	fp, _ := fres["plans"].([]any)
	if len(fp) != 1 {
		t.Fatalf("--status failed = %d want 1", len(fp))
	}
	m, _ := fp[0].(map[string]any)
	if m["correlation_id"] != "plan-2" || m["plan_name"] != "backup" {
		t.Errorf("failed plan = %v / %v want plan-2 / backup", m["correlation_id"], m["plan_name"])
	}
}
