// SPDX-License-Identifier: MIT

package runtime_test

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/contract/cadenceapi"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// openScheduleKernel opens the kernel the way the COMPOSITION ROOT does
// (W3.49): the typed schedule library is the daemon's file, so the daemon
// opens it and hands the kernel the contract's host port.
//
// The kernel opens no library of its own any more — a kernel opened without
// one holds a nil port and every schedule surface says so.
func openScheduleKernel(t *testing.T, cfg runtime.Config) *runtime.Kernel {
	t.Helper()
	store, err := cadence.OpenStore(filepath.Join(cfg.BaseDir, "cadence"))
	if err != nil {
		t.Fatalf("open the schedule store: %v", err)
	}
	cfg.Schedules = store
	k, err := runtime.Open(cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return k
}

// TestTheScheduleLibraryStaysANilPortWithoutOne: a kernel opened without a
// library hands over a NIL port, not a port wrapping nothing.
//
// The check is on the CONCRETE value: a nil *Store handed to an interface
// parameter is already a non-nil interface, and the surfaces would bind a
// reader that is not there.
func TestTheScheduleLibraryStaysANilPortWithoutOne(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })

	if got := k.Schedules(); got != nil {
		t.Fatalf("a kernel with no library must yield a nil port, got %v", got)
	}
}

// TestTheScheduleFaceIsTheHostPort: the kernel's own face is the contract's
// HOST port — the wide one, because the control plane, the guardians and the
// app layer all drive the library through it.
//
// The width is checked STATICALLY, on the accessor itself: a narrowed face
// would still answer the read side, so only the declared return type can
// tell the two apart.
func TestTheScheduleFaceIsTheHostPort(t *testing.T) {
	k := openScheduleKernel(t, runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	t.Cleanup(func() { _ = k.Close() })

	got := reflect.TypeOf(k.Schedules).Out(0)
	want := reflect.TypeOf((*cadenceapi.HostStore)(nil)).Elem()
	if got != want {
		t.Fatalf("the kernel's schedule face is %v, want the host port %v", got, want)
	}
}

// TestTheSchedulePortAnswersTheLiveLibrary: the port is not a shape — it
// answers the LIVE rows. A schedule added through the port has to be in the
// library the daemon opened.
func TestTheSchedulePortAnswersTheLiveLibrary(t *testing.T) {
	k := openScheduleKernel(t, runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	t.Cleanup(func() { _ = k.Close() })

	if got := k.Schedules().List(); len(got) != 0 {
		t.Fatalf("a fresh library is not empty: %v", got)
	}
	if _, err := k.Schedules().Add("morning brief", time.Hour, "", "operator", time.Now()); err != nil {
		t.Fatalf("Add: %v", err)
	}
	all := k.Schedules().List()
	if len(all) != 1 || all[0].Intent != "morning brief" {
		t.Fatalf("the port must answer the live library, got %v", all)
	}
}

// TestTheScheduleEnginePortIsTheContractPort: the resident the daemon hands
// over is held as the contract's port — the kernel names the shape, never
// the implementation.
func TestTheScheduleEnginePortIsTheContractPort(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = k.Close() })

	// Without a resident the answer is a nil port, not a panic.
	if got := k.ScheduleEngine(); got != nil {
		t.Fatalf("a kernel with no resident must yield a nil port, got %v", got)
	}

	// The setter takes the CONTRACT's port.
	var _ cadenceapi.Engine = fakeResident{}
	k.SetScheduleEngine(fakeResident{})
	if got := k.ScheduleEngine(); got == nil {
		t.Fatal("the resident was not handed over")
	}
}

// fakeResident is a plain struct that satisfies the engine port. It proves
// the port carries no module's types beyond the contract's rows.
type fakeResident struct{}

func (fakeResident) Start(context.Context)          {}
func (fakeResident) Wait()                          {}
func (fakeResident) WaitIdle(context.Context) error { return nil }
func (fakeResident) RunningCount() int              { return 0 }
