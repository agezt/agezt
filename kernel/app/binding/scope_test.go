// SPDX-License-Identifier: MIT

package binding

import (
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
	if scope.BaseDir != dir {
		t.Errorf("the scope's own directory is %q, want %q", scope.BaseDir, dir)
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

// A handle that is not the daemon's own refuses: the families that take a
// handle refuse, and the scope is not filled.
func TestTheScopeRefusesAHandleThatIsNotTheDaemons(t *testing.T) {
	scope := ScopeFromHandle("not a daemon", t.TempDir())
	if scope.Filled() {
		t.Fatal("a foreign handle filled the scope")
	}
	if scope.JournalPorts.Events != nil {
		t.Error("the foreign handle filled a family's port")
	}
	if scope.FilesRunner == nil {
		t.Error("the foreign handle left the files' runner empty — a nil runner is not a refusal")
	}
	if scope.SteerRuns == nil {
		t.Error("the foreign handle left the steer family's runs empty — a nil is not a refusal")
	}
	if scope.PlanRunnerPorts.RunPlan == nil {
		t.Error("the foreign handle left the plan runner without a refusal")
	}
}

// The refusing scope is what an adapter that was built without one asks for:
// a family is asked, never called into nothing.
func TestTheRefusingScopeRefusesEveryFamilyThatTakesAHandle(t *testing.T) {
	scope := RefusingScope()
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

// Resolve is the scope for what the adapter was handed: a scope stays the
// scope it is, and a handle is filled into one.
func TestResolveKeepsAScopeAndFillsAHandle(t *testing.T) {
	dir := t.TempDir()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })

	filled := ScopeFromKernel(k, dir)
	if got := Resolve(filled, dir); !got.Filled() || got.BaseDir != dir {
		t.Errorf("resolve kept %+v, want the scope it was given", got.BaseDir)
	}
	if got := Resolve(k, dir); !got.Filled() || got.BaseDir != dir {
		t.Errorf("resolve filled %q, want the daemon's own scope", got.BaseDir)
	}
	if got := Resolve("not a daemon", dir); got.Filled() {
		t.Error("resolve filled a scope from a foreign handle")
	}
}
