// SPDX-License-Identifier: MIT

package main

// Provenance: the daemon's contract test for the skill library (W3.55) —
//             that the composition root opens the library's FILE and hands the
//             kernel the contract's ports.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/agezt/agezt/kernel/contract/skillapi"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestOpenAppKernelWiresTheSkillLibrary: the daemon end — the library is a
// FILE on disk under the base dir, and the port the kernel holds is the
// injected one.
func TestOpenAppKernelWiresTheSkillLibrary(t *testing.T) {
	dir := t.TempDir()
	k, err := openAppKernel(kernelruntime.Config{BaseDir: dir, Provider: mock.New()})
	if err != nil {
		t.Fatalf("openAppKernel: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })

	if k.Forge() == nil {
		t.Fatal("the daemon must hand the kernel a skill curator")
	}
	// The library's home is a DIRECTORY under the base dir (the file itself
	// appears with the first skill — the store writes on mutation).
	if fi, err := os.Stat(filepath.Join(dir, "skills")); err != nil || !fi.IsDir() {
		t.Fatalf("the daemon must open the skill library's directory: %v", err)
	}
	sk, _, err := k.Forge().Create("seed", skillapi.CreateSpec{Name: "diagnose-ci", Body: "steps"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "skills", "skills.json")); err != nil {
		t.Fatalf("the daemon's library must be a file on disk: %v", err)
	}
	if got, found, _ := k.Forge().Get(sk.ID); !found || got.Name != "diagnose-ci" {
		t.Fatalf("the daemon's curator must answer the live rows, got %+v (found=%v)", got, found)
	}
}

// TestTheSkillLibraryIsTheInjectedOne: the daemon uses the INJECTED model,
// not one of its own. A daemon that opened its own library over the injected
// one would silently ignore what the host handed over.
func TestTheSkillLibraryIsTheInjectedOne(t *testing.T) {
	dir := t.TempDir()
	cfg := withSkill(t, kernelruntime.Config{BaseDir: dir, Provider: mock.New()})
	k, err := openAppKernel(cfg)
	if err != nil {
		t.Fatalf("openAppKernel: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })

	sk, _, err := k.Forge().Create("seed", skillapi.CreateSpec{Name: "injected", Body: "steps"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, found, _ := k.Forge().Get(sk.ID)
	if !found || got.Name != "injected" {
		t.Fatalf("the daemon must use the injected library, got %+v (found=%v)", got, found)
	}
}
