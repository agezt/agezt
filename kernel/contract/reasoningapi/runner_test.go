// SPDX-License-Identifier: MIT

package reasoningapi

import (
	"context"
	"reflect"
	"testing"
)

// TestEachReasoningPortHasExactlyOneMethod: the council, the conductor and
// the research tool each drive ONE verb of the daemon's reasoning surface.
//
// One method each, not one fat port: the council tool has no business
// driving the conductor, and a fake in a tool's test must not have to
// implement a panel it never calls. The three are combined into `Runner` only
// where the daemon needs to carry them as one row.
func TestEachReasoningPortHasExactlyOneMethod(t *testing.T) {
	for _, tc := range []struct {
		name string
		port interface{}
		want string
		in   int
	}{
		{"CouncilRunner", (*CouncilRunner)(nil), "Council", 5},     // ctx, corr, question, members, rounds
		{"ConductorRunner", (*ConductorRunner)(nil), "Conduct", 3}, // ctx, corr, cfg
		{"ResearchRunner", (*ResearchRunner)(nil), "Research", 4},  // ctx, corr, question, opts
	} {
		typ := reflect.TypeOf(tc.port).Elem()
		if typ.Kind() != reflect.Interface {
			t.Errorf("%s must be an interface, got %v", tc.name, typ.Kind())
			continue
		}
		if typ.NumMethod() != 1 {
			t.Errorf("%s has %d methods, want 1", tc.name, typ.NumMethod())
			continue
		}
		m, ok := typ.MethodByName(tc.want)
		if !ok {
			t.Errorf("%s must have a %s method", tc.name, tc.want)
			continue
		}
		// An interface method's type carries no receiver: In(0) is the first
		// ARGUMENT.
		if m.Type.NumIn() != tc.in {
			t.Errorf("%s.%s must take %d arguments, got %v", tc.name, tc.want, tc.in, m.Type)
		}
		if m.Type.NumOut() != 2 {
			t.Errorf("%s.%s must answer 2 values, got %v", tc.name, tc.want, m.Type)
		}
	}
}

// TestTheReasoningRunnerCarriesTheThreePorts: the combined port is the three
// one-verb ports and nothing more — a fourth verb would be a new reach into
// the daemon's reasoning surface, and it would have to be earned here.
func TestTheReasoningRunnerCarriesTheThreePorts(t *testing.T) {
	typ := reflect.TypeOf((*Runner)(nil)).Elem()
	if typ.NumMethod() != 3 {
		t.Fatalf("the combined port has %d methods, want 3", typ.NumMethod())
	}
	for _, name := range []string{"Council", "Conduct", "Research"} {
		if _, ok := typ.MethodByName(name); !ok {
			t.Errorf("the combined port must have a %s method", name)
		}
	}
}

// TestTheReasoningPortsNameOnlyContractRows: the ports' rows are the
// contract's own. A module's type in a port's signature would drag that
// module into every holder of the port — the leak the contract layer exists
// to stop.
func TestTheReasoningPortsNameOnlyContractRows(t *testing.T) {
	council, _ := reflect.TypeOf((*CouncilRunner)(nil)).Elem().MethodByName("Council")
	if council.Type.In(3) != reflect.TypeOf([]CouncilMember{}) {
		t.Errorf("Council must take the contract's members, got %v", council.Type.In(3))
	}
	if council.Type.Out(0) != reflect.TypeOf(CouncilResult{}) {
		t.Errorf("Council must answer the contract's result, got %v", council.Type.Out(0))
	}
	conductor, _ := reflect.TypeOf((*ConductorRunner)(nil)).Elem().MethodByName("Conduct")
	if conductor.Type.In(2) != reflect.TypeOf(ConductorConfig{}) {
		t.Errorf("Conduct must take the contract's config, got %v", conductor.Type.In(2))
	}
	research, _ := reflect.TypeOf((*ResearchRunner)(nil)).Elem().MethodByName("Research")
	if research.Type.In(3) != reflect.TypeOf(ResearchOptions{}) {
		t.Errorf("Research must take the contract's options, got %v", research.Type.In(3))
	}
}

// A port is only worth anything if something can answer it: the context is
// the first argument of every verb, and it must be the standard one.
func TestTheReasoningPortsTakeAContext(t *testing.T) {
	ctxType := reflect.TypeOf((*context.Context)(nil)).Elem()
	for _, tc := range []struct {
		name string
		port interface{}
		m    string
	}{
		{"CouncilRunner", (*CouncilRunner)(nil), "Council"},
		{"ConductorRunner", (*ConductorRunner)(nil), "Conduct"},
		{"ResearchRunner", (*ResearchRunner)(nil), "Research"},
	} {
		typ := reflect.TypeOf(tc.port).Elem()
		m, ok := typ.MethodByName(tc.m)
		if !ok {
			t.Errorf("%s must have a %s method", tc.name, tc.m)
			continue
		}
		if m.Type.In(0) != ctxType {
			t.Errorf("%s.%s must take a context first, got %v", tc.name, tc.m, m.Type.In(0))
		}
	}
}
