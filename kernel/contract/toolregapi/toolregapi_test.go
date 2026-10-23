// SPDX-License-Identifier: MIT

package toolregapi

import (
	"context"
	"reflect"
	"testing"
)

// TestThePreOpenPortHasExactlyOneMethod: the pre-Open phase's port is the
// whole of what a tool may do to the daemon's configuration before the kernel
// opens — install the script runner. A second method would be a new reach
// into the daemon's runtime, and it would have to be earned here.
func TestThePreOpenPortHasExactlyOneMethod(t *testing.T) {
	typ := reflect.TypeOf((*Config)(nil)).Elem()
	if typ.Kind() != reflect.Interface {
		t.Fatalf("Config must be an interface, got %v", typ.Kind())
	}
	if typ.NumMethod() != 1 {
		t.Fatalf("the port has %d methods, want 1", typ.NumMethod())
	}
	m, ok := typ.MethodByName("SetScriptRunner")
	if !ok {
		t.Fatal("the port must have a SetScriptRunner method")
	}
	// An interface method's type carries no receiver.
	if m.Type.NumIn() != 1 {
		t.Errorf("SetScriptRunner must take one argument, got %v", m.Type)
	}
	if m.Type.In(0) != reflect.TypeOf((*ScriptRunner)(nil)).Elem() {
		t.Errorf("SetScriptRunner must take the contract's runner, got %v", m.Type.In(0))
	}
	if m.Type.NumOut() != 0 {
		t.Errorf("SetScriptRunner must answer nothing, got %v", m.Type)
	}
}

// TestTheRunnerHasExactlyOneMethod: the contract declares the runner the
// daemon's own runner interface has — one verb, the same signature. A
// renamed or reshaped verb would stop the daemon's runner from satisfying
// the contract's one, and the wiring would break at compile time in a place
// nobody looks.
func TestTheRunnerHasExactlyOneMethod(t *testing.T) {
	typ := reflect.TypeOf((*ScriptRunner)(nil)).Elem()
	if typ.NumMethod() != 1 {
		t.Fatalf("the runner has %d methods, want 1", typ.NumMethod())
	}
	m, ok := typ.MethodByName("RunScript")
	if !ok {
		t.Fatal("the runner must have a RunScript method")
	}
	// RunScript(ctx, language, code, inputJSON) (output string, isError bool, err error)
	if m.Type.NumIn() != 4 {
		t.Fatalf("RunScript must take (ctx, language, code, input), got %v", m.Type)
	}
	if m.Type.In(0) != reflect.TypeOf((*context.Context)(nil)).Elem() {
		t.Errorf("the first argument must be the context, got %v", m.Type.In(0))
	}
	for i := 1; i < 4; i++ {
		if m.Type.In(i) != reflect.TypeOf("") {
			t.Errorf("argument %d must be a string, got %v", i, m.Type.In(i))
		}
	}
	if m.Type.NumOut() != 3 {
		t.Fatalf("RunScript must answer (output, isError, err), got %v", m.Type)
	}
	if m.Type.Out(0) != reflect.TypeOf("") || m.Type.Out(1).Kind() != reflect.Bool {
		t.Errorf("RunScript must answer (string, bool, error), got %v", m.Type)
	}
}
