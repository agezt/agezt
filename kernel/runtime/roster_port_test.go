// SPDX-License-Identifier: MIT

package runtime_test

// Provenance: the kernel's contract test for the W3.56 slice — that the
//             kernel holds the roster through the CONTRACT's port, that it
//             never opens a roster file of its own, and that a kernel given
//             no roster answers "no agents" instead of calling into nothing.

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestTheKernelHoldsTheContractsPort: the roster reaches the kernel through
// the contract's port — not through the module's concrete type. A kernel that
// named the module's type could not be given a fake, and the layers above it
// could not speak to it without naming the module.
func TestTheKernelHoldsTheContractsPort(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })

	// The assignment itself is the assertion: the accessor answers the
	// CONTRACT's port type, so the layers above the kernel may hold it
	// without naming the module. A kernel given no roster holds a NIL port.
	var port agentapi.Store = k.Roster()
	if port != nil {
		t.Fatal("a kernel given no roster must hold a NIL port")
	}
	// The accessor must answer the contract's NAMED type. An anonymous
	// interface with the same verbs would still compile — and would be a
	// SECOND shape of the same port, drifting from the contract on its own.
	m, ok := reflect.TypeOf(k).MethodByName("Roster")
	if !ok {
		t.Fatal("the kernel has no Roster accessor")
	}
	if got, want := m.Type.Out(0), reflect.TypeOf((*agentapi.Store)(nil)).Elem(); got != want {
		t.Fatalf("the roster accessor answers %v, want the contract's store port %v", got, want)
	}
}

// TestTheKernelNeverOpensARosterOfItsOwn: the roster's FILE is the daemon's
// business (W3.56). A kernel that opened one would be a module reaching back
// into the composition root's job — and the daemon's own roster would then
// be a SECOND store over the same file.
func TestTheKernelNeverOpensARosterOfItsOwn(t *testing.T) {
	dir := t.TempDir()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })

	if _, err := os.Stat(filepath.Join(dir, "roster")); err == nil {
		t.Fatal("the kernel must not open a roster directory of its own")
	}
}

// TestAKernelWithoutARosterAnswersNoAgents: the missing-roster case is
// answered ONCE, in the kernel: the reads say "no agents" and the writes
// refuse. Neither may panic, and neither may pretend the write succeeded.
func TestAKernelWithoutARosterAnswersNoAgents(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })

	if _, found := k.RosterGet("ops"); found {
		t.Fatal("a kernel with no roster has no agents")
	}
	if got := k.RosterList(); len(got) != 0 {
		t.Fatalf("a kernel with no roster lists no agents, got %d", len(got))
	}
	if _, err := k.AddProfile(agentapi.Profile{Slug: "ops"}); err == nil {
		t.Fatal("a kernel with no roster must REFUSE the write, not swallow it")
	}
	if _, err := k.SetProfileEnabled("ops", false); err == nil {
		t.Fatal("a kernel with no roster must REFUSE the pause, not swallow it")
	}
	if _, err := k.SetProfileRetired("ops", true); err == nil {
		t.Fatal("a kernel with no roster must REFUSE the retire, not swallow it")
	}
	if _, _, err := k.UpdateProfile("ops", func(*agentapi.Profile) {}); err == nil {
		t.Fatal("a kernel with no roster must REFUSE the edit, not swallow it")
	}
	if _, err := k.RemoveProfile("ops"); err == nil {
		t.Fatal("a kernel with no roster must REFUSE the removal, not swallow it")
	}
}

// TestTheInjectedRosterIsTheLiveOne: the daemon's roster is the kernel's
// roster. A profile written through the kernel's own writes must be readable
// back through the port, and through the nil-safe reads.
func TestTheInjectedRosterIsTheLiveOne(t *testing.T) {
	k, err := runtime.Open(withRoster(t, runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()}))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })

	saved, err := k.AddProfile(agentapi.Profile{Slug: "ops", Name: "Ops"})
	if err != nil {
		t.Fatalf("AddProfile: %v", err)
	}
	got, found := k.RosterGet("ops")
	if !found || got.ID != saved.ID {
		t.Fatalf("the injected roster must answer the profile, got %+v (found=%v)", got, found)
	}
	if list := k.RosterList(); len(list) != 1 || list[0].Slug != "ops" {
		t.Fatalf("the injected roster must list the profile, got %+v", list)
	}
	// The port itself is the same store — the daemon's file is the kernel's
	// file, not a copy of it.
	if count := k.Roster().Count(); count != 1 {
		t.Fatalf("the roster port holds %d profiles, want 1", count)
	}
}
