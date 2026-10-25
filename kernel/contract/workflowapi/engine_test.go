// SPDX-License-Identifier: MIT

package workflowapi

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/approvalapi"
	"github.com/agezt/agezt/kernel/contract/eventapi"
	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/toolapi"
)

// TestTheEnginePortCarriesTheEightVerbs: the engine port is EIGHT verbs wide —
// the durable CRUD (Save, Restore, SetEnabled, Remove), the graph run, the
// single-node test and the copilot's two moves (Draft, Refine).
//
// The width is the port's contract with every holder, and it is deliberately
// NOT the plugin's: the read port (`Store`) stays exactly two verbs wide,
// because a plugin that could remove or disable a stored workflow would be
// editing the daemon's library. The host drives the whole lifecycle, so the
// host's port carries all eight.
func TestTheEnginePortCarriesTheEightVerbs(t *testing.T) {
	port := reflect.TypeOf((*Engine)(nil)).Elem()
	if port.NumMethod() != 8 {
		t.Fatalf("the engine port has %d verbs, want 8", port.NumMethod())
	}
	want := map[string]bool{
		"Save":       true,
		"Restore":    true,
		"SetEnabled": true,
		"Remove":     true,
		"Run":        true,
		"TestNode":   true,
		"Draft":      true,
		"Refine":     true,
	}
	for i := 0; i < port.NumMethod(); i++ {
		name := port.Method(i).Name
		if !want[name] {
			t.Errorf("the engine port grew an unexpected verb %q", name)
		}
		delete(want, name)
	}
	for name := range want {
		t.Errorf("the engine port lost the verb %q", name)
	}
}

// TestTheReadPortIsExactlyTwoVerbs: the library's READ port is the plugin's
// port, and it is exactly two verbs wide — Get and List.
//
// This is the width rule of the whole slice: the port is as wide as the
// WEAKEST carrier may hold. A plugin that could save, remove or disable a
// stored workflow would be editing the daemon's library, so the read port
// stays a reader.
func TestTheReadPortIsExactlyTwoVerbs(t *testing.T) {
	port := reflect.TypeOf((*Store)(nil)).Elem()
	if port.NumMethod() != 2 {
		t.Fatalf("the read port has %d verbs, want 2", port.NumMethod())
	}
	for _, name := range []string{"Get", "List"} {
		if _, ok := port.MethodByName(name); !ok {
			t.Errorf("the read port lost the verb %q", name)
		}
	}
}

// TestTheHostStoreIsWiderThanTheReadPort: the host store carries the read side
// and the four lifecycle verbs the host drives. A host store that DROPPED the
// read verbs would leave the plugin's port and the host's port unrelated, and
// the two shapes would drift.
func TestTheHostStoreIsWiderThanTheReadPort(t *testing.T) {
	host := reflect.TypeOf((*HostStore)(nil)).Elem()
	if host.NumMethod() != 6 {
		t.Fatalf("the host store has %d verbs, want 6", host.NumMethod())
	}
	read := reflect.TypeOf((*Store)(nil)).Elem()
	for i := 0; i < read.NumMethod(); i++ {
		m := read.Method(i)
		if _, ok := host.MethodByName(m.Name); !ok {
			t.Errorf("the host store does not carry the read verb %q", m.Name)
		}
	}
	for _, name := range []string{"Save", "Restore", "SetEnabled", "Remove"} {
		if _, ok := host.MethodByName(name); !ok {
			t.Errorf("the host store lost the lifecycle verb %q", name)
		}
	}
}

// TestTheHostPortsCarryTheTenCapabilities: the engine calls out to exactly ten
// host capabilities, and every one of them is the host's OWN governed path —
// the module holds no provider, no tool registry and no bus of its own, so a
// workflow run keeps the host's policy checks, its audit arc and its spend
// attribution.
//
// The set is pinned here because it is the shape of the host's surface: a port
// that lost a capability would refuse the node that needs it, and a port that
// grew one would let the module reach past the host's governance.
func TestTheHostPortsCarryTheTenCapabilities(t *testing.T) {
	typ := reflect.TypeOf(Ports{})
	if typ.NumField() != 10 {
		t.Fatalf("the host ports carry %d capabilities, want 10", typ.NumField())
	}
	want := map[string]reflect.Type{
		"Publish": reflect.TypeOf(func(eventapi.Spec) {}),
		"Complete": reflect.TypeOf(func(context.Context, string, string, llm.CompletionRequest) (*llm.CompletionResponse, error) {
			return nil, nil
		}),
		"InvokeTool": reflect.TypeOf(func(context.Context, string, string, string, json.RawMessage) (toolapi.Result, error) {
			return toolapi.Result{}, nil
		}),
		"RunCode": reflect.TypeOf(func(context.Context, string, string, string, string, string) (toolapi.Result, error) {
			return toolapi.Result{}, nil
		}),
		"ScriptRunnerReady": reflect.TypeOf(func() bool { return false }),
		"Approve":           reflect.TypeOf(func(context.Context, approvalapi.SubmitSpec) approvalapi.Outcome { return approvalapi.Outcome{} }),
		"Model":             reflect.TypeOf(func(context.Context) string { return "" }),
		"DefaultModel":      reflect.TypeOf(func(context.Context) string { return "" }),
		"HasProvider":       reflect.TypeOf(func() bool { return false }),
		"Halted":            reflect.TypeOf(func() bool { return false }),
	}
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		wantType, ok := want[f.Name]
		if !ok {
			t.Errorf("the host ports grew an unexpected capability %q", f.Name)
			continue
		}
		if f.Type != wantType {
			t.Errorf("the %s capability is %s, want %s", f.Name, f.Type, wantType)
		}
		delete(want, f.Name)
	}
	for name := range want {
		t.Errorf("the host ports lost the capability %q", name)
	}
}

