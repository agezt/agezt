// SPDX-License-Identifier: MIT

package okrapi

import (
	"reflect"
	"testing"
)

// TestTheStorePortIsExactlyNineVerbsWide: the store port is what the layers
// ABOVE the module drive — nine verbs, no more.
//
// The width is pinned because a WIDER port would let a caller reach past the
// library's own rules (the store validates, assigns ids and protects the
// linked-task set), and a NARROWER one would force a second shape to be kept
// in step with this one.
func TestTheStorePortIsExactlyNineVerbsWide(t *testing.T) {
	typ := reflect.TypeOf((*Store)(nil)).Elem()
	if typ.Kind() != reflect.Interface {
		t.Fatalf("Store must be an interface, got %v", typ.Kind())
	}
	if typ.NumMethod() != 9 {
		t.Fatalf("the port has %d methods, want 9", typ.NumMethod())
	}
	for _, name := range []string{"Create", "Get", "List", "AddKeyResult", "LinkTask", "UnlinkTask", "SetStatus", "Archive", "ObjectivesForTask"} {
		if _, ok := typ.MethodByName(name); !ok {
			t.Errorf("the port must have a %s method", name)
		}
	}
}

// TestTheStorePortNamesOnlyContractRows: every type the port names must be
// the contract's row or a builtin. A module's type in a port's signature would
// drag that module into every layer that holds the port.
func TestTheStorePortNamesOnlyContractRows(t *testing.T) {
	typ := reflect.TypeOf((*Store)(nil)).Elem()
	objective := reflect.TypeOf(Objective{})
	spec := reflect.TypeOf(CreateSpec{})
	filter := reflect.TypeOf(Filter{})
	status := reflect.TypeOf(StatusActive)
	errType := reflect.TypeOf((*error)(nil)).Elem()
	str := reflect.TypeOf("")
	boolean := reflect.TypeOf(false)
	intType := reflect.TypeOf(0)
	int64Type := reflect.TypeOf(int64(0))
	slice := reflect.TypeOf([]Objective{})

	check := func(name string, in, out []reflect.Type) {
		m, ok := typ.MethodByName(name)
		if !ok {
			t.Fatalf("the port must have a %s method", name)
		}
		if m.Type.NumIn() != len(in) {
			t.Errorf("%s takes %d arguments, want %d", name, m.Type.NumIn(), len(in))
			return
		}
		for i := range in {
			if m.Type.In(i) != in[i] {
				t.Errorf("%s argument %d is %v, want %v", name, i, m.Type.In(i), in[i])
			}
		}
		if m.Type.NumOut() != len(out) {
			t.Errorf("%s answers %d values, want %d", name, m.Type.NumOut(), len(out))
			return
		}
		for i := range out {
			if m.Type.Out(i) != out[i] {
				t.Errorf("%s answer %d is %v, want %v", name, i, m.Type.Out(i), out[i])
			}
		}
	}

	check("Create", []reflect.Type{spec, int64Type}, []reflect.Type{objective, errType})
	check("Get", []reflect.Type{str}, []reflect.Type{objective, boolean})
	check("List", []reflect.Type{filter}, []reflect.Type{slice})
	check("AddKeyResult", []reflect.Type{str, str, intType, int64Type}, []reflect.Type{objective, errType})
	check("LinkTask", []reflect.Type{str, str, str, int64Type}, []reflect.Type{objective, errType})
	check("UnlinkTask", []reflect.Type{str, str, str, int64Type}, []reflect.Type{objective, errType})
	check("SetStatus", []reflect.Type{str, status, int64Type}, []reflect.Type{objective, errType})
	check("Archive", []reflect.Type{str, int64Type}, []reflect.Type{objective, errType})
	check("ObjectivesForTask", []reflect.Type{str}, []reflect.Type{reflect.TypeOf([]string{})})
}

