// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"testing"

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
