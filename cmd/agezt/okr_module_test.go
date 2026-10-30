// SPDX-License-Identifier: MIT

package main

// The composition root wires the objective library.
//
// The library is the WORK MODULE's (W3.69): the module keeps the store, the
// goals and the rollup rule, and the daemon holds the module's face. The root
// is the one place that knows both — it opens the store and builds the
// module's service over it — so the root is where the wiring belongs.
//
// A daemon whose root forgot the wiring would hold a library and answer "not
// available" on every objective verb: the file would be there and the goals
// would not be.

import (
	"path/filepath"
	"testing"

	"github.com/agezt/agezt/kernel/contract/okrapi"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The root wires the objective library's module.
//
// The objective verbs answer, and their answer lands in the daemon's own file:
// the module drives the store the daemon opened.
func TestTheRootWiresTheObjectiveLibraryModule(t *testing.T) {
	dir := t.TempDir()
	k, err := openAppKernel(kernelruntime.Config{BaseDir: dir, Provider: mock.New()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })

	goal, err := k.CreateObjective("corr", okrapi.CreateSpec{Title: "wired"})
	if err != nil {
		t.Fatalf("the root did not wire the objective library module: %v", err)
	}
	if goal.Title != "wired" {
		t.Fatalf("the library did not take the goal: %+v", goal)
	}
	if _, err := k.AddObjectiveKeyResult("corr", goal.ID, "criterion", 1); err != nil {
		t.Fatalf("the root did not wire the objective library module: %v", err)
	}
	if _, err := filepath.Glob(filepath.Join(dir, "okr", "*.json")); err != nil {
		t.Fatalf("the library's file: %v", err)
	}
}
