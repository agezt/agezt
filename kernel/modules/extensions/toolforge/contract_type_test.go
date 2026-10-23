// SPDX-License-Identifier: MIT

package toolforge

import (
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/toolforgeapi"
)

// TestTheRowsAreTheContractRows: every row the module keeps is the
// CONTRACT'S row, not a copy of it. A copy would drift — the module would
// then keep a shape the contract no longer describes, and the agent-facing
// tool (which speaks the contract) would no longer fit it.
func TestTheRowsAreTheContractRows(t *testing.T) {
	for _, c := range []struct {
		name     string
		module   any
		contract any
	}{
		{"ScriptTool", ScriptTool{}, toolforgeapi.ScriptTool{}},
	} {
		if reflect.TypeOf(c.module) != reflect.TypeOf(c.contract) {
			t.Errorf("the %s row drifted from the contract: %v vs %v",
				c.name, reflect.TypeOf(c.module), reflect.TypeOf(c.contract))
		}
	}
	if reflect.TypeOf(StatusDraft) != reflect.TypeOf(toolforgeapi.StatusDraft) {
		t.Errorf("the Status row drifted from the contract: %v vs %v",
			reflect.TypeOf(StatusDraft), reflect.TypeOf(toolforgeapi.StatusDraft))
	}
}

// TestTheStatusesAreTheContracts: the lifecycle values are wire, so the
// module and the contract must spell them the same way.
func TestTheStatusesAreTheContracts(t *testing.T) {
	if StatusDraft != toolforgeapi.StatusDraft ||
		StatusActive != toolforgeapi.StatusActive ||
		StatusQuarantined != toolforgeapi.StatusQuarantined {
		t.Errorf("the lifecycle values drifted: %q %q %q",
			StatusDraft, StatusActive, StatusQuarantined)
	}
}

// TestTheRuleIsTheContracts: the draft gate is ONE rule — the contract's.
// The module delegates it, so a second copy cannot drift. This test compares
// the two verdicts on every kind of input, message included: a rule that
// refuses for the wrong reason is a rule nobody can debug.
func TestTheRuleIsTheContracts(t *testing.T) {
	cases := []ScriptTool{
		{Name: "fetch_weather", Description: "fetches it", Language: "python", Code: "print(1)"},
		{Name: "", Description: "d", Language: "python", Code: "c"},
		{Name: "Bad", Description: "d", Language: "python", Code: "c"},
		{Name: "a", Description: "", Language: "python", Code: "c"},
		{Name: "a", Description: "d", Language: "", Code: "c"},
		{Name: "a", Description: "d", Language: "python", Code: ""},
		{Name: "a", Description: "d", Language: "python", Code: "c", InputSchema: "{oops"},
		{Name: "a", Description: "d", Language: "python", Code: "c", InputSchema: `{"type":"object"}`},
	}
	for _, st := range cases {
		gotErr, wantErr := Validate(st), toolforgeapi.Validate(st)
		if (gotErr == nil) != (wantErr == nil) {
			t.Errorf("the module and the contract disagree on %+v: %v vs %v", st, gotErr, wantErr)
			continue
		}
		if gotErr != nil && gotErr.Error() != wantErr.Error() {
			t.Errorf("the module's error %q differs from the contract's %q", gotErr, wantErr)
		}
	}
}

// TestTheCapsAreTheContracts: the size limits the module enforces are the
// contract's — a module that kept its own numbers would refuse a tool the
// contract accepts.
func TestTheCapsAreTheContracts(t *testing.T) {
	if maxCodeBytes != toolforgeapi.MaxCodeBytes ||
		maxDescBytes != toolforgeapi.MaxDescBytes ||
		maxSchemaBytes != toolforgeapi.MaxSchemaBytes {
		t.Errorf("the caps drifted: %d %d %d",
			maxCodeBytes, maxDescBytes, maxSchemaBytes)
	}
}

// TestTheStoreIsTheContractsPort: the module's store answers the contract's
// read port, so the agent-facing tool can be handed the store without ever
// naming this module.
func TestTheStoreIsTheContractsPort(t *testing.T) {
	var _ toolforgeapi.Store = (*Store)(nil)
}
