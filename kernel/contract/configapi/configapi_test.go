// SPDX-License-Identifier: MIT

package configapi

import (
	"context"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/agentapi"
)

// TestThePortHasExactlyThreeMethods: the config tool's port is the whole of
// what a plugin may ask of the live daemon — the agent-scoped write, the
// live-apply reload and the run's own overrides. A fourth method would be a
// new reach into the daemon, and it would have to be earned here.
func TestThePortHasExactlyThreeMethods(t *testing.T) {
	typ := reflect.TypeOf((*Kernel)(nil)).Elem()
	if typ.Kind() != reflect.Interface {
		t.Fatalf("Kernel must be an interface, got %v", typ.Kind())
	}
	if typ.NumMethod() != 3 {
		t.Fatalf("the port has %d methods, want 3: %v", typ.NumMethod(), methodNames(typ))
	}
}

// TestThePortAsksForTheThreeWrites: the port's shape IS the contract — the
// tool cannot ask for more than the daemon promised, and the daemon cannot
// answer less.
func TestThePortAsksForTheThreeWrites(t *testing.T) {
	typ := reflect.TypeOf((*Kernel)(nil)).Elem()

	update, ok := typ.MethodByName("UpdateProfile")
	if !ok {
		t.Fatal("the port must have an UpdateProfile method")
	}
	if update.Type.NumIn() != 2 || update.Type.In(1) != reflect.TypeOf(func(*agentapi.Profile) {}) {
		t.Fatalf("UpdateProfile must take a ref and a mutator, got %v", update.Type)
	}
	if update.Type.NumOut() != 3 || update.Type.Out(0) != reflect.TypeOf(agentapi.Profile{}) {
		t.Fatalf("UpdateProfile must answer (Profile, bool, error), got %v", update.Type)
	}
	if update.Type.Out(1).Kind() != reflect.Bool || update.Type.Out(2) != reflect.TypeOf((*error)(nil)).Elem() {
		t.Fatalf("UpdateProfile must answer (Profile, bool, error), got %v", update.Type)
	}

	reload, ok := typ.MethodByName("Reload")
	if !ok {
		t.Fatal("the port must have a Reload method")
	}
	// An interface method's type carries no receiver.
	if reload.Type.NumIn() != 0 || reload.Type.NumOut() != 1 {
		t.Fatalf("Reload must take nothing and answer the error, got %v", reload.Type)
	}
	if reload.Type.Out(0) != reflect.TypeOf((*error)(nil)).Elem() {
		t.Fatalf("Reload must answer the error only, got %v", reload.Type)
	}

	overrides, ok := typ.MethodByName("AgentConfigOverrides")
	if !ok {
		t.Fatal("the port must have an AgentConfigOverrides method")
	}
	if overrides.Type.NumIn() != 1 || overrides.Type.In(0) != reflect.TypeOf((*context.Context)(nil)).Elem() {
		t.Fatalf("AgentConfigOverrides must take a context, got %v", overrides.Type)
	}
	if overrides.Type.NumOut() != 1 || overrides.Type.Out(0) != reflect.TypeOf(map[string]string{}) {
		t.Fatalf("AgentConfigOverrides must answer a map, got %v", overrides.Type)
	}
}

// methodNames lists the port's methods — for the failure messages.
func methodNames(typ reflect.Type) []string {
	out := make([]string, 0, typ.NumMethod())
	for i := 0; i < typ.NumMethod(); i++ {
		out = append(out, typ.Method(i).Name)
	}
	return out
}
