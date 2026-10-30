// SPDX-License-Identifier: MIT

package binding

import (
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The scope is the composition root's gift to the adapter: every family's
// ports, filled from the daemon's own kernel.
//
// The root holds the handle; the scope is what leaves the root.
func TestTheScopeIsFilledFromTheDaemonsOwnKernel(t *testing.T) {
	dir := t.TempDir()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })

	scope := ScopeFromKernel(k, dir)
	if !scope.Filled() {
		t.Fatal("the scope was not filled from the daemon's own kernel")
	}
	if scope.Directory != dir {
		t.Errorf("the scope's own directory is %q, want %q", scope.Directory, dir)
	}
	if scope.JournalPorts.Events == nil {
		t.Error("the scope carries no journal — the journal is the family's own port")
	}
	if !scope.BusPorts.Ready() {
		t.Error("the scope carries no bus — the bus is the family's own port")
	}
	if scope.ToolsPorts.Registry == nil {
		t.Error("the scope carries no tools — the tools are the family's own port")
	}
	if scope.CouncilPorts.Store() == nil {
		t.Error("the scope carries no council store — the council is the family's own port")
	}
}

// A scope with no daemon behind it refuses: the families that take a handle
// refuse, and the scope is not filled.
//
// There is no way to fill a scope from an untyped handle any more — the
// scope's own door closed with the adapter's — so "no daemon" is a nil
// kernel, and the refusal is the same one.
func TestTheScopeRefusesWhenThereIsNoDaemon(t *testing.T) {
	scope := ScopeFromKernel(nil, t.TempDir())
	if scope.Filled() {
		t.Fatal("a scope with no daemon filled itself")
	}
	if scope.JournalPorts.Events != nil {
		t.Error("a scope with no daemon filled a family's port")
	}
	if scope.FilesRunner == nil {
		t.Error("a scope with no daemon left the files' runner empty — a nil runner is not a refusal")
	}
	if scope.SteerRuns == nil {
		t.Error("a scope with no daemon left the steer family's runs empty — a nil is not a refusal")
	}
	if scope.PlanRunnerPorts.RunPlan == nil {
		t.Error("a scope with no daemon left the plan runner without a refusal")
	}
}

// The refusing scope is what an adapter that was built without one asks for:
// a family is asked, never called into nothing.
func TestTheRefusingScopeRefusesEveryFamilyThatTakesAHandle(t *testing.T) {
	scope := RefusingScope("")
	if scope.Filled() {
		t.Error("the refusing scope claims to be filled")
	}
	if scope.FilesRunner == nil || scope.SteerRuns == nil {
		t.Fatal("the refusing scope left a family's handle-taking port empty")
	}
	if scope.PlanRunnerPorts.RunPlan == nil {
		t.Error("the refusing scope left the plan runner without a refusal")
	}
	if scope.ResearchEngines.Research == nil {
		t.Error("the refusing scope left the research family without a refusal")
	}
}

// The refusing scope keeps its DIRECTORY.
//
// A scope that refuses still knows where it is: the adapter's own directory
// comes from the scope, and a scope that lost its directory would point the
// adapter at nothing.
func TestTheRefusingScopeKeepsItsDirectory(t *testing.T) {
	scope := RefusingScope(t.TempDir())
	if scope.Directory == "" {
		t.Error("the refusing scope lost its directory")
	}
	if scope.Filled() {
		t.Error("the refusing scope claims to be filled")
	}
}

// The scope's fields are PORT names, never the kernel's verb names.
//
// A scope that carried a kernel verb's name would name a door it does not
// have: the kernel's `BaseDir` is the daemon's own directory, and the scope's
// directory is the scope's. The rule is the reflection — no field of the scope
// is named like a verb of the kernel.
//
// The rule is about the WHOLE name: a field that carries a family as a prefix
// (`MemoryPorts`) names the family's port, and the verb it resembles is not
// the field's name.
func TestTheScopesFieldsAreNeverTheKernelsVerbs(t *testing.T) {
	kernel := reflect.TypeOf(&runtime.Kernel{})
	scope := reflect.TypeOf(Scope{})
	for i := 0; i < scope.NumField(); i++ {
		name := scope.Field(i).Name
		if _, ok := kernel.MethodByName(name); ok {
			t.Errorf("the scope's %s field is the kernel's own verb — a scope names ports, not verbs", name)
		}
	}
}
