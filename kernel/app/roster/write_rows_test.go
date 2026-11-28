// SPDX-License-Identifier: MIT

package roster

// The roster's write half has its request rows in the contract
// (kernel/contract/opapi): the agent's ref is a text, a patch field knows
// whether it was sent at all, a list of texts is a list and nothing else, and a
// count is an integer.
//
// The family's own rules stay in the family: a field that was not sent is left
// alone, a field that was sent is patched even when it was sent empty, the
// task update reads presence rather than value, and the enabled flag keeps the
// lenient spelling it has always had.
//
// The rows are read through the daemon's door, so a mistyped field is refused
// by the door with its own wording, before any effect — and a field that was
// not sent is not the same answer as a field that was sent as empty.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	core "github.com/agezt/agezt/kernel/modules/agents/roster"
)

// TestTheRosterWriteRowsCarryTheFamiliesOwnRules says the write half's patch
// rules stay in the family: an unsent field is left alone, a sent field is
// patched even when it is empty, the task update reads presence rather than
// value, and the enabled flag keeps its lenient spelling.
func TestTheRosterWriteRowsCarryTheFamiliesOwnRules(t *testing.T) {
	// The capability patch: a field that was not sent is left alone, and a
	// field that was sent is patched — even when it was sent empty.
	f := newPermFixture()
	f.profiles["a"] = core.Profile{Slug: "a", Enabled: true, TrustCeiling: "L2", Workdir: "w"}
	ops, err := PermissionOperations(func(context.Context) *PermissionService { return f.service() })
	if err != nil {
		t.Fatal(err)
	}
	d, err := app.NewDispatcher(ops, app.Dependencies{Auth: rosterAuth{opapi.Principal{Kind: opapi.Operator}}, Router: rosterRoute{}, Audit: &recordingAudit{}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Dispatch(context.Background(), opapi.Caller{}, "agent_capabilities", json.RawMessage(`{"ref":"a","trust_ceiling":""}`), nil); err != nil {
		t.Fatal(err)
	}
	got := f.profiles["a"]
	if got.TrustCeiling != "" || got.Workdir != "w" {
		t.Fatalf("a sent field is patched; an unsent field stays: %+v", got)
	}
	// A mistyped field is a usage error the door names — before any effect.
	writes := f.updates
	for _, raw := range []string{
		`{"ref":"a","trust_ceiling":3}`,
		`{"ref":"a","tool_allow":"x"}`,
		`{"ref":"a","tool_deny":[1]}`,
		`{"ref":"a","max_cost_mc":"many"}`,
	} {
		if _, err := d.Dispatch(context.Background(), opapi.Caller{}, "agent_capabilities", json.RawMessage(raw), nil); err == nil {
			t.Fatalf("a mistyped field was accepted %s", raw)
		} else if !strings.Contains(err.Error(), "has wrong type") {
			t.Fatalf("%s: %v", raw, err)
		}
	}
	if f.updates != writes {
		t.Fatal("a refused body wrote")
	}
	// The enabled flag keeps its lenient spelling: a browser's query carries
	// "true", not true.
	enabled, calls := setEnabledFixture(nil)
	enabledOps, err := SetEnabledOperations(func(context.Context) *SetEnabledService { return enabled })
	if err != nil {
		t.Fatal(err)
	}
	ed, err := app.NewDispatcher(enabledOps, app.Dependencies{Auth: rosterAuth{opapi.Principal{Kind: opapi.Operator}}, Router: rosterRoute{}, Audit: &recordingAudit{}})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		raw  string
		want string
	}{
		{`{"ref":"a","enabled":true}`, "a=true"},
		{`{"ref":"a","enabled":"TRUE"}`, "a=true"},
		{`{"ref":"a","enabled":"1"}`, "a=true"},
		{`{"ref":"a","enabled":"yes"}`, "a=false"},
		{`{"ref":"a"}`, "a=false"},
	} {
		calls.sets = nil
		if _, err := ed.Dispatch(context.Background(), opapi.Caller{}, "agent_set_enabled", json.RawMessage(tc.raw), nil); err != nil {
			t.Fatalf("%s: %v", tc.raw, err)
		}
		if len(calls.sets) != 1 || calls.sets[0] != tc.want {
			t.Fatalf("%s set %v, want %s", tc.raw, calls.sets, tc.want)
		}
	}
	// The task update reads presence, not value: a sent flat field clears the
	// task's field, and an unsent one leaves it alone.
	tasks, store, taskCalls := taskUpdateFixture(t)
	taskOps, err := TaskUpdateOperations(func(context.Context) *TaskUpdateService { return tasks })
	if err != nil {
		t.Fatal(err)
	}
	td, err := app.NewDispatcher(taskOps, app.Dependencies{Auth: rosterAuth{opapi.Principal{Kind: opapi.Operator}}, Router: rosterRoute{}, Audit: &recordingAudit{}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := td.Dispatch(context.Background(), opapi.Caller{}, "agent_task_update", json.RawMessage(`{"ref":"ops","op":"add","title":" check ","description":" d ","scope":"cycle"}`), nil); err != nil {
		t.Fatal(err)
	}
	stored, _ := store.Get("ops")
	if len(stored.TaskList) != 1 {
		t.Fatalf("the task was not stored: %+v", stored.TaskList)
	}
	id := stored.TaskList[0].ID
	if _, err := td.Dispatch(context.Background(), opapi.Caller{}, "agent_task_update", json.RawMessage(`{"ref":"ops","id":"`+id+`","description":""}`), nil); err != nil {
		t.Fatal(err)
	}
	stored, _ = store.Get("ops")
	if stored.TaskList[0].Description != "" || stored.TaskList[0].Scope != "cycle" {
		t.Fatalf("a sent field clears; an unsent field stays: %+v", stored.TaskList[0])
	}
	if len(taskCalls.refs) != 2 {
		t.Fatalf("both writes went through: %v", taskCalls.refs)
	}
	// A mistyped task or a mistyped flat field is refused by the door — before
	// any effect.
	writes = len(taskCalls.refs)
	for _, raw := range []string{
		`{"ref":"ops","op":"add","title":3}`,
		`{"ref":"ops","op":"add","title":"t","task":["nope"]}`,
		`{"ref":"ops","op":"add","title":"t","task":null}`,
	} {
		if _, err := td.Dispatch(context.Background(), opapi.Caller{}, "agent_task_update", json.RawMessage(raw), nil); err == nil {
			t.Fatalf("a mistyped field was accepted %s", raw)
		} else if !strings.Contains(err.Error(), "has wrong type") {
			t.Fatalf("%s: %v", raw, err)
		}
	}
	if len(taskCalls.refs) != writes {
		t.Fatal("a refused body wrote")
	}
}
