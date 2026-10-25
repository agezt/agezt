// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/contract/cadenceapi"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestOpenAppKernelWiresTheScheduleLibrary: the typed schedule library is the
// DAEMON's file (W3.49) — the kernel holds the contract's host port and the
// composition root opens the store.
//
// The daemon end: the port is live, and the library is a FILE under the
// daemon's base directory — a schedule saved through the kernel has to land
// in the file the next start reads.
func TestOpenAppKernelWiresTheScheduleLibrary(t *testing.T) {
	dir := t.TempDir()
	k, err := openAppKernel(kernelruntime.Config{BaseDir: dir, Provider: mock.New()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })

	if k.Schedules() == nil {
		t.Fatal("the daemon did not wire the schedule library — the kernel holds no port")
	}

	if _, err := k.Schedules().Add("morning brief", time.Hour, "", "operator", time.Now()); err != nil {
		t.Fatalf("Add: %v", err)
	}

	// The library is a FILE, and the daemon owns it.
	if _, err := os.Stat(filepath.Join(dir, "cadence", "schedules.json")); err != nil {
		t.Fatalf("the daemon did not open the library's own file: %v", err)
	}
}

// TestTheScheduleEngineIsBuiltOverTheInjectedLibrary: the daemon builds the
// resident over the library it was HANDED (W3.49) — not over one it opened
// behind the caller's back. A caller that injects a library must have its
// library used, or the injection would be a lie.
func TestTheScheduleEngineIsBuiltOverTheInjectedLibrary(t *testing.T) {
	dir := t.TempDir()
	store, err := cadence.OpenStore(filepath.Join(dir, "injected"))
	if err != nil {
		t.Fatalf("open the schedule store: %v", err)
	}
	k, err := openAppKernel(kernelruntime.Config{
		BaseDir:   dir,
		Provider:  mock.New(),
		Schedules: store,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })

	entry, err := k.Schedules().Add("injected", time.Hour, "", "operator", time.Now())
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	// The injected library holds the schedule — the daemon did not open a
	// second library of its own.
	if _, found := store.Get(entry.ID); !found {
		t.Fatal("the daemon saved into its own library, not the injected one")
	}
}

// TestTheSchedulePortIsTheContractPort: the daemon's tool wiring hands the
// plugin the contract's NARROW port (Store), while the kernel holds the wide
// one (HostStore) — the plugin may not name the module, and it may not drive
// the library beyond what the tool needs.
func TestTheSchedulePortIsTheContractPort(t *testing.T) {
	dir := t.TempDir()
	k, err := openAppKernel(kernelruntime.Config{BaseDir: dir, Provider: mock.New()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })

	var narrow cadenceapi.Store = schedulesPort(k)
	if narrow == nil {
		t.Fatal("the daemon must supply the schedule port to the tools")
	}
	// The wide port is the kernel's face; the narrow one is what the plugin
	// receives.
	if got := schedulesPort(k); got != cadenceapi.Store(k.Schedules()) {
		t.Fatal("the tool's port must be the kernel's live library")
	}
}
