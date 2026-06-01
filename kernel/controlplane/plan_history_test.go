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

// TestPlanStats_Aggregates — `agt plan stats` counts completed/failed/running
// and computes the success rate over terminal plans (M84).
func TestPlanStats_Aggregates(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	started := func(corr string) {
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "plan", Kind: event.KindPlanStarted, Actor: "scheduler",
			CorrelationID: corr, Payload: map[string]any{"plan_name": "p", "node_count": 1},
		})
	}
	term := func(corr string, kind event.Kind) {
		_, _ = k.Bus().Publish(event.Spec{
			Subject: "plan", Kind: kind, Actor: "scheduler",
			CorrelationID: corr, Payload: map[string]any{"plan_name": "p"},
		})
	}
	started("p1")
	term("p1", event.KindPlanCompleted)
	started("p2")
	term("p2", event.KindPlanCompleted)
	started("p3")
	term("p3", event.KindPlanFailed)
	started("p4") // running

	res, err := c.Call(context.Background(), controlplane.CmdPlanStats, nil)
	if err != nil {
		t.Fatal(err)
	}
	if tot, _ := res["total"].(float64); tot != 4 {
		t.Errorf("total = %v want 4", res["total"])
	}
	if comp, _ := res["completed"].(float64); comp != 2 {
		t.Errorf("completed = %v want 2", res["completed"])
	}
	if fail, _ := res["failed"].(float64); fail != 1 {
		t.Errorf("failed = %v want 1", res["failed"])
	}
	if run, _ := res["running"].(float64); run != 1 {
		t.Errorf("running = %v want 1", res["running"])
	}
	// success rate over terminal (3): 2/3.
	if rate, _ := res["success_rate"].(float64); rate < 0.66 || rate > 0.67 {
		t.Errorf("success_rate = %v want ~0.667", rate)
	}
}
