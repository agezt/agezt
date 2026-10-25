// SPDX-License-Identifier: MIT

package runtime_test

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/standingapi"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/standing"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// openStandingKernel opens the kernel the way the COMPOSITION ROOT does
// (W3.50): the standing-order library is the daemon's file, so the daemon
// opens it and hands the kernel the contract's store port.
//
// The kernel opens no library of its own any more — a kernel opened without
// one holds a nil port and every standing surface says so.
func openStandingKernel(t *testing.T, cfg runtime.Config) *runtime.Kernel {
	t.Helper()
	store, err := standing.Open(filepath.Join(cfg.BaseDir, "standing"))
	if err != nil {
		t.Fatalf("open the standing store: %v", err)
	}
	cfg.Standing = store
	k, err := runtime.Open(cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return k
}

// TestTheStandingLibraryStaysANilPortWithoutOne: a kernel opened without a
// library hands over a NIL port, not a port wrapping nothing.
//
// The check is on the CONCRETE value: a nil *Store handed to an interface
// parameter is already a non-nil interface, and the surfaces would bind a
// reader that is not there.
func TestTheStandingLibraryStaysANilPortWithoutOne(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })

	if got := k.Standing(); got != nil {
		t.Fatalf("a kernel with no library must yield a nil port, got %v", got)
	}
}

// TestTheStandingFaceIsTheStorePort: the kernel's own face is the contract's
// STORE port — the seven verbs the module's public surface has.
//
// The width is checked STATICALLY, on the accessor itself: a narrowed face
// would still answer the read side, so only the declared return type can tell
// the two apart.
func TestTheStandingFaceIsTheStorePort(t *testing.T) {
	k := openStandingKernel(t, runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	t.Cleanup(func() { _ = k.Close() })

	got := reflect.TypeOf(k.Standing).Out(0)
	want := reflect.TypeOf((*standingapi.Store)(nil)).Elem()
	if got != want {
		t.Fatalf("the kernel's standing face is %v, want the store port %v", got, want)
	}
}

// TestTheStandingPortAnswersTheLiveLibrary: the port is not a shape — it
// answers the LIVE rows. An order added through the port has to be in the
// library the daemon opened.
func TestTheStandingPortAnswersTheLiveLibrary(t *testing.T) {
	k := openStandingKernel(t, runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	t.Cleanup(func() { _ = k.Close() })

	if got := k.Standing().List(); len(got) != 0 {
		t.Fatalf("a fresh library is not empty: %v", got)
	}
	added, err := k.Standing().Add(standingapi.Order{
		Name:     "watch the deploys",
		Triggers: []standingapi.Trigger{{Type: standingapi.TriggerEvent, Subject: "deploy.*"}},
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if added.ID == "" {
		t.Fatal("the port must answer a row with an id")
	}
	all := k.Standing().List()
	if len(all) != 1 || all[0].Name != "watch the deploys" {
		t.Fatalf("the port must answer the live library, got %v", all)
	}
	if got := k.Standing().Count(); got != 1 {
		t.Fatalf("the port counted %d orders, want 1", got)
	}
}

// TestTheKernelRefusesWithoutALibrary: a kernel with no library answers the
// standing surfaces with "not available" — not a panic, and not a silent
// "nothing to do".
func TestTheKernelRefusesWithoutALibrary(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })

	if _, err := k.AddStanding(standingapi.Order{Name: "x"}); err == nil {
		t.Fatal("adding an order without a library must refuse")
	}
	if _, err := k.SetStandingEnabled("any", false); err == nil {
		t.Fatal("pausing an order without a library must refuse")
	}
	if _, _, err := k.UpdateStanding("any", func(*standingapi.Order) {}); err == nil {
		t.Fatal("editing an order without a library must refuse")
	}
	if _, err := k.RemoveStanding("any"); err == nil {
		t.Fatal("removing an order without a library must refuse")
	}
	if got := k.StandingList(); got != nil {
		t.Fatalf("a kernel with no library lists %v", got)
	}
}

// TestTheKernelEditOfAnUnknownOrderIsNotFound: the kernel turns the library's
// "not found" into "the order does not exist" — the edit path reports
// updated=false with no error, the way the schedule edit path does.
func TestTheKernelEditOfAnUnknownOrderIsNotFound(t *testing.T) {
	k := openStandingKernel(t, runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	t.Cleanup(func() { _ = k.Close() })

	_, found, err := k.UpdateStanding("missing", func(*standingapi.Order) {})
	if err != nil || found {
		t.Fatalf("editing an unknown order must report not-found, got found=%v err=%v", found, err)
	}
}

// TestTheKernelForwardsTheLibrarysError: the kernel does not swallow the
// library's own error — a caller that asks about an unknown id through the
// store port sees the contract's ErrNotFound.
func TestTheKernelForwardsTheLibrarysError(t *testing.T) {
	k := openStandingKernel(t, runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	t.Cleanup(func() { _ = k.Close() })

	_, err := k.Standing().SetEnabled("missing", false)
	if !errors.Is(err, standingapi.ErrNotFound) {
		t.Fatalf("the port must answer the contract's error, got %v", err)
	}
}
