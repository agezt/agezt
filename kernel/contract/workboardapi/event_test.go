// SPDX-License-Identifier: MIT

package workboardapi

import (
	"strings"
	"testing"
)

// The board's ANSWERS both sides name.
//
// The refusal and the payload moved here because the host and the module
// have to be able to say the same thing without either naming the other: the
// host may not name the module, and the module may not name the host.

// The refusal is the contract's row.
//
// A daemon with no board answers no board, and the answer both sides hold is
// this one.
func TestTheNoStoreRefusalIsTheContracts(t *testing.T) {
	if ErrNoStore == nil {
		t.Fatal("the contract must carry the no-store refusal")
	}
	if !strings.Contains(ErrNoStore.Error(), "not available") {
		t.Errorf("the refusal says %q, want it to say the board is not available", ErrNoStore.Error())
	}
}

// The payload carries the task and the action.
//
// It is the shape the tools read, so it is a row: the board publishes it, the
// daemon's proof path publishes it, and both spell it the same way.
func TestTheTaskEventPayloadCarriesTheTaskAndTheAction(t *testing.T) {
	task := Task{ID: "task-1", Title: "owned", Status: StatusReady, Priority: 2, Assignee: "agent", Tenant: "acme"}
	payload := TaskEventPayload(task, "claimed", map[string]any{"agent": "agent", "run_id": "run-1"})
	if payload["id"] != "task-1" {
		t.Errorf("id = %v, want %q", payload["id"], "task-1")
	}
	if payload["title"] != "owned" {
		t.Errorf("title = %v, want %q", payload["title"], "owned")
	}
	if payload["status"] != string(StatusReady) {
		t.Errorf("status = %v, want %q", payload["status"], string(StatusReady))
	}
	if payload["priority"] != 2 {
		t.Errorf("priority = %v, want 2", payload["priority"])
	}
	if payload["assignee"] != "agent" {
		t.Errorf("assignee = %v, want %q", payload["assignee"], "agent")
	}
	if payload["tenant"] != "acme" {
		t.Errorf("tenant = %v, want %q", payload["tenant"], "acme")
	}
	if payload["action"] != "claimed" {
		t.Errorf("action = %v, want %q", payload["action"], "claimed")
	}
	if payload["agent"] != "agent" {
		t.Errorf("agent = %v, want %q", payload["agent"], "agent")
	}
	if payload["run_id"] != "run-1" {
		t.Errorf("run_id = %v, want %q", payload["run_id"], "run-1")
	}
}

// The payload says what is, not what is blank.
//
// An extra that carries nothing is left out: a payload full of empty fields
// says nothing at all.
func TestTheTaskEventPayloadDropsTheBlankExtras(t *testing.T) {
	payload := TaskEventPayload(Task{ID: "task-2"}, "blocked", map[string]any{"actor": "", "reason": "stuck"})
	if _, ok := payload["actor"]; ok {
		t.Error("a blank extra must not reach the payload")
	}
	if payload["reason"] != "stuck" {
		t.Errorf("reason = %v, want %q", payload["reason"], "stuck")
	}
}
