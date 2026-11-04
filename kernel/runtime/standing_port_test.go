// SPDX-License-Identifier: MIT

package runtime_test

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/standingapi"
	"github.com/agezt/agezt/kernel/event"
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
//
// The library's LIFECYCLE is the standing package's (W3.76x), so the
// composition root hands over the BUILDER as well: the kernel calls it with
// the store and its own publisher, and holds the library's FACE.
func openStandingKernel(t *testing.T, cfg runtime.Config) *runtime.Kernel {
	t.Helper()
	store, err := standing.Open(filepath.Join(cfg.BaseDir, "standing"))
	if err != nil {
		t.Fatalf("open the standing store: %v", err)
	}
	cfg.Standing = store
	cfg.StandingBuilder = func(store standingapi.Store, publish func(event.Spec)) standingapi.Lifecycle {
		return standing.NewService(standing.Dependencies{Store: store, Publish: publish})
	}
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

// TestTheStandingServiceFaceIsTheLibrarysPort: the daemon's other standing
// face is the LIBRARY'S service — the contract's lifecycle port.
//
// The daemon holds no verb per standing operation any more (W3.76x): the
// library's rules and its lifecycle live together, and the daemon hands over
// the library's face. The width is checked STATICALLY, like the store port's.
func TestTheStandingServiceFaceIsTheLibrarysPort(t *testing.T) {
	k := openStandingKernel(t, runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	t.Cleanup(func() { _ = k.Close() })

	got := reflect.TypeOf(k.StandingService).Out(0)
	want := reflect.TypeOf((*standingapi.Lifecycle)(nil)).Elem()
	if got != want {
		t.Fatalf("the kernel's standing service face is %v, want the lifecycle port %v", got, want)
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

// TestTheDaemonHoldsNoStandingFaceWithoutALibrary: a kernel with no library
// holds NO face.
//
// The library's lifecycle is the library's (W3.76x): the daemon used to hold
// the four writes over the library's own operations, and it holds none now. A
// daemon with no library therefore holds no face — and the layers above (the
// applications, the agent-facing tool) refuse in its place.
func TestTheDaemonHoldsNoStandingFaceWithoutALibrary(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })

	if k.StandingService() != nil {
		t.Fatal("a kernel with no library must hold no standing face")
	}
	if got := k.Standing(); got != nil {
		t.Fatalf("a kernel with no library must hold a nil store port, got %v", got)
	}
}

// TestTheStandingFaceAnswersTheLibrary: the face answers the LIVE library —
// an order added through the face is in the library the daemon opened.
func TestTheStandingFaceAnswersTheLibrary(t *testing.T) {
	k := openStandingKernel(t, runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	t.Cleanup(func() { _ = k.Close() })

	added, err := k.StandingService().AddStanding(standingapi.Order{
		Name:     "watch the deploys",
		Triggers: []standingapi.Trigger{{Type: standingapi.TriggerEvent, Subject: "deploy.*"}},
	})
	if err != nil {
		t.Fatalf("AddStanding: %v", err)
	}
	if added.ID == "" {
		t.Fatal("the face must answer a row with an id")
	}
	if got := k.Standing().List(); len(got) != 1 || got[0].Name != "watch the deploys" {
		t.Fatalf("the face must answer the live library, got %v", got)
	}
	if got := k.StandingService().StandingList(); len(got) != 1 {
		t.Fatalf("the face must list the live library, got %v", got)
	}
}

// TestTheKernelEditOfAnUnknownOrderIsNotFound: the library turns its own
// "not found" into "the order does not exist" — the edit path reports
// updated=false with no error, the way the schedule edit path does.
func TestTheKernelEditOfAnUnknownOrderIsNotFound(t *testing.T) {
	k := openStandingKernel(t, runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	t.Cleanup(func() { _ = k.Close() })

	_, found, err := k.StandingService().UpdateStanding("missing", func(*standingapi.Order) {})
	if err != nil || found {
		t.Fatalf("editing an unknown order must report not-found, got found=%v err=%v", found, err)
	}
}

// TestTheKernelForwardsTheLibrarysError: the daemon does not swallow the
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
