// SPDX-License-Identifier: MIT

package main

// The composition root wires the board.
//
// The board is the WORK MODULE's (W5.6): the module keeps the store, the
// lifecycle and the retry rule, and the daemon holds the module's face. The
// root is the one place that knows both — it opens the store and builds the
// module's service over it — so the root is where the wiring belongs.
//
// A daemon whose root forgot the wiring would hold a store and answer "not
// available" on every board verb: the file would be there and the board would
// not be.

import (
	"path/filepath"
	"testing"

	"github.com/agezt/agezt/kernel/contract/workboardapi"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The root wires the board's module.
//
// The board's verbs answer, and their answer lands in the daemon's own file:
// the module drives the store the daemon opened.
func TestTheRootWiresTheTaskBoardModule(t *testing.T) {
	dir := t.TempDir()
	k, err := openAppKernel(kernelruntime.Config{BaseDir: dir, Provider: mock.New()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })

	task, created, err := k.CreateWorkboardTask("corr", workboardapi.CreateSpec{Title: "wired"})
	if err != nil {
		t.Fatalf("the root did not wire the board module: %v", err)
	}
	if !created || task.Title != "wired" {
		t.Fatalf("the board did not take the task: %+v", task)
	}
	if _, err := k.ClaimWorkboardTask("corr", task.ID, "agent", "run"); err != nil {
		t.Fatalf("the root did not wire the board module: %v", err)
	}
	if _, err := filepath.Glob(filepath.Join(dir, "workboard", "*.json")); err != nil {
		t.Fatalf("the board's file: %v", err)
	}
}
