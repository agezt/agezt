// SPDX-License-Identifier: MIT

package main

// Provenance: the daemon's contract test for the memory (W3.57) — that the
//             composition root opens the memory's FILE, hands the kernel the
//             contract's port, and injects the structured-output path the
//             distillers need.

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/agezt/agezt/kernel/contract/memoryapi"
	"github.com/agezt/agezt/kernel/modules/knowledge/memory"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestOpenAppKernelWiresTheMemory: the daemon end — the memory is a FILE on
// disk under the base dir, and the port the kernel holds is the injected one.
//
// The file is the point: a fact written through the kernel has to land in the
// file the NEXT start reads, or the agent would forget itself on every
// restart.
func TestOpenAppKernelWiresTheMemory(t *testing.T) {
	dir := t.TempDir()
	k, err := openAppKernel(kernelruntime.Config{BaseDir: dir, Provider: mock.New()})
	if err != nil {
		t.Fatalf("openAppKernel: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })

	if k.Memory() == nil {
		t.Fatal("the daemon must hand the kernel a memory port")
	}
	// The memory's home is a DIRECTORY under the base dir (the file itself
	// appears with the first write — the store writes on mutation).
	if fi, err := os.Stat(filepath.Join(dir, "memory")); err != nil || !fi.IsDir() {
		t.Fatalf("the daemon must open the memory's directory: %v", err)
	}
	if _, _, err := k.Memory().Remember("corr", memoryapi.RememberSpec{
		Type: memoryapi.TypeFact, Subject: "topic", Content: "a durable daemon fact",
	}); err != nil {
		t.Fatalf("Remember: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "memory", "memory.json")); err != nil {
		t.Fatalf("the daemon's memory must be a file on disk: %v", err)
	}
	if got := k.MemoryCount(); got != 1 {
		t.Fatalf("the daemon's memory must hold the written fact, got %d records", got)
	}
}

// TestTheMemoryIsTheInjectedOne: the daemon uses the INJECTED memory, not one
// of its own. A daemon that opened its own memory over the injected one would
// silently ignore what the host handed over.
func TestTheMemoryIsTheInjectedOne(t *testing.T) {
	dir := t.TempDir()
	store, err := memory.Open(filepath.Join(dir, "injected"))
	if err != nil {
		t.Fatalf("open the memory store: %v", err)
	}
	k, err := openAppKernel(kernelruntime.Config{
		BaseDir:     dir,
		Provider:    mock.New(),
		MemoryStore: store,
		MemoryBuilder: func(deps memoryapi.Deps) memoryapi.Manager {
			return memory.NewManager(deps)
		},
	})
	if err != nil {
		t.Fatalf("openAppKernel: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })

	if _, _, err := k.Memory().Remember("corr", memoryapi.RememberSpec{
		Type: memoryapi.TypeFact, Subject: "topic", Content: "a durable injected fact",
	}); err != nil {
		t.Fatalf("Remember: %v", err)
	}
	if got := store.Count(); got != 1 {
		t.Fatalf("the daemon must use the injected memory, got %d records in it", got)
	}
	// And the daemon's OWN memory directory was never created: the injected
	// store replaced it, not accompanied it.
	if _, err := os.Stat(filepath.Join(dir, "memory")); err == nil {
		t.Fatal("the daemon must not open a second memory over the injected one")
	}
}

// TestTheDaemonWiresTheStructuredOutputPath: the distillers need the
// structured-output path, and it lives in ANOTHER module — so the daemon
// injects it rather than letting the memory module reach for it.
//
// The path is the only way a distillation pass can turn a model's answer into
// a typed value. Without it the distillers REFUSE — and an agent would
// silently stop learning, with no error anywhere else.
func TestTheDaemonWiresTheStructuredOutputPath(t *testing.T) {
	dir := t.TempDir()
	prov := mock.New(mock.FinalText(`{"facets":[{"facet":"preferences","content":"be blunt"}]}`))
	k, err := openAppKernel(kernelruntime.Config{BaseDir: dir, Provider: prov})
	if err != nil {
		t.Fatalf("openAppKernel: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })

	if _, _, err := k.Memory().Remember("corr", memoryapi.RememberSpec{
		Type: memoryapi.TypeFact, Subject: "operator habits", Content: "the operator prefers blunt answers",
	}); err != nil {
		t.Fatalf("Remember: %v", err)
	}
	report, err := k.DistillProfile(context.Background(), "corr")
	if err != nil {
		t.Fatalf("DistillProfile: %v", err)
	}
	if report.FacetsWritten != 1 {
		t.Fatalf("the daemon's structured-output path must drive the distillation, got %+v", report)
	}
}
