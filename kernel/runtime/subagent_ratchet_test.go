// SPDX-License-Identifier: MIT

package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The delegation engine — the policy that decides WHO may delegate WHAT,
// under WHICH bounds, with WHICH identity — moved to the runs module in
// W3.76f (kernel/modules/runs/delegation), together with the kernel/delegation
// vocabulary package it belonged to.
//
// The daemon keeps the MECHANISM: the doors (subagent_doors.go) and the child
// loop the engine asks it to run. What it must NOT keep is a second copy of
// the engine — its tallies, its spawn handles, its guards.
//
// These tests are the ratchet.

// TestTheDelegationEngineLivesInTheRunsModule pins the move itself: the
// daemon's delegation files are GONE, the module's are there.
func TestTheDelegationEngineLivesInTheRunsModule(t *testing.T) {
	for _, gone := range []string{
		"subagent.go", "subagent_prep.go", "subagent_exec.go",
		"subagent_run.go", "subagent_spend.go", "subagent_tool.go",
	} {
		if _, err := os.Stat(gone); err == nil {
			t.Fatalf("the daemon kept %s; the delegation engine is the module's", gone)
		}
	}
	if _, err := os.Stat(filepath.Join("..", "delegation")); err == nil {
		t.Fatal("the daemon kept the kernel/delegation vocabulary package; it moved to the module")
	}
	for _, want := range []string{"doc.go", "service.go", "prep.go", "exec.go", "spend.go", "tool.go"} {
		if _, err := os.Stat(filepath.Join("..", "modules", "runs", "delegation", want)); err != nil {
			t.Fatalf("the runs module must carry %s: %v", want, err)
		}
	}
}

// TestTheDaemonHoldsNoDelegationTally is the ratchet on the daemon's side:
// the fan-out tally (M46), the tree-total tally (M629) and the async spawn
// handles (M881) are the ENGINE's state. The daemon must not keep a copy.
func TestTheDaemonHoldsNoDelegationTally(t *testing.T) {
	forbidden := []string{
		"fanout   map[string]int",
		"tree     map[string]int",
		"spawns   map[string]",
		"fanoutMu sync.Mutex",
		"treeMu   sync.Mutex",
		"spawnsMu sync.Mutex",
	}
	for _, name := range forbidden {
		if hasDaemonField(t, name) {
			t.Fatalf("the daemon kept the delegation tally %q; the engine owns it", name)
		}
	}
}

// TestTheDaemonHoldsNoDelegationEngine pins the other half: the daemon's
// private delegation verbs are gone. The engine owns them.
func TestTheDaemonHoldsNoDelegationEngine(t *testing.T) {
	forbidden := []string{
		"func (k *Kernel) prepareSubAgent(",
		"func (k *Kernel) executeSubAgent(",
		"func (k *Kernel) runSubAgent(",
		"func (k *Kernel) runSubAgentAsync(",
		"func (k *Kernel) awaitSubAgent(",
		"func (k *Kernel) validateDelegationManager(",
		"func (k *Kernel) subAgentSpendMicrocents(",
	}
	for _, name := range forbidden {
		if hasDaemonDeclaration(t, name) {
			t.Fatalf("the daemon kept the delegation verb %q", name)
		}
	}
}

// TestTheDaemonKeepsTheDelegationDoors pins the daemon's HALF of the move:
// the door file and the child-loop callback stay — they are the mechanism the
// engine asks the daemon for.
func TestTheDaemonKeepsTheDelegationDoors(t *testing.T) {
	src, err := os.ReadFile("subagent_doors.go")
	if err != nil {
		t.Fatalf("read subagent_doors.go: %v", err)
	}
	text := string(src)
	for _, door := range []string{
		"func (h delegationHost) RosterGet(",
		"func (h delegationHost) Model()",
		"func (h delegationHost) System()",
		"func (h delegationHost) Halted()",
		"func (h delegationHost) HaltedError()",
		"func (h delegationHost) NewCorrelation()",
		"func (h delegationHost) Publish(",
		"func (h delegationHost) RangeJournal(",
		"func (h delegationHost) RegisterSteer(",
		"func (h delegationHost) UnregisterSteer(",
		"func (h delegationHost) RecordSkillOutcome(",
		"func (h delegationHost) BaseLoopConfig(",
		"func (h delegationHost) CompleteChildLifecycle(",
		"func (h delegationHost) ProfileModel(",
		"func delegationRunChild(",
		"func delegationConfig(",
	} {
		if !strings.Contains(text, door) {
			t.Fatalf("the daemon must keep the door %q", door)
		}
	}
}

// TestTheDaemonHoldsTheEngineNotACopy pins the direction: the daemon holds a
// POINTER to the module's engine, and the module never imports the daemon.
func TestTheDaemonHoldsTheEngineNotACopy(t *testing.T) {
	if !hasDaemonField(t, "delegation *delegation.Service") {
		t.Fatal("the daemon must hold the runs module's delegation engine")
	}
	dir := filepath.Join("..", "modules", "runs", "delegation")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read the delegation engine: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		src, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		if strings.Contains(string(src), `"github.com/agezt/agezt/kernel/runtime`) {
			t.Fatalf("%s imports the daemon; the engine must not name it", e.Name())
		}
	}
}