// TestTheRowsTheHostPortsCarryAreTheContracts: the capability rows travel
// with the port — the event spec, the approval request and verdict, the
// completion and the tool result. They are the CONTRACT'S rows, so the module
// and the host speak the same shapes.
func TestTheRowsTheHostPortsCarryAreTheContracts(t *testing.T) {
	typ := reflect.TypeOf(Ports{})
	if got := typ.Field(0).Type.String(); got != "func(eventapi.Spec)" {
		t.Fatalf("the publish capability carries %s", got)
	}
	var _ eventapi.Spec
	var _ approvalapi.SubmitSpec
	var _ approvalapi.Outcome
	var _ llm.CompletionRequest
	var _ toolapi.Result
}

// TestTheProvenanceRowCarriesTheWakeContext: the provenance is WHY a run
// exists, and the host fills it in from its own wake context — the module
// never reads another package's context keys.
//
// The row is pinned field by field: a dropped field would silently lose the
// schedule id or the parent correlation, and the run would be journalled
// without the reason it existed.
func TestTheProvenanceRowCarriesTheWakeContext(t *testing.T) {
	typ := reflect.TypeOf(Provenance{})
	want := []string{
		"Source",
		"Reason",
		"ScheduleID",
		"StandingID",
		"StandingName",
		"TriggerSubject",
		"ParentCorrelation",
	}
	if typ.NumField() != len(want) {
		t.Fatalf("the provenance carries %d fields, want %d", typ.NumField(), len(want))
	}
	for i, name := range want {
		if got := typ.Field(i).Name; got != name {
			t.Errorf("provenance field %d = %q, want %q", i, got, name)
		}
	}
}

// TestTheTriggerRowCarriesWhatFired: the trigger says what fired a workflow:
// the kind, the matched subject and a human label. The host's trigger runner
// carries it, and the host may not name a module's row.
func TestTheTriggerRowCarriesWhatFired(t *testing.T) {
	typ := reflect.TypeOf(Trigger{})
	want := []string{"Source", "Subject", "Reason"}
	if typ.NumField() != len(want) {
		t.Fatalf("the trigger carries %d fields, want %d", typ.NumField(), len(want))
	}
	for i, name := range want {
		if got := typ.Field(i).Name; got != name {
			t.Errorf("trigger field %d = %q, want %q", i, got, name)
		}
	}
}

// TestTheHaltSentinelIsTheContracts: the halt is the HOST's report (the
// kernel is stopped), so the sentinel lives in the contract layer and the
// host maps it to its own. A second spelling would make the host's halt
// invisible to the engine.
func TestTheHaltSentinelIsTheContracts(t *testing.T) {
	if ErrHalted == nil {
		t.Fatal("the halt sentinel is missing")
	}
	if !errors.Is(ErrHalted, ErrHalted) {
		t.Fatal("the halt sentinel does not compare to itself")
	}
}

// TestTheEnginePortIsImplementableByAPlainStruct: the port is an ordinary
// interface — a test double can satisfy it without naming a module. That is
// the point of a port: the host names the SHAPE, not the implementation.
func TestTheEnginePortIsImplementableByAPlainStruct(t *testing.T) {
	var _ Engine = fakeEngine{}
}

// fakeEngine is a plain struct that satisfies the engine port. It exists to
// prove the port carries no module's types beyond the contract's rows.
type fakeEngine struct{}

func (fakeEngine) Save(string, Workflow) (Workflow, bool, error) { return Workflow{}, false, nil }
func (fakeEngine) Restore(string, Workflow, string) (Workflow, bool, error) {
	return Workflow{}, false, nil
}
func (fakeEngine) SetEnabled(string, string, bool) (Workflow, error) { return Workflow{}, nil }
func (fakeEngine) Remove(string, string) (bool, error)               { return false, nil }
func (fakeEngine) Run(context.Context, string, string, any, Provenance) (RunResult, error) {
	return RunResult{}, nil
}
func (fakeEngine) TestNode(context.Context, string, Workflow, string, map[string]any, any) (NodeTestResult, error) {
	return NodeTestResult{}, nil
}
func (fakeEngine) Draft(context.Context, string, string, string) (Workflow, error) {
	return Workflow{}, nil
}
func (fakeEngine) Refine(context.Context, string, Workflow, string) (Workflow, error) {
	return Workflow{}, nil
}

// compile-time proof that the rows the ports carry stay typed
var (
	_ = json.RawMessage{}
)
