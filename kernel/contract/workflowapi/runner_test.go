// SPDX-License-Identifier: MIT

package workflowapi

import (
	"context"
	"reflect"
	"testing"
)

// TestTheWorkflowRunnerPortHasExactlyThreeMethods: the workflow port is the
// whole of what the workflow tool may do to the daemon's library — save a
// workflow, arm it, run it. A fourth method would be a new reach into the
// daemon, and it would have to be earned here.
func TestTheWorkflowRunnerPortHasExactlyThreeMethods(t *testing.T) {
	typ := reflect.TypeOf((*Runner)(nil)).Elem()
	if typ.Kind() != reflect.Interface {
		t.Fatalf("Runner must be an interface, got %v", typ.Kind())
	}
	if typ.NumMethod() != 3 {
		t.Fatalf("the port has %d methods, want 3", typ.NumMethod())
	}
	for _, name := range []string{"SaveWorkflow", "SetWorkflowEnabled", "RunWorkflow"} {
		if _, ok := typ.MethodByName(name); !ok {
			t.Errorf("the port must have a %s method", name)
		}
	}
}

// TestTheWorkflowRunnerNamesOnlyContractRows: every type the port names must
// be a contract row or a builtin. A module's type in a port's signature
// would drag that module into every plugin that holds the port — the very
// leak the contract layer exists to stop — and it would break the moment the
// module moved.
func TestTheWorkflowRunnerNamesOnlyContractRows(t *testing.T) {
	typ := reflect.TypeOf((*Runner)(nil)).Elem()
	workflow := reflect.TypeOf(Workflow{})
	runResult := reflect.TypeOf(RunResult{})
	errType := reflect.TypeOf((*error)(nil)).Elem()
	str := reflect.TypeOf("")
	ctxType := reflect.TypeOf((*context.Context)(nil)).Elem()

	save, _ := typ.MethodByName("SaveWorkflow")
	// SaveWorkflow(corr string, w Workflow) (Workflow, bool, error)
	if save.Type.NumIn() != 2 || save.Type.In(0) != str || save.Type.In(1) != workflow {
		t.Errorf("SaveWorkflow must take (string, Workflow), got %v", save.Type)
	}
	if save.Type.NumOut() != 3 || save.Type.Out(0) != workflow || save.Type.Out(1) != reflect.TypeOf(false) || save.Type.Out(2) != errType {
		t.Errorf("SaveWorkflow must answer (Workflow, bool, error), got %v", save.Type)
	}

	enable, _ := typ.MethodByName("SetWorkflowEnabled")
	// SetWorkflowEnabled(corr, ref string, enabled bool) (Workflow, error)
	if enable.Type.NumIn() != 3 || enable.Type.In(0) != str || enable.Type.In(1) != str || enable.Type.In(2) != reflect.TypeOf(false) {
		t.Errorf("SetWorkflowEnabled must take (string, string, bool), got %v", enable.Type)
	}
	if enable.Type.NumOut() != 2 || enable.Type.Out(0) != workflow || enable.Type.Out(1) != errType {
		t.Errorf("SetWorkflowEnabled must answer (Workflow, error), got %v", enable.Type)
	}

	run, _ := typ.MethodByName("RunWorkflow")
	// RunWorkflow(ctx, corr, ref string, payload any) (RunResult, error)
	if run.Type.NumIn() != 4 || run.Type.In(0) != ctxType || run.Type.In(1) != str || run.Type.In(2) != str {
		t.Errorf("RunWorkflow must take (ctx, string, string, any), got %v", run.Type)
	}
	if run.Type.NumOut() != 2 || run.Type.Out(0) != runResult || run.Type.Out(1) != errType {
		t.Errorf("RunWorkflow must answer (RunResult, error), got %v", run.Type)
	}
}
