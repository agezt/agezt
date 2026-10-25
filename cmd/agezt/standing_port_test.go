// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/agezt/agezt/kernel/contract/standingapi"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/standing"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestOpenAppKernelWiresTheStandingLibrary: the standing-order library is the
// DAEMON's file (W3.50) — the kernel holds the contract's store port and the
// composition root opens the store.
//
// The daemon end: the port is live, and the library is a FILE under the
// daemon's base directory — an order saved through the kernel has to land in
// the file the next start reads.
func TestOpenAppKernelWiresTheStandingLibrary(t *testing.T) {
	dir := t.TempDir()
	k, err := openAppKernel(kernelruntime.Config{BaseDir: dir, Provider: mock.New()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })

	if k.Standing() == nil {
		t.Fatal("the daemon did not wire the standing library — the kernel holds no port")
	}

	if _, err := k.Standing().Add(standingapi.Order{
		Name:     "watch the deploys",
		Triggers: []standingapi.Trigger{{Type: standingapi.TriggerEvent, Subject: "deploy.*"}},
	}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	// The library is a FILE, and the daemon owns it.
	if _, err := os.Stat(filepath.Join(dir, "standing", "standing.json")); err != nil {
		t.Fatalf("the daemon did not open the library's own file: %v", err)
	}
}

// TestTheStandingLibraryIsTheInjectedOne: the daemon builds its residents
// over the library it was HANDED (W3.50) — not over one it opened behind the
// caller's back. A caller that injects a library must have its library used,
// or the injection would be a lie.
func TestTheStandingLibraryIsTheInjectedOne(t *testing.T) {
	dir := t.TempDir()
	store, err := standing.Open(filepath.Join(dir, "injected"))
	if err != nil {
		t.Fatalf("open the standing store: %v", err)
	}
	k, err := openAppKernel(kernelruntime.Config{
		BaseDir:  dir,
		Provider: mock.New(),
		Standing: store,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })

	order, err := k.Standing().Add(standingapi.Order{
		Name:     "injected",
		Triggers: []standingapi.Trigger{{Type: standingapi.TriggerEvent, Subject: "fixture"}},
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	// The injected library holds the order — the daemon did not open a
	// second library of its own.
	if _, found := store.Get(order.ID); !found {
		t.Fatal("the daemon saved into its own library, not the injected one")
	}
}
