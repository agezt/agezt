// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/approval"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestApprovalsLog_JoinsRequestAndOutcome — `agt approvals log` joins
// approval.requested with the terminal granted/denied by approval_id, and
// --denied keeps only denials/timeouts (M87).
func TestApprovalsLog_JoinsRequestAndOutcome(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	reqd := func(id, cap, tool string) {
		k.Bus().Publish(event.Spec{
			Subject: "approval", Kind: event.KindApprovalRequested, Actor: "agent",
			Payload: map[string]any{"approval_id": id, "capability": cap, "tool_name": tool, "reason": "r"},
		})
	}
	resolved := func(id string, kind event.Kind, by string) {
		k.Bus().Publish(event.Spec{
			Subject: "approval", Kind: kind, Actor: "agent",
			Payload: map[string]any{"approval_id": id, "decision": "x", "resolved_by": by},
		})
	}
	reqd("a1", "shell", "shell")
	resolved("a1", event.KindApprovalGranted, "alice")
	reqd("a2", "net", "http")
	resolved("a2", event.KindApprovalDenied, "bob")
	reqd("a3", "fs", "file") // pending

	res, err := c.Call(context.Background(), controlplane.CmdApprovalsLog, nil)
	if err != nil {
		t.Fatal(err)
	}
	all, _ := res["approvals"].([]any)
	if len(all) != 3 {
		t.Fatalf("approvals = %d want 3", len(all))
	}
	// Find a1 → granted by alice.
	var a1 map[string]any
	for _, raw := range all {
		m, _ := raw.(map[string]any)
		if m["approval_id"] == "a1" {
			a1 = m
		}
	}
	if a1 == nil || a1["status"] != "granted" || a1["resolved_by"] != "alice" {
		t.Errorf("a1 = %v want granted by alice", a1)
	}

	// --denied → only a2.
	dres, err := c.Call(context.Background(), controlplane.CmdApprovalsLog,
		map[string]any{"denied": true})
	if err != nil {
		t.Fatal(err)
	}
	dn, _ := dres["approvals"].([]any)
	if len(dn) != 1 {
		t.Fatalf("--denied = %d want 1", len(dn))
	}
	m, _ := dn[0].(map[string]any)
	if m["approval_id"] != "a2" {
		t.Errorf("denied = %v want a2", m["approval_id"])
	}
}

// TestApprovalsStats_Aggregates — `agt approvals stats` counts granted/denied/
// timeout/pending and computes the grant rate over resolved requests (M88).
func TestApprovalsStats_Aggregates(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	reqd := func(id, cap string) {
		k.Bus().Publish(event.Spec{
			Subject: "approval", Kind: event.KindApprovalRequested, Actor: "agent",
			Payload: map[string]any{"approval_id": id, "capability": cap},
		})
	}
	res2 := func(id string, kind event.Kind) {
		k.Bus().Publish(event.Spec{
			Subject: "approval", Kind: kind, Actor: "agent",
			Payload: map[string]any{"approval_id": id},
		})
	}
	reqd("a1", "shell")
	res2("a1", event.KindApprovalGranted)
	reqd("a2", "net")
	res2("a2", event.KindApprovalDenied)
	reqd("a3", "fs")
	res2("a3", event.KindApprovalGranted)
	reqd("a4", "shell") // pending

	res, err := c.Call(context.Background(), controlplane.CmdApprovalsStats, nil)
	if err != nil {
		t.Fatal(err)
	}
	if tot, _ := res["total"].(float64); tot != 4 {
		t.Errorf("total = %v want 4", res["total"])
	}
	if g, _ := res["granted"].(float64); g != 2 {
		t.Errorf("granted = %v want 2", res["granted"])
	}
	if p, _ := res["pending"].(float64); p != 1 {
		t.Errorf("pending = %v want 1", res["pending"])
	}
	// grant rate over resolved (3): 2/3.
	if rate, _ := res["grant_rate"].(float64); rate < 0.66 || rate > 0.67 {
		t.Errorf("grant_rate = %v want ~0.667", rate)
	}
	byCap, _ := res["denied_by_capability"].(map[string]any)
	if n, _ := byCap["net"].(float64); n != 1 {
		t.Errorf("denied_by_capability[net] = %v want 1", byCap["net"])
	}
}

func TestApprovals_PendingIncludesIntentAndEffectMetadata(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	done := make(chan approval.Outcome, 1)
	submitCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		done <- k.Approvals().Submit(submitCtx, approval.SubmitSpec{
			Capability:            "file.delete",
			ToolName:              "file",
			Input:                 `{"path":"legacy/2023"}`,
			Reason:                "confirm exact scope",
			Actor:                 "agent-test",
			CorrelationID:         "corr-test",
			EffectClass:           "irreversible",
			PredictedEffects:      []string{"delete legacy files"},
			AffectedResources:     []string{"path:legacy/2023"},
			RollbackNotes:         "restore from backup",
			Confidence:            0.4,
			CanonicalIntent:       "clean files",
			HarmfulInterpretation: "could delete non-cache files",
			AmbiguityScore:        0.85,
			RegretAxes:            map[string]float64{"informational": 0.9},
			ConfirmationPrompt:    "Confirm exact cleanup scope?",
		})
	}()

	deadline := time.Now().Add(time.Second)
	for k.Approvals().PendingCount() == 0 && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	if k.Approvals().PendingCount() == 0 {
		cancel()
		<-done
		t.Fatal("approval never became pending")
	}
	t.Cleanup(func() {
		if p := k.Approvals().Pending(); len(p) > 0 {
			_ = k.Approvals().Resolve(p[0].ID, approval.DecisionDeny, "cleanup", "test")
			<-done
		}
	})

	res, err := c.Call(context.Background(), controlplane.CmdApprovals, nil)
	if err != nil {
		t.Fatal(err)
	}
	pending, _ := res["pending"].([]any)
	if len(pending) != 1 {
		t.Fatalf("pending = %d want 1", len(pending))
	}
	row, _ := pending[0].(map[string]any)
	if row["canonical_intent"] != "clean files" || row["harmful_interpretation"] == "" {
		t.Fatalf("intent metadata missing: %+v", row)
	}
	if row["effect_class"] != "irreversible" || row["rollback_notes"] != "restore from backup" {
		t.Fatalf("effect metadata missing: %+v", row)
	}
	axes, _ := row["regret_axes"].(map[string]any)
	if axes["informational"] == nil {
		t.Fatalf("regret axes missing: %+v", row["regret_axes"])
	}
	resources, _ := row["affected_resources"].([]any)
	if len(resources) != 1 || resources[0] != "path:legacy/2023" {
		t.Fatalf("affected resources = %+v", row["affected_resources"])
	}
}
