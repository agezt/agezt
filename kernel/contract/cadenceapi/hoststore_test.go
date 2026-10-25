// SPDX-License-Identifier: MIT

package cadenceapi

import (
	"context"
	"reflect"
	"testing"
	"time"
)

// TestTheHostStoreCarriesTheTwentyVerbs: the host's port over the schedule
// library is TWENTY verbs wide — the tool's eleven plus the nine the host
// drives on its own (the control plane retires a schedule, the guardians
// re-arm one, the app layer edits its cadence).
//
// The width is pinned because it IS the contract: the kernel holds this port
// and every holder of it answers these exact verbs. A port that lost a verb
// would leave a holder compiling against a shape that no longer exists; a
// port that grew one would let a holder reach past the host's business.
func TestTheHostStoreCarriesTheTwentyVerbs(t *testing.T) {
	port := reflect.TypeOf((*HostStore)(nil)).Elem()
	if port.NumMethod() != 20 {
		t.Fatalf("the host port has %d verbs, want 20", port.NumMethod())
	}
	want := []string{
		// the tool's surface (Store)
		"Add", "AddDaily", "AddOnce", "AddContinuous", "SetAssure", "SetAgent",
		"SetWorkflowTarget", "SetSystemTaskTarget", "SetToolTarget", "Remove", "List",
		// the host's own surface
		"AddWindow", "Get", "Count", "SetEnabled", "SetIntent", "SetModel",
		"SetIntentTarget", "Reschedule", "RunNow",
	}
	for _, name := range want {
		if _, ok := port.MethodByName(name); !ok {
			t.Errorf("the host port lost the verb %q", name)
		}
	}
}

// TestTheHostPortIsWiderThanTheToolPort: the host's port is a SUPERSET of the
// tool's (Store). The tool may not name a module's implementation, so it
// takes the narrow port; the host drives the wide one; and the module's store
// answers both.
//
// The superset relation is the width rule made mechanical: the two shapes
// cannot drift apart, because the host's port embeds the tool's.
func TestTheHostPortIsWiderThanTheToolPort(t *testing.T) {
	host := reflect.TypeOf((*HostStore)(nil)).Elem()
	tool := reflect.TypeOf((*Store)(nil)).Elem()
	if tool.NumMethod() != 11 {
		t.Fatalf("the tool port has %d verbs, want 11", tool.NumMethod())
	}
	for i := 0; i < tool.NumMethod(); i++ {
		m := tool.Method(i)
		if _, ok := host.MethodByName(m.Name); !ok {
			t.Errorf("the host port does not carry the tool's verb %q", m.Name)
		}
	}
}

// TestTheEnginePortCarriesTheFourVerbs: the live cadence resident's port is
// FOUR verbs: start it, wait for the in-flight runs, wait until it is idle,
// and ask how many runs are in flight.
//
// The port is deliberately small — the resident's own firing logic belongs to
// the module, and the host only reads its state.
func TestTheEnginePortCarriesTheFourVerbs(t *testing.T) {
	port := reflect.TypeOf((*Engine)(nil)).Elem()
	if port.NumMethod() != 4 {
		t.Fatalf("the engine port has %d verbs, want 4", port.NumMethod())
	}
	for _, name := range []string{"Start", "Wait", "WaitIdle", "RunningCount"} {
		if _, ok := port.MethodByName(name); !ok {
			t.Errorf("the engine port lost the verb %q", name)
		}
	}
}

// TestTheEnginePortCarriesTheContractRows: the port speaks the contract's
// rows — the entry, the time, the context. The resident is a module's
// implementation, and the host hands it over as a port, so the two must
// speak the same shapes.
func TestTheEnginePortCarriesTheContractRows(t *testing.T) {
	typ := reflect.TypeOf((*Engine)(nil)).Elem()
	want := map[string]reflect.Type{
		"Start":        reflect.TypeOf(func(context.Context) {}),
		"Wait":         reflect.TypeOf(func() {}),
		"WaitIdle":     reflect.TypeOf(func(context.Context) error { return nil }),
		"RunningCount": reflect.TypeOf(func() int { return 0 }),
	}
	for i := 0; i < typ.NumMethod(); i++ {
		m := typ.Method(i)
		wantType, ok := want[m.Name]
		if !ok {
			t.Errorf("the engine port grew an unexpected verb %q", m.Name)
			continue
		}
		if m.Type != wantType {
			t.Errorf("the %s verb is %s, want %s", m.Name, m.Type, wantType)
		}
		delete(want, m.Name)
	}
	for name := range want {
		t.Errorf("the engine port lost the verb %q", name)
	}
}

// TestTheRowsAreTheContracts: the schedule row and its helpers live in the
// contract layer, so the module and the host read the same shape.
func TestTheRowsAreTheContracts(t *testing.T) {
	var _ Entry
	var _ time.Time
	if reflect.TypeOf(Entry{}).Name() != "Entry" {
		t.Fatalf("the schedule row must be named Entry, got %v", reflect.TypeOf(Entry{}).Name())
	}
}
