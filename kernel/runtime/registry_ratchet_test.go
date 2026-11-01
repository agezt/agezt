// SPDX-License-Identifier: MIT

package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The live-run registry — the running correlations, their cancels, the halt
// latch, the drain accounting and the suspend latch — moved to the runs
// module in W3.76d (kernel/modules/runs/registry), together with the
// lifecycle manager that owns the halt/resume/cancel surface over it.
//
// The daemon keeps the DOORS: the verbs below and the Kernel's own run
// state. What it must NOT keep is a second copy of the store — the registry
// is the module's, and the daemon speaks to it through the field.
//
// These tests are the ratchet.

// TestTheRunRegistryLivesInTheRunsModule pins the move itself: the daemon's
// lifecycle sub-package is GONE, the module's is there, and the daemon holds
// the module's store.
func TestTheRunRegistryLivesInTheRunsModule(t *testing.T) {
	if _, err := os.Stat(filepath.Join("..", "runtime", "lifecycle")); err == nil {
		t.Fatal("the daemon kept its lifecycle sub-package; the run lifecycle is the module's")
	}
	for _, want := range []string{"registry", "lifecycle"} {
		if _, err := os.Stat(filepath.Join("..", "modules", "runs", want)); err != nil {
			t.Fatalf("the runs module must carry %s: %v", want, err)
		}
	}
}

// TestTheDaemonHoldsNoRunRegistryField is the ratchet on the daemon's side:
// the daemon must not keep its own copy of the store's fields. The registry
// is the module's data, and the daemon holds a POINTER to it.
func TestTheDaemonHoldsNoRunRegistryField(t *testing.T) {
	forbidden := []string{
		"runs       map[string]context.CancelFunc",
		"runsMu   sync.Mutex",
		"halted bool",
		"runWG      sync.WaitGroup",
		"suspending atomic.Bool",
	}
	for _, name := range forbidden {
		if hasDaemonField(t, name) {
			t.Fatalf("the daemon kept the registry field %q; the run registry is the module's", name)
		}
	}
}

// TestTheDaemonHoldsNoRegistryAccessor pins the other half: the daemon used
// to expose six accessors for the lifecycle manager's benefit (Halted,
// SetHalted, Runs, RunsMu, RunWG, Suspending). The manager holds the
// registry itself now, so those six doors are gone.
func TestTheDaemonHoldsNoRegistryAccessor(t *testing.T) {
	forbidden := []string{
		"func (k *Kernel) Halted()",
		"func (k *Kernel) SetHalted(",
		"func (k *Kernel) Runs()",
		"func (k *Kernel) RunsMu()",
		"func (k *Kernel) RunWG()",
		"func (k *Kernel) Suspending()",
	}
	for _, name := range forbidden {
		if hasDaemonDeclaration(t, name) {
			t.Fatalf("the daemon kept the registry accessor %q", name)
		}
	}
}

// TestTheDaemonKeepsTheLifecycleDoors pins the daemon's HALF of the move:
// the halt/resume/cancel verbs stay (they are the doors), and the Kernel
// keeps holding the module's lifecycle manager.
func TestTheDaemonKeepsTheLifecycleDoors(t *testing.T) {
	src, err := os.ReadFile("runtime_lifecycle.go")
	if err != nil {
		t.Fatalf("read runtime_lifecycle.go: %v", err)
	}
	text := string(src)
	for _, door := range []string{
		"func (k *Kernel) IsHalted()",
		"func (k *Kernel) Halt()",
		"func (k *Kernel) HaltWith(",
		"func (k *Kernel) Resume()",
		"func (k *Kernel) ResumeWith(",
		"func (k *Kernel) CancelRun(",
		"func (k *Kernel) DrainAndHalt(",
		"func (k *Kernel) ActiveRuns()",
		"func (k *Kernel) ActiveRunIDs()",
	} {
		if !strings.Contains(text, door) {
			t.Fatalf("the daemon must keep the door %q", door)
		}
	}
}

// TestTheDaemonTalksToTheModuleThroughTheRegistry pins the direction of the
// move: the daemon's run state is registered in the module's store, never in
// a map of its own.
func TestTheDaemonTalksToTheModuleThroughTheRegistry(t *testing.T) {
	for _, f := range []string{"runexec_lifecycle.go", "accessors_misc.go"} {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		text := string(src)
		if strings.Contains(text, "k.runs[") || strings.Contains(text, "delete(k.runs") {
			t.Fatalf("%s still writes the registry's map directly", f)
		}
		if !strings.Contains(text, "k.registry.") {
			t.Fatalf("%s must register through the module's store", f)
		}
	}
	// The async delegation registers its child in the same store. The code
	// moved to the runs module's delegation engine in W3.76f, so the ratchet
	// watches the module's file now.
	mod, err := os.ReadFile(filepath.Join("..", "modules", "runs", "delegation", "exec.go"))
	if err != nil {
		t.Fatalf("read the delegation engine: %v", err)
	}
	text := string(mod)
	if !strings.Contains(text, "reg.Register(") {
		t.Fatal("the delegation engine must register an async child in the module's store")
	}
	if strings.Contains(text, "k.runs[") {
		t.Fatal("the delegation engine still writes the registry's map directly")
	}
}

// hasDaemonField reports whether the daemon's own package declares a struct
// field whose declaration matches the given text.
func hasDaemonField(t *testing.T, name string) bool {
	t.Helper()
	for _, f := range []string{"runtime_kernel.go"} {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		for _, line := range strings.Split(string(src), "\n") {
			if strings.Contains(line, name) {
				return true
			}
		}
	}
	return false
}
