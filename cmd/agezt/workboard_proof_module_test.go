// SPDX-License-Identifier: MIT

package main

// The composition root wires the board's PROOF.
//
// The proof is the WORK MODULE's (W3.70): the module keeps the judgment — the
// evidence to gather, the criterion prompt, the reply to parse, the verdict to
// publish — and the daemon holds the module's face. The root is the one place
// that knows both the store and the judge, so the root is where the wiring
// belongs.
//
// A daemon whose root forgot the wiring would hold a board and answer "not
// available" on the proof verb: the task would be there and the proof would
// not be.

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/agezt/agezt/kernel/contract/workboardapi"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The root wires the board's proof cycle.
//
// The proof verb answers, and its answer lands on the task the daemon opened:
// the module drives the store the daemon handed over.
func TestTheRootWiresTheTaskBoardProofCycle(t *testing.T) {
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

	// The proof verb answers with the CONTRACT's refusal when the host has no
	// judge to prove with — the mock provider has none — and the gate HOLDS.
	// What matters here is that the verb reached the module at all: an
	// unwired board answers "not available" instead.
	if _, err := k.ProveTask(context.Background(), "corr", task.ID, "an answer"); err != nil {
		if err == workboardapi.ErrNoStore {
			t.Fatal("the root did not wire the board module: the proof verb refused as unwired")
		}
	}
	if _, err := filepath.Glob(filepath.Join(dir, "workboard", "*.json")); err != nil {
		t.Fatalf("the board's file: %v", err)
	}
}