// TestThePortCarriesNoClock: the port takes the CALLER's clock (nowMS) — it
// must not read one itself.
//
// A port that reads the clock cannot be tested deterministically, and the
// contract layer may not name one (archcheck's principle P4).
func TestThePortCarriesNoClock(t *testing.T) {
	typ := reflect.TypeOf((*Store)(nil)).Elem()
	for i := 0; i < typ.NumMethod(); i++ {
		m := typ.Method(i)
		for j := 0; j < m.Type.NumIn(); j++ {
			if m.Type.In(j).String() == "time.Time" {
				t.Errorf("%s must not take a time.Time — the clock is the caller's", m.Name)
			}
		}
	}
}

// TestTheSentinelsAreTheContracts: the errors the layers above the module
// compare against must be the contract's, so the module never has to be named
// to answer "does this objective exist?".
func TestTheSentinelsAreTheContracts(t *testing.T) {
	if ErrNotFound == nil || ErrKRNotFound == nil || ErrInvalidStatus == nil {
		t.Fatal("the contract must own the sentinels")
	}
	if ErrNotFound.Error() != "okr: not found" {
		t.Fatalf("the not-found sentinel reads %q", ErrNotFound.Error())
	}
	if ErrKRNotFound.Error() != "okr: key result not found" {
		t.Fatalf("the key-result sentinel reads %q", ErrKRNotFound.Error())
	}
	if ErrInvalidStatus.Error() != "okr: invalid status" {
		t.Fatalf("the invalid-status sentinel reads %q", ErrInvalidStatus.Error())
	}
}

// TestTheRollupRuleTravelsWithTheRow: the RULE that decides whether a goal
// counts as met lives with the ROW.
//
// A Go type alias cannot carry methods, so a rule that stayed in the module
// could not be reached by the layers above it — and this rule is exactly what
// decides whether an objective is achieved. The check is on the CONTRACT's
// type: the method has to be there, and it has to be right.
func TestTheRollupRuleTravelsWithTheRow(t *testing.T) {
	o := Objective{
		ID: "o-1",
		KeyResults: []KeyResult{
			{ID: "kr-1", Title: "ship", Target: 2, TaskIDs: []string{"t-1", "t-2"}},
			{ID: "kr-2", Title: "measure", Target: 0, TaskIDs: []string{"t-3"}},
		},
	}
	done := map[string]bool{"t-1": true, "t-3": true}
	p := o.Progress(func(id string) bool { return done[id] })

	if p.ObjectiveID != "o-1" {
		t.Fatalf("the rollup lost the objective id: %q", p.ObjectiveID)
	}
	if len(p.KeyResults) != 2 {
		t.Fatalf("the rollup has %d key results, want 2", len(p.KeyResults))
	}
	// kr-1: 1 of 2 done -> 50%, not achieved.
	if p.KeyResults[0].Done != 1 || p.KeyResults[0].Total != 2 || p.KeyResults[0].Percent != 50 || p.KeyResults[0].Achieved {
		t.Fatalf("the first key result rolled up wrong: %+v", p.KeyResults[0])
	}
	// kr-2: no explicit target -> the linked-task count is the target; 1 of 1
	// done -> achieved.
	if p.KeyResults[1].Target != 1 || p.KeyResults[1].Percent != 100 || !p.KeyResults[1].Achieved {
		t.Fatalf("the second key result rolled up wrong: %+v", p.KeyResults[1])
	}
	if p.Achieved {
		t.Fatal("an objective with one unmet key result is not achieved")
	}
	if p.Percent != 75 {
		t.Fatalf("the objective rolled up to %d%%, want 75%%", p.Percent)
	}

	// Every key result met -> the objective is achieved.
	done["t-2"] = true
	if p = o.Progress(func(id string) bool { return done[id] }); !p.Achieved || p.Percent != 100 {
		t.Fatalf("a fully met objective rolled up to %+v", p)
	}

	// An objective with no key results is never achieved: there is nothing to
	// measure, and "no goals" must not read as "all goals met".
	empty := Objective{ID: "o-2"}
	if p = empty.Progress(func(string) bool { return true }); p.Achieved {
		t.Fatal("an objective with no key results must not be achieved")
	}
}
