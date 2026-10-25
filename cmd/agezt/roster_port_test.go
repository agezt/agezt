// SPDX-License-Identifier: MIT

package main

// Provenance: the daemon's contract test for the roster (W3.56) — that the
//             composition root opens the roster's FILE and hands the kernel
//             the contract's port.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/modules/agents/roster"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestOpenAppKernelWiresTheRoster: the daemon end — the roster is a FILE on
// disk under the base dir, and the port the kernel holds is the injected one.
//
// The file is the point: an agent written through the kernel has to land in
// the file the NEXT start reads, or the fleet would forget itself on every
// restart.
func TestOpenAppKernelWiresTheRoster(t *testing.T) {
	dir := t.TempDir()
	k, err := openAppKernel(kernelruntime.Config{BaseDir: dir, Provider: mock.New()})
	if err != nil {
		t.Fatalf("openAppKernel: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })

	if k.Roster() == nil {
		t.Fatal("the daemon must hand the kernel a roster port")
	}
	// The roster's home is a DIRECTORY under the base dir (the file itself
	// appears with the first agent — the store writes on mutation).
	if fi, err := os.Stat(filepath.Join(dir, "roster")); err != nil || !fi.IsDir() {
		t.Fatalf("the daemon must open the roster's directory: %v", err)
	}
	if _, err := k.AddProfile(agentapi.Profile{Slug: "ops", Name: "Ops"}); err != nil {
		t.Fatalf("AddProfile: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "roster", "roster.json")); err != nil {
		t.Fatalf("the daemon's roster must be a file on disk: %v", err)
	}
	if got, found := k.RosterGet("ops"); !found || got.Slug != "ops" {
		t.Fatalf("the daemon's roster must answer the live rows, got %+v (found=%v)", got, found)
	}
}

// TestTheRosterIsTheInjectedOne: the daemon uses the INJECTED roster, not one
// of its own. A daemon that opened its own roster over the injected one would
// silently ignore what the host handed over.
func TestTheRosterIsTheInjectedOne(t *testing.T) {
	dir := t.TempDir()
	store, err := roster.Open(filepath.Join(dir, "injected"))
	if err != nil {
		t.Fatalf("open the roster store: %v", err)
	}
	k, err := openAppKernel(kernelruntime.Config{
		BaseDir:  dir,
		Provider: mock.New(),
		Roster:   store,
	})
	if err != nil {
		t.Fatalf("openAppKernel: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })

	if _, err := k.AddProfile(agentapi.Profile{Slug: "injected-agent"}); err != nil {
		t.Fatalf("AddProfile: %v", err)
	}
	got, found := store.Get("injected-agent")
	if !found || got.Slug != "injected-agent" {
		t.Fatalf("the daemon must use the injected roster, got %+v (found=%v)", got, found)
	}
	// And the daemon's OWN roster directory was never created: the injected
	// store replaced it, not accompanied it.
	if _, err := os.Stat(filepath.Join(dir, "roster")); err == nil {
		t.Fatal("the daemon must not open a second roster over the injected one")
	}
}
