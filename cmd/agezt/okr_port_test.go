// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/agezt/agezt/kernel/contract/okrapi"
	"github.com/agezt/agezt/kernel/modules/work/okr"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestOpenAppKernelWiresTheObjectiveLibrary: the objective library is the
// DAEMON's (W3.53) — the kernel holds the contract's port and the daemon
// opens the file.
//
// The daemon end: the port is live, and the library is a DIRECTORY under the
// daemon's base directory — an objective saved through the port has to land in
// the file the next start reads.
func TestOpenAppKernelWiresTheObjectiveLibrary(t *testing.T) {
	dir := t.TempDir()
	k, err := openAppKernel(kernelruntime.Config{BaseDir: dir, Provider: mock.New()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })

	if k.OKR() == nil {
		t.Fatal("the daemon did not wire the objective library — the kernel holds no store port")
	}
	if _, err := k.OKR().Create(okrapi.CreateSpec{Title: "seed"}, 1000); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "okr", "okr.json")); err != nil {
		t.Fatalf("the daemon did not open the library's own directory: %v", err)
	}
}

// TestTheObjectiveLibraryIsTheInjectedOne: the daemon uses the library it was
// HANDED (W3.53) — not one it opened behind the caller's back. A caller that
// injects a library must have its library used, or the injection would be a
// lie.
func TestTheObjectiveLibraryIsTheInjectedOne(t *testing.T) {
	dir := t.TempDir()
	store, err := okr.OpenStore(filepath.Join(dir, "injected"))
	if err != nil {
		t.Fatalf("open okr store: %v", err)
	}
	k, err := openAppKernel(kernelruntime.Config{
		BaseDir:  dir,
		Provider: mock.New(),
		OKRStore: store,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })

	if _, err := k.OKR().Create(okrapi.CreateSpec{Title: "seed"}, 1000); err != nil {
		t.Fatalf("Create: %v", err)
	}
	// The injected library holds the file — the daemon did not open a second
	// directory of its own.
	if _, err := os.Stat(filepath.Join(dir, "injected", "okr.json")); err != nil {
		t.Fatalf("the daemon saved into its own directory, not the injected one: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "okr", "okr.json")); err == nil {
		t.Fatal("the daemon opened a second library of its own beside the injected one")
	}
}
