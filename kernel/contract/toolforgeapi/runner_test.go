// SPDX-License-Identifier: MIT

package toolforgeapi

import (
	"context"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/approvalapi"
)

// TestTheRunnerPortCarriesExactlyFourVerbs: the forge's lifecycle is draft,
// update, test, ask the operator to promote — and nothing else.
//
// The tool that drives it is a plugin, so the registry's row cannot be the
// live kernel; it is this port. A fifth verb would be a new reach into the
// forge, and it would have to be earned here.
func TestTheRunnerPortCarriesExactlyFourVerbs(t *testing.T) {
	typ := reflect.TypeOf((*Runner)(nil)).Elem()
	if typ.Kind() != reflect.Interface {
		t.Fatalf("Runner must be an interface, got %v", typ.Kind())
	}
	if typ.NumMethod() != 4 {
		t.Fatalf("the port has %d methods, want 4: %v", typ.NumMethod(), typ)
	}
	for _, name := range []string{"DraftScriptTool", "UpdateScriptTool", "TestScriptTool", "RequestToolPromotion"} {
		if _, ok := typ.MethodByName(name); !ok {
			t.Errorf("the port must have a %s method", name)
		}
	}
}

// TestTheRunnerPortNamesOnlyTheContractsRows: every type the port names is
// the contract's — the script tool and the operator's verdict.
//
// The kernel satisfies this port WITHOUT an adapter, and that is the claim
// this pins: the kernel's own methods speak `toolforge.ScriptTool` (an alias
// of the contract's row) and `approval.Decision` (an alias of the contract's
// row), so the two are the SAME types, not two shapes kept in step by hand.
func TestTheRunnerPortNamesOnlyTheContractsRows(t *testing.T) {
	typ := reflect.TypeOf((*Runner)(nil)).Elem()
	scriptTool := reflect.TypeOf(ScriptTool{})
	decision := reflect.TypeOf(approvalapi.Decision(""))

	draft, _ := typ.MethodByName("DraftScriptTool")
	if draft.Type.NumIn() != 2 || draft.Type.In(1) != scriptTool {
		t.Errorf("DraftScriptTool must take the contract's script tool, got %v", draft.Type)
	}
	if draft.Type.NumOut() != 2 || draft.Type.Out(0) != scriptTool {
		t.Errorf("DraftScriptTool must answer the contract's script tool, got %v", draft.Type)
	}

	update, _ := typ.MethodByName("UpdateScriptTool")
	if update.Type.NumIn() != 3 {
		t.Errorf("UpdateScriptTool must take 3 arguments, got %v", update.Type)
	}
	if update.Type.In(2) != reflect.TypeOf(func(*ScriptTool) {}) {
		t.Errorf("UpdateScriptTool must take a mutator over the contract's row, got %v", update.Type.In(2))
	}
	if update.Type.NumOut() != 3 || update.Type.Out(0) != scriptTool {
		t.Errorf("UpdateScriptTool must answer the contract's script tool, got %v", update.Type)
	}

	test, _ := typ.MethodByName("TestScriptTool")
	if test.Type.NumIn() != 4 {
		t.Errorf("TestScriptTool must take 4 arguments, got %v", test.Type)
	}
	if test.Type.In(0) != reflect.TypeOf((*context.Context)(nil)).Elem() {
		t.Errorf("TestScriptTool must take the context first, got %v", test.Type.In(0))
	}
	if test.Type.NumOut() != 3 || test.Type.Out(0) != scriptTool {
		t.Errorf("TestScriptTool must answer the contract's script tool, got %v", test.Type)
	}

	promote, _ := typ.MethodByName("RequestToolPromotion")
	if promote.Type.NumIn() != 3 {
		t.Errorf("RequestToolPromotion must take 3 arguments, got %v", promote.Type)
	}
	if promote.Type.NumOut() != 4 {
		t.Errorf("RequestToolPromotion must answer 4 values, got %v", promote.Type)
	}
	if promote.Type.Out(1) != decision {
		t.Errorf("RequestToolPromotion must answer the contract's verdict, got %v", promote.Type.Out(1))
	}
}
