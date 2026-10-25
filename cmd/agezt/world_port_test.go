// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/agezt/agezt/kernel/contract/worldmodelapi"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestOpenAppKernelWiresTheWorldModel: the world model is the DAEMON's
// (W3.54) — the kernel holds the contract's ports and the daemon opens the
// file.
//
// The daemon end: the port is live, the BUILDER is the daemon's, and the
// library is a DIRECTORY under the daemon's base directory — an entity saved
// through the port has to land in the file the next start reads.
func TestOpenAppKernelWiresTheWorldModel(t *testing.T) {
	dir := t.TempDir()
	k, err := openAppKernel(kernelruntime.Config{BaseDir: dir, Provider: mock.New()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })

	if k.World() == nil {
		t.Fatal("the daemon did not wire the world model — the kernel holds no graph port")
	}
	if _, _, err := k.World().Upsert("corr", worldmodelapi.UpsertSpec{Name: "seed"}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "worldmodel", "worldmodel.json")); err != nil {
		t.Fatalf("the daemon did not open the model's own directory: %v", err)
	}
}

// TestTheWorldModelIsTheInjectedOne: the daemon uses the library it was
// HANDED (W3.54) — not one it opened behind the caller's back. A caller that
// injects a library must have its library used, or the injection would be a
// lie.
func TestTheWorldModelIsTheInjectedOne(t *testing.T) {
	dir := t.TempDir()
	cfg := withWorld(t, kernelruntime.Config{BaseDir: dir, Provider: mock.New()})
	k, err := openAppKernel(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })

	if _, _, err := k.World().Upsert("corr", worldmodelapi.UpsertSpec{Name: "seed"}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	// The injected library holds the file — the daemon did not open a second
	// directory of its own.
	if _, err := os.Stat(filepath.Join(dir, "worldmodel", "worldmodel.json")); err != nil {
		t.Fatalf("the daemon saved into its own directory, not the injected one: %v", err)
	}
}
