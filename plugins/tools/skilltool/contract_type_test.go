// SPDX-License-Identifier: MIT

package skilltool

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/skillapi"
)

// TestTheToolSpeaksTheContractRows: the tool's rows ARE the contract's rows.
// A copy would drift — the tool would publish a shape the contract no longer
// describes, and the daemon's Forge (which builds the contract's row) would
// no longer fit the tool.
func TestTheToolSpeaksTheContractRows(t *testing.T) {
	// The tool's own functions take the contract's row.
	if got := reflect.TypeOf(skillView); got.In(0) != reflect.TypeOf(skillapi.Skill{}) {
		t.Fatalf("the tool's view takes %v, want the contract's row", got.In(0))
	}
	if got := reflect.TypeOf(okEntry); got.In(1) != reflect.TypeOf(skillapi.Skill{}) {
		t.Fatalf("the tool's entry takes %v, want the contract's row", got.In(1))
	}
}

// TestTheToolIsBoundThroughTheContractPorts: the tool's Bind takes the
// contract's two ports — the Forge and the bundle store — so the daemon can
// hand it the implementations without the tool naming a module.
func TestTheToolIsBoundThroughTheContractPorts(t *testing.T) {
	bt := reflect.TypeOf((*Tool).Bind)
	if bt.NumIn() != 3 { // the receiver plus two ports
		t.Fatalf("Bind takes %d arguments, want the receiver and two ports", bt.NumIn()-1)
	}
	if bt.In(1) != reflect.TypeOf((*skillapi.Forge)(nil)).Elem() {
		t.Fatalf("Bind's first port is %v, want the contract's forge port", bt.In(1))
	}
	if bt.In(2) != reflect.TypeOf((*skillapi.BundleStore)(nil)).Elem() {
		t.Fatalf("Bind's second port is %v, want the contract's bundle port", bt.In(2))
	}
}

// TestTheSkillViewWireShape: the view the agent reads is the tool's own wire,
// and the agent acts on it — the shape is pinned key for key.
func TestTheSkillViewWireShape(t *testing.T) {
	v := skillView(skillapi.Skill{
		ID: "0123456789abcdef", Name: "diagnose-ci", Description: "when CI fails",
		Status: skillapi.StatusActive, Version: "1.0.0",
		Metrics: skillapi.Metrics{Uses: 3},
	})
	for _, k := range []string{"id", "name", "status", "version", "uses"} {
		if _, ok := v[k]; !ok {
			t.Errorf("the skill view is missing %q: %+v", k, v)
		}
	}
	if v["id"] != "0123456789ab" {
		t.Errorf("the view's id = %v, want the short id", v["id"])
	}
	if v["status"] != "active" {
		t.Errorf("the view's status = %v, want active", v["status"])
	}
	if v["uses"] != 3 {
		t.Errorf("the view's uses = %v, want 3", v["uses"])
	}
	// The optional fields stay off the wire when they are empty.
	bare := skillView(skillapi.Skill{ID: "0123456789abcdef", Name: "n"})
	if _, ok := bare["description"]; ok {
		t.Errorf("an empty description must stay off the wire: %+v", bare)
	}
	if _, ok := bare["triggers"]; ok {
		t.Errorf("an empty trigger list must stay off the wire: %+v", bare)
	}
}

// TestTheToolPublishesTheContractsRow: the tool publishes what the Forge
// said — the row the contract describes, with the runtime's own values.
func TestTheToolPublishesTheContractsRow(t *testing.T) {
	f := newFake()
	tool := newTool(f)
	out, isErr := invoke(t, tool, map[string]any{
		"op": "learn", "name": "x", "description": "d", "body": "b",
	})
	if isErr {
		t.Fatalf("learn errored: %+v", out)
	}
	if out["status"] != string(skillapi.StatusDraft) {
		t.Errorf("the published status = %v, want %q", out["status"], skillapi.StatusDraft)
	}
	if out["version"] != skillapi.DefaultVersion {
		t.Errorf("the published version = %v, want %q", out["version"], skillapi.DefaultVersion)
	}
}

// TestTheUnboundToolSaysSo: a tool with no Forge must report that rather
// than dereference nil.
func TestTheUnboundToolSaysSo(t *testing.T) {
	res, err := New().Invoke(context.Background(), json.RawMessage(`{"op":"list"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Fatalf("an unbound tool must report that, got: %s", res.Output)
	}
	if !strings.Contains(res.Output, "not available") {
		t.Fatalf("the unbound tool must say it is not available: %s", res.Output)
	}
}
