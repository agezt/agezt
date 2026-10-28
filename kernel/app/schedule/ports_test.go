// SPDX-License-Identifier: MIT

package schedule

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The schedule application's ports are filled by the APPLICATION from the
// kernel handle — the host never binds them itself.
//
// The test pins what that wiring buys: the schedules the kernel holds are the
// schedules the application reads, pauses and removes. A port bound to another
// library would leave the operator managing schedules the daemon never fires.
func TestTheSchedulePortsReachTheKernelSchedules(t *testing.T) {
	dir := t.TempDir()
	store, err := cadence.OpenStore(filepath.Join(dir, "cadence"))
	if err != nil {
		t.Fatalf("cadence.OpenStore: %v", err)
	}
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New(), Schedules: store})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()

	ports := PortsFromKernel(k)

	entry, err := ports.Store.Add("check disks", time.Hour, "", "operator", time.Now())
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	// The ports' own listing sees the schedule.
	if entries := ports.Entries(); len(entries) != 1 || entries[0].ID != entry.ID {
		t.Fatalf("Entries = %+v, want the one schedule the port added", entries)
	}

	// The service reads the same library.
	svc := New(ports.Store, Host{}, time.Now)
	list, err := svc.List(context.Background(), ListInput{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if list.Count != 1 {
		t.Fatalf("List count = %d, want 1", list.Count)
	}

	// Pausing one goes through the port and lands.
	paused, err := ports.SetEnabled(entry.ID, false)
	if err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	if !paused {
		t.Error("SetEnabled = false, want the pause to land")
	}
	entries := ports.Entries()
	if len(entries) != 1 || entries[0].Enabled {
		t.Fatalf("Entries = %+v, want the paused schedule", entries)
	}

	// Removing one goes through the port and lands.
	removed, err := ports.Remove(entry.ID)
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if !removed {
		t.Error("Remove = false, want the schedule that was there to be removed")
	}
	if entries := ports.Entries(); len(entries) != 0 {
		t.Errorf("Entries = %+v, want none after the removal", entries)
	}
}

// A daemon with NO schedule library holds a nil port (W3.49): the
// application sees no schedules rather than calling into nothing.
//
// The port carries that discipline now: the adapter used to spell the
// nil-check itself, once per surface, and a surface that forgot it would have
// called into nothing.
func TestTheSchedulePortsSurviveADaemonWithNoScheduleLibrary(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()

	ports := PortsFromKernel(k)
	if ports.Store != nil {
		t.Error("the store port is not nil — the daemon holds no schedule library")
	}
	if entries := ports.Entries(); entries != nil {
		t.Errorf("Entries = %+v, want none from a daemon with no schedule library", entries)
	}
	removed, err := ports.Remove("nothing")
	if err != nil || removed {
		t.Errorf("Remove = %v %v, want nothing removed and no error", removed, err)
	}
	paused, err := ports.SetEnabled("nothing", false)
	if err != nil || paused {
		t.Errorf("SetEnabled = %v %v, want nothing paused and no error", paused, err)
	}
}
