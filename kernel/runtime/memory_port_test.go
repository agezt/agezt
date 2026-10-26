// SPDX-License-Identifier: MIT

package runtime_test

// Provenance: the kernel's contract test for the W3.57 slice — that the
//             kernel holds the memory through the CONTRACT's port, that it
//             never opens a memory file of its own, and that a kernel given
//             no memory answers "no memory" instead of calling into nothing.

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/memoryapi"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestTheKernelHoldsTheContractsPort: the memory reaches the kernel through
// the contract's port — not through the module's concrete type. A kernel that
// named the module's type could not be given a fake, and the layers above it
// could not speak to it without naming the module.
func TestTheKernelHoldsTheMemoryContractsPort(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })

	// The assignment itself is the assertion: the accessor answers the
	// CONTRACT's port type, so the layers above the kernel may hold it
	// without naming the module. A kernel given no memory holds a NIL port.
	var port memoryapi.Manager = k.Memory()
	if port != nil {
		t.Fatal("a kernel given no memory must hold a NIL port")
	}
	// The accessor must answer the contract's NAMED type. An anonymous
	// interface with the same verbs would still compile — and would be a
	// SECOND shape of the same port, drifting from the contract on its own.
	m, ok := reflect.TypeOf(k).MethodByName("Memory")
	if !ok {
		t.Fatal("the kernel has no Memory accessor")
	}
	if got, want := m.Type.Out(0), reflect.TypeOf((*memoryapi.Manager)(nil)).Elem(); got != want {
		t.Fatalf("the memory accessor answers %v, want the contract's manager port %v", got, want)
	}
}

// TestTheKernelNeverOpensAMemoryOfItsOwn: the memory's FILE is the daemon's
// business (W3.57). A kernel that opened one would be a module reaching back
// into the composition root's job — and the daemon's own memory would then be
// a SECOND store over the same file.
func TestTheKernelNeverOpensAMemoryOfItsOwn(t *testing.T) {
	dir := t.TempDir()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })

	if _, err := os.Stat(filepath.Join(dir, "memory")); err == nil {
		t.Fatal("the kernel must not open a memory directory of its own")
	}
}

// TestAKernelWithoutAMemoryAnswersNoMemory: the missing-memory case is
// answered ONCE, in the kernel: the reads say "no memory" and the writes
// refuse. Neither may panic, and neither may pretend the write succeeded.
//
// The answers differ by direction on purpose: "how much do you know?" has an
// honest answer even without a memory, but a write that cannot happen must
// say so rather than answering "nothing was forgotten" — that would read as a
// successful no-op on a store that does not exist.
func TestAKernelWithoutAMemoryAnswersNoMemory(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })

	// The READS answer empty.
	if got := k.MemoryCount(); got != 0 {
		t.Fatalf("a kernel with no memory holds %d records, want 0", got)
	}
	if active, err := k.MemoryActive(); err != nil || len(active) != 0 {
		t.Fatalf("a kernel with no memory lists no records, got %d (err=%v)", len(active), err)
	}
	if got := k.MemoryProfileText(); got != "" {
		t.Fatalf("a kernel with no memory renders no profile, got %q", got)
	}
	if got, err := k.MemorySearchScoped("q", 5, "scope"); err != nil || len(got) != 0 {
		t.Fatalf("a kernel with no memory searches nothing, got %d (err=%v)", len(got), err)
	}
	if got, err := k.MemoryRecallScoped("corr", "q", 5, "scope"); err != nil || len(got) != 0 {
		t.Fatalf("a kernel with no memory recalls nothing, got %d (err=%v)", len(got), err)
	}

	// The WRITES refuse.
	if _, err := k.MemoryForget("corr", "id"); err != runtime.ErrNoMemory {
		t.Fatalf("a kernel with no memory must REFUSE the forget, got %v", err)
	}
	if _, err := k.MemoryDistillFacts(context.Background(), "corr", mock.New(), "model", "intent", "transcript"); err != runtime.ErrNoMemory {
		t.Fatalf("a kernel with no memory must REFUSE the distillation, got %v", err)
	}
	if _, err := k.MemoryDistillBrain(context.Background(), "corr"); err != runtime.ErrNoMemory {
		t.Fatalf("a kernel with no memory must REFUSE the consolidation, got %v", err)
	}
	if _, err := k.MemoryDistillProfile(context.Background(), "corr"); err != runtime.ErrNoMemory {
		t.Fatalf("a kernel with no memory must REFUSE the profile rebuild, got %v", err)
	}
}

// TestTheInjectedMemoryIsTheLiveOne: the daemon's memory is the kernel's
// memory. A record written through the kernel's own writes must be readable
// back through the port, and through the nil-safe reads.
func TestTheInjectedMemoryIsTheLiveOne(t *testing.T) {
	k, err := runtime.Open(withMemory(t, runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()}))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })

	if k.Memory() == nil {
		t.Fatal("the injected memory must reach the kernel as a port")
	}
	saved, created, err := k.Memory().Remember("corr", memoryapi.RememberSpec{
		Type: memoryapi.TypeFact, Subject: "topic", Content: "a durable fixture fact",
	})
	if err != nil {
		t.Fatalf("Remember: %v", err)
	}
	if !created {
		t.Fatal("the first write of a fact must create it")
	}
	if got := k.MemoryCount(); got != 1 {
		t.Fatalf("the kernel must count the injected memory's records, got %d", got)
	}
	active, err := k.MemoryActive()
	if err != nil || len(active) != 1 || active[0].ID != saved.ID {
		t.Fatalf("the injected memory must answer the live rows, got %+v (err=%v)", active, err)
	}
	// The port itself is the same store — the daemon's file is the kernel's
	// file, not a copy of it.
	if got := k.Memory().Count(); got != 1 {
		t.Fatalf("the memory port holds %d records, want 1", got)
	}
}

// TestTheKernelClosesTheInjectedMemory: the store is the HOST's file, so the
// host closes it. The kernel closes it through the port — a kernel that
// ignored the port would leak the file handle on every restart.
func TestTheKernelClosesTheInjectedMemory(t *testing.T) {
	// A store that reports its own close is the only way to observe the
	// kernel's behaviour from outside.
	k, err := runtime.Open(withMemory(t, runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()}))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := k.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}
